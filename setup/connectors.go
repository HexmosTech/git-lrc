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

func defaultAPIURL(apiURL string) string {
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		return CloudAPIURL
	}
	return apiURL
}

// ValidateGeminiKey checks the key against LiveReview's validate-key endpoint.
func ValidateGeminiKey(result *SetupResult, geminiKey string, apiURL string) (bool, string, error) {
	apiURL = defaultAPIURL(apiURL)

	reqBody := ValidateKeyRequest{
		Provider: "gemini",
		APIKey:   geminiKey,
		Model:    DefaultGeminiModel,
	}

	client := network.NewSetupClient(30 * time.Second)
	resp, err := network.SetupValidateConnectorKey(client, apiURL, result.OrgID, reqBody, result.AccessToken)
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

// createConnector posts a connector creation request to LiveReview, sharing
// the URL defaulting, network call, and error handling used by every
// connector-creation flow (leader, helper, ...). errLabel is used to phrase
// error messages, e.g. "connector" or "helper connector".
func createConnector(result *SetupResult, apiURL string, reqBody CreateConnectorRequest, geminiKey string, errLabel string) error {
	apiURL = defaultAPIURL(apiURL)

	client := network.NewSetupClient(30 * time.Second)
	resp, err := network.SetupCreateConnector(client, apiURL, result.OrgID, reqBody, result.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", errLabel, err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("create %s returned %d: %s", errLabel, resp.StatusCode, redactConnectorErrorBody(resp.Body, geminiKey))
	}

	return nil
}

// CreateGeminiConnector creates a Gemini AI connector in LiveReview.
func CreateGeminiConnector(result *SetupResult, geminiKey string, apiURL string) error {
	reqBody := CreateConnectorRequest{
		ProviderName:  "gemini",
		APIKey:        geminiKey,
		ConnectorName: "Gemini Flash",
		SelectedModel: DefaultGeminiModel,
		DisplayOrder:  0,
		Role:          "leader",
	}

	return createConnector(result, apiURL, reqBody, geminiKey, "connector")
}

// CreateGeminiHelperConnector creates a Gemini Flash-Lite helper connector in
// LiveReview, reusing the same free Gemini key as the leader connector. This
// powers Adaptive Review (see LiveReview's leader/helper model feature) out
// of the box for onboarded orgs, with no second key prompt.
func CreateGeminiHelperConnector(result *SetupResult, geminiKey string, apiURL string) error {
	reqBody := CreateConnectorRequest{
		ProviderName:  "gemini",
		APIKey:        geminiKey,
		ConnectorName: "Gemini Flash-Lite (Helper)",
		SelectedModel: DefaultGeminiHelperModel,
		DisplayOrder:  0,
		Role:          "helper",
	}

	return createConnector(result, apiURL, reqBody, geminiKey, "helper connector")
}
