package setup

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProvisionSelfHostedUserLoginAndCreateAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user": map[string]any{"id": 7, "email": "dev@example.com"},
				"tokens": map[string]any{
					"access_token":  "access-token",
					"refresh_token": "refresh-token",
					"token_type":    "Bearer",
				},
				"organizations": []map[string]any{
					{"id": 42, "name": "Acme", "role": "owner"},
				},
			})
		case "/api/v1/orgs/42/api-keys":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
				t.Fatalf("unexpected auth header: %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"api_key":   map[string]any{"id": 1, "label": "LRC CLI Key"},
				"plain_key": "lr_test_key",
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	result, err := ProvisionSelfHostedUser(server.URL, SelfHostedLoginRequest{
		Email:    "dev@example.com",
		Password: "secret",
	}, nil, nil)
	if err != nil {
		t.Fatalf("ProvisionSelfHostedUser: %v", err)
	}
	if result.Email != "dev@example.com" {
		t.Fatalf("unexpected email: %q", result.Email)
	}
	if result.OrgID != "42" {
		t.Fatalf("unexpected org id: %q", result.OrgID)
	}
	if result.PlainAPIKey != "lr_test_key" {
		t.Fatalf("unexpected api key: %q", result.PlainAPIKey)
	}
}

func TestCheckSelfHostedSetupRequired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/setup-status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"setup_required": true,
			"user_count":     0,
		})
	}))
	defer server.Close()

	required, err := CheckSelfHostedSetupRequired(server.URL)
	if err != nil {
		t.Fatalf("CheckSelfHostedSetupRequired: %v", err)
	}
	if !required {
		t.Fatal("expected setup_required=true")
	}
}
