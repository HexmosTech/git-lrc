package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
	"github.com/urfave/cli/v2"
)

const (
	cloudAPIURL        = "https://livereview.hexmos.com"
	hexmosSigninBase   = "https://hexmos.com/signin"
	geminiKeysURL      = "https://aistudio.google.com/api-keys"
	defaultGeminiModel = "gemini-2.5-flash"
	setupTimeout       = 5 * time.Minute
	issuesURL          = "https://github.com/HexmosTech/git-lrc/issues/new"
)

// ── ANSI color helpers ──────────────────────────────────────────────

const (
	cReset  = "\033[0m"
	cBold   = "\033[1m"
	cDim    = "\033[2m"
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
	cRed    = "\033[31m"
	cCyan   = "\033[36m"
	cBlue   = "\033[34m"
)

// colorsEnabled reports whether the terminal supports ANSI colors.
// On Windows, colors are disabled unless running in Windows Terminal or similar.
func colorsEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if runtime.GOOS == "windows" {
		// Windows Terminal and modern terminals set WT_SESSION or TERM_PROGRAM
		if os.Getenv("WT_SESSION") != "" || os.Getenv("TERM_PROGRAM") != "" {
			return true
		}
		return false
	}
	return true
}

func init() {
	if !colorsEnabled() {
		// Zero out all color constants by reassigning via package-level vars
		setupColors = false
	}
}

var setupColors = true

// c returns the ANSI code if colors are enabled, else empty string.
func clr(code string) string {
	if setupColors {
		return code
	}
	return ""
}

// hyperlink renders an OSC 8 clickable terminal hyperlink.
// Falls back to plain text on terminals that don't support it.
func hyperlink(linkURL, text string) string {
	if !setupColors {
		return text + " (" + linkURL + ")"
	}
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", linkURL, text)
}

// ── Setup debug logger ──────────────────────────────────────────────

// setupLog captures debug output during setup for issue reporting.
type setupLog struct {
	entries []string
	logFile string
}

func newSetupLog() *setupLog {
	logFile := ""
	if homeDir, err := os.UserHomeDir(); err == nil {
		logFile = filepath.Join(homeDir, ".lrc-setup.log")
	} else {
		// Fall back to temp dir if home dir unavailable (e.g. restricted environments)
		logFile = filepath.Join(os.TempDir(), "lrc-setup.log")
	}
	sl := &setupLog{logFile: logFile}
	sl.write("=== lrc setup started at %s ===", time.Now().Format(time.RFC3339))
	sl.write("lrc version: %s  build: %s  commit: %s", version, buildTime, gitCommit)
	sl.write("os: %s/%s", runtime.GOOS, runtime.GOARCH)
	return sl
}

func (sl *setupLog) write(format string, args ...interface{}) {
	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
	sl.entries = append(sl.entries, entry)
}

func (sl *setupLog) flush() {
	content := strings.Join(sl.entries, "\n") + "\n"
	if err := os.WriteFile(sl.logFile, []byte(content), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not write debug log to %s: %v\n", sl.logFile, err)
	}
}

// buildIssueURL creates a pre-filled GitHub issue URL with log contents.
func (sl *setupLog) buildIssueURL(errMsg string) string {
	// Truncate log to fit in a URL (GitHub has limits ~8000 chars for URL)
	logContent := strings.Join(sl.entries, "\n")
	const maxLogLen = 4000
	if len(logContent) > maxLogLen {
		logContent = logContent[len(logContent)-maxLogLen:]
		logContent = "...(truncated)\n" + logContent
	}

	body := fmt.Sprintf("## `lrc setup` failed\n\n**Error:** `%s`\n\n**Version:** %s (%s, %s)\n**OS:** %s/%s\n\n<details>\n<summary>Debug log</summary>\n\n```\n%s\n```\n</details>\n",
		errMsg, version, buildTime, gitCommit, runtime.GOOS, runtime.GOARCH, logContent)

	params := url.Values{}
	params.Set("title", "lrc setup: "+errMsg)
	params.Set("body", body)
	params.Set("labels", "bug,setup")

	return issuesURL + "?" + params.Encode()
}

