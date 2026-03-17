package setup

import (
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
