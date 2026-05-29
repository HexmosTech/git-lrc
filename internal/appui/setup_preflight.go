package appui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/network"
	setuptpl "github.com/HexmosTech/git-lrc/setup"
	uicfg "github.com/HexmosTech/git-lrc/ui"
)

var loadSetupRuntimeConfig = uicfg.LoadRuntimeConfig
var newSetupPreflightClient = network.NewSetupClient

type setupPreflightResult struct {
	Session          *setupResult
	Connectors       []aiConnectorRemote
	AuthReady        bool
	HasConnector     bool
	SessionRefreshed bool
	APIKeyRecovered  bool
}

type setupRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type setupRefreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func runSetupPreflight(selectedAPIURL string, slog *setupLog) (*setupPreflightResult, error) {
	apiURL := strings.TrimSpace(selectedAPIURL)
	if apiURL == "" {
		apiURL = cloudAPIURL
	}

	cfg, err := loadSetupRuntimeConfig(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to load existing setup config: %w", err)
	}
	if cfg.ConfigMissing {
		return nil, nil
	}
	if strings.TrimSpace(cfg.ConfigErr) != "" {
		return nil, fmt.Errorf("failed to inspect existing setup config: %s", cfg.ConfigErr)
	}
	if strings.TrimSpace(cfg.APIURL) != apiURL {
		slog.write("setup preflight skipped: selected api_url=%s differs from config api_url=%s", apiURL, cfg.APIURL)
		return nil, nil
	}
	if strings.TrimSpace(cfg.JWT) == "" || strings.TrimSpace(cfg.OrgID) == "" {
		slog.write("setup preflight skipped: existing config missing jwt or org_id")
		return nil, nil
	}

	client := newSetupPreflightClient(20 * time.Second)
	connectors, authenticated, refreshed, err := setupPreflightConnectors(client, cfg, slog)
	if err != nil {
		return nil, err
	}
	if !authenticated {
		return nil, nil
	}

	result := &setupPreflightResult{
		AuthReady:        true,
		Connectors:       connectors,
		HasConnector:     len(connectors) > 0,
		SessionRefreshed: refreshed,
	}

	if strings.TrimSpace(cfg.APIKey) == "" {
		apiKey, err := setupPreflightCreateAPIKey(client, cfg)
		if err != nil {
			return nil, err
		}
		cfg.APIKey = strings.TrimSpace(apiKey)
		result.APIKeyRecovered = true
		if err := persistOrgContextToConfig(cfg.ConfigPath, cfg.OrgID, cfg.OrgName, cfg.APIKey); err != nil {
			return nil, fmt.Errorf("failed to persist recovered api_key: %w", err)
		}
	}

	result.Session = setupResultFromRuntimeConfig(cfg)
	return result, nil
}

func setupPreflightConnectors(client *network.Client, cfg *uiRuntimeConfig, slog *setupLog) ([]aiConnectorRemote, bool, bool, error) {
	resp, err := network.SetupListConnectors(client, cfg.APIURL, cfg.OrgID, cfg.JWT)
	if err != nil {
		return nil, false, false, fmt.Errorf("setup preflight connector probe failed: %w", err)
	}

	refreshed := false
	if resp.StatusCode == http.StatusUnauthorized {
		if strings.TrimSpace(cfg.RefreshJWT) == "" {
			slog.write("setup preflight skipped: connector probe unauthorized and refresh_token missing")
			return nil, false, false, nil
		}

		if err := setupPreflightRefreshTokens(client, cfg); err != nil {
			slog.write("setup preflight skipped: token refresh failed: %v", err)
			return nil, false, false, nil
		}
		refreshed = true

		resp, err = network.SetupListConnectors(client, cfg.APIURL, cfg.OrgID, cfg.JWT)
		if err != nil {
			return nil, false, true, fmt.Errorf("setup preflight connector retry failed: %w", err)
		}
	}

	if resp.StatusCode == http.StatusUnauthorized {
		slog.write("setup preflight skipped: connector probe remained unauthorized after refresh")
		return nil, false, refreshed, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false, refreshed, fmt.Errorf("setup preflight connector probe returned %d: %s", resp.StatusCode, strings.TrimSpace(string(resp.Body)))
	}
	if len(strings.TrimSpace(string(resp.Body))) == 0 {
		return []aiConnectorRemote{}, true, refreshed, nil
	}

	var connectors []aiConnectorRemote
	if err := json.Unmarshal(resp.Body, &connectors); err != nil {
		return nil, false, refreshed, fmt.Errorf("failed to decode existing connector inventory: %w", err)
	}

	return connectors, true, refreshed, nil
}

