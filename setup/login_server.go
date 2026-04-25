package setup

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
)

// BuildSigninURL builds the Hexmos signin URL for setup callback flow.
func BuildSigninURL(callbackURL string) (string, error) {
	cbURL, err := url.Parse(callbackURL)
	if err != nil {
		return "", fmt.Errorf("invalid callback url: %w", err)
	}
	if cbURL.Scheme != "http" && cbURL.Scheme != "https" {
		return "", fmt.Errorf("invalid callback url scheme: %s", cbURL.Scheme)
	}
	if cbURL.Scheme == "http" && cbURL.Hostname() != "127.0.0.1" && cbURL.Hostname() != "localhost" {
		return "", fmt.Errorf("callback url must use localhost/127.0.0.1")
	}

	signinBase, err := url.Parse(HexmosSigninBase)
	if err != nil {
		return "", fmt.Errorf("invalid signin base url: %w", err)
	}
	if signinBase.Scheme != "https" || signinBase.Host == "" {
		return "", fmt.Errorf("signin base url must be a valid https url")
	}

	q := signinBase.Query()
	q.Set("app", "livereview")
	q.Set("appRedirectURI", callbackURL)
	signinBase.RawQuery = q.Encode()
	return signinBase.String(), nil
}

// StartTemporaryServer starts an HTTP server with the given listener and handler.
// Any non-closed serve error is sent to errCh.
func StartTemporaryServer(listener net.Listener, handler http.Handler, errCh chan<- error) *http.Server {
	server := &http.Server{Handler: handler}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("server error: %w", err)
		}
	}()
	return server
}
