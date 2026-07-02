package setup

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedactConnectorErrorBody_RedactsSecrets(t *testing.T) {
	secret := "AIzaSy-very-secret-key"
	body := []byte("validation failed for key AIzaSy-very-secret-key with provider error")

	redacted := redactConnectorErrorBody(body, secret)
	if redacted == string(body) {
		t.Fatalf("expected redaction to change error body")
	}
	if contains := (redacted == "" || redacted == "<empty response body>"); contains {
		t.Fatalf("expected non-empty redacted output")
	}
	if wantAbsent := secret; wantAbsent != "" && strings.Contains(redacted, wantAbsent) {
		t.Fatalf("expected secret to be redacted, got %q", redacted)
	}
}

func TestRedactConnectorErrorBody_EmptyBody(t *testing.T) {
	redacted := redactConnectorErrorBody(nil, "anything")
	if redacted != "<empty response body>" {
		t.Fatalf("expected empty body marker, got %q", redacted)
	}
}

func TestCreateGeminiHelperConnector_SendsHelperRoleAndLiteModel(t *testing.T) {
	var gotBody CreateConnectorRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("failed to unmarshal request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	result := &SetupResult{OrgID: "1", AccessToken: "test-token"}
	if err := CreateGeminiHelperConnector(result, "test-gemini-key", server.URL); err != nil {
		t.Fatalf("CreateGeminiHelperConnector() error = %v", err)
	}

	if gotBody.Role != "helper" {
		t.Errorf("expected role=%q, got %q", "helper", gotBody.Role)
	}
	if gotBody.SelectedModel != DefaultGeminiHelperModel {
		t.Errorf("expected selected_model=%q, got %q", DefaultGeminiHelperModel, gotBody.SelectedModel)
	}
	if gotBody.APIKey != "test-gemini-key" {
		t.Errorf("expected the same key passed to the leader connector to be reused, got %q", gotBody.APIKey)
	}
}

func TestCreateGeminiConnector_SendsLeaderRole(t *testing.T) {
	var gotBody CreateConnectorRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	result := &SetupResult{OrgID: "1", AccessToken: "test-token"}
	if err := CreateGeminiConnector(result, "test-gemini-key", server.URL); err != nil {
		t.Fatalf("CreateGeminiConnector() error = %v", err)
	}

	if gotBody.Role != "leader" {
		t.Errorf("expected role=%q, got %q", "leader", gotBody.Role)
	}
	if gotBody.SelectedModel != DefaultGeminiModel {
		t.Errorf("expected selected_model=%q, got %q", DefaultGeminiModel, gotBody.SelectedModel)
	}
}