func setupPreflightRefreshTokens(client *network.Client, cfg *uiRuntimeConfig) error {
	resp, err := network.SetupRefreshTokens(client, cfg.APIURL, setupRefreshTokenRequest{RefreshToken: cfg.RefreshJWT})
	if err != nil {
		return fmt.Errorf("refresh request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh returned %d: %s", resp.StatusCode, strings.TrimSpace(string(resp.Body)))
	}

	var payload setupRefreshTokenResponse
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return fmt.Errorf("failed to parse refresh response: %w", err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return fmt.Errorf("refresh response missing access_token")
	}

	cfg.JWT = strings.TrimSpace(payload.AccessToken)
	if strings.TrimSpace(payload.RefreshToken) != "" {
		cfg.RefreshJWT = strings.TrimSpace(payload.RefreshToken)
	}

	if err := persistAuthTokensToConfig(cfg.ConfigPath, cfg.JWT, cfg.RefreshJWT); err != nil {
		return fmt.Errorf("failed to persist refreshed tokens: %w", err)
	}

	return nil
}

func setupPreflightCreateAPIKey(client *network.Client, cfg *uiRuntimeConfig) (string, error) {
	resp, err := network.SetupCreateAPIKey(client, cfg.APIURL, cfg.OrgID, setuptpl.CreateAPIKeyRequest{Label: "LRC Setup Recovery Key"}, cfg.JWT)
	if err != nil {
		return "", fmt.Errorf("failed to create api key from existing session: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create api key returned %d: %s", resp.StatusCode, strings.TrimSpace(string(resp.Body)))
	}

	var payload setuptpl.CreateAPIKeyResponse
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return "", fmt.Errorf("failed to parse api key response: %w", err)
	}
	if strings.TrimSpace(payload.PlainKey) == "" {
		return "", fmt.Errorf("create api key response missing plain_key")
	}

	return strings.TrimSpace(payload.PlainKey), nil
}

func setupResultFromRuntimeConfig(cfg *uiRuntimeConfig) *setupResult {
	claims := decodeJWTClaims(cfg.JWT)
	return &setupResult{
		Email:        firstNonEmpty(cfg.UserEmail, claims["email"]),
		FirstName:    firstNonEmpty(cfg.FirstName, claims["given_name"], claims["first_name"]),
		LastName:     firstNonEmpty(cfg.LastName, claims["family_name"], claims["last_name"]),
		AvatarURL:    firstNonEmpty(cfg.AvatarURL, claims["picture"], claims["avatar_url"]),
		UserID:       strings.TrimSpace(cfg.UserID),
		OrgID:        strings.TrimSpace(cfg.OrgID),
		OrgName:      strings.TrimSpace(cfg.OrgName),
		AccessToken:  strings.TrimSpace(cfg.JWT),
		RefreshToken: strings.TrimSpace(cfg.RefreshJWT),
		PlainAPIKey:  strings.TrimSpace(cfg.APIKey),
	}
}

func existingSetupLabel(preflight *setupPreflightResult) string {
	if preflight == nil || preflight.Session == nil {
		return "this machine"
	}
	label := strings.TrimSpace(preflight.Session.Email)
	if label == "" {
		label = strings.TrimSpace(preflight.Session.UserID)
	}
	if label == "" {
		label = "this machine"
	}
	return label
}

func printSetupAlreadyReady(preflight *setupPreflightResult) {
	label := existingSetupLabel(preflight)
	connectorWord := "connectors"
	if len(preflight.Connectors) == 1 {
		connectorWord = "connector"
	}

	fmt.Printf("  %s✅ Existing LiveReview setup is already ready%s\n", clr(cGreen), clr(cReset))
	fmt.Printf("  %s   Account:%s %s%s%s\n", clr(cDim), clr(cReset), clr(cBold), label, clr(cReset))
	fmt.Printf("  %s   AI:%s %d %s configured\n", clr(cDim), clr(cReset), len(preflight.Connectors), connectorWord)
	if preflight.SessionRefreshed {
		fmt.Printf("  %s   Refreshed your saved LiveReview session.%s\n", clr(cDim), clr(cReset))
	}
	if preflight.APIKeyRecovered {
		fmt.Printf("  %s   Recovered a missing API key from your existing session.%s\n", clr(cDim), clr(cReset))
	}
	fmt.Println()
}

func printSetupReuseExistingSession(preflight *setupPreflightResult) {
	label := existingSetupLabel(preflight)
	fmt.Printf("  %s%sStep 1/2%s  🔑 Reuse existing Hexmos session\n", clr(cBold), clr(cBlue), clr(cReset))
	fmt.Println()
	fmt.Printf("  %s✅ Reusing existing session for %s%s%s\n", clr(cGreen), clr(cBold), label, clr(cReset))
	if preflight != nil && preflight.Session != nil && strings.TrimSpace(preflight.Session.OrgName) != "" {
		fmt.Printf("  %s   Organization: %s%s\n", clr(cDim), preflight.Session.OrgName, clr(cReset))
	}
	if preflight != nil && preflight.SessionRefreshed {
		fmt.Printf("  %s   Refreshed your saved LiveReview session before continuing.%s\n", clr(cDim), clr(cReset))
	}
	if preflight != nil && preflight.APIKeyRecovered {
		fmt.Printf("  %s   Recovered a missing API key from your existing session.%s\n", clr(cDim), clr(cReset))
	}
	fmt.Println()
}
