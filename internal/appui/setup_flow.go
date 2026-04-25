package appui

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"

	setuptpl "github.com/HexmosTech/git-lrc/setup"
	"github.com/HexmosTech/git-lrc/storage"
	"github.com/urfave/cli/v2"
)

// RunSetup is the handler for "lrc setup".
func RunSetup(c *cli.Context) error {
	slog := newSetupLog()

	fmt.Println()
	fmt.Printf("  %s%s🔧 git-lrc setup%s\n", clr(cBold), clr(cCyan), clr(cReset))
	fmt.Printf("  %s───────────────────%s\n", clr(cDim), clr(cReset))
	fmt.Println()

	targetAPIURL := resolveSetupTargetAPIURL(c.String("api-url"))
	selectedAPIURL, err := resolveSetupAPIURL(c, slog, targetAPIURL)
	if err != nil {
		return setupError(slog, err)
	}

	if err := backupExistingConfig(slog); err != nil {
		return setupError(slog, err)
	}

	fmt.Printf("  %s%sStep 1/2%s  🔑 Authenticate with Hexmos\n", clr(cBold), clr(cBlue), clr(cReset))
	fmt.Println()
	slog.write("phase 1: starting hexmos login flow")

	result, err := runHexmosLoginFlow(slog, selectedAPIURL)
	if err != nil {
		return setupError(slog, fmt.Errorf("authentication failed: %w", err))
	}

	fmt.Printf("  %s✅ Authenticated as %s%s%s\n", clr(cGreen), clr(cBold), result.Email, clr(cReset))
	if result.OrgName != "" {
		fmt.Printf("  %s   Organization: %s%s\n", clr(cDim), result.OrgName, clr(cReset))
	}
	fmt.Println()
	slog.write("phase 1 complete: user=%s org=%s", result.Email, result.OrgID)

	fmt.Printf("  %s%sStep 2/2%s  🤖 Configure AI (Gemini)\n", clr(cBold), clr(cBlue), clr(cReset))
	fmt.Println()
	fmt.Printf("  You need a Gemini API key for AI-powered code reviews.\n")
	fmt.Printf("  Get a free key from: %s\n", hyperlink(geminiKeysURL, clr(cCyan)+geminiKeysURL+clr(cReset)))
	fmt.Println()
	slog.write("phase 2: prompting for gemini key")

	if err := openURL(geminiKeysURL); err != nil {
		slog.write("warning: failed to auto-open Gemini keys URL: %v", err)
		fmt.Printf("  %s⚠ Could not open browser automatically.%s Open this URL manually: %s\n", clr(cYellow), clr(cReset), hyperlink(geminiKeysURL, clr(cCyan)+geminiKeysURL+clr(cReset)))
		fmt.Println()
	}

	geminiKey, err := promptGeminiKey(result, selectedAPIURL, slog)
	if err != nil {
		return setupError(slog, fmt.Errorf("gemini setup failed: %w", err))
	}

	slog.write("creating gemini connector")
	if err := createGeminiConnector(result, geminiKey, selectedAPIURL); err != nil {
		return setupError(slog, fmt.Errorf("failed to create AI connector: %w", err))
	}
	fmt.Printf("  %s✅ Gemini connector created%s %s(model: %s)%s\n", clr(cGreen), clr(cReset), clr(cDim), defaultGeminiModel, clr(cReset))
	fmt.Println()
	slog.write("gemini connector created")

	if err := writeConfig(result, selectedAPIURL); err != nil {
		return setupError(slog, fmt.Errorf("failed to write config: %w", err))
	}
	slog.write("config written to ~/.lrc.toml")

	printSetupSuccess(result)

	if err := storage.RemoveSetupLogFile(slog.logFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.write("warning: could not remove log file: %v", err)
	}
	return nil
}

func resolveSetupTargetAPIURL(raw string) string {
	target := strings.TrimSpace(raw)
	target = strings.TrimRight(target, "/")
	if target == "" {
		return cloudAPIURL
	}
	return target
}

