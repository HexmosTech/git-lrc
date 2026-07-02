package appui

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
			Role:          "leader",
		},
		{
			ID:            8,
			ProviderName:  "gemini",
			ConnectorName: "Gemini Flash-Lite (Helper)",
			APIKey:        "gkey2",
			SelectedModel: "gemini-2.5-flash-lite",
			DisplayOrder:  1,
			Role:          "helper",
		},
		{
			ID:            9,
			ProviderName:  "gemini",
			ConnectorName: "Legacy connector with no role",
			APIKey:        "gkey3",
			SelectedModel: "gemini-2.5-flash",
			DisplayOrder:  2,
			Role:          "",
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

	if strings.Count(updated, `role = "leader"`) != 2 {
		t.Fatalf("expected two connectors (explicit leader + empty-role fallback) rendered with role=\"leader\", got: %s", updated)
	}
	if !strings.Contains(updated, `role = "helper"`) {
		t.Fatalf("expected the helper connector to render role=\"helper\", got: %s", updated)
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

func TestPersistReauthSessionToConfigPreservesAPIURL(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".lrc.toml")

	original := `api_key = "old-key"
api_url = "http://localhost:8888"
custom_setting = "keep-me"
jwt = "old-jwt"
`
	if err := os.WriteFile(configPath, []byte(original), 0600); err != nil {
		t.Fatalf("write original config: %v", err)
	}

	result := &setupResult{
		PlainAPIKey:  "new-key",
		Email:        "user@example.com",
		FirstName:    "Jane",
		LastName:     "Doe",
		AvatarURL:    "https://cdn.hexmos.com/u/jane.png",
		UserID:       "u-1",
		OrgID:        "o-1",
		OrgName:      "Acme",
		AccessToken:  "new-jwt",
		RefreshToken: "new-refresh",
	}

	if err := persistReauthSessionToConfig(configPath, cloudAPIURL, result); err != nil {
		t.Fatalf("persist reauth session: %v", err)
	}

	updatedBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	updated := string(updatedBytes)

	if !strings.Contains(updated, `api_url = "http://localhost:8888"`) {
		t.Fatalf("expected existing api_url to be preserved")
	}
	if !strings.Contains(updated, `custom_setting = "keep-me"`) {
		t.Fatalf("expected unrelated config fields to be preserved")
	}
	if !strings.Contains(updated, `api_key = "new-key"`) {
		t.Fatalf("expected api_key to be updated")
	}
	if !strings.Contains(updated, `jwt = "new-jwt"`) {
		t.Fatalf("expected jwt to be updated")
	}
	if !strings.Contains(updated, `refresh_token = "new-refresh"`) {
		t.Fatalf("expected refresh_token to be updated")
	}
}