// setupResult holds the data collected during the setup flow.
type setupResult struct {
	Email        string
	FirstName    string
	LastName     string
	UserID       string
	OrgID        string
	OrgName      string
	AccessToken  string
	RefreshToken string
	PlainAPIKey  string
}

// hexmosCallbackData models the ?data= JSON from Hexmos Login redirect.
type hexmosCallbackData struct {
	Result struct {
		JWT  string `json:"jwt"`
		Data struct {
			Email         string `json:"email"`
			Username      string `json:"username"`
			FirstName     string `json:"first_name"`
			LastName      string `json:"last_name"`
			ProfilePicURL string `json:"profilePicUrl"`
		} `json:"data"`
	} `json:"result"`
}

// ensureCloudUserRequest is the body for POST /api/v1/auth/ensure-cloud-user.
type ensureCloudUserRequest struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// ensureCloudUserResponse models the response from ensure-cloud-user.
// ID fields use json.Number because the API may return them as integers.
type ensureCloudUserResponse struct {
	Status string      `json:"status"`
	UserID json.Number `json:"user_id"`
	OrgID  json.Number `json:"org_id"`
	Email  string      `json:"email"`
	User   struct {
		ID        json.Number `json:"id"`
		Email     string      `json:"email"`
		FirstName string      `json:"first_name"`
		LastName  string      `json:"last_name"`
	} `json:"user"`
	Tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    string `json:"expires_at"`
	} `json:"tokens"`
	Organizations []struct {
		ID   json.Number `json:"id"`
		Name string      `json:"name"`
	} `json:"organizations"`
}

// createAPIKeyRequest is the body for POST /api/v1/orgs/:org_id/api-keys.
type createAPIKeyRequest struct {
	Label string `json:"label"`
}

// createAPIKeyResponse models the response from creating an API key.
type createAPIKeyResponse struct {
	APIKey struct {
		ID    json.Number `json:"id"`
		Label string      `json:"label"`
	} `json:"api_key"`
	PlainKey string `json:"plain_key"`
}

// validateKeyRequest is the body for POST /api/v1/aiconnectors/validate-key.
type validateKeyRequest struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model,omitempty"`
}

// validateKeyResponse models the response from validate-key.
type validateKeyResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}

// createConnectorRequest is the body for POST /api/v1/aiconnectors.
type createConnectorRequest struct {
	ProviderName  string `json:"provider_name"`
	APIKey        string `json:"api_key"`
	ConnectorName string `json:"connector_name"`
	SelectedModel string `json:"selected_model"`
	DisplayOrder  int    `json:"display_order"`
}

