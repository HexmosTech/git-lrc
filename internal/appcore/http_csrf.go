package appcore

import (
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// isStateChangingMethod reports whether an HTTP method can mutate review state.
func isStateChangingMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

// requireCSRFProtection rejects state-changing requests that a browser could send
// from another origin. Browsers send Origin (or Referer) on such requests, so a
// cross-origin value is rejected before the handler runs. A body-bearing request
// must also be application/json, which keeps simple cross-site form posts
// (text/plain, application/x-www-form-urlencoded, multipart/form-data) from
// reaching a JSON handler. Requests with neither Origin nor Referer come from
// non-browser clients and are allowed; requireSession still gates them.
func requireCSRFProtection(r *http.Request) bool {
	if !isStateChangingMethod(r.Method) {
		return true
	}
	if !requestOriginAllowed(r) {
		return false
	}
	if requestHasBody(r) && !isJSONRequest(r) {
		return false
	}
	return true
}

// requestOriginAllowed validates the request's browser-supplied origin headers.
// It fails closed when a present Origin or Referer cannot be parsed or does not
// match the host this server was reached on.
func requestOriginAllowed(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		return sameOriginAsRequest(origin, r.Host)
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		return sameOriginAsRequest(referer, r.Host)
	}
	return true
}

func sameOriginAsRequest(rawURL, requestHost string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Host, requestHost)
}

func requestHasBody(r *http.Request) bool {
	if r.Body == nil || r.Body == http.NoBody {
		return false
	}
	return r.ContentLength > 0 || len(r.TransferEncoding) > 0
}

func isJSONRequest(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return false
	}
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}
