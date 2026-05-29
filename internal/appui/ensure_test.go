package appui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureReviewReadySucceedsWithExistingConnector(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/aiconnectors" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"provider_name":"gemini","connector_name":"Gemini Flash","selected_model":"gemini-2.5-flash","display_order":1}]`))
	}))
	defer server.Close()

	configPath := filepath.Join(tmpHome, ".lrc.toml")
	configBody := fmt.Sprintf(`api_url = %q
api_key = "lr_key_123"
jwt = "jwt-1"
refresh_token = "ref-1"
org_id = "o-1"
user_email = "user@example.com"
`, server.URL)
	if err := os.WriteFile(configPath, []byte(configBody), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := ensureReviewReady(server.URL); err != nil {
		t.Fatalf("expected readiness success, got %v", err)
	}
}

func TestEnsureReviewReadyFailsWithoutConnector(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/aiconnectors" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	configPath := filepath.Join(tmpHome, ".lrc.toml")
	configBody := fmt.Sprintf(`api_url = %q
api_key = "lr_key_123"
jwt = "jwt-1"
refresh_token = "ref-1"
org_id = "o-1"
user_email = "user@example.com"
`, server.URL)
	if err := os.WriteFile(configPath, []byte(configBody), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	err := ensureReviewReady(server.URL)
	if err == nil {
		t.Fatalf("expected readiness failure when no connector exists")
	}
	if !strings.Contains(err.Error(), "no AI connector is configured") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "lrc ui") {
		t.Fatalf("expected remediation steps in error: %v", err)
	}
}

func TestEnsureReviewReadyFailsWithoutSetup(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	err := ensureReviewReady(cloudAPIURL)
	if err == nil {
		t.Fatalf("expected readiness failure when config is missing")
	}
	if !strings.Contains(err.Error(), "lrc setup") {
		t.Fatalf("expected setup guidance in error: %v", err)
	}
}
