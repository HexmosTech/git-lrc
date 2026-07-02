package setup

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/network"
)

// IsCloudAPIURL reports whether apiURL targets the hosted LiveReview cloud backend.
// Empty apiURL is treated as cloud because setup defaults to CloudAPIURL.
func IsCloudAPIURL(apiURL string) bool {
	normalized := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if normalized == "" {
		return true
	}
	
	client := network.NewSetupClient(5 * time.Second)
	if resp, err := network.SetupUIConfig(client, normalized); err == nil && resp.StatusCode == 200 {
		var uiConfig struct {
			IsCloud bool `json:"isCloud"`
		}
		if err := json.Unmarshal(resp.Body, &uiConfig); err == nil {
			return uiConfig.IsCloud
		}
	}

	return strings.EqualFold(normalized, CloudAPIURL) || strings.Contains(normalized, "hexmos.com")
}
