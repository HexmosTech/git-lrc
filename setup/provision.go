package setup

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/HexmosTech/git-lrc/network"
)

// ProvisionLiveReviewUser calls ensure-cloud-user and creates an API key.
// Optional logf receives debug log messages.
func ProvisionLiveReviewUser(cbData *HexmosCallbackData, logf func(format string, args ...interface{})) (*SetupResult, error) {
	log := func(format string, args ...interface{}) {
		if logf != nil {
			logf(format, args...)
		}
	}

	reqBody := EnsureCloudUserRequest{
		Email:     cbData.Result.Data.Email,
		FirstName: cbData.Result.Data.FirstName,
		LastName:  cbData.Result.Data.LastName,
		Source:    "git-lrc",
	}

	client := network.NewSetupClient(30 * time.Second)
	resp, err := network.SetupEnsureCloudUser(client, CloudAPIURL, reqBody, cbData.Result.JWT)
	if err != nil {
		return nil, fmt.Errorf("failed to contact LiveReview API: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		log("ensure-cloud-user failed: status=%d body=%s", resp.StatusCode, string(resp.Body))
		return nil, fmt.Errorf("ensure-cloud-user returned %d: %s", resp.StatusCode, string(resp.Body))
	}

	log("ensure-cloud-user: status=%d", resp.StatusCode)

	var ensureResp EnsureCloudUserResponse
	if err := json.Unmarshal(resp.Body, &ensureResp); err != nil {
		log("ensure-cloud-user parse error: %v  body=%s", err, string(resp.Body))
		return nil, fmt.Errorf("failed to parse ensure-cloud-user response: %w", err)
	}

	result := &SetupResult{
		Email:        ensureResp.Email,
		FirstName:    ensureResp.User.FirstName,
		LastName:     ensureResp.User.LastName,
		AvatarURL:    cbData.Result.Data.ProfilePicURL,
		UserID:       ensureResp.UserID.String(),
		OrgID:        ensureResp.OrgID.String(),
		AccessToken:  ensureResp.Tokens.AccessToken,
		RefreshToken: ensureResp.Tokens.RefreshToken,
	}

	if len(ensureResp.Organizations) > 0 {
		result.OrgName = ensureResp.Organizations[0].Name
		if result.OrgID == "" {
			result.OrgID = ensureResp.Organizations[0].ID.String()
		}
	}

	apiKeyReq := CreateAPIKeyRequest{Label: "LRC CLI Key"}
	apiKeyURL := network.SetupCreateAPIKeyURL(CloudAPIURL, result.OrgID)
	log("creating API key: POST %s", apiKeyURL)
	resp2, err := network.SetupCreateAPIKey(client, CloudAPIURL, result.OrgID, apiKeyReq, result.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}
	if resp2.StatusCode != http.StatusCreated && resp2.StatusCode != http.StatusOK {
		// Do not log/echo response bodies here because create-key responses can contain plaintext secrets.
		log("create API key failed: status=%d", resp2.StatusCode)
		return nil, fmt.Errorf("create API key returned %d", resp2.StatusCode)
	}

	log("API key created: status=%d", resp2.StatusCode)

	var apiKeyResp CreateAPIKeyResponse
	if err := json.Unmarshal(resp2.Body, &apiKeyResp); err != nil {
		return nil, fmt.Errorf("failed to parse API key response: %w", err)
	}

	result.PlainAPIKey = apiKeyResp.PlainKey
	return result, nil
}
