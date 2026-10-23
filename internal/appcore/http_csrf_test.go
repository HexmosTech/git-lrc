package appcore

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireCSRFProtection(t *testing.T) {
	const requestHost = "localhost:8080"

	tests := []struct {
		name        string
		method      string
		host        string
		body        string
		contentType string
		origin      string
		referer     string
		want        bool
	}{
		{
			name:        "same-origin json post allowed",
			method:      http.MethodPost,
			host:        requestHost,
			body:        `{"message":"feat: ok"}`,
			contentType: "application/json",
			origin:      "http://localhost:8080",
			want:        true,
		},
		{
			name:        "same-origin json post with charset allowed",
			method:      http.MethodPost,
			host:        requestHost,
			body:        `{"message":"feat: ok"}`,
			contentType: "application/json; charset=utf-8",
			origin:      "http://localhost:8080",
			want:        true,
		},
		{
			name:        "same-origin json suffix media type allowed",
			method:      http.MethodPost,
			host:        requestHost,
			body:        `{"message":"feat: ok"}`,
			contentType: "application/vnd.livereview+json",
			origin:      "http://localhost:8080",
			want:        true,
		},
		{
			name:        "cross-origin json post rejected",
			method:      http.MethodPost,
			host:        requestHost,
			body:        `{"message":"csrf"}`,
			contentType: "application/json",
			origin:      "http://evil.com",
			want:        false,
		},
		{
			name:        "cross-origin text plain post rejected",
			method:      http.MethodPost,
			host:        requestHost,
			body:        `{"message":"csrf"}`,
			contentType: "text/plain",
			origin:      "http://evil.com",
			want:        false,
		},
		{
			name:        "cross-origin origin with matching hostname but different port rejected",
			method:      http.MethodPost,
			host:        requestHost,
			body:        `{"message":"csrf"}`,
			contentType: "application/json",
			origin:      "http://localhost:9090",
			want:        false,
		},
		{
			name:        "null origin rejected",
			method:      http.MethodPost,
			host:        requestHost,
			body:        `{"message":"csrf"}`,
			contentType: "application/json",
			origin:      "null",
			want:        false,
		},
		{
			name:        "cross-origin referer rejected",
			method:      http.MethodPost,
			host:        requestHost,
			body:        `{"message":"csrf"}`,
			contentType: "application/json",
			referer:     "http://evil.com/page",
			want:        false,
		},
		{
			name:        "same-origin referer allowed",
			method:      http.MethodPost,
			host:        requestHost,
			body:        `{"message":"feat: ok"}`,
			contentType: "application/json",
			referer:     "http://localhost:8080/?r=rid",
			want:        true,
		},
		{
			name:        "same-origin text plain post rejected",
			method:      http.MethodPost,
			host:        requestHost,
			body:        `{"message":"csrf"}`,
			contentType: "text/plain",
			origin:      "http://localhost:8080",
			want:        false,
		},
		{
			name:        "same-origin form post rejected",
			method:      http.MethodPost,
			host:        requestHost,
			body:        `message=csrf`,
			contentType: "application/x-www-form-urlencoded",
			origin:      "http://localhost:8080",
			want:        false,
		},
		{
			name:   "body without content type rejected",
			method: http.MethodPost,
			host:   requestHost,
			body:   `{"message":"csrf"}`,
			origin: "http://localhost:8080",
			want:   false,
		},
		{
			name:   "non-browser json post without origin allowed",
			method: http.MethodPost,
			host:   requestHost,
			body:   `{"message":"feat: ok"}`,
			// net/http would normally attach a content type; the handler's JSON
			// parser is still only reachable after the session check.
			contentType: "application/json",
			want:        true,
		},
		{
			name:   "non-browser text plain post without origin rejected",
			method: http.MethodPost,
			host:   requestHost,
			body:   `{"message":"csrf"}`,
			// A simple form post has no Origin only in older browsers; the
			// content-type requirement still blocks it.
			contentType: "text/plain",
			want:        false,
		},
		{
			name:   "bodyless same-origin post allowed",
			method: http.MethodPost,
			host:   requestHost,
			origin: "http://localhost:8080",
			want:   true,
		},
		{
			name:   "bodyless cross-origin post rejected",
			method: http.MethodPost,
			host:   requestHost,
			origin: "http://evil.com",
			want:   false,
		},
		{
			name:   "bodyless same-origin patch allowed",
			method: http.MethodPatch,
			host:   requestHost,
			origin: "http://localhost:8080",
			want:   true,
		},
		{
			name:   "bodyless cross-origin patch rejected",
			method: http.MethodPatch,
			host:   requestHost,
			origin: "http://evil.com",
			want:   false,
		},
		{
			name:        "read-only cross-origin get allowed",
			method:      http.MethodGet,
			host:        requestHost,
			body:        `{"message":"ignored"}`,
			contentType: "text/plain",
			origin:      "http://evil.com",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body *strings.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			} else {
				body = strings.NewReader("")
			}
			req := httptest.NewRequest(tt.method, "/commit?r=rid", body)
			req.Host = tt.host
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.referer != "" {
				req.Header.Set("Referer", tt.referer)
			}

			if got := requireCSRFProtection(req); got != tt.want {
				t.Fatalf("requireCSRFProtection() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequireSessionRejectsCrossOriginStateChange(t *testing.T) {
	called := false
	handler := requireSession("rid", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/commit?r=rid", strings.NewReader(`{"message":"CSRF-commit"}`))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Origin", "http://evil.com")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if called {
		t.Fatal("handler ran for a cross-origin state-changing request")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if !strings.Contains(rec.Body.String(), "csrf_rejected") {
		t.Fatalf("body = %q, want csrf_rejected error", rec.Body.String())
	}
}

func TestRequireSessionAllowsSameOriginStateChange(t *testing.T) {
	called := false
	handler := requireSession("rid", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/commit?r=rid", strings.NewReader(`{"message":"feat: ok"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+req.Host)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called {
		t.Fatal("handler did not run for a same-origin JSON request")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