// runSetup is the handler for "lrc setup".
func runSetup(c *cli.Context) error {
	slog := newSetupLog()

	fmt.Println()
	fmt.Printf("  %s%s🔧 LiveReview CLI Setup%s\n", clr(cBold), clr(cCyan), clr(cReset))
	fmt.Printf("  %s─────────────────────────%s\n", clr(cDim), clr(cReset))
	fmt.Println()

	// Phase 0: Backup existing config if present
	if err := backupExistingConfig(slog); err != nil {
		return setupError(slog, err)
	}

	// Phase 1: Hexmos Login via browser
	fmt.Printf("  %s%sStep 1/2%s  🔑 Authenticate with Hexmos\n", clr(cBold), clr(cBlue), clr(cReset))
	fmt.Println()
	slog.write("phase 1: starting hexmos login flow")

	result, err := runHexmosLoginFlow(slog)
	if err != nil {
		return setupError(slog, fmt.Errorf("authentication failed: %w", err))
	}

	fmt.Printf("  %s✅ Authenticated as %s%s%s\n", clr(cGreen), clr(cBold), result.Email, clr(cReset))
	if result.OrgName != "" {
		fmt.Printf("  %s   Organization: %s%s\n", clr(cDim), result.OrgName, clr(cReset))
	}
	fmt.Println()
	slog.write("phase 1 complete: user=%s org=%s", result.Email, result.OrgID)

	// Phase 2: Gemini API key
	fmt.Printf("  %s%sStep 2/2%s  🤖 Configure AI (Gemini)\n", clr(cBold), clr(cBlue), clr(cReset))
	fmt.Println()
	fmt.Printf("  You need a Gemini API key for AI-powered code reviews.\n")
	fmt.Printf("  Get a free key from: %s\n", hyperlink(geminiKeysURL, clr(cCyan)+geminiKeysURL+clr(cReset)))
	fmt.Println()
	slog.write("phase 2: prompting for gemini key")

	openBrowser(geminiKeysURL)

	geminiKey, err := promptGeminiKey(result, slog)
	if err != nil {
		return setupError(slog, fmt.Errorf("gemini setup failed: %w", err))
	}

	// Create AI connector
	slog.write("creating gemini connector")
	if err := createGeminiConnector(result, geminiKey); err != nil {
		return setupError(slog, fmt.Errorf("failed to create AI connector: %w", err))
	}
	fmt.Printf("  %s✅ Gemini connector created%s %s(model: %s)%s\n", clr(cGreen), clr(cReset), clr(cDim), defaultGeminiModel, clr(cReset))
	fmt.Println()
	slog.write("gemini connector created")

	// Phase 3: Write config
	if err := writeConfig(result); err != nil {
		return setupError(slog, fmt.Errorf("failed to write config: %w", err))
	}
	slog.write("config written to ~/.lrc.toml")

	// Phase 4: Success message
	printSetupSuccess(result)

	// Clean up log on success (no need to keep it)
	if err := os.Remove(slog.logFile); err != nil && !os.IsNotExist(err) {
		slog.write("warning: could not remove log file: %v", err)
	}
	return nil
}

// setupError logs the error, writes the debug log, and prints a helpful message with issue link.
func setupError(slog *setupLog, err error) error {
	errMsg := err.Error()
	slog.write("ERROR: %s", errMsg)
	slog.flush()

	fmt.Println()
	fmt.Printf("  %s%s❌ Setup failed%s\n", clr(cBold), clr(cRed), clr(cReset))
	fmt.Printf("  %s%s%s\n", clr(cRed), errMsg, clr(cReset))
	fmt.Println()
	fmt.Printf("  %sDebug log saved to:%s %s%s%s\n", clr(cDim), clr(cReset), clr(cYellow), slog.logFile, clr(cReset))
	fmt.Println()

	issueURL := slog.buildIssueURL(errMsg)
	fmt.Printf("  %s🐛 Report this issue:%s\n", clr(cBold), clr(cReset))
	fmt.Printf("     %s\n", hyperlink(issueURL, clr(cCyan)+issuesURL+clr(cReset)))
	fmt.Println()
	fmt.Printf("  %s(The link above pre-fills the issue with your debug log)%s\n", clr(cDim), clr(cReset))
	fmt.Println()

	return err
}

// backupExistingConfig backs up ~/.lrc.toml if it exists and contains an api_key.
func backupExistingConfig(slog *setupLog) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		slog.write("cannot determine home directory: %v", err)
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".lrc.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		slog.write("no existing config found")
		return nil // file doesn't exist, nothing to back up
	}

	// Parse TOML to check for a real api_key value (not just a comment)
	k := koanf.New(".")
	if err := k.Load(rawbytes.Provider(data), toml.Parser()); err == nil {
		if k.String("api_key") == "" {
			return nil // no api_key value, not a meaningful config
		}
	}

	backupPath := configPath + ".bak." + time.Now().Format("20060102-150405")
	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return fmt.Errorf("failed to backup existing config: %w", err)
	}

	slog.write("backed up existing config to %s", backupPath)
	fmt.Printf("  %s📦 Existing config backed up to:%s %s%s%s\n", clr(cYellow), clr(cReset), clr(cDim), backupPath, clr(cReset))
	fmt.Println()
	return nil
}

