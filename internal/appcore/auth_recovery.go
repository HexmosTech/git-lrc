package appcore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	cfgutil "github.com/HexmosTech/git-lrc/config"
	"github.com/HexmosTech/git-lrc/configpath"
	"github.com/HexmosTech/git-lrc/internal/reviewapi"
	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	"github.com/HexmosTech/git-lrc/network"
	"github.com/HexmosTech/git-lrc/storage"
)

const liveReviewAPIKeyInvalidCode = "LIVE_REVIEW_API_KEY_INVALID"

type createAPIKeyRuntimeRequest struct {
	Label string `json:"label"`
}

type createAPIKeyRuntimeResponse struct {
	PlainKey string `json:"plain_key"`
}

type refreshTokenRuntimeRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type refreshTokenRuntimeResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type authRecoveryDiagnostic struct {
	Timestamp       string `json:"timestamp"`
	Phase           string `json:"phase"`
	TriggerCode     string `json:"trigger_code"`
	Recovered       bool   `json:"recovered"`
	RetryAttempted  bool   `json:"retry_attempted"`
	UsedRefreshFlow bool   `json:"used_refresh_flow"`

	HasOrgID        bool `json:"has_org_id"`
	HasJWT          bool `json:"has_jwt"`
	HasRefreshToken bool `json:"has_refresh_token"`

	APIURL        string `json:"api_url"`
	ClientOS      string `json:"client_os"`
	ClientArch    string `json:"client_arch"`
	ClientVersion string `json:"client_version"`

	FailureReason string `json:"failure_reason,omitempty"`
	DurationMS    int64  `json:"duration_ms"`
}

func parseAPIErrorCode(rawBody string) (string, error) {
	trimmed := strings.TrimSpace(rawBody)
	if trimmed == "" {
		return "", nil
	}
	var payload struct {
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return "", fmt.Errorf("failed to parse error_code from response body: %w", err)
	}
	return strings.TrimSpace(payload.ErrorCode), nil
}

func isLiveReviewAPIKeyInvalid(err error) bool {
	var apiErr *reviewmodel.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		return false
	}
	code, parseErr := parseAPIErrorCode(apiErr.Body)
	if parseErr != nil {
		fmt.Printf("Warning: could not parse unauthorized error response: %v\n", parseErr)
		return false
	}
	return code == liveReviewAPIKeyInvalidCode
}

func submitReviewWithRecovery(config Config, base64Diff, repoName string, verbose bool) (reviewmodel.DiffReviewCreateResponse, Config, error) {
	submitResp, err := reviewapi.SubmitReview(config.APIURL, config.APIKey, base64Diff, repoName, verbose)
	if err == nil {
		return submitResp, config, nil
	}
	if !isLiveReviewAPIKeyInvalid(err) {
		return reviewmodel.DiffReviewCreateResponse{}, config, err
	}

	recoveredConfig, recErr := recoverAPIKeyAndTokens(config, "submit")
	if recErr != nil {
		return reviewmodel.DiffReviewCreateResponse{}, config, fmt.Errorf("auto-recovery failed after %s: %w", liveReviewAPIKeyInvalidCode, recErr)
	}

	fmt.Println("Retrying review submission with refreshed credentials...")
	retryResp, retryErr := reviewapi.SubmitReview(recoveredConfig.APIURL, recoveredConfig.APIKey, base64Diff, repoName, verbose)
	if retryErr != nil {
		return reviewmodel.DiffReviewCreateResponse{}, recoveredConfig, retryErr
	}
	return retryResp, recoveredConfig, nil
}

func pollReviewWithRecovery(config Config, reviewID string, pollInterval, timeout time.Duration, verbose bool, cancel <-chan struct{}, statusSink func(string)) (*reviewmodel.DiffReviewResponse, Config, error) {
	result, err := reviewapi.PollReview(config.APIURL, config.APIKey, reviewID, pollInterval, timeout, verbose, cancel, statusSink)
	if err == nil {
		return result, config, nil
	}
	if !isLiveReviewAPIKeyInvalid(err) {
		return nil, config, err
	}

	recoveredConfig, recErr := recoverAPIKeyAndTokens(config, "poll")
	if recErr != nil {
		return nil, config, fmt.Errorf("auto-recovery failed after %s: %w", liveReviewAPIKeyInvalidCode, recErr)
	}

	fmt.Println("Retrying review polling with refreshed credentials...")
	retryResult, retryErr := reviewapi.PollReview(recoveredConfig.APIURL, recoveredConfig.APIKey, reviewID, pollInterval, timeout, verbose, cancel, statusSink)
	if retryErr != nil {
		return nil, recoveredConfig, retryErr
	}
	return retryResult, recoveredConfig, nil
}

