package network

import (
	"fmt"
	"strings"
)

const (
	selfUpdateReleaseManifestURL = "https://f005.backblazeb2.com/file/hexmos/lrc/latest.json"
	selfUpdatePublicDownloadBase = "https://f005.backblazeb2.com/file/hexmos"
)

func SetupEnsureCloudUserURL(baseURL string) string {
	return strings.TrimSuffix(baseURL, "/") + "/api/v1/auth/ensure-cloud-user"
}

func SetupOnboardURL(baseURL string) string {
	return strings.TrimSuffix(baseURL, "/") + "/api/v1/auth/onboard"
}

func SetupAuthLoginURL(baseURL string) string {
	return strings.TrimSuffix(baseURL, "/") + "/api/v1/auth/login"
}

func SetupUIConfigURL(baseURL string) string {
	return strings.TrimSuffix(baseURL, "/") + "/api/v1/ui-config"
}

func SetupAuthSetupStatusURL(baseURL string) string {
	return strings.TrimSuffix(baseURL, "/") + "/api/v1/auth/setup-status"
}

func SetupAuthSetupURL(baseURL string) string {
	return strings.TrimSuffix(baseURL, "/") + "/api/v1/auth/setup"
}

func SetupAuthRefreshURL(baseURL string) string {
	return strings.TrimSuffix(baseURL, "/") + "/api/v1/auth/refresh"
}

func SetupCreateAPIKeyURL(baseURL, orgID string) string {
	return fmt.Sprintf("%s/api/v1/orgs/%s/api-keys", strings.TrimSuffix(baseURL, "/"), orgID)
}

func SetupValidateConnectorKeyURL(baseURL string) string {
	return strings.TrimSuffix(baseURL, "/") + "/api/v1/aiconnectors/validate-key"
}

func SetupCreateConnectorURL(baseURL string) string {
	return strings.TrimSuffix(baseURL, "/") + "/api/v1/aiconnectors"
}

func ReviewSubmitURL(apiURL string) string {
	return strings.TrimSuffix(apiURL, "/") + "/api/v1/diff-review"
}

func ReviewCLIUsageURL(apiURL string) string {
	return strings.TrimSuffix(apiURL, "/") + "/api/v1/diff-review/cli-used"
}

func ReviewPollURL(apiURL, reviewID string) string {
	return strings.TrimSuffix(apiURL, "/") + "/api/v1/diff-review/" + reviewID
}

func SelfUpdateManifestURL() string {
	return selfUpdateReleaseManifestURL
}

func SelfUpdateBinaryURL(binaryPath string) string {
	if strings.HasPrefix(binaryPath, "http://") || strings.HasPrefix(binaryPath, "https://") {
		return binaryPath
	}
	return fmt.Sprintf("%s/%s", strings.TrimRight(selfUpdatePublicDownloadBase, "/"), strings.TrimLeft(binaryPath, "/"))
}
