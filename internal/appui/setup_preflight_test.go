package appui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	uicfg "github.com/HexmosTech/git-lrc/ui"
)

func TestRunSetupPreflightReadyWithExistingConnector(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/aiconnectors" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer jwt-1" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		if got := r.Header.Get("X-Org-Context"); got != "o-1" {
			t.Fatalf("unexpected org context header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{
  "id": 7,
  "provider_name": "gemini",
  "connector_name": "Gemini Flash",
  "selected_model": "gemini-2.5-flash",
  "display_order": 1
}]`))
	}))
	defer server.Close()

	configPath := filepath.Join(tmpHome, ".lrc.toml")
	configBody := fmt.Sprintf(`api_url = %q
api_key = "lr_key_123"
jwt = "jwt-1"
refresh_token = "ref-1"
org_id = "o-1"
org_name = "Acme"
user_email = "user@example.com"
`, server.URL)
	if err := os.WriteFile(configPath, []byte(configBody), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loadSetupRuntimeConfig = uicfg.LoadRuntimeConfig
	t.Cleanup(func() {
		loadSetupRuntimeConfig = uicfg.LoadRuntimeConfig
	})

	preflight, err := runSetupPreflight(server.URL, newSetupLog())
	if err != nil {
		t.Fatalf("run setup preflight: %v", err)
	}
	if preflight == nil {
		t.Fatalf("expected preflight result")
	}
	if !preflight.AuthReady {
		t.Fatalf("expected auth-ready preflight")
	}
	if !preflight.HasConnector {
		t.Fatalf("expected connector inventory to short-circuit setup")
	}
	if preflight.Session == nil || preflight.Session.Email != "user@example.com" {
		t.Fatalf("expected existing session details, got %#v", preflight.Session)
	}
	if preflight.APIKeyRecovered {
		t.Fatalf("did not expect api key recovery when api_key already exists")
	}
	if preflight.SessionRefreshed {
		t.Fatalf("did not expect token refresh on a valid session")
	}
	if len(preflight.Connectors) != 1 || preflight.Connectors[0].ProviderName != "gemini" {
		t.Fatalf("unexpected connector inventory: %#v", preflight.Connectors)
	}
	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(updated), `api_key = "lr_key_123"`) {
		t.Fatalf("preflight should preserve existing api_key")
	}
}

func TestRunSetupPreflightRefreshesAndRecoversAPIKey(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	connectorCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/aiconnectors":
			connectorCalls++
			if connectorCalls == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"expired"}`))
				return
			}
			if got := r.Header.Get("Authorization"); got != "Bearer jwt-2" {
				t.Fatalf("unexpected authorization header on retry: %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		case "/api/v1/auth/refresh":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"jwt-2","refresh_token":"ref-2"}`))
		case "/api/v1/orgs/o-1/api-keys":
			if got := r.Header.Get("Authorization"); got != "Bearer jwt-2" {
				t.Fatalf("unexpected authorization header on create key: %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"plain_key":"lr_key_999","api_key":{"id":1,"label":"LRC Setup Recovery Key"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configPath := filepath.Join(tmpHome, ".lrc.toml")
	configBody := fmt.Sprintf(`api_url = %q
jwt = "jwt-1"
refresh_token = "ref-1"
org_id = "o-1"
org_name = "Acme"
user_email = "user@example.com"
`, server.URL)
	if err := os.WriteFile(configPath, []byte(configBody), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	preflight, err := runSetupPreflight(server.URL, newSetupLog())
	if err != nil {
		t.Fatalf("run setup preflight: %v", err)
	}
	if preflight == nil || !preflight.AuthReady {
		t.Fatalf("expected refreshed authenticated preflight, got %#v", preflight)
	}
	if preflight.HasConnector {
		t.Fatalf("expected zero connectors after recovery")
	}
	if !preflight.SessionRefreshed {
		t.Fatalf("expected preflight to refresh expired token")
	}
	if !preflight.APIKeyRecovered {
		t.Fatalf("expected preflight to recover missing api_key")
	}
	if preflight.Session == nil || preflight.Session.AccessToken != "jwt-2" || preflight.Session.RefreshToken != "ref-2" {
		t.Fatalf("expected refreshed session details, got %#v", preflight.Session)
	}
	if preflight.Session.PlainAPIKey != "lr_key_999" {
		t.Fatalf("expected recovered api_key, got %#v", preflight.Session)
	}

	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(updated)
	for _, expected := range []string{
		`jwt = "jwt-2"`,
		`refresh_token = "ref-2"`,
		`api_key = "lr_key_999"`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("updated config missing %s in %s", expected, content)
		}
	}
}

func TestRunSetupPreflightSkipsWhenAPIURLDiffers(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	configPath := filepath.Join(tmpHome, ".lrc.toml")
	configBody := `api_url = "https://other.example.com"
api_key = "lr_key_123"
jwt = "jwt-1"
refresh_token = "ref-1"
org_id = "o-1"
user_email = "user@example.com"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	preflight, err := runSetupPreflight("https://target.example.com", newSetupLog())
	if err != nil {
		t.Fatalf("run setup preflight: %v", err)
	}
	if preflight != nil {
		t.Fatalf("expected preflight to skip when selected api_url differs, got %#v", preflight)
	}
}
