package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistConnectorsToConfigPreservesExistingContent(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".lrc.toml")

	original := `api_key = "abc123"
api_url = "https://livereview.hexmos.com"
jwt = "token"
org_id = "42"
`
	if err := os.WriteFile(configPath, []byte(original), 0600); err != nil {
		t.Fatalf("write original config: %v", err)
	}

	connectors := []aiConnectorRemote{
		{
			ID:            7,
			ProviderName:  "gemini",
			ConnectorName: "Gemini Flash",
			APIKey:        "gkey",
			SelectedModel: "gemini-2.5-flash",
			DisplayOrder:  1,
		},
	}

	if err := persistConnectorsToConfig(configPath, connectors); err != nil {
		t.Fatalf("persist connectors: %v", err)
	}

	updatedBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	updated := string(updatedBytes)

	if strings.TrimSpace(updated) == "" {
		t.Fatalf("config became empty")
	}

	if !strings.Contains(updated, `api_key = "abc123"`) {
		t.Fatalf("existing config keys were not preserved")
	}

	if !strings.Contains(updated, aiConnectorsSectionBegin) || !strings.Contains(updated, aiConnectorsSectionEnd) {
		t.Fatalf("managed ai_connectors section missing")
	}

	if !strings.Contains(updated, `[[ai_connectors]]`) || !strings.Contains(updated, `provider_name = "gemini"`) {
		t.Fatalf("connector data not written to managed section")
	}
}

func TestPersistAuthTokensToConfigUpdatesExistingTokens(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".lrc.toml")

	original := `api_key = "abc123"
api_url = "https://livereview.hexmos.com"
org_id = "42"
jwt = "old-token"
refresh_token = "old-refresh"
`
	if err := os.WriteFile(configPath, []byte(original), 0600); err != nil {
		t.Fatalf("write original config: %v", err)
	}

	if err := persistAuthTokensToConfig(configPath, "new-token", "new-refresh"); err != nil {
		t.Fatalf("persist auth tokens: %v", err)
	}

	updatedBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	updated := string(updatedBytes)

	if !strings.Contains(updated, `api_key = "abc123"`) {
		t.Fatalf("existing config keys were not preserved")
	}

	if !strings.Contains(updated, `jwt = "new-token"`) {
		t.Fatalf("jwt value was not updated")
	}

	if !strings.Contains(updated, `refresh_token = "new-refresh"`) {
		t.Fatalf("refresh token value was not updated")
	}
}