// runHexmosLoginFlow starts a temporary server, opens the browser for Hexmos Login,
// waits for the callback, and provisions the user in LiveReview.
func runHexmosLoginFlow(slog *setupLog) (*setupResult, error) {
	// Start listener on random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to start listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	// Channel to receive callback data
	dataCh := make(chan *hexmosCallbackData, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()

	// Landing page: auto-redirect to Hexmos Login
	signinURL := fmt.Sprintf("%s?app=livereview&appRedirectURI=%s",
		hexmosSigninBase, url.QueryEscape(callbackURL))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, setupLandingHTML, signinURL, signinURL)
	})

	// Callback handler: receives ?data= from Hexmos Login
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		dataParam := r.URL.Query().Get("data")
		if dataParam == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, setupErrorHTML)
			errCh <- fmt.Errorf("no data parameter in callback")
			return
		}

		var cbData hexmosCallbackData
		if err := json.Unmarshal([]byte(dataParam), &cbData); err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, setupErrorHTML)
			errCh <- fmt.Errorf("failed to parse callback data: %w", err)
			return
		}

		if cbData.Result.JWT == "" || cbData.Result.Data.Email == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, setupErrorHTML)
			errCh <- fmt.Errorf("incomplete callback data (missing JWT or email)")
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, setupSuccessHTML)
		dataCh <- &cbData
	})

	server := &http.Server{Handler: mux}

	// Start server in background
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("server error: %w", err)
		}
	}()

	localURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	fmt.Printf("  🌐 Opening browser for Hexmos Login...\n")
	fmt.Printf("     %sIf it doesn't open, visit:%s %s\n", clr(cDim), clr(cReset), hyperlink(localURL, clr(cCyan)+localURL+clr(cReset)))
	fmt.Println()
	slog.write("local server on port %d, signin url: %s", port, signinURL)

	openBrowser(localURL)

	// Wait for callback or timeout
	var cbData *hexmosCallbackData
	select {
	case cbData = <-dataCh:
		// success
	case err := <-errCh:
		server.Shutdown(context.Background())
		return nil, err
	case <-time.After(setupTimeout):
		server.Shutdown(context.Background())
		return nil, fmt.Errorf("timed out waiting for login (5 minutes)")
	}

	// Shut down the temporary server
	go server.Shutdown(context.Background())

	slog.write("callback received, provisioning user")

	// Provision user in LiveReview
	return provisionLiveReviewUser(cbData, slog)
}

