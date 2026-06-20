package setup

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/network"
)

// ProvisionLiveReviewUser calls ensure-cloud-user and creates an API key.
// Optional logf receives debug log messages.
func ProvisionLiveReviewUser(cbData *HexmosCallbackData, apiURL string, logf func(format string, args ...interface{})) (*SetupResult, error) {
	log := func(format string, args ...interface{}) {
		if logf != nil {
			logf(format, args...)
		}
	}

	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		apiURL = CloudAPIURL
	}

	reqBody := EnsureCloudUserRequest{
		Email:     cbData.Result.Data.Email,
		FirstName: cbData.Result.Data.FirstName,
		LastName:  cbData.Result.Data.LastName,
		Source:    "git-lrc",
	}

	client := network.NewSetupClient(30 * time.Second)
	resp, err := network.SetupEnsureCloudUser(client, apiURL, reqBody, cbData.Result.JWT)
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

	plainKey, err := createSetupAPIKey(client, apiURL, result.OrgID, result.AccessToken, log)
	if err != nil {
		return nil, err
	}

	result.PlainAPIKey = plainKey
	return result, nil
}