func resolveSetupAPIURL(c *cli.Context, slog *setupLog, targetAPIURL string) (string, error) {
	keepAPIURL := c.Bool("keep-api-url")
	replaceAPIURL := c.Bool("replace-api-url")
	if keepAPIURL && replaceAPIURL {
		return "", fmt.Errorf("cannot combine --keep-api-url and --replace-api-url")
	}

	details, err := setuptpl.ReadExistingConfigDetails()
	if err != nil {
		return "", fmt.Errorf("failed to inspect existing config: %w", err)
	}

	if !details.Exists {
		slog.write("setup preflight: no existing config, using setup target api_url=%s", targetAPIURL)
		return targetAPIURL, nil
	}

	fmt.Printf("  %s⚠ Existing config detected:%s %s%s%s\n", clr(cYellow), clr(cReset), clr(cDim), details.Path, clr(cReset))
	if strings.TrimSpace(details.APIURL) != "" {
		fmt.Printf("  %sCurrent api_url:%s %s%s%s\n", clr(cDim), clr(cReset), clr(cCyan), details.APIURL, clr(cReset))
	} else {
		fmt.Printf("  %sCurrent config has no explicit api_url.%s\n", clr(cDim), clr(cReset))
	}
	fmt.Printf("  %sSetup target api_url:%s %s%s%s\n", clr(cDim), clr(cReset), clr(cCyan), targetAPIURL, clr(cReset))

	if keepAPIURL {
		selected := retainedAPIURL(details.APIURL, targetAPIURL)
		slog.write("setup preflight: selected keep-api-url, api_url=%s", selected)
		fmt.Printf("  %sUsing existing api_url.%s\n\n", clr(cGreen), clr(cReset))
		return selected, nil
	}

	if replaceAPIURL {
		slog.write("setup preflight: selected replace-api-url, api_url=%s", targetAPIURL)
		fmt.Printf("  %sReplacing api_url with setup target.%s\n\n", clr(cYellow), clr(cReset))
		return targetAPIURL, nil
	}

	nonInteractive := c.Bool("yes") || !isInteractiveSetupStdin()
	if nonInteractive {
		return "", fmt.Errorf("config exists at %s; pass --keep-api-url or --replace-api-url before running non-interactively", details.Path)
	}

	keep, promptErr := promptSetupYesNo("  Keep existing api_url?", true)
	if promptErr != nil {
		return "", fmt.Errorf("failed to read setup preflight choice: %w", promptErr)
	}

	if keep {
		selected := retainedAPIURL(details.APIURL, targetAPIURL)
		slog.write("setup preflight: interactive keep selected, api_url=%s", selected)
		fmt.Printf("  %sUsing existing api_url.%s\n\n", clr(cGreen), clr(cReset))
		return selected, nil
	}

	slog.write("setup preflight: interactive replace selected, api_url=%s", targetAPIURL)
	fmt.Printf("  %sReplacing api_url with setup target.%s\n\n", clr(cYellow), clr(cReset))
	return targetAPIURL, nil
}

func retainedAPIURL(existing string, fallback string) string {
	trimmed := strings.TrimSpace(existing)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func promptSetupYesNo(question string, defaultYes bool) (bool, error) {
	if !isInteractiveSetupStdin() {
		return defaultYes, nil
	}

	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}

	fmt.Printf("%s %s: ", question, suffix)
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read setup confirmation input: %w", err)
	}

	trimmed := strings.ToLower(strings.TrimSpace(answer))
	if trimmed == "" {
		return defaultYes, nil
	}
	if trimmed == "y" || trimmed == "yes" {
		return true, nil
	}
	if trimmed == "n" || trimmed == "no" {
		return false, nil
	}

	return defaultYes, nil
}

func isInteractiveSetupStdin() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// backupExistingConfig backs up ~/.lrc.toml if it exists and contains an api_key.
func backupExistingConfig(slog *setupLog) error {
	backupPath, err := setuptpl.BackupExistingConfig(slog.write)
	if err != nil {
		return err
	}
	if backupPath != "" {
		fmt.Printf("  %s📦 Existing config backed up to:%s %s%s%s\n", clr(cYellow), clr(cReset), clr(cDim), backupPath, clr(cReset))
		fmt.Println()
	}
	return nil
}