// provisionLiveReviewUser calls ensure-cloud-user and creates an API key.
func provisionLiveReviewUser(cbData *hexmosCallbackData, slog *setupLog) (*setupResult, error) {
	// Step 1: ensure-cloud-user
	reqBody := ensureCloudUserRequest{
		Email:     cbData.Result.Data.Email,
		FirstName: cbData.Result.Data.FirstName,
		LastName:  cbData.Result.Data.LastName,
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", cloudAPIURL+"/api/v1/auth/ensure-cloud-user", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cbData.Result.JWT)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to contact LiveReview API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read ensure-cloud-user response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		slog.write("ensure-cloud-user failed: status=%d body=%s", resp.StatusCode, string(respBody))
		return nil, fmt.Errorf("ensure-cloud-user returned %d: %s", resp.StatusCode, string(respBody))
	}

	slog.write("ensure-cloud-user: status=%d", resp.StatusCode)

	var ensureResp ensureCloudUserResponse
	if err := json.Unmarshal(respBody, &ensureResp); err != nil {
		slog.write("ensure-cloud-user parse error: %v  body=%s", err, string(respBody))
		return nil, fmt.Errorf("failed to parse ensure-cloud-user response: %w", err)
	}

	result := &setupResult{
		Email:        ensureResp.Email,
		FirstName:    ensureResp.User.FirstName,
		LastName:     ensureResp.User.LastName,
		UserID:       ensureResp.UserID.String(),
		OrgID:        ensureResp.OrgID.String(),
		AccessToken:  ensureResp.Tokens.AccessToken,
		RefreshToken: ensureResp.Tokens.RefreshToken,
	}

	// Use the first org name if available
	if len(ensureResp.Organizations) > 0 {
		result.OrgName = ensureResp.Organizations[0].Name
		// Prefer the org_id from the response, but fall back to first org
		if result.OrgID == "" {
			result.OrgID = ensureResp.Organizations[0].ID.String()
		}
	}

	// Step 2: create API key
	apiKeyReq := createAPIKeyRequest{Label: "LRC CLI Key"}
	apiKeyJSON, err := json.Marshal(apiKeyReq)
	if err != nil {
		return nil, err
	}

	apiKeyURL := fmt.Sprintf("%s/api/v1/orgs/%s/api-keys", cloudAPIURL, result.OrgID)
	slog.write("creating API key: POST %s", apiKeyURL)
	req2, err := http.NewRequest("POST", apiKeyURL, bytes.NewReader(apiKeyJSON))
	if err != nil {
		return nil, err
	}
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+result.AccessToken)

	resp2, err := client.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}
	defer resp2.Body.Close()

	respBody2, err := io.ReadAll(resp2.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read API key response: %w", err)
	}
	if resp2.StatusCode != http.StatusCreated && resp2.StatusCode != http.StatusOK {
		slog.write("create API key failed: status=%d body=%s", resp2.StatusCode, string(respBody2))
		return nil, fmt.Errorf("create API key returned %d: %s", resp2.StatusCode, string(respBody2))
	}

	slog.write("API key created: status=%d", resp2.StatusCode)

	var apiKeyResp createAPIKeyResponse
	if err := json.Unmarshal(respBody2, &apiKeyResp); err != nil {
		return nil, fmt.Errorf("failed to parse API key response: %w", err)
	}

	result.PlainAPIKey = apiKeyResp.PlainKey
	return result, nil
}

// promptGeminiKey reads the Gemini API key from stdin with up to 3 attempts.
func promptGeminiKey(result *setupResult, slog *setupLog) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	for attempt := 1; attempt <= 3; attempt++ {
		fmt.Printf("  %s🔑 Paste your Gemini API key:%s ", clr(cBold), clr(cReset))
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}

		key := strings.TrimSpace(line)
		if key == "" {
			fmt.Printf("  %s⚠  Key cannot be empty. Please try again.%s\n", clr(cYellow), clr(cReset))
			continue
		}

		slog.write("validating gemini key (attempt %d)", attempt)

		// Validate the key
		valid, msg, err := validateGeminiKey(result, key)
		if err != nil {
			slog.write("gemini key validation error: %v", err)
			fmt.Printf("  %s❌ Validation error: %v%s\n", clr(cRed), err, clr(cReset))
			if attempt < 3 {
				fmt.Printf("  %sPlease try again.%s\n", clr(cDim), clr(cReset))
			}
			continue
		}

		if !valid {
			slog.write("gemini key invalid: %s", msg)
			fmt.Printf("  %s❌ Invalid key: %s%s\n", clr(cRed), msg, clr(cReset))
			if attempt < 3 {
				fmt.Printf("  %sPlease try again.%s\n", clr(cDim), clr(cReset))
			}
			continue
		}

		slog.write("gemini key validated successfully")
		fmt.Printf("  %s✅ Key validated%s\n", clr(cGreen), clr(cReset))
		return key, nil
	}

	return "", fmt.Errorf("failed to provide a valid Gemini API key after 3 attempts")
}