func recoverAPIKeyAndTokens(config Config, phase string) (Config, error) {
	started := time.Now()
	diag := authRecoveryDiagnostic{
		Timestamp:       started.UTC().Format(time.RFC3339),
		Phase:           phase,
		TriggerCode:     liveReviewAPIKeyInvalidCode,
		Recovered:       false,
		RetryAttempted:  true,
		UsedRefreshFlow: false,
		HasOrgID:        strings.TrimSpace(config.OrgID) != "",
		HasJWT:          strings.TrimSpace(config.JWT) != "",
		HasRefreshToken: strings.TrimSpace(config.RefreshToken) != "",
		APIURL:          config.APIURL,
		ClientOS:        runtime.GOOS,
		ClientArch:      runtime.GOARCH,
		ClientVersion:   version,
	}

	fmt.Println("LiveReview reported LIVE_REVIEW_API_KEY_INVALID.")
	fmt.Println("Attempting automatic API key recovery using your existing session...")

	if strings.TrimSpace(config.OrgID) == "" {
		diag.FailureReason = "missing org_id in config"
		reportDiagnosticWriteError(persistAuthRecoveryDiagnostic(&diag, time.Since(started)))
		fmt.Println("Automatic recovery unavailable: missing org_id in ~/.lrc.toml.")
		return config, fmt.Errorf("missing org_id in config")
	}

	// Try re-issuing API key with current JWT first.
	newKey, createStatus, createBody, err := createAPIKeyWithJWT(config.APIURL, config.OrgID, config.JWT)
	if err == nil {
		updated := config
		updated.APIKey = newKey
		if persistErr := persistConfigUpdates(updated.ConfigPath, updated.APIURL, map[string]string{"api_key": newKey}); persistErr != nil {
			diag.FailureReason = fmt.Sprintf("persist api_key failed: %v", persistErr)
			reportDiagnosticWriteError(persistAuthRecoveryDiagnostic(&diag, time.Since(started)))
			return config, fmt.Errorf("generated a new API key but failed to persist config: %w", persistErr)
		}
		if updated.ConfigPath == "" {
			resolved, pathErr := resolveConfigPath(updated.ConfigPath)
			if pathErr == nil {
				updated.ConfigPath = resolved
			}
		}
		fmt.Println("Updated ~/.lrc.toml with a newly issued API key.")
		diag.Recovered = true
		reportDiagnosticWriteError(persistAuthRecoveryDiagnostic(&diag, time.Since(started)))
		return updated, nil
	}

	if createStatus != http.StatusUnauthorized || strings.TrimSpace(config.RefreshToken) == "" {
		diag.FailureReason = fmt.Sprintf("create API key failed before refresh: status=%d", createStatus)
		reportDiagnosticWriteError(persistAuthRecoveryDiagnostic(&diag, time.Since(started)))
		return config, fmt.Errorf("create API key failed: %w body=%s", err, strings.TrimSpace(createBody))
	}

	diag.UsedRefreshFlow = true
	fmt.Println("Current JWT appears expired/invalid. Attempting refresh-token recovery...")
	newJWT, newRefresh, refreshStatus, refreshBody, refreshErr := refreshSessionTokens(config.APIURL, config.RefreshToken)
	if refreshErr != nil {
		diag.FailureReason = fmt.Sprintf("refresh token failed: status=%d", refreshStatus)
		reportDiagnosticWriteError(persistAuthRecoveryDiagnostic(&diag, time.Since(started)))
		return config, fmt.Errorf("failed to refresh session: %w body=%s", refreshErr, strings.TrimSpace(refreshBody))
	}

	updated := config
	updated.JWT = newJWT
	if strings.TrimSpace(newRefresh) != "" {
		updated.RefreshToken = newRefresh
	}
	if persistErr := persistConfigUpdates(updated.ConfigPath, updated.APIURL, map[string]string{"jwt": updated.JWT, "refresh_token": updated.RefreshToken}); persistErr != nil {
		diag.FailureReason = fmt.Sprintf("persist refreshed tokens failed: %v", persistErr)
		reportDiagnosticWriteError(persistAuthRecoveryDiagnostic(&diag, time.Since(started)))
		return config, fmt.Errorf("refreshed session tokens but failed to persist them: %w", persistErr)
	}
	if updated.ConfigPath == "" {
		resolved, pathErr := resolveConfigPath(updated.ConfigPath)
		if pathErr == nil {
			updated.ConfigPath = resolved
		}
	}
	fmt.Println("Session refreshed and tokens persisted to ~/.lrc.toml.")

	newKey, createStatus, createBody, err = createAPIKeyWithJWT(updated.APIURL, updated.OrgID, updated.JWT)
	if err != nil {
		diag.FailureReason = fmt.Sprintf("create API key after refresh failed: status=%d", createStatus)
		reportDiagnosticWriteError(persistAuthRecoveryDiagnostic(&diag, time.Since(started)))
		return config, fmt.Errorf("failed to create API key after refresh: %w body=%s", err, strings.TrimSpace(createBody))
	}

	updated.APIKey = newKey
	if persistErr := persistConfigUpdates(updated.ConfigPath, updated.APIURL, map[string]string{"api_key": updated.APIKey}); persistErr != nil {
		diag.FailureReason = fmt.Sprintf("persist recovered api_key failed: %v", persistErr)
		reportDiagnosticWriteError(persistAuthRecoveryDiagnostic(&diag, time.Since(started)))
		return config, fmt.Errorf("recovered API key but failed to persist config: %w", persistErr)
	}
	if updated.ConfigPath == "" {
		resolved, pathErr := resolveConfigPath(updated.ConfigPath)
		if pathErr == nil {
			updated.ConfigPath = resolved
		}
	}
	fmt.Println("Updated ~/.lrc.toml with a newly issued API key.")

	diag.Recovered = true
	reportDiagnosticWriteError(persistAuthRecoveryDiagnostic(&diag, time.Since(started)))
	return updated, nil
}