// isCodespace returns the public HTTPS callback URL for GitHub Codespaces
func isCodespace(port int) (string, bool) {
	name := os.Getenv("CODESPACE_NAME")
	domain := os.Getenv("GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN")
	if name == "" || domain == "" {
		return "", false
	}
	return fmt.Sprintf("https://%s-%d.%s/callback", name, port, domain), true
}

// runHexmosLoginFlow starts a temporary server, opens the browser for Hexmos Login,
// waits for the callback, and provisions the user in LiveReview.
func runHexmosLoginFlow(slog *setupLog, apiURL string) (*setupResult, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to start listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	if csURL, ok := isCodespace(port); ok {
		callbackURL = csURL
	}

	dataCh := make(chan *hexmosCallbackData, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()

	signinURL, err := setuptpl.BuildSigninURL(callbackURL)
	if err != nil {
		return nil, fmt.Errorf("failed to build signin url: %w", err)
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := setupLandingPageTemplate.Execute(w, struct{ SigninURL string }{SigninURL: signinURL}); err != nil {
			http.Error(w, "failed to render setup page", http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		dataParam := r.URL.Query().Get("data")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		cbData, err := setuptpl.ProcessLoginCallback(
			dataParam,
			func() error { return setupErrorPageTemplate.Execute(w, nil) },
			func() error { return setupSuccessPageTemplate.Execute(w, nil) },
			slog.write,
		)
		if err != nil {
			errCh <- err
			return
		}
		dataCh <- cbData
	})

	server := setuptpl.StartTemporaryServer(listener, mux, errCh)

	localURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	fmt.Printf("  🌐 Opening browser for Hexmos Login...\n")
	fmt.Printf("     %sIf it doesn't open, visit:%s %s\n", clr(cDim), clr(cReset), hyperlink(localURL, clr(cCyan)+localURL+clr(cReset)))
	fmt.Println()
	slog.write("local server on port %d, signin url: %s", port, signinURL)

	if err := openURL(localURL); err != nil {
		slog.write("warning: failed to auto-open local login URL: %v", err)
		fmt.Printf("  %s⚠ Could not open browser automatically.%s Continue by opening: %s\n", clr(cYellow), clr(cReset), hyperlink(localURL, clr(cCyan)+localURL+clr(cReset)))
		fmt.Println()
	}

	cbData, err := setuptpl.WaitForLoginCallback(dataCh, errCh, server, setupTimeout)
	if err != nil {
		return nil, err
	}

	slog.write("callback received, provisioning user")

	return provisionLiveReviewUser(cbData, apiURL, slog)
}

// provisionLiveReviewUser calls ensure-cloud-user and creates an API key.
func provisionLiveReviewUser(cbData *hexmosCallbackData, apiURL string, slog *setupLog) (*setupResult, error) {
	return setuptpl.ProvisionLiveReviewUser(cbData, apiURL, slog.write)
}

// promptGeminiKey reads the Gemini API key from stdin with up to 3 attempts.
func promptGeminiKey(result *setupResult, apiURL string, slog *setupLog) (string, error) {
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

		valid, msg, err := validateGeminiKey(result, key, apiURL)
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

func validateGeminiKey(result *setupResult, geminiKey string, apiURL string) (bool, string, error) {
	return setuptpl.ValidateGeminiKey(result, geminiKey, apiURL)
}

func createGeminiConnector(result *setupResult, geminiKey string, apiURL string) error {
	return setuptpl.CreateGeminiConnector(result, geminiKey, apiURL)
}

func writeConfig(result *setupResult, apiURL string) error {
	return setuptpl.WriteConfigWithOptions(result, setuptpl.WriteConfigOptions{APIURL: strings.TrimSpace(apiURL)})
}

func writeFileAtomically(path string, data []byte, mode os.FileMode) error {
	return setuptpl.WriteFileAtomically(path, data, mode)
}