// validateGeminiKey checks the key against LiveReview's validate-key endpoint.
func validateGeminiKey(result *setupResult, geminiKey string) (bool, string, error) {
	reqBody := validateKeyRequest{
		Provider: "gemini",
		APIKey:   geminiKey,
		Model:    defaultGeminiModel,
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return false, "", err
	}

	req, err := http.NewRequest("POST", cloudAPIURL+"/api/v1/aiconnectors/validate-key",
		bytes.NewReader(bodyJSON))
	if err != nil {
		return false, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+result.AccessToken)
	req.Header.Set("X-Org-Context", result.OrgID)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, "", fmt.Errorf("failed to validate key: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, "", fmt.Errorf("failed to read validation response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("validate-key returned %d: %s", resp.StatusCode, string(body))
	}

	var valResp validateKeyResponse
	if err := json.Unmarshal(body, &valResp); err != nil {
		return false, "", fmt.Errorf("failed to parse validation response: %w", err)
	}

	return valResp.Valid, valResp.Message, nil
}

// createGeminiConnector creates a Gemini AI connector in LiveReview.
func createGeminiConnector(result *setupResult, geminiKey string) error {
	reqBody := createConnectorRequest{
		ProviderName:  "gemini",
		APIKey:        geminiKey,
		ConnectorName: "Gemini Flash",
		SelectedModel: defaultGeminiModel,
		DisplayOrder:  0,
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", cloudAPIURL+"/api/v1/aiconnectors",
		bytes.NewReader(bodyJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+result.AccessToken)
	req.Header.Set("X-Org-Context", result.OrgID)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create connector: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read connector response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("create connector returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// writeConfig writes the setup results to ~/.lrc.toml.
func writeConfig(result *setupResult) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to determine home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".lrc.toml")

	content := fmt.Sprintf(`# LiveReview CLI configuration
# Generated by: lrc setup
# Date: %s

api_key = %q
api_url = %q
user_email = %q
user_id = %q
org_id = %q
jwt = %q
refresh_token = %q
`,
		time.Now().Format(time.RFC3339),
		result.PlainAPIKey,
		cloudAPIURL,
		result.Email,
		result.UserID,
		result.OrgID,
		result.AccessToken,
		result.RefreshToken,
	)

	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// printSetupSuccess prints the final success message.
func printSetupSuccess(result *setupResult) {
	keyPreview := result.PlainAPIKey
	if len(keyPreview) > 16 {
		keyPreview = keyPreview[:16] + "..."
	}

	fmt.Println()
	fmt.Printf("  %s%s🎉 Setup Complete!%s\n", clr(cBold), clr(cGreen), clr(cReset))
	fmt.Printf("  %s─────────────────────────%s\n", clr(cDim), clr(cReset))
	fmt.Println()
	fmt.Printf("  %s📧 Email:%s    %s\n", clr(cBold), clr(cReset), result.Email)
	if result.OrgName != "" {
		fmt.Printf("  %s🏢 Org:%s      %s\n", clr(cBold), clr(cReset), result.OrgName)
	}
	fmt.Printf("  %s🔑 API Key:%s  %s%s%s %s(~/.lrc.toml)%s\n", clr(cBold), clr(cReset), clr(cYellow), keyPreview, clr(cReset), clr(cDim), clr(cReset))
	fmt.Printf("  %s🤖 AI:%s       Gemini connector %s(%s)%s\n", clr(cBold), clr(cReset), clr(cDim), defaultGeminiModel, clr(cReset))
	fmt.Println()
	fmt.Printf("  %sIn a git repo with staged changes:%s\n", clr(cDim), clr(cReset))
	fmt.Println()
	fmt.Printf("    %s$ %sgit add .%s\n", clr(cDim), clr(cReset), clr(cReset))
	fmt.Printf("    %s$ %sgit lrc review%s        %s# AI-powered code review%s\n", clr(cDim), clr(cGreen), clr(cReset), clr(cDim), clr(cReset))
	fmt.Printf("    %s$ %sgit lrc review --vouch%s %s# mark as manually reviewed%s\n", clr(cDim), clr(cGreen), clr(cReset), clr(cDim), clr(cReset))
	fmt.Printf("    %s$ %sgit lrc review --skip%s  %s# skip review for this change%s\n", clr(cDim), clr(cGreen), clr(cReset), clr(cDim), clr(cReset))
	fmt.Println()
}

// HTML templates for the temporary setup server

const setupLandingHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>LiveReview Setup</title>
  <style>
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      display: flex;
      justify-content: center;
      align-items: center;
      min-height: 100vh;
      margin: 0;
      background: #f5f5f5;
      color: #333;
    }
    .card {
      background: white;
      border-radius: 12px;
      padding: 48px;
      box-shadow: 0 2px 12px rgba(0,0,0,0.1);
      text-align: center;
      max-width: 480px;
    }
    h1 { margin: 0 0 16px; font-size: 24px; }
    p { color: #666; line-height: 1.5; }
    a { color: #4F46E5; }
    .spinner {
      width: 40px; height: 40px;
      border: 4px solid #e5e7eb;
      border-top-color: #4F46E5;
      border-radius: 50%%;
      animation: spin 0.8s linear infinite;
      margin: 0 auto 24px;
    }
    @keyframes spin { to { transform: rotate(360deg); } }
  </style>
</head>
<body>
  <div class="card">
    <div class="spinner"></div>
    <h1>Redirecting to Hexmos Login</h1>
    <p>You'll be redirected automatically. If not, <a href="%s">click here</a>.</p>
  </div>
  <script>window.location.href = %q;</script>
</body>
</html>`

const setupSuccessHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>LiveReview Setup - Success</title>
  <style>
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      display: flex;
      justify-content: center;
      align-items: center;
      min-height: 100vh;
      margin: 0;
      background: #f5f5f5;
      color: #333;
    }
    .card {
      background: white;
      border-radius: 12px;
      padding: 48px;
      box-shadow: 0 2px 12px rgba(0,0,0,0.1);
      text-align: center;
      max-width: 480px;
    }
    h1 { margin: 0 0 16px; font-size: 24px; color: #059669; }
    p { color: #666; line-height: 1.5; }
    .check {
      width: 48px; height: 48px;
      background: #059669;
      border-radius: 50%;
      display: flex;
      align-items: center;
      justify-content: center;
      margin: 0 auto 24px;
      color: white;
      font-size: 24px;
    }
  </style>
</head>
<body>
  <div class="card">
    <div class="check">&#10003;</div>
    <h1>Authentication Successful</h1>
    <p>You can close this tab and return to your terminal to complete the setup.</p>
  </div>
</body>
</html>`

const setupErrorHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>LiveReview Setup - Error</title>
  <style>
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      display: flex;
      justify-content: center;
      align-items: center;
      min-height: 100vh;
      margin: 0;
      background: #f5f5f5;
      color: #333;
    }
    .card {
      background: white;
      border-radius: 12px;
      padding: 48px;
      box-shadow: 0 2px 12px rgba(0,0,0,0.1);
      text-align: center;
      max-width: 480px;
    }
    h1 { margin: 0 0 16px; font-size: 24px; color: #DC2626; }
    p { color: #666; line-height: 1.5; }
    .icon {
      width: 48px; height: 48px;
      background: #DC2626;
      border-radius: 50%;
      display: flex;
      align-items: center;
      justify-content: center;
      margin: 0 auto 24px;
      color: white;
      font-size: 24px;
    }
  </style>
</head>
<body>
  <div class="card">
    <div class="icon">&#10007;</div>
    <h1>Authentication Failed</h1>
    <p>Something went wrong. Please close this tab and try running <code>lrc setup</code> again.</p>
  </div>
</body>
</html>`