func createAPIKeyWithJWT(apiURL, orgID, jwtToken string) (string, int, string, error) {
	if strings.TrimSpace(jwtToken) == "" {
		return "", 0, "", fmt.Errorf("missing jwt token")
	}

	client := network.NewSetupClient(20 * time.Second)
	resp, err := network.SetupCreateAPIKey(client, apiURL, orgID, createAPIKeyRuntimeRequest{Label: "LRC Auto-Recovery Key"}, jwtToken)
	if err != nil {
		return "", 0, "", err
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, string(resp.Body), fmt.Errorf("create API key returned %d", resp.StatusCode)
	}

	var payload createAPIKeyRuntimeResponse
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return "", resp.StatusCode, string(resp.Body), fmt.Errorf("failed to parse create key response: %w", err)
	}
	if strings.TrimSpace(payload.PlainKey) == "" {
		return "", resp.StatusCode, string(resp.Body), fmt.Errorf("create key response missing plain_key")
	}

	return payload.PlainKey, resp.StatusCode, string(resp.Body), nil
}

func refreshSessionTokens(apiURL, refreshToken string) (string, string, int, string, error) {
	client := network.NewSetupClient(20 * time.Second)
	resp, err := network.SetupRefreshTokens(client, apiURL, refreshTokenRuntimeRequest{RefreshToken: refreshToken})
	if err != nil {
		return "", "", 0, "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", resp.StatusCode, string(resp.Body), fmt.Errorf("refresh returned %d", resp.StatusCode)
	}

	var payload refreshTokenRuntimeResponse
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return "", "", resp.StatusCode, string(resp.Body), fmt.Errorf("failed to parse refresh response: %w", err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", "", resp.StatusCode, string(resp.Body), fmt.Errorf("refresh response missing access_token")
	}

	return payload.AccessToken, payload.RefreshToken, resp.StatusCode, string(resp.Body), nil
}

func resolveConfigPath(configPath string) (string, error) {
	if strings.TrimSpace(configPath) != "" {
		return strings.TrimSpace(configPath), nil
	}
	return configpath.ResolveConfigPath()
}

func persistConfigUpdates(configPath, apiURL string, updates map[string]string) error {
	resolvedConfigPath, err := resolveConfigPath(configPath)
	if err != nil {
		return err
	}

	content := ""
	if existingBytes, err := storage.ReadConfigFile(resolvedConfigPath); err == nil {
		content = string(existingBytes)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	if strings.TrimSpace(content) == "" && strings.TrimSpace(apiURL) != "" {
		content = cfgutil.UpsertQuotedConfigValue(content, "api_url", apiURL)
	}

	for key, value := range updates {
		if strings.TrimSpace(value) == "" {
			continue
		}
		content = cfgutil.UpsertQuotedConfigValue(content, key, value)
	}

	return storage.WriteFileAtomically(resolvedConfigPath, []byte(content), 0600)
}

func persistAuthRecoveryDiagnostic(diag *authRecoveryDiagnostic, elapsed time.Duration) error {
	if diag == nil {
		return fmt.Errorf("diagnostic payload is nil")
	}
	diag.DurationMS = elapsed.Milliseconds()

	homeDir, err := configpath.ResolveHomeDir()
	if err != nil {
		return fmt.Errorf("failed to determine home directory for diagnostics: %w", err)
	}
	dir := filepath.Join(homeDir, ".lrc-diagnostics")
	if err := storage.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create diagnostics directory %s: %w", dir, err)
	}

	payload, err := json.MarshalIndent(diag, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal recovery diagnostics: %w", err)
	}

	filename := fmt.Sprintf("api-key-recovery-%s.json", time.Now().UTC().Format("20060102-150405.000"))
	path := filepath.Join(dir, filename)
	if err := storage.WriteFileAtomically(path, payload, 0600); err != nil {
		return fmt.Errorf("failed to persist diagnostics file %s: %w", path, err)
	}

	fmt.Printf("Recovery diagnostics saved: %s\n", path)
	return nil
}

func reportDiagnosticWriteError(err error) {
	if err != nil {
		fmt.Printf("Warning: failed to persist recovery diagnostics: %v\n", err)
	}
}
