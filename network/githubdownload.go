package network

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// githubDownloadAllowedHosts are the only hosts a GitHub release download may
// touch, including every redirect hop. github.com issues the initial 302;
// the objects/release-assets hosts serve the actual asset bytes.
var githubDownloadAllowedHosts = map[string]bool{
	"github.com":                           true,
	"objects.githubusercontent.com":        true,
	"release-assets.githubusercontent.com": true,
}

func validateGitHubDownloadURL(fullURL string) error {
	parsed, err := url.Parse(fullURL)
	if err != nil {
		return fmt.Errorf("invalid download URL %q: %w", fullURL, err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("download URL must use https: %s", fullURL)
	}
	if !githubDownloadAllowedHosts[strings.ToLower(parsed.Host)] {
		return fmt.Errorf("download URL host %q is not a trusted GitHub host", parsed.Host)
	}
	return nil
}

// NewGitHubDownloadClient creates an HTTP client for downloading GitHub
// release assets. Unlike NewHTTPClient it follows cross-host redirects, but
// only onto the trusted GitHub asset hosts - release downloads always 302
// from github.com to an *.githubusercontent.com host.
func NewGitHubDownloadClient() *Client {
	return &Client{
		httpClient: &http.Client{
			// No overall timeout: assets are tens of MB and download time
			// varies with connection speed. Per-request cancellation is the
			// caller's concern.
			Timeout: 0,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return validateGitHubDownloadURL(req.URL.String())
			},
		},
	}
}

// GitHubDownloadTo streams a GitHub release asset into dst, returning the
// final HTTP status code. onProgress (optional) is called after each chunk
// with (downloaded, total); total is -1 when Content-Length is unknown.
func GitHubDownloadTo(client *Client, fullURL string, dst io.Writer, onProgress func(downloaded, total int64)) (int, error) {
	if err := validateGitHubDownloadURL(fullURL); err != nil {
		return 0, err
	}

	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create download request: %w", err)
	}

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("download failed with status %d for %s", resp.StatusCode, fullURL)
	}

	var w io.Writer = dst
	if onProgress != nil {
		w = &progressWriter{dst: dst, total: resp.ContentLength, onProgress: onProgress}
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return resp.StatusCode, fmt.Errorf("failed to stream download body: %w", err)
	}

	return resp.StatusCode, nil
}
