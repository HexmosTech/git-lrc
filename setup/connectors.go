package setup

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/network"
)

func redactConnectorErrorBody(body []byte, secrets ...string) string {
	msg := string(body)
	for _, secret := range secrets {
		if strings.TrimSpace(secret) == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, secret, "[REDACTED]")
	}
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "<empty response body>"
	}
	return msg
}

// ValidateGeminiKey checks the key against LiveReview's validate-key endpoint.
func ValidateGeminiKey(result *SetupResult, geminiKey string) (bool, string, error) {
	reqBody := ValidateKeyRequest{
		Provider: "gemini",
		APIKey:   geminiKey,
		Model:    DefaultGeminiModel,
	}

	client := network.NewSetupClient(30 * time.Second)
	resp, err := network.SetupValidateConnectorKey(client, CloudAPIURL, result.OrgID, reqBody, result.AccessToken)
	if err != nil {
		return false, "", fmt.Errorf("failed to validate key: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// Redact submitted provider key from surfaced errors to avoid secret leakage.
		return false, "", fmt.Errorf("validate-key returned %d: %s", resp.StatusCode, redactConnectorErrorBody(resp.Body, geminiKey))
	}

	var valResp ValidateKeyResponse
	if err := json.Unmarshal(resp.Body, &valResp); err != nil {
		return false, "", fmt.Errorf("failed to parse validation response: %w", err)
	}

	return valResp.Valid, valResp.Message, nil
}

// CreateGeminiConnector creates a Gemini AI connector in LiveReview.
func CreateGeminiConnector(result *SetupResult, geminiKey string) error {
	reqBody := CreateConnectorRequest{
		ProviderName:  "gemini",
		APIKey:        geminiKey,
		ConnectorName: "Gemini Flash",
		SelectedModel: DefaultGeminiModel,
		DisplayOrder:  0,
	}

	client := network.NewSetupClient(30 * time.Second)
	resp, err := network.SetupCreateConnector(client, CloudAPIURL, result.OrgID, reqBody, result.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to create connector: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("create connector returned %d: %s", resp.StatusCode, redactConnectorErrorBody(resp.Body, geminiKey))
	}

	return nil
}
