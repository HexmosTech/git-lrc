package network

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"
)

type Client struct {
	httpClient *http.Client
}

type Response struct {
	StatusCode int
	Body       []byte
	Header     http.Header
}

func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) == 0 {
				return nil
			}
			if req.URL.Host != via[0].URL.Host {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

func NewClient(timeout time.Duration) *Client {
	return &Client{
		httpClient: NewHTTPClient(timeout),
	}
}

const maxRetries = 3

// doWithRetry encapsulates the core HTTP execution with exponential backoff and jitter.
func (c *Client) doWithRetry(reqBody []byte, reqBuilder func(io.Reader) (*http.Request, error)) (*Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		var bodyReader io.Reader
		if reqBody != nil {
			bodyReader = bytes.NewReader(reqBody)
		}

		req, reqErr := reqBuilder(bodyReader)
		if reqErr != nil {
			return nil, reqErr
		}

		resp, err = c.httpClient.Do(req)

		// Determine if the failure is transient
		shouldRetry := false
		if err != nil {
			shouldRetry = true
		} else if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			shouldRetry = true
		}

		if !shouldRetry || attempt == maxRetries {
			break
		}

		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}

		// Calculate backoff: wait = base * 2^attempt + jitter
		baseWait := time.Duration(500*(1<<attempt)) * time.Millisecond
		jitter := time.Duration(rand.Intn(200)) * time.Millisecond
		time.Sleep(baseWait + jitter)
	}

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("failed to read response body: %w", readErr)
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       bodyBytes,
		Header:     resp.Header,
	}, nil
}

func (c *Client) DoJSON(method, url string, payload any, bearerToken, orgContext string, headers map[string]string) (*Response, error) {
	var bodyJSON []byte
	var err error
	if payload != nil {
		bodyJSON, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
	}

	builder := func(bodyReader io.Reader) (*http.Request, error) {
		req, err := http.NewRequest(method, url, bodyReader)
		if err != nil {
			return nil, err
		}

		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if bearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+bearerToken)
		}
		if orgContext != "" {
			req.Header.Set("X-Org-Context", orgContext)
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		return req, nil
	}

	return c.doWithRetry(bodyJSON, builder)
}

func (c *Client) Do(method, url string, body []byte, headers map[string]string) (*Response, error) {
	builder := func(bodyReader io.Reader) (*http.Request, error) {
		req, err := http.NewRequest(method, url, bodyReader)
		if err != nil {
			return nil, err
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		return req, nil
	}

	return c.doWithRetry(body, builder)
}
