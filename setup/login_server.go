package setup

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// SigninURLEnv allows self-hosted deployments to point the signin flow at an
// explicit URL, overriding both the cloud default and the api_url-derived value.
const SigninURLEnv = "LRC_SIGNIN_URL"

// ResolveSigninBase returns the signin base URL to use for the given LiveReview
// api_url. Resolution order:
//  1. the LRC_SIGNIN_URL override, if set;
//  2. the cloud default (HexmosSigninBase) when api_url is empty or the cloud URL;
//  3. otherwise "<api_url>/signin", so a self-hosted instance signs in against
//     itself instead of hexmos.com.
func ResolveSigninBase(apiURL string) (string, error) {
	if override := strings.TrimSpace(os.Getenv(SigninURLEnv)); override != "" {
		return override, nil
	}

	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" || apiURL == CloudAPIURL {
		return HexmosSigninBase, nil
	}

	base, err := url.Parse(apiURL)
	if err != nil {
		return "", fmt.Errorf("invalid api url for signin derivation: %w", err)
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return "", fmt.Errorf("api url scheme must be http or https for signin derivation, got %q", base.Scheme)
	}
	if base.Host == "" {
		return "", fmt.Errorf("api url must include a host for signin derivation")
	}

	return strings.TrimSuffix(apiURL, "/") + "/signin", nil
}

// BuildSigninURL builds the signin URL for the setup callback flow. The signin
// base is derived from apiURL so self-hosted LiveReview instances sign in
// against their own server rather than the cloud hexmos.com domain.
func BuildSigninURL(callbackURL, apiURL string) (string, error) {
	cbURL, err := url.Parse(callbackURL)
	if err != nil {
		return "", fmt.Errorf("invalid callback url: %w", err)
	}
	if cbURL.Scheme != "http" && cbURL.Scheme != "https" {
		return "", fmt.Errorf("invalid callback url scheme: %s", cbURL.Scheme)
	}
	if !isAllowedSetupCallbackHost(cbURL) {
		return "", fmt.Errorf("callback url must use localhost/127.0.0.1 or the current GitHub Codespaces forwarded host")
	}

	signinBaseStr, err := ResolveSigninBase(apiURL)
	if err != nil {
		return "", err
	}
	signinBase, err := url.Parse(signinBaseStr)
	if err != nil {
		return "", fmt.Errorf("invalid signin base url: %w", err)
	}
	if signinBase.Scheme != "http" && signinBase.Scheme != "https" {
		return "", fmt.Errorf("signin base url must be a valid http(s) url")
	}
	if signinBase.Host == "" {
		return "", fmt.Errorf("signin base url must include a host")
	}

	q := signinBase.Query()
	q.Set("app", "livereview")
	q.Set("appRedirectURI", callbackURL)
	signinBase.RawQuery = q.Encode()
	return signinBase.String(), nil
}

func isAllowedSetupCallbackHost(cbURL *url.URL) bool {
	host := cbURL.Hostname()
	if host == "127.0.0.1" || host == "localhost" {
		return true
	}
	if cbURL.Scheme != "https" {
		return false
	}

	name := os.Getenv("CODESPACE_NAME")
	domain := os.Getenv("GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN")
	if name == "" || domain == "" {
		return false
	}

	return strings.HasPrefix(host, name+"-") && strings.HasSuffix(host, "."+domain)
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
