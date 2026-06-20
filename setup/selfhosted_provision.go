package setup

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/network"
)

// ProvisionSelfHostedUser authenticates against a self-hosted LiveReview API using
// email/password login (or initial admin setup) and creates an LRC CLI API key.
func ProvisionSelfHostedUser(apiURL string, creds SelfHostedLoginRequest, initialAdmin *SelfHostedSetupAdminRequest, logf func(format string, args ...interface{})) (*SetupResult, error) {
	log := func(format string, args ...interface{}) {
		if logf != nil {
			logf(format, args...)
		}
	}

	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		return nil, fmt.Errorf("self-hosted api_url is required")
	}

	client := network.NewSetupClient(30 * time.Second)

	var authResp SelfHostedAuthResponse
	if initialAdmin != nil {
		log("creating initial self-hosted admin user")
		resp, err := network.SetupInitialAdmin(client, apiURL, *initialAdmin)
		if err != nil {
			return nil, fmt.Errorf("failed to contact LiveReview API: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			log("initial admin setup failed: status=%d body=%s", resp.StatusCode, string(resp.Body))
			return nil, fmt.Errorf("initial admin setup returned %d: %s", resp.StatusCode, strings.TrimSpace(string(resp.Body)))
		}
		if err := json.Unmarshal(resp.Body, &authResp); err != nil {
			return nil, fmt.Errorf("failed to parse initial admin setup response: %w", err)
		}
	} else {
		log("logging in to self-hosted LiveReview API")
		resp, err := network.SetupAuthLogin(client, apiURL, creds)
		if err != nil {
			return nil, fmt.Errorf("failed to contact LiveReview API: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			log("login failed: status=%d body=%s", resp.StatusCode, string(resp.Body))
			return nil, fmt.Errorf("login returned %d: %s", resp.StatusCode, strings.TrimSpace(string(resp.Body)))
		}
		if err := json.Unmarshal(resp.Body, &authResp); err != nil {
			return nil, fmt.Errorf("failed to parse login response: %w", err)
		}
	}

	result, err := setupResultFromSelfHostedAuth(&authResp)
	if err != nil {
		return nil, err
	}

	plainKey, err := createSetupAPIKey(client, apiURL, result.OrgID, result.AccessToken, log)
	if err != nil {
		return nil, err
	}
	result.PlainAPIKey = plainKey
	return result, nil
}

// CheckSelfHostedSetupRequired reports whether the target API needs first-time admin setup.
func CheckSelfHostedSetupRequired(apiURL string) (bool, error) {
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		return false, fmt.Errorf("self-hosted api_url is required")
	}

	client := network.NewSetupClient(10 * time.Second)
	resp, err := network.SetupAuthSetupStatus(client, apiURL)
	if err != nil {
		return false, fmt.Errorf("failed to contact LiveReview API: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("setup-status returned %d: %s", resp.StatusCode, strings.TrimSpace(string(resp.Body)))
	}

	var status SelfHostedSetupStatusResponse
	if err := json.Unmarshal(resp.Body, &status); err != nil {
		return false, fmt.Errorf("failed to parse setup-status response: %w", err)
	}
	return status.SetupRequired, nil
}

func setupResultFromSelfHostedAuth(authResp *SelfHostedAuthResponse) (*SetupResult, error) {
	if authResp == nil {
		return nil, fmt.Errorf("login response is nil")
	}
	if strings.TrimSpace(authResp.Tokens.AccessToken) == "" {
		return nil, fmt.Errorf("login response missing access_token")
	}
	if len(authResp.Organizations) == 0 {
		return nil, fmt.Errorf("login response missing organizations")
	}

	org := authResp.Organizations[0]
	orgID := strings.TrimSpace(org.ID.String())
	if orgID == "" || orgID == "0" {
		return nil, fmt.Errorf("login response missing organization id")
	}

	return &SetupResult{
		Email:        strings.TrimSpace(authResp.User.Email),
		UserID:       strings.TrimSpace(authResp.User.ID.String()),
		OrgID:        orgID,
		OrgName:      strings.TrimSpace(org.Name),
		AccessToken:  strings.TrimSpace(authResp.Tokens.AccessToken),
		RefreshToken: strings.TrimSpace(authResp.Tokens.RefreshToken),
	}, nil
}

func createSetupAPIKey(client *network.Client, apiURL, orgID, accessToken string, log func(format string, args ...interface{})) (string, error) {
	apiKeyReq := CreateAPIKeyRequest{Label: "LRC CLI Key"}
	apiKeyURL := network.SetupCreateAPIKeyURL(apiURL, orgID)
	log("creating API key: POST %s", apiKeyURL)

	resp, err := network.SetupCreateAPIKey(client, apiURL, orgID, apiKeyReq, accessToken)
	if err != nil {
		return "", fmt.Errorf("failed to create API key: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		log("create API key failed: status=%d", resp.StatusCode)
		return "", fmt.Errorf("create API key returned %d", resp.StatusCode)
	}

	var apiKeyResp CreateAPIKeyResponse
	if err := json.Unmarshal(resp.Body, &apiKeyResp); err != nil {
		return "", fmt.Errorf("failed to parse API key response: %w", err)
	}
	if strings.TrimSpace(apiKeyResp.PlainKey) == "" {
		return "", fmt.Errorf("create API key response missing plain_key")
	}

	log("API key created: status=%d", resp.StatusCode)
	return strings.TrimSpace(apiKeyResp.PlainKey), nil
}
