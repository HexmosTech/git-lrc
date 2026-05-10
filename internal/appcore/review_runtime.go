package appcore

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/HexmosTech/git-lrc/attestation"
	"github.com/HexmosTech/git-lrc/configpath"
	"github.com/HexmosTech/git-lrc/interactive/input"
	"github.com/HexmosTech/git-lrc/internal/appcore/decisionruntime"
	"github.com/HexmosTech/git-lrc/internal/decisionflow"
	"github.com/HexmosTech/git-lrc/internal/reviewapi"
	"github.com/HexmosTech/git-lrc/internal/reviewdb"
	"github.com/HexmosTech/git-lrc/internal/reviewhtml"
	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	"github.com/HexmosTech/git-lrc/internal/reviewopts"
	"github.com/HexmosTech/git-lrc/internal/selfupdate"
	"github.com/HexmosTech/git-lrc/internal/staticserve"
	"github.com/HexmosTech/git-lrc/network"
	"github.com/HexmosTech/git-lrc/storage"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

func newRuntimeHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) == 0 {
				return nil
			}
			if req.URL.Host != via[0].URL.Host {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

func liveReviewAuthFailureError(apiURL, technicalDetails string) error {
	technical := strings.TrimSpace(technicalDetails)
	if technical == "" {
		technical = "(empty response body)"
	}

	return fmt.Errorf("LiveReview authentication failed for review submission.\n\nNext steps:\n  1. Run: lrc ui\n  2. Login or re-authenticate\n  3. Retry: git lrc\n\nThis is LiveReview review-submission authentication, not your AI connector provider key.\n\nTechnical details:\nAPI URL: %s\n%s", apiURL, technical)
}

func formatLiveReviewTechnicalDetails(rawBody string) string {
	trimmed := strings.TrimSpace(rawBody)
	if trimmed == "" {
		return "(empty response body)"
	}

	var payload reviewmodel.APIErrorPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return trimmed
	}

	var lines []string
	if strings.TrimSpace(payload.ErrorCode) != "" {
		lines = append(lines, fmt.Sprintf("error_code: %s", payload.ErrorCode))
	}
	if strings.TrimSpace(payload.Error) != "" {
		lines = append(lines, fmt.Sprintf("error: %s", payload.Error))
	}
	if payload.Envelope != nil {
		for _, line := range formatEnvelopeUsageLines(payload.Envelope) {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return trimmed
	}

	lines = append(lines, fmt.Sprintf("raw_response: %s", trimmed))
	return strings.Join(lines, "\n")
}

func runReviewWithOptions(opts reviewopts.Options) error {
	verbose := opts.Verbose
	defer func() {
		if err := selfupdate.ApplyPendingUpdateIfAny(verbose); err != nil && verbose {
			log.Printf("pending self-update apply failed: %v", err)
		}
	}()

	var tempHTMLPath string
	var commitMsgPath string
	attestationAction := ""
	attestationWritten := false
	initialMsg := sanitizeInitialMessage(opts.InitialMsg)

	// Determine if this is a post-commit review (reviewing already-committed code, read-only)
	// vs a pre-commit review (reviewing staged changes before commit, can commit from UI)
	// When --commit flag is used, we're always reviewing historical commits (read-only mode)
	isPostCommitReview := opts.DiffSource == "commit"

	// Interactive flow (Web UI with commit actions) is the default when --serve is enabled
	// BUT: disable interactive actions when reviewing historical commits (isPostCommitReview)
	// Skip interactive mode if explicitly using --skip, not serving, or reviewing history
	useInteractive := !opts.Skip && opts.Serve && !isPostCommitReview

	// Short-circuit skip: collect diff for coverage tracking, write attestation, exit
	if opts.Skip {
		attestationAction = "skipped"
		var cov attestation.CoverageResult
		// Collect diff to record in DB for coverage tracking (best-effort)
		diffContent, diffErr := collectDiffWithOptions(opts)
		if diffErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not collect diff for coverage tracking: %v\n", diffErr)
		} else if len(diffContent) > 0 {
			parsedFiles, parseErr := parseDiffToFiles(diffContent)
			if parseErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not parse diff for coverage tracking: %v\n", parseErr)
			} else {
				var covErr error
				cov, covErr = reviewdb.RecordAndComputeCoverage("skipped", parsedFiles, "", verbose)
				if covErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: coverage computation failed: %v\n", covErr)
				}
			}
		}
		if cov.Iterations == 0 {
			cov.Iterations = 1
		}
		if err := ensureAttestationFull(attestationPayload{
			Action:           attestationAction,
			Iterations:       cov.Iterations,
			PriorAICovPct:    cov.PriorAICovPct,
			PriorReviewCount: cov.PriorReviewCount,
		}, verbose, &attestationWritten); err != nil {
			return err
		}
		if verbose {
			log.Printf("Review skipped by --skip; attestation recorded (iter:%d, coverage:%.0f%%)", cov.Iterations, cov.PriorAICovPct)
		} else {
			fmt.Printf("LiveReview: skipped (iter:%d, coverage:%.0f%%)\n", cov.Iterations, cov.PriorAICovPct)
		}
		return nil
	}

	// Short-circuit vouch: collect diff, compute coverage, write attestation, exit
	if opts.Vouch {
		attestationAction = "vouched"
		diffContent, diffErr := collectDiffWithOptions(opts)
		if diffErr != nil {
			return fmt.Errorf("failed to collect diff for vouch: %w", diffErr)
		}
		if len(diffContent) == 0 {
			return fmt.Errorf("no diff content to vouch for")
		}
		parsedFiles, parseErr := parseDiffToFiles(diffContent)
		if parseErr != nil {
			return fmt.Errorf("failed to parse diff for vouch: %w", parseErr)
		}
		cov, _ := reviewdb.RecordAndComputeCoverage("vouched", parsedFiles, "", verbose)
		if cov.Iterations == 0 {
			cov.Iterations = 1
		}
		if err := ensureAttestationFull(attestationPayload{
			Action:           attestationAction,
			Iterations:       cov.Iterations,
			PriorAICovPct:    cov.PriorAICovPct,
			PriorReviewCount: cov.PriorReviewCount,
		}, verbose, &attestationWritten); err != nil {
			return err
		}
		if verbose {
			log.Printf("Review vouched; attestation recorded (iter:%d, coverage:%.0f%%)", cov.Iterations, cov.PriorAICovPct)
		} else {
			fmt.Printf("LiveReview: vouched (iter:%d, coverage:%.0f%%)\n", cov.Iterations, cov.PriorAICovPct)
		}
		return nil
	}

	if opts.Precommit {
		gitDir, err := reviewapi.ResolveGitDir()
		if err != nil {
			return fmt.Errorf("precommit mode requires a git repository: %w", err)
		}
		commitMsgPath = filepath.Join(gitDir, commitMessageFile)
		_ = clearCommitMessageFile(commitMsgPath)
	}

	// Handle --force: delete existing attestation if present
	// Skip attestation logic for post-commit reviews
	if !isPostCommitReview {
		if opts.Force {
			if existing, err := existingAttestationAction(); err == nil && existing != "" {
				if err := deleteAttestationForCurrentTree(); err != nil {
					if verbose {
						log.Printf("Failed to remove existing attestation for current tree: %v", err)
					}
				} else if verbose {
					log.Printf("Removed existing attestation for current tree (action=%s); rerunning review", existing)
				}
			}
		} else {
			// Check if attestation exists and fail with guidance if --force not used
			if existing, err := existingAttestationAction(); err == nil && existing != "" {
				return cli.Exit(fmt.Sprintf("LiveReview: attestation already present for current tree (%s); use --force to rerun", existing), 1)
			}
		}
	}

	fakeMode := isFakeReviewBuild()
	var err error

	// Load configuration from config file or overrides.
	// Fake mode does not require API credentials.
	var config *Config
	if fakeMode {
		config = &Config{APIURL: reviewopts.DefaultAPIURL, APIKey: ""}
		if strings.TrimSpace(opts.APIURL) != "" {
			config.APIURL = opts.APIURL
		}
		if verbose {
			log.Printf("Fake review mode enabled (reviewMode=%s)", reviewMode)
		}
	} else {
		config, err = loadConfigValues(opts.APIKey, opts.APIURL, verbose)
		if err != nil {
			return err
		}
	}

	// Determine repo name
	repoName := opts.RepoName
	if repoName == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		repoName = filepath.Base(cwd)
	}

	if verbose {
		log.Printf("Repository name: %s", repoName)
		log.Printf("API URL: %s", config.APIURL)
	}

	var result *reviewmodel.DiffReviewResponse

	// Collect diff
	diffContent, err := collectDiffWithOptions(opts)
	if err != nil {
		return fmt.Errorf("failed to collect diff: %w", err)
	}

	if len(diffContent) == 0 {
		return fmt.Errorf("no diff content collected")
	}

	var fakeBaseFiles []reviewmodel.DiffReviewFileResult
	if fakeMode {
		fakeBaseFiles, err = parseDiffToFiles(diffContent)
		if err != nil {
			return fmt.Errorf("failed to parse diff for fake review mode: %w", err)
		}
	}

	if verbose {
		log.Printf("Collected %d bytes of diff content", len(diffContent))
	}

	// Create ZIP archive
	zipData, err := reviewapi.CreateZipArchive(diffContent)
	if err != nil {
		return fmt.Errorf("failed to create zip archive: %w", err)
	}

	if verbose {
		log.Printf("Created ZIP archive: %d bytes", len(zipData))
	}

	// Base64 encode
	base64Diff := base64.StdEncoding.EncodeToString(zipData)

	// Save bundle if requested
	if bundlePath := opts.SaveBundle; bundlePath != "" {
		if err := saveBundleForInspection(bundlePath, diffContent, zipData, base64Diff, verbose); err != nil {
			return fmt.Errorf("failed to save bundle: %w", err)
		}
	}

	// Submit review
	var submissionFailed bool
	var submissionBlockedReason string
	var submitResp reviewmodel.DiffReviewCreateResponse
	if fakeMode {
		submitResp = buildFakeSubmitResponse()
	} else {
		var updatedConfig Config
		submitResp, updatedConfig, err = submitReviewWithRecovery(*config, base64Diff, repoName, verbose)
		config = &updatedConfig
	}
	if err != nil {
		// Handle 413 Request Entity Too Large - prompt user to skip if interactive
		var apiErr *reviewmodel.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
			return liveReviewAuthFailureError(config.APIURL, formatLiveReviewTechnicalDetails(apiErr.Body))
		}
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusRequestEntityTooLarge {
			isInteractive := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
			if isInteractive {
				fmt.Printf("\n⚠️  Review submission failed: The diff is too large for the API (Status 413).\n")
				fmt.Print("Do you want to skip the review and proceed with the commit? [y/N]: ")

				reader := bufio.NewReader(os.Stdin)
				response, rErr := reader.ReadString('\n')
				if rErr != nil {
					// Fallback to error if we can't read input
					return fmt.Errorf("failed to read input during 413 handling: %w (original error: %v)", rErr, err)
				}
				response = strings.ToLower(strings.TrimSpace(response))

				if response == "y" || response == "yes" {
					fmt.Println("Proceeding with skipped review...")
					attestationAction = "skipped"
					if err := ensureAttestation(attestationAction, verbose, &attestationWritten); err != nil {
						return err
					}
					// Return nil to indicate success (review skipped, but process continues)
					return nil
				}
				// User declined to skip, return specific error without body
				return fmt.Errorf("review submission aborted by user (diff too large)")
			}
		}
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusForbidden || apiErr.StatusCode == http.StatusTooManyRequests) {
			submissionFailed = true
			submissionBlockedReason = "Usage quota exceeded"
			var upgradeURL string
			
			// Try to parse structured error payload for a better message
			var payload reviewmodel.APIErrorPayload
			if jErr := json.Unmarshal([]byte(apiErr.Body), &payload); jErr == nil && payload.Error != "" {
				submissionBlockedReason = payload.Error
				upgradeURL = payload.UpgradeURL
			}

			// If not serving the UI, fail fast with a clear message instead of proceeding to an empty-ID poll
			if !opts.Serve {
				fmt.Fprintf(os.Stderr, "\033[31m%s\033[0m\n", submissionBlockedReason)
				if upgradeURL != "" {
					fullUpgradeURL := upgradeURL
					if strings.HasPrefix(upgradeURL, "/") {
						base := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(config.APIURL, "/"), "/api"), "/api/v1")
						fullUpgradeURL = base + upgradeURL
					}
					fmt.Fprintf(os.Stderr, "\033[33mUpgrade your plan at: \033[0m%s\n", highlightURL(fullUpgradeURL))
				}
				return ErrAuthHandled // Silently exit via main.go's error handler
			}
			err = nil // Continue to UI to show the blocking message there
		}
		if err != nil {
			return fmt.Errorf("failed to submit review: %w", err)
		}
	}

	reviewID := submitResp.ReviewID
	reviewURL := buildReviewURL(config.APIURL, reviewID)

	// Track whether progressive loading mode is active
	progressiveLoadingActive := false

	// Shared decision channel for progressive loading (will be used after review completes)
	type progressiveDecision struct {
		code    int
		message string
		push    bool
	}
	var progressiveDecisionChan chan progressiveDecision
	var progressiveDecide func(code int, message string, push bool)
	var progressiveSubmit func(source decisionruntime.Source, code int, message string, push bool) bool
	var progressiveDecideOnce sync.Once
	progressiveRuntime := decisionruntime.New(decisionflow.PhaseReviewRunning)

	// In precommit mode, ensure unbuffered output
	if opts.Precommit {
		// Force flush and set unbuffered
		syncFileSafely(os.Stdout)
		syncFileSafely(os.Stderr)
	}

	// Track CLI usage (best-effort, non-blocking)
	if !fakeMode {
		go reviewapi.TrackCLIUsage(config.APIURL, config.APIKey, verbose)
	}
	selfupdate.StartAutoUpdateCheck(verbose)

	var fakeWait time.Duration
	if fakeMode {
		fakeWait, err = fakeReviewWaitDuration()
		if err != nil {
			return err
		}
	}

	// Generate and serve skeleton HTML immediately if --serve is enabled
	// Auto-enable serve when no HTML path specified and not in post-commit mode
	autoServeEnabled := !opts.Serve && opts.SaveHTML == "" && !isPostCommitReview
	if autoServeEnabled {
		opts.Serve = true
	}

	// Recalculate useInteractive now that opts.Serve may have been auto-enabled
	// This is critical for Case 1 (hook-based terminal invocation) where serve is auto-enabled
	// and we need the interactive flow with commit/push/skip options
	useInteractive = !opts.Skip && opts.Serve && !isPostCommitReview
	reviewMetadata := buildDecisionMetadata(reviewID, submitResp.UserEmail, submitResp.FriendlyName, reviewURL)
	var runningDraftHub *draftHub
	if useInteractive {
		runningDraftHub = newDraftHub(initialMsg)
	}

	if !useInteractive && !submissionFailed {
		fmt.Printf("Review submitted, ID: %s\n", reviewID)
		if submitResp.UserEmail != "" {
			fmt.Printf("Account: %s\n", submitResp.UserEmail)
		}
		if submitResp.FriendlyName != "" {
			fmt.Printf("Title: %s\n", submitResp.FriendlyName)
		}
		if reviewURL != "" {
			fmt.Printf("Review link: %s\n", highlightURL(reviewURL))
		}
		if submitResp.Envelope != nil {
			printEnvelopeUsageSummary("submission", submitResp.Envelope)
		}
	}

	if opts.Serve {
		// Parse the diff content to generate file structures for immediate display
		filesFromDiff, parseErr := parseDiffToFiles(diffContent)
		if parseErr != nil && verbose {
			log.Printf("Warning: failed to parse diff for skeleton HTML: %v", parseErr)
		}

		// Initialize global review state for API-based UI
		reviewStateMu.Lock()
		currentReviewState = NewReviewState(reviewID, filesFromDiff, useInteractive, isPostCommitReview, initialMsg, config.APIURL)
		if submissionFailed {
			currentReviewState.Status = "failed"
			currentReviewState.ErrorSummary = submissionBlockedReason
			currentReviewState.SetBlocked(true)
		}
		reviewStateMu.Unlock()

		// Start serving immediately in background
		serveListener, selectedPort, err := pickServePort(opts.Port, 10)
		if err != nil {
			return fmt.Errorf("failed to find available port: %w", err)
		}
		if selectedPort != opts.Port {
			fmt.Printf("Port %d is busy; serving on %d instead.\n", opts.Port, selectedPort)
			opts.Port = selectedPort
		}

		serveURL := fmt.Sprintf("http://localhost:%d", opts.Port)
		if !useInteractive {
			fmt.Printf("\n🌐 Review available at: %s\n", highlightURL(serveURL))
			fmt.Printf("   Comments will appear progressively as review runs\n\n")
		}

		// Auto-open the review in the default browser
		openURL(serveURL)

		// Mark that progressive loading is active
		progressiveLoadingActive = true

		// Initialize decision channel for progressive loading
		progressiveDecisionChan = make(chan progressiveDecision, 1)
		progressiveDecide = func(code int, message string, push bool) {
			progressiveDecideOnce.Do(func() {
				progressiveDecisionChan <- progressiveDecision{code: code, message: message, push: push}
			})
		}
		progressiveSubmit = func(source decisionruntime.Source, code int, message string, push bool) bool {
			outcome := progressiveRuntime.TryDecide(decisionruntime.Decision{
				Source:  source,
				Code:    code,
				Message: message,
				Push:    push,
			})
			if outcome.Err != nil || !outcome.Accepted {
				return false
			}
			chosen, ok := progressiveRuntime.Chosen()
			if !ok {
				return false
			}
			if runningDraftHub != nil {
				runningDraftHub.Freeze()
			}
			progressiveDecide(chosen.Code, chosen.Message, chosen.Push)
			return true
		}
		handleProgressiveDecision := func(w http.ResponseWriter, code int, message string, push bool) {
			outcome := progressiveRuntime.TryDecide(decisionruntime.Decision{Source: decisionruntime.SourceWeb, Code: code, Message: message, Push: push})
			if outcome.Err != nil {
				if errors.Is(outcome.Err, decisionruntime.ErrAlreadyResolved) {
					http.Error(w, outcome.Err.Error(), http.StatusConflict)
					return
				}

				reqErr, ok := outcome.Err.(*decisionflow.RequestError)
				if !ok {
					http.Error(w, outcome.Err.Error(), http.StatusBadRequest)
					return
				}
				http.Error(w, reqErr.Error(), reqErr.StatusCode())
				return
			}

			if !outcome.Accepted {
				http.Error(w, "decision rejected", http.StatusConflict)
				return
			}

			chosen, ok := progressiveRuntime.Chosen()
			if !ok {
				http.Error(w, "decision resolution failed", http.StatusConflict)
				return
			}

			if runningDraftHub != nil {
				runningDraftHub.Freeze()
			}
			progressiveDecide(chosen.Code, chosen.Message, chosen.Push)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}

		// Start server in background
		go func() {
			mux := http.NewServeMux()
			// Serve static assets (JS, CSS) from embedded filesystem
			mux.Handle("/static/", http.StripPrefix("/static/", staticserve.GetStaticHandler()))

			// Serve index.html from embedded filesystem (no file on disk needed)
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				htmlBytes, err := staticserve.ReadFile("index.html")
				if err != nil {
					http.Error(w, "Failed to load page", http.StatusInternalServerError)
					return
				}
				if _, err := io.Copy(w, bytes.NewReader(htmlBytes)); err != nil && verbose {
					log.Printf("failed to write index response: %v", err)
				}
			})

			// API endpoint for review state - frontend polls this
			mux.HandleFunc("/api/review", func(w http.ResponseWriter, r *http.Request) {
				reviewStateMu.RLock()
				state := currentReviewState
				reviewStateMu.RUnlock()

				if state == nil {
					http.Error(w, "No review in progress", http.StatusNotFound)
					return
				}
				state.ServeHTTP(w, r)
			})

			mux.HandleFunc("/api/runtime/usage-chip", func(w http.ResponseWriter, r *http.Request) {
				handleRuntimeUsageChip(w, r, config, verbose)
			})

			if runningDraftHub != nil {
				mux.HandleFunc("/api/draft", func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodGet {
						handleDraftGet(w, r, runningDraftHub)
						return
					}
					if r.Method == http.MethodPost {
						handleDraftUpdate(w, r, runningDraftHub, draftSourceWeb)
						return
					}
					w.WriteHeader(http.StatusMethodNotAllowed)
				})
				mux.HandleFunc("/api/draft/events", func(w http.ResponseWriter, r *http.Request) {
					handleDraftEvents(w, r, runningDraftHub)
				})
			}

			// Functional commit handlers that work with the decision channel
			mux.HandleFunc("/commit", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				msg := readCommitMessageFromRequest(r)
				if runningDraftHub != nil {
					if snap, err := runningDraftHub.Update(msg, draftSourceWeb, 0); err == nil {
						msg = snap.Text
					}
				}
				handleProgressiveDecision(w, decisionflow.DecisionCommit, msg, false)
			})
			mux.HandleFunc("/commit-push", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				msg := readCommitMessageFromRequest(r)
				if runningDraftHub != nil {
					if snap, err := runningDraftHub.Update(msg, draftSourceWeb, 0); err == nil {
						msg = snap.Text
					}
				}
				handleProgressiveDecision(w, decisionflow.DecisionCommit, msg, true)
			})
			mux.HandleFunc("/skip", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				msg := readCommitMessageFromRequest(r)
				if runningDraftHub != nil {
					if snap, err := runningDraftHub.Update(msg, draftSourceWeb, 0); err == nil {
						msg = snap.Text
					}
				}
				handleProgressiveDecision(w, decisionflow.DecisionSkip, msg, false)
			})
			mux.HandleFunc("/vouch", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				msg := readCommitMessageFromRequest(r)
				if runningDraftHub != nil {
					if snap, err := runningDraftHub.Update(msg, draftSourceWeb, 0); err == nil {
						msg = snap.Text
					}
				}
				handleProgressiveDecision(w, decisionflow.DecisionVouch, msg, false)
			})
			mux.HandleFunc("/abort", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				if runningDraftHub != nil {
					runningDraftHub.Freeze()
				}
				handleProgressiveDecision(w, decisionflow.DecisionAbort, "", false)
			})
			mux.HandleFunc("/handoff", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "Failed to read request body", http.StatusBadRequest)
					return
				}
				handleProgressiveDecision(w, decisionflow.DecisionHandoff, string(body), false)
			})
			// Proxy endpoint for review-events API to avoid CORS
			mux.HandleFunc("/api/v1/diff-review/", func(w http.ResponseWriter, r *http.Request) {
				if fakeMode {
					if r.Method != http.MethodGet {
						w.WriteHeader(http.StatusMethodNotAllowed)
						return
					}
					if !strings.HasSuffix(r.URL.Path, "/events") {
						http.NotFound(w, r)
						return
					}

					reviewStateMu.RLock()
					state := currentReviewState
					reviewStateMu.RUnlock()
					if state == nil {
						http.Error(w, "No review in progress", http.StatusNotFound)
						return
					}

					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(buildFakeEventsResponse(state.Snapshot())); err != nil {
						if verbose {
							log.Printf("failed to write fake events response: %v", err)
						}
					}
					return
				}

				handleReviewEventsProxy(w, r, *config, reviewID, verbose)
			})

			server := &http.Server{Handler: mux}
			if err := server.Serve(serveListener); err != nil && err != http.ErrServerClosed {
				if verbose {
					log.Printf("progressive server failed: %v", err)
				}
			}
		}()
		time.Sleep(100 * time.Millisecond) // Give server time to start
	}

	// For post-commit reviews, just poll and get results without interactive flow
	if isPostCommitReview {
		setStatusUI, stopStatusUI, statusUIDone, statusUIAbort := startTerminalStatusBubbleTea(
			"Historical review in progress",
			"Read-only mode. No commit actions are available.",
			reviewMetadata,
		)
		setStatusUI("Status: waiting for review")

		stopPoll := make(chan struct{})
		var stopPollOnce sync.Once
		stopPollFn := func() { stopPollOnce.Do(func() { close(stopPoll) }) }

		var pollErr error
		pollDone := make(chan struct{})
		go func() {
			if submissionFailed || reviewID == "" {
				// Don't poll if submission failed; it leads to unclear 404 errors
				close(pollDone)
				return
			}

			if fakeMode {
				result, pollErr = pollReviewFake(reviewID, opts.PollInterval, fakeWait, verbose, stopPoll, fakeBaseFiles, setStatusUI)
			} else {
				var updatedConfig Config
				result, updatedConfig, pollErr = pollReviewWithRecovery(*config, reviewID, opts.PollInterval, opts.Timeout, verbose, stopPoll, setStatusUI)
				config = &updatedConfig
			}
			close(pollDone)
		}()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(sigChan)

		pollFinished := false
		select {
		case <-pollDone:
			pollFinished = true
		case <-statusUIAbort:
			stopPollFn()
			<-pollDone
		case <-sigChan:
			stopPollFn()
			<-pollDone
		}

		stopStatusUI()
		<-statusUIDone

		if !pollFinished {
			if errors.Is(pollErr, reviewapi.ErrPollCancelled) {
				return nil
			}
		}
		if pollErr != nil {
			var apiErr *reviewmodel.APIError
			if errors.As(pollErr, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
				return liveReviewAuthFailureError(config.APIURL, formatLiveReviewTechnicalDetails(apiErr.Body))
			}
			// If progressive loading is active, don't crash - keep server running to show error
			if progressiveLoadingActive {
				fmt.Printf("\n⚠️  Review failed: %v\n", pollErr)
				fmt.Printf("   Error details available in browser at: http://localhost:%d\n", opts.Port)
				fmt.Printf("   Press Ctrl-C to exit\n\n")
				// Create result with error so HTML can display it
				result = &reviewmodel.DiffReviewResponse{
					Status:  "failed",
					Summary: fmt.Sprintf("Review failed: %v", pollErr),
					Message: pollErr.Error(),
				}
				// Update review state with error
				reviewStateMu.Lock()
				if currentReviewState != nil {
					currentReviewState.SetFailed(pollErr.Error())
				}
				reviewStateMu.Unlock()
			} else {
				if reviewURL != "" {
					return fmt.Errorf("failed to poll review (see %s): %w", reviewURL, pollErr)
				}
				return fmt.Errorf("failed to poll review: %w", pollErr)
			}
		} else {
			// Update review state with final result
			reviewStateMu.Lock()
			if currentReviewState != nil {
				currentReviewState.UpdateFromResult(result)
			}
			reviewStateMu.Unlock()
		}

		if opts.Serve && progressiveLoadingActive {
			setStatusUI, stopStatusUI, statusUIDone, statusUIAbort = startTerminalStatusBubbleTea(
				"Historical review ready",
				"Viewing in browser. This mode remains read-only.",
				reviewMetadata,
			)
			if pollErr != nil {
				setStatusUI("status: review failed - check browser details")
			} else {
				setStatusUI("status: completed - press Ctrl-C to exit")
			}
			select {
			case <-statusUIAbort:
			case <-sigChan:
			}
			stopStatusUI()
			<-statusUIDone
			return nil
		}
		// No attestation for post-commit reviews
	}

	// Interactive path (default): set up decision channels for Ctrl-C / Ctrl-S and poll
	decisionCode := -1
	decisionMessage := ""
	decisionPush := false
	if useInteractive {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(sigChan)

		interactiveRuntime := decisionruntime.New(decisionflow.PhaseReviewRunning)
		decisionChan := make(chan terminalDecision, 1)
		var decisionOnce sync.Once
		submitInteractiveDecision := func(source decisionruntime.Source, d terminalDecision) bool {
			outcome := interactiveRuntime.TryDecide(decisionruntime.Decision{Source: source, Code: d.Code, Message: d.Message, Push: d.Push})
			if outcome.Err != nil || !outcome.Accepted {
				return false
			}
			chosen, ok := interactiveRuntime.Chosen()
			if !ok {
				return false
			}
			if runningDraftHub != nil {
				runningDraftHub.Freeze()
			}
			decisionOnce.Do(func() {
				decisionChan <- terminalDecision{Code: chosen.Code, Message: chosen.Message, Push: chosen.Push}
			})
			return true
		}

		prompt := decisionPrompt{
			Title:                  "Review in progress",
			Description:            "Choose terminal action now or continue waiting.",
			Metadata:               reviewMetadata,
			InitialText:            initialMsg,
			AllowAbort:             true,
			AllowSkip:              true,
			AllowVouch:             true,
			RequireMessageForSkip:  true,
			RequireMessageForVouch: true,
		}
		publishTerminalDraft := func(text string) int64 {
			if runningDraftHub == nil {
				return 0
			}
			snap, err := runningDraftHub.Update(text, draftSourceTerminal, 0)
			if err != nil {
				return 0
			}
			return snap.Version
		}
		openEditor := func() (string, int64, error) {
			if runningDraftHub == nil {
				return "", 0, fmt.Errorf("draft sync unavailable")
			}
			current := runningDraftHub.Snapshot()
			edited, err := openDraftInEditor(current.Text)
			if err != nil {
				return "", 0, err
			}
			snap, err := runningDraftHub.Update(edited, draftSourceEditor, current.Version)
			if err != nil {
				return "", 0, err
			}
			return snap.Text, snap.Version, nil
		}

		tuiDecisionCh, setTUIStatus, setTUIDraft, stopTUI, tuiDone := startTerminalDecisionBubbleTea(prompt, publishTerminalDraft, openEditor)
		if runningDraftHub != nil {
			updates, unsubscribe := runningDraftHub.Subscribe()
			defer unsubscribe()
			go func() {
				for snap := range updates {
					setTUIDraft(snap.Text, snap.Version)
				}
			}()
		}
		setTUIStatus("status: waiting for review")
		go func() {
			for d := range tuiDecisionCh {
				submitInteractiveDecision(decisionruntime.SourceTerminal, d)
			}
		}()

		// Ctrl-C -> abort commit
		go func() {
			<-sigChan
			submitInteractiveDecision(decisionruntime.SourceSignal, terminalDecision{Code: decisionflow.DecisionAbort})
		}()

		// Poll concurrently and race with decisions
		var pollResult *reviewmodel.DiffReviewResponse
		var pollErr error
		var pollUpdatedConfig Config
		pollUsedRecovery := false
		pollDone := make(chan struct{})
		var webDecisionChan <-chan progressiveDecision
		if progressiveLoadingActive && progressiveDecisionChan != nil {
			webDecisionChan = progressiveDecisionChan
		}
		stopPoll := make(chan struct{})
		var stopPollOnce sync.Once
		stopPollFn := func() { stopPollOnce.Do(func() { close(stopPoll) }) }
		go func() {
			defer close(pollDone)
			if submissionFailed || reviewID == "" {
				return
			}
			if fakeMode {
				pollResult, pollErr = pollReviewFake(reviewID, opts.PollInterval, fakeWait, verbose, stopPoll, fakeBaseFiles, setTUIStatus)
			} else {
				pollUsedRecovery = true
				pollResult, pollUpdatedConfig, pollErr = pollReviewWithRecovery(*config, reviewID, opts.PollInterval, opts.Timeout, verbose, stopPoll, setTUIStatus)
			}
		}()

		var pollFinished bool
		select {
		case d := <-decisionChan:
			decisionCode = d.Code
			decisionMessage = d.Message
			decisionPush = d.Push
			stopTUI()
			<-tuiDone
			stopPollFn()
		case webDecision := <-webDecisionChan:
			decisionCode = webDecision.code
			decisionMessage = webDecision.message
			decisionPush = webDecision.push
			stopTUI()
			<-tuiDone
			stopPollFn()
		case <-pollDone:
			pollFinished = true
		}

		if pollFinished {
			stopTUI()
			<-tuiDone
			if pollUsedRecovery {
				config = &pollUpdatedConfig
			}
			interactiveRuntime.SetPhase(decisionflow.PhaseReviewComplete)
			if progressiveLoadingActive {
				progressiveRuntime.SetPhase(decisionflow.PhaseReviewComplete)
			}
			// Prefer a user decision if it arrives within a short grace window after poll finishes
			select {
			case d := <-decisionChan:
				decisionCode = d.Code
				decisionMessage = d.Message
				decisionPush = d.Push
				// got user decision
			case webDecision := <-webDecisionChan:
				decisionCode = webDecision.code
				decisionMessage = webDecision.message
				decisionPush = webDecision.push
			case <-time.After(300 * time.Millisecond):
				// no decision quickly; proceed with poll result
			}
			if pollErr != nil {
				if errors.Is(pollErr, reviewapi.ErrPollCancelled) {
					if decisionCode != -1 {
						message := decisionMessage
						if strings.TrimSpace(message) == "" {
							message = initialMsg
						}
						return executeDecision(decisionCode, message, decisionPush, decisionExecutionContext{
							precommit:          opts.Precommit,
							verbose:            verbose,
							initialMsg:         initialMsg,
							commitMsgPath:      commitMsgPath,
							diffContent:        diffContent,
							reviewID:           reviewID,
							attestationWritten: &attestationWritten,
						})
					}
					return nil
				}
				// If progressive loading is active, don't crash - let server keep running to show error
				if progressiveLoadingActive {
					var apiErr *reviewmodel.APIError
					if errors.As(pollErr, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
						syncedPrintf("\n⚠️  LiveReview authentication failed for review updates.\n")
						syncedPrintf("   Run: lrc ui\n")
						syncedPrintf("   Login or re-authenticate, then retry: git lrc\n")
						syncedPrintf("   This is LiveReview review-submission authentication, not your AI connector provider key.\n")
						syncedPrintf("\nTechnical details:\n")
						syncedPrintf("%s\n\n", formatLiveReviewTechnicalDetails(apiErr.Body))
					} else {
						syncedPrintf("\n⚠️  Review failed: %v\n", pollErr)
						syncedPrintf("   Error details available in browser at: http://localhost:%d\n\n", opts.Port)
					}
					// Create empty result - error will be delivered via completion event, not in Summary
					result = &reviewmodel.DiffReviewResponse{
						Status:  "failed",
						Summary: "",
						Message: pollErr.Error(),
					}
					// Update review state with error
					reviewStateMu.Lock()
					if currentReviewState != nil {
						currentReviewState.SetFailed(pollErr.Error())
					}
					reviewStateMu.Unlock()
				} else {
					var apiErr *reviewmodel.APIError
					if errors.As(pollErr, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
						return liveReviewAuthFailureError(config.APIURL, formatLiveReviewTechnicalDetails(apiErr.Body))
					}
					if reviewURL != "" {
						return fmt.Errorf("failed to poll review (see %s): %w", reviewURL, pollErr)
					}
					return fmt.Errorf("failed to poll review: %w", pollErr)
				}
			} else {
				result = pollResult
				// Update review state with final result
				reviewStateMu.Lock()
				if currentReviewState != nil && pollResult != nil {
					currentReviewState.UpdateFromResult(pollResult)
				}
				reviewStateMu.Unlock()
			}
			attestationAction = "reviewed"
			if err := recordCoverageAndAttest("reviewed", diffContent, reviewID, verbose, &attestationWritten); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
		}

		// If a decision happened before we proceed, act now
		if decisionCode != -1 {
			message := decisionMessage
			if strings.TrimSpace(message) == "" {
				message = initialMsg
			}
			return executeDecision(decisionCode, message, decisionPush, decisionExecutionContext{
				precommit:          opts.Precommit,
				verbose:            verbose,
				initialMsg:         initialMsg,
				commitMsgPath:      commitMsgPath,
				diffContent:        diffContent,
				reviewID:           reviewID,
				attestationWritten: &attestationWritten,
			})
		}
	}

	// Apply default HTML serve for interactive/non-post-commit reviews
	if !isPostCommitReview {
		autoHTMLPath, err := reviewopts.ApplyDefaultHTMLServe(&opts)
		if err != nil {
			return err
		}
		tempHTMLPath = autoHTMLPath
	}

	// Clean up temp HTML file on exit
	if tempHTMLPath != "" {
		defer func() {
			if err := storage.RemoveTempHTMLFile(tempHTMLPath); err == nil {
				if verbose {
					log.Printf("Removed temporary HTML file: %s", tempHTMLPath)
				}
			} else if verbose {
				log.Printf("Could not remove temporary HTML file %s: %v", tempHTMLPath, err)
			}
		}()
	}

	// Save JSON response if requested
	if jsonPath := opts.SaveJSON; jsonPath != "" {
		if err := saveJSONResponse(jsonPath, result, verbose); err != nil {
			return fmt.Errorf("failed to save JSON response: %w", err)
		}
	}

	// Save formatted text output if requested
	if textPath := opts.SaveText; textPath != "" {
		if err := saveTextOutput(textPath, result, verbose); err != nil {
			return fmt.Errorf("failed to save text output: %w", err)
		}
	}

	// Save HTML output if requested
	// Skip if progressive loading is active - the browser already has the skeleton HTML
	// and will receive error/completion via the events API
	if htmlPath := opts.SaveHTML; htmlPath != "" && !progressiveLoadingActive {
		if err := saveHTMLOutput(htmlPath, result, verbose, useInteractive, isPostCommitReview, initialMsg, reviewID, config.APIURL, config.APIKey); err != nil {
			return fmt.Errorf("failed to save HTML output: %w", err)
		}

		// Ensure we're on a fresh line after status updates
		fmt.Printf("\n")

		if tempHTMLPath != "" {
			fmt.Printf("HTML review saved to (auto-selected): %s\n", htmlPath)
		} else {
			fmt.Printf("HTML review saved to: %s\n", htmlPath)
		}
	}

	// Handle serve mode
	if opts.Serve {
		htmlPath := opts.SaveHTML

		// Only pick a new port if progressive loading is NOT active (server not already running)
		var nonProgressiveListener net.Listener
		if !progressiveLoadingActive {
			var selectedPort int
			var err error
			nonProgressiveListener, selectedPort, err = pickServePort(opts.Port, 10)
			if err != nil {
				return fmt.Errorf("failed to find available port: %w", err)
			}
			if selectedPort != opts.Port {
				fmt.Printf("Port %d is busy; serving on %d instead.\n", opts.Port, selectedPort)
				opts.Port = selectedPort
			}
		}

		// Interactive prompt for commit decision (default for all non-skip runs)
		if useInteractive {
			if err := ensureAttestation(attestationAction, verbose, &attestationWritten); err != nil {
				return err
			}

			// If progressive loading was active, the server is already running.
			// Don't start a new server - wait for decisions from HTTP or terminal.
			if progressiveLoadingActive {
				// Progressive loading active - server already running on opts.Port
				prompt := decisionPrompt{
					Title:                   "Review complete. Choose action",
					Description:             "Use terminal keys or the web UI buttons.",
					Metadata:                reviewMetadata,
					InitialText:             initialMsg,
					FocusMessage:            true,
					AllowCommit:             true,
					AllowPush:               true,
					AllowAbort:              true,
					RequireMessageForCommit: true,
				}
				publishTerminalDraft := func(text string) int64 {
					if runningDraftHub == nil {
						return 0
					}
					snap, err := runningDraftHub.Update(text, draftSourceTerminal, 0)
					if err != nil {
						return 0
					}
					return snap.Version
				}
				openEditor := func() (string, int64, error) {
					if runningDraftHub == nil {
						return "", 0, fmt.Errorf("draft sync unavailable")
					}
					current := runningDraftHub.Snapshot()
					edited, err := openDraftInEditor(current.Text)
					if err != nil {
						return "", 0, err
					}
					snap, err := runningDraftHub.Update(edited, draftSourceEditor, current.Version)
					if err != nil {
						return "", 0, err
					}
					return snap.Text, snap.Version, nil
				}
				tuiDecisionCh, _, setTUIDraft, stopTUI, tuiDone := startTerminalDecisionBubbleTea(prompt, publishTerminalDraft, openEditor)
				if runningDraftHub != nil {
					updates, unsubscribe := runningDraftHub.Subscribe()
					defer unsubscribe()
					go func() {
						for snap := range updates {
							setTUIDraft(snap.Text, snap.Version)
						}
					}()
				}

				// Signals still map to abort decisions.
				sigChan := make(chan os.Signal, 1)
				signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
				defer signal.Stop(sigChan)

				go func() {
					<-sigChan
					if progressiveSubmit != nil {
						progressiveSubmit(decisionruntime.SourceSignal, decisionflow.DecisionAbort, "", false)
					}
				}()

				go func() {
					for d := range tuiDecisionCh {
						if progressiveSubmit != nil {
							progressiveSubmit(decisionruntime.SourceTerminal, d.Code, d.Message, d.Push)
						}
					}
				}()

				// Wait for decision from either HTTP endpoint or terminal
				decision := <-progressiveDecisionChan
				stopTUI()
				<-tuiDone
				return executeDecision(decision.code, decision.message, decision.push, decisionExecutionContext{
					precommit:          opts.Precommit,
					verbose:            verbose,
					initialMsg:         initialMsg,
					commitMsgPath:      commitMsgPath,
					diffContent:        diffContent,
					reviewID:           reviewID,
					attestationWritten: &attestationWritten,
				})
			} else {
				// No progressive loading - use normal serveHTMLInteractive
				code, msg, push, err := serveHTMLInteractive(htmlPath, opts.Port, nonProgressiveListener, initialMsg, reviewMetadata, false)
				if err != nil {
					return err
				}
				code = normalizeDecisionCode(code)

				if opts.Precommit {
					exitCode := precommitExitCodeForDecision(code)
					// Hook path: persist commit message/push request for downstream hooks and exit with hook code
					if commitMsgPath != "" {
						if exitCode == decisionflow.DecisionCommit {
							msgToPersist := msg
							if strings.TrimSpace(msgToPersist) == "" {
								msgToPersist = initialMsg
							}

							if strings.TrimSpace(msgToPersist) != "" {
								if err := persistCommitMessage(commitMsgPath, msgToPersist); err != nil {
									fmt.Fprintf(os.Stderr, "Warning: failed to store commit message: %v\n", err)
								}
							} else {
								_ = clearCommitMessageFile(commitMsgPath)
							}
						} else {
							_ = clearCommitMessageFile(commitMsgPath)
						}
					}

					if exitCode == decisionflow.DecisionCommit && push {
						if err := persistPushRequest(commitMsgPath); err != nil {
							fmt.Fprintf(os.Stderr, "Warning: failed to store push request: %v\n", err)
						}
					} else {
						_ = clearPushRequest(commitMsgPath)
					}

					return cli.Exit("", exitCode)
				}

				// Non-hook interactive: execute commit (and optional push) directly
				return executeDecision(code, msg, push, decisionExecutionContext{
					precommit:          false,
					verbose:            verbose,
					initialMsg:         initialMsg,
					commitMsgPath:      commitMsgPath,
					diffContent:        diffContent,
					reviewID:           reviewID,
					attestationWritten: &attestationWritten,
				})
			}
		}

		// Non-interactive serve: just host HTML (skip if progressive loading was active - server already running)
		if !progressiveLoadingActive {
			serveURL := fmt.Sprintf("http://localhost:%d", opts.Port)
			fmt.Printf("Serving HTML review at: %s\n", highlightURL(serveURL))
			if err := serveHTML(htmlPath, opts.Port, nonProgressiveListener); err != nil {
				return fmt.Errorf("failed to serve HTML: %w", err)
			}
		} else {
			// Progressive loading is active - server is already running in background goroutine
			// We need to block and wait for Ctrl-C so the server keeps running
			if !isPostCommitReview {
				fmt.Printf("\n📋 Review in progress.\n")
				fmt.Printf("   Press Ctrl-C to exit.\n\n")
				sigChan := make(chan os.Signal, 1)
				signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
				defer signal.Stop(sigChan)
				<-sigChan
				fmt.Println("\nExiting...")
				return nil
			}
		}
	}

	// Render result to stdout (skip in interactive mode or when serving - handled by UI)
	if !useInteractive && !opts.Serve {
		if err := renderResult(result, opts.Output); err != nil {
			return fmt.Errorf("failed to render result: %w", err)
		}
	}

	// Only write attestation for pre-commit reviews, not post-commit reviews
	if !isPostCommitReview {
		if err := ensureAttestation(attestationAction, verbose, &attestationWritten); err != nil {
			return err
		}
	}

	return nil
}

var standardTokenExclusions = []string{
	`:(exclude)package-lock.json`,
	`:(exclude)yarn.lock`,
	`:(exclude)pnpm-lock.yaml`,
	`:(exclude)go.sum`,
	`:(exclude)Cargo.lock`,
	`:(exclude)poetry.lock`,
	`:(exclude)Gemfile.lock`,
}

// collectDiffWithOptions securely intercepts diff collection by filtering out lockfiles globally and running the local Offline Security scanner synchronously before any payload bubbles backwards gracefully.
func collectDiffWithOptions(opts reviewopts.Options) ([]byte, error) {
	diffContent, err := collectDiffWithOptionsRaw(opts)
	if err != nil {
		return nil, err
	}

	// [Offline PII/Secret Pre-Flight Scanner]
	// Run offline secret scanning right as the subsystem collects bytes, protecting BOTH standard --review AND --vouch.
	if err := ScanDiffForSecrets(diffContent); err != nil {
		fmt.Fprintf(os.Stderr, "\n[FATAL] %v\n", err)
		return nil, cli.Exit(err.Error(), 1)
	}

	return diffContent, nil
}

func collectDiffWithOptionsRaw(opts reviewopts.Options) ([]byte, error) {
	diffSource := opts.DiffSource
	verbose := opts.Verbose

	switch diffSource {
	case "staged":
		if verbose {
			log.Println("Collecting staged changes (excluding standard lockfiles)...")
		}
		args := append([]string{"diff", "--staged", "--", "."}, standardTokenExclusions...)
		return reviewapi.RunGitCommand(args...)

	case "working":
		if verbose {
			log.Println("Collecting working tree changes (excluding standard lockfiles)...")
		}
		args := append([]string{"diff", "--", "."}, standardTokenExclusions...)
		return reviewapi.RunGitCommand(args...)

	case "commit":
		commitVal := opts.CommitVal
		if commitVal == "" {
			return nil, fmt.Errorf("--commit is required when diff-source=commit")
		}
		if verbose {
			log.Printf("Collecting diff for commit: %s (excluding standard lockfiles)", commitVal)
		}
		if strings.Contains(commitVal, "..") {
			// It's a commit range, use git diff
			args := append([]string{"diff", commitVal, "--"}, standardTokenExclusions...)
			return reviewapi.RunGitCommand(args...)
		}
		// Single commit, use git show to get the commit's changes
		args := append([]string{"show", "--format=", commitVal, "--"}, standardTokenExclusions...)
		return reviewapi.RunGitCommand(args...)

	case "range":
		rangeVal := opts.RangeVal
		if rangeVal == "" {
			return nil, fmt.Errorf("--range is required when diff-source=range")
		}
		if verbose {
			log.Printf("Collecting diff for range: %s (excluding standard lockfiles)", rangeVal)
		}
		args := append([]string{"diff", rangeVal, "--"}, standardTokenExclusions...)
		return reviewapi.RunGitCommand(args...)

	case "file":
		filePath := opts.DiffFile
		if filePath == "" {
			return nil, fmt.Errorf("--diff-file is required when diff-source=file")
		}
		if verbose {
			log.Printf("Reading diff from file (no automatic exclusions applied): %s", filePath)
		}
		return storage.ReadDiffFile(filePath)

	default:
		return nil, fmt.Errorf("invalid diff-source: %s (must be staged, working, commit, range, or file)", diffSource)
	}
}

// runCommitAndMaybePush commits the staged changes and optionally pushes with safety checks.
func runCommitAndMaybePush(message string, push bool, verbose bool) error {
	msg := strings.TrimSpace(message)
	commitArgs := []string{"commit"}
	if msg != "" {
		commitArgs = append(commitArgs, "-m", msg)
	}

	// Ensure git starts printing on a fresh terminal line.
	fmt.Println()
	syncFileSafely(os.Stdout)

	commitCmd := exec.Command("git", commitArgs...)
	if msg == "" {
		// Let git launch the standard editor interactively when no -m is provided.
		commitCmd.Stdin = os.Stdin
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			if tty, err := openTTY(); err == nil {
				defer tty.Close()
				commitCmd.Stdin = tty
			}
		}
	}
	commitCmd.Stdout = os.Stdout
	commitCmd.Stderr = os.Stderr
	// Set env var to prevent hook recursion in prepare-commit-msg.
	commitCmd.Env = append(os.Environ(), "LRC_SKIP_REVIEW=1")
	if verbose {
		if msg == "" {
			log.Printf("Running git commit (editor/default message, LRC_SKIP_REVIEW=1)")
		} else {
			log.Printf("Running git commit (LRC_SKIP_REVIEW=1)")
		}
	}
	if err := commitCmd.Run(); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	if !push {
		return nil
	}

	// Guarded push: check we're not detached, have upstream, sync with ff-only, then push
	if err := exec.Command("git", "symbolic-ref", "-q", "HEAD").Run(); err != nil {
		fmt.Println("Skipping push – detached HEAD")
		return nil
	}

	upBytes, err := exec.Command("git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}").Output()
	if err != nil {
		branchBytes, branchErr := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
		if branchErr != nil {
			return fmt.Errorf("failed to resolve current branch for upstream bootstrap: %w", branchErr)
		}
		branchName := strings.TrimSpace(string(branchBytes))
		if branchName == "" || branchName == "HEAD" {
			return fmt.Errorf("failed to resolve a valid branch name for upstream bootstrap")
		}

		fmt.Printf("No upstream configured for %s. Creating upstream on origin...\n", branchName)
		bootstrapPushCmd := exec.Command("git", "push", "--no-progress", "-u", "origin", "HEAD")
		bootstrapPushCmd.Stdout = os.Stdout
		bootstrapPushCmd.Stderr = os.Stderr
		if err := bootstrapPushCmd.Run(); err != nil {
			return fmt.Errorf("git push -u origin HEAD failed: %w", err)
		}

		fmt.Printf("✅ Push complete and upstream configured: origin/%s\n", branchName)
		return nil
	}
	upstream := strings.TrimSpace(string(upBytes))
	parts := strings.SplitN(upstream, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		fmt.Println("Skipping push – unable to resolve upstream")
		return nil
	}
	remote, branch := parts[0], parts[1]

	fmt.Printf("Fetching %s...\n", remote)
	fetchCmd := exec.Command("git", "fetch", "--prune", "--no-progress", remote)
	fetchCmd.Stdout = os.Stdout
	fetchCmd.Stderr = os.Stderr
	if err := fetchCmd.Run(); err != nil {
		fmt.Println("Skipping push – fetch failed")
		return nil
	}

	fmt.Println("Attempting fast-forward merge...")
	mergeCmd := exec.Command("git", "merge", "--ff-only", "@{u}")
	mergeCmd.Stdout = os.Stdout
	mergeCmd.Stderr = os.Stderr
	if err := mergeCmd.Run(); err != nil {
		fmt.Println("Skipping push – fast-forward merge failed (remote has diverged)")
		return nil
	}

	if verbose {
		log.Printf("Pushing HEAD to %s/%s", remote, branch)
	}
	fmt.Printf("Pushing to %s/%s...\n", remote, branch)
	pushCmd := exec.Command("git", "push", "--no-progress", remote, "HEAD:"+branch)
	pushCmd.Stdout = os.Stdout
	pushCmd.Stderr = os.Stderr
	if err := pushCmd.Run(); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}

	fmt.Println("✅ Push complete")
	return nil
}

func renderResult(result *reviewmodel.DiffReviewResponse, format string) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)

	case "pretty":
		return renderPretty(result)

	default:
		return fmt.Errorf("invalid output format: %s (must be json or pretty)", format)
	}
}

func renderPretty(result *reviewmodel.DiffReviewResponse) error {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("LIVEREVIEW RESULTS")
	fmt.Println(strings.Repeat("=", 80))

	if result.Envelope != nil {
		printEnvelopeUsageSummary("result", result.Envelope)
	}

	if result.Summary != "" {
		fmt.Println("\nSummary:")
		fmt.Println(result.Summary)
	}

	if len(result.Files) == 0 {
		fmt.Println("\nNo files reviewed or no comments generated.")
		return nil
	}

	fmt.Printf("\n%d file(s) with comments:\n", len(result.Files))

	for _, file := range result.Files {
		fmt.Println("\n" + strings.Repeat("-", 80))
		fmt.Printf("FILE: %s\n", file.FilePath)
		fmt.Println(strings.Repeat("-", 80))

		if len(file.Comments) == 0 {
			fmt.Println("  No comments for this file.")
			continue
		}

		for _, comment := range file.Comments {
			severity := strings.ToUpper(comment.Severity)
			if severity == "" {
				severity = "INFO"
			}

			fmt.Printf("\n  [%s] Line %d", severity, comment.Line)
			if comment.Category != "" {
				fmt.Printf(" (%s)", comment.Category)
			}
			fmt.Println()

			// Indent comment content
			lines := strings.Split(comment.Content, "\n")
			for _, line := range lines {
				fmt.Printf("    %s\n", line)
			}
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Printf("Review complete: %d total comment(s)\n", countTotalComments(result.Files))
	fmt.Println(strings.Repeat("=", 80) + "\n")

	return nil
}

func printEnvelopeUsageSummary(label string, envelope *reviewmodel.PlanUsageEnvelope) {
	if envelope == nil {
		return
	}
	fmt.Printf("\nUsage (%s):\n", label)
	for _, line := range formatEnvelopeUsageLines(envelope) {
		fmt.Printf("  - %s\n", line)
	}
}

func formatEnvelopeUsageLines(envelope *reviewmodel.PlanUsageEnvelope) []string {
	if envelope == nil {
		return nil
	}
	lines := make([]string, 0, 12)
	if envelope.PlanCode != "" {
		lines = append(lines, fmt.Sprintf("plan_code: %s", envelope.PlanCode))
	}
	if envelope.PlanCode == "free_30k" {
		lines = append(lines, "ai_mode: BYOK required on free plan")
	} else if envelope.PlanCode == "team_32usd" {
		lines = append(lines, "ai_mode: Auto by default on team plan (BYOK optional)")
	}
	if envelope.UsagePercent != nil {
		lines = append(lines, fmt.Sprintf("usage_percent: %d%%", *envelope.UsagePercent))
	}
	if envelope.LOCUsedMonth != nil || envelope.LOCLimitMonth != nil || envelope.LOCRemainMonth != nil {
		used := "?"
		limit := "?"
		remain := "?"
		if envelope.LOCUsedMonth != nil {
			used = fmt.Sprintf("%d", *envelope.LOCUsedMonth)
		}
		if envelope.LOCLimitMonth != nil {
			limit = fmt.Sprintf("%d", *envelope.LOCLimitMonth)
		}
		if envelope.LOCRemainMonth != nil {
			remain = fmt.Sprintf("%d", *envelope.LOCRemainMonth)
		}
		lines = append(lines, fmt.Sprintf("loc_used/limit/remaining: %s/%s/%s", used, limit, remain))
	}
	if envelope.ResetAt != "" {
		lines = append(lines, fmt.Sprintf("reset_at: %s", envelope.ResetAt))
	}
	if envelope.ThresholdState != "" {
		lines = append(lines, fmt.Sprintf("threshold_state: %s", envelope.ThresholdState))
	}
	if envelope.TrialReadOnly {
		lines = append(lines, "trial_readonly: true")
	}
	if envelope.Blocked {
		lines = append(lines, "blocked: true")
	}
	if envelope.OperationBillableLOC != nil {
		lines = append(lines, fmt.Sprintf("operation_billable_loc: %d", *envelope.OperationBillableLOC))
	}
	if envelope.OperationType != "" {
		lines = append(lines, fmt.Sprintf("operation_type: %s", envelope.OperationType))
	}
	if envelope.AIExecutionMode != "" {
		lines = append(lines, fmt.Sprintf("ai_execution_mode: %s", envelope.AIExecutionMode))
	}
	if envelope.AIExecutionSource != "" {
		lines = append(lines, fmt.Sprintf("ai_execution_source: %s", envelope.AIExecutionSource))
	}
	return lines
}

func countTotalComments(files []reviewmodel.DiffReviewFileResult) int {
	total := 0
	for _, file := range files {
		total += len(file.Comments)
	}
	return total
}

// Config holds the CLI configuration
type Config struct {
	APIKey       string
	APIURL       string
	OrgID        string
	JWT          string
	RefreshToken string
	ConfigPath   string
}

// loadConfigValues attempts to load configuration from ~/.lrc.toml, then applies CLI/env overrides
func loadConfigValues(apiKeyOverride, apiURLOverride string, verbose bool) (*Config, error) {
	config := &Config{}

	// Try to load from config file first
	configPath, err := configpath.ResolveConfigPath()
	var k *koanf.Koanf
	if err == nil {
		config.ConfigPath = configPath
		if _, err := os.Stat(configPath); err == nil {
			// Config file exists, try to load it
			k = koanf.New(".")
			if err := k.Load(file.Provider(configPath), toml.Parser()); err != nil {
				return nil, fmt.Errorf("failed to load config file %s: %w", configPath, err)
			}
			if verbose {
				log.Printf("Loaded config from: %s", configPath)
			}
		}
	}

	// Load API key: CLI/env overrides config file
	if apiKeyOverride != "" {
		config.APIKey = apiKeyOverride
		if verbose {
			log.Println("Using API key from CLI flag or environment variable")
		}
	} else if k != nil && k.String("api_key") != "" {
		config.APIKey = k.String("api_key")
		if verbose {
			log.Println("Using API key from config file")
		}
	} else {
		return nil, fmt.Errorf("API key not provided. Set via --api-key flag, LRC_API_KEY environment variable, or api_key in ~/.lrc.toml")
	}

	// Load API URL: CLI/env overrides config file
	if apiURLOverride != "" && apiURLOverride != reviewopts.DefaultAPIURL {
		config.APIURL = apiURLOverride
		if verbose {
			log.Println("Using API URL from CLI flag or environment variable")
		}
	} else if k != nil && k.String("api_url") != "" {
		config.APIURL = k.String("api_url")
		if verbose {
			log.Println("Using API URL from config file")
		}
	} else {
		config.APIURL = reviewopts.DefaultAPIURL
		if verbose {
			log.Printf("Using default API URL: %s", config.APIURL)
		}
	}

	if k != nil {
		config.OrgID = strings.TrimSpace(k.String("org_id"))
		config.JWT = strings.TrimSpace(k.String("jwt"))
		config.RefreshToken = strings.TrimSpace(k.String("refresh_token"))
	}

	return config, nil
}

// saveBundleForInspection saves the bundle in multiple formats for inspection
func saveBundleForInspection(path string, diffContent, zipData []byte, base64Diff string, verbose bool) error {
	// Create a comprehensive bundle file with sections
	var buf bytes.Buffer

	buf.WriteString("# LiveReview Bundle Inspection File\n")
	buf.WriteString("# Generated: " + time.Now().Format(time.RFC3339) + "\n\n")

	buf.WriteString("## SECTION 1: Original Diff Content\n")
	buf.WriteString("## This is the raw diff that was collected\n")
	buf.WriteString("## " + strings.Repeat("-", 76) + "\n\n")
	buf.Write(diffContent)
	buf.WriteString("\n\n")

	buf.WriteString("## SECTION 2: Zip Archive Info\n")
	buf.WriteString("## " + strings.Repeat("-", 76) + "\n")
	buf.WriteString(fmt.Sprintf("## Zip size: %d bytes\n", len(zipData)))
	buf.WriteString("## Contains: diff.txt\n\n")

	buf.WriteString("## SECTION 3: Base64 Encoded Bundle (sent to API)\n")
	buf.WriteString("## This is what gets transmitted in the API request\n")
	buf.WriteString("## " + strings.Repeat("-", 76) + "\n\n")
	buf.WriteString(base64Diff)
	buf.WriteString("\n")

	if err := storage.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return err
	}

	if verbose {
		log.Printf("Bundle saved to: %s (%d bytes)", path, buf.Len())
	}

	return nil
}

// saveJSONResponse saves the raw JSON response to a file
func saveJSONResponse(path string, result *reviewmodel.DiffReviewResponse, verbose bool) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if err := storage.WriteFile(path, data, 0644); err != nil {
		return err
	}

	if verbose {
		log.Printf("JSON response saved to: %s (%d bytes)", path, len(data))
	}

	return nil
}

// saveTextOutput saves formatted text output with special markers for easy comment navigation
func saveTextOutput(path string, result *reviewmodel.DiffReviewResponse, verbose bool) error {
	var buf bytes.Buffer

	// Use a distinctive marker that's easy to search for
	const commentMarker = ">>>COMMENT<<<"

	buf.WriteString("=" + strings.Repeat("=", 79) + "\n")
	buf.WriteString("LIVEREVIEW RESULTS - TEXT FORMAT\n")
	buf.WriteString("=" + strings.Repeat("=", 79) + "\n")
	buf.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format(time.RFC3339)))
	buf.WriteString("\nSearch for '" + commentMarker + "' to jump between review comments\n")
	buf.WriteString("=" + strings.Repeat("=", 79) + "\n\n")

	if result.Summary != "" {
		buf.WriteString("SUMMARY:\n")
		buf.WriteString(result.Summary)
		buf.WriteString("\n\n")
	}

	totalComments := countTotalComments(result.Files)
	buf.WriteString(fmt.Sprintf("TOTAL FILES: %d\n", len(result.Files)))
	buf.WriteString(fmt.Sprintf("TOTAL COMMENTS: %d\n\n", totalComments))

	if len(result.Files) == 0 {
		buf.WriteString("No files reviewed or no comments generated.\n")
	} else {
		for fileIdx, file := range result.Files {
			buf.WriteString("\n" + strings.Repeat("=", 80) + "\n")
			buf.WriteString(fmt.Sprintf("FILE %d/%d: %s\n", fileIdx+1, len(result.Files), file.FilePath))
			buf.WriteString(strings.Repeat("=", 80) + "\n")

			if len(file.Comments) == 0 {
				buf.WriteString("\n  No comments for this file.\n")
				continue
			}

			buf.WriteString(fmt.Sprintf("\n  %d comment(s) on this file\n\n", len(file.Comments)))

			// Create a map of line numbers to comments for easy lookup
			commentsByLine := make(map[int][]reviewmodel.DiffReviewComment)
			for _, comment := range file.Comments {
				commentsByLine[comment.Line] = append(commentsByLine[comment.Line], comment)
			}

			// Process each hunk and insert comments inline
			for hunkIdx, hunk := range file.Hunks {
				if hunkIdx > 0 {
					buf.WriteString("\n")
				}

				// Parse and render the hunk with line numbers
				renderHunkWithComments(&buf, hunk, commentsByLine, commentMarker)
			}
		}
	}

	buf.WriteString("\n" + strings.Repeat("=", 80) + "\n")
	buf.WriteString(fmt.Sprintf("END OF REVIEW - %d total comment(s)\n", totalComments))
	buf.WriteString(strings.Repeat("=", 80) + "\n")

	if err := storage.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return err
	}

	if verbose {
		log.Printf("Text output saved to: %s (%d bytes)", path, buf.Len())
		log.Printf("Search for '%s' in the file to navigate between comments", commentMarker)
	}

	return nil
}

// renderHunkWithComments renders a diff hunk with line numbers and inline comments
func renderHunkWithComments(buf *bytes.Buffer, hunk reviewmodel.DiffReviewHunk, commentsByLine map[int][]reviewmodel.DiffReviewComment, marker string) {
	// Write hunk header
	buf.WriteString(strings.Repeat("-", 80) + "\n")
	buf.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n",
		hunk.OldStartLine, hunk.OldLineCount,
		hunk.NewStartLine, hunk.NewLineCount))
	buf.WriteString(strings.Repeat("-", 80) + "\n")

	// Parse the hunk content line by line
	lines := strings.Split(hunk.Content, "\n")
	oldLine := hunk.OldStartLine
	newLine := hunk.NewStartLine

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		// Skip the hunk header line if it's in the content
		if strings.HasPrefix(line, "@@") {
			continue
		}

		var oldNum, newNum string
		var diffLine string

		if strings.HasPrefix(line, "-") {
			// Deleted line - only old line number
			oldNum = fmt.Sprintf("%4d", oldLine)
			newNum = "    "
			diffLine = line
			oldLine++
		} else if strings.HasPrefix(line, "+") {
			// Added line - only new line number
			oldNum = "    "
			newNum = fmt.Sprintf("%4d", newLine)
			diffLine = line

			// Check for comments on this new line
			if comments, hasComment := commentsByLine[newLine]; hasComment {
				// First write the diff line
				buf.WriteString(fmt.Sprintf("%s | %s | %s\n", oldNum, newNum, diffLine))

				// Then write all comments for this line
				for _, comment := range comments {
					buf.WriteString(fmt.Sprintf("\n%s ", marker))
					severity := strings.ToUpper(comment.Severity)
					if severity == "" {
						severity = "INFO"
					}
					buf.WriteString(fmt.Sprintf("[%s] Line %d", severity, comment.Line))
					if comment.Category != "" {
						buf.WriteString(fmt.Sprintf(" (%s)", comment.Category))
					}
					buf.WriteString("\n" + strings.Repeat("-", 80) + "\n")

					// Write comment content with indentation
					commentLines := strings.Split(comment.Content, "\n")
					for _, cl := range commentLines {
						buf.WriteString("  " + cl + "\n")
					}
					buf.WriteString(strings.Repeat("-", 80) + "\n\n")
				}
				newLine++
				continue
			}

			newLine++
		} else {
			// Context line - both line numbers
			oldNum = fmt.Sprintf("%4d", oldLine)
			newNum = fmt.Sprintf("%4d", newLine)
			diffLine = " " + line
			oldLine++
			newLine++
		}

		buf.WriteString(fmt.Sprintf("%s | %s | %s\n", oldNum, newNum, diffLine))
	}

	buf.WriteString("\n")
}

// parseDiffToFiles parses raw git diff content into file structures for HTML display
func parseDiffToFiles(diffContent []byte) ([]reviewmodel.DiffReviewFileResult, error) {
	if len(diffContent) == 0 {
		return nil, fmt.Errorf("empty diff content")
	}

	var files []reviewmodel.DiffReviewFileResult
	diffStr := string(diffContent)
	// Handle both LF (\n) and CRLF (\r\n) line endings for cross-platform compatibility
	lines := strings.FieldsFunc(diffStr, func(r rune) bool {
		return r == '\n' || r == '\r'
	})

	var currentFile *reviewmodel.DiffReviewFileResult
	var currentHunk *reviewmodel.DiffReviewHunk
	var hunkLines []string

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// New file header: diff --git a/path b/path
		if strings.HasPrefix(line, "diff --git") {
			// Save previous file if exists
			if currentFile != nil {
				if currentHunk != nil && len(hunkLines) > 0 {
					currentHunk.Content = strings.Join(hunkLines, "\n")
					currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
				}
				files = append(files, *currentFile)
			}

			// Extract file path (after b/)
			parts := strings.Split(line, " ")
			filePath := ""
			for _, part := range parts {
				if strings.HasPrefix(part, "b/") {
					filePath = strings.TrimPrefix(part, "b/")
					break
				}
			}

			currentFile = &reviewmodel.DiffReviewFileResult{
				FilePath: filePath,
				Hunks:    []reviewmodel.DiffReviewHunk{},
				Comments: []reviewmodel.DiffReviewComment{},
			}
			currentHunk = nil
			hunkLines = nil
			continue
		}

		// Hunk header: @@ -old_start,old_count +new_start,new_count @@
		if strings.HasPrefix(line, "@@") && currentFile != nil {
			// Save previous hunk if exists
			if currentHunk != nil && len(hunkLines) > 0 {
				currentHunk.Content = strings.Join(hunkLines, "\n")
				currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
			}

			// Parse hunk header
			re := regexp.MustCompile(`@@ -(\d+),?(\d*) \+(\d+),?(\d*) @@`)
			matches := re.FindStringSubmatch(line)
			if len(matches) >= 4 {
				oldStart, _ := strconv.Atoi(matches[1])
				oldCount, _ := strconv.Atoi(matches[2])
				if oldCount == 0 {
					oldCount = 1
				}
				newStart, _ := strconv.Atoi(matches[3])
				newCount, _ := strconv.Atoi(matches[4])
				if newCount == 0 {
					newCount = 1
				}

				currentHunk = &reviewmodel.DiffReviewHunk{
					OldStartLine: oldStart,
					OldLineCount: oldCount,
					NewStartLine: newStart,
					NewLineCount: newCount,
				}
				hunkLines = []string{line} // Include the header
			}
			continue
		}

		// Hunk content lines (-, +, or space prefix)
		if currentHunk != nil && (strings.HasPrefix(line, "-") || strings.HasPrefix(line, "+") || strings.HasPrefix(line, " ")) {
			hunkLines = append(hunkLines, line)
		}
	}

	// Save last file and hunk
	if currentFile != nil {
		if currentHunk != nil && len(hunkLines) > 0 {
			currentHunk.Content = strings.Join(hunkLines, "\n")
			currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
		}
		files = append(files, *currentFile)
	}

	return files, nil
}

// saveHTMLOutput saves formatted HTML output with GitHub-style review UI

func saveHTMLOutput(path string, result *reviewmodel.DiffReviewResponse, verbose bool, interactive bool, isPostCommitReview bool, initialMsg, reviewID, apiURL, apiKey string) error {
	// Prepare template data
	data := reviewhtml.PrepareHTMLData(result, interactive, isPostCommitReview, initialMsg, reviewID, apiURL, apiKey)

	// Render HTML using template
	htmlContent, err := staticserve.RenderPreactHTML(data)
	if err != nil {
		return fmt.Errorf("failed to render HTML template: %w", err)
	}

	// Write to file
	if err := storage.WriteFile(path, []byte(htmlContent), 0644); err != nil {
		return err
	}

	if verbose {
		log.Printf("HTML output saved to: %s (%d bytes)", path, len(htmlContent))
		log.Printf("Open in browser: file://%s", path)
	}

	return nil
}

// renderHTMLFile renders a single file's diff and comments as HTML

// serveHTML starts an HTTP server to serve the HTML file
func serveHTML(htmlPath string, port int, ln net.Listener) error {
	absPath, err := filepath.Abs(htmlPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("HTML file not found: %w", err)
	}

	url := fmt.Sprintf("http://localhost:%d", port)
	log.Printf("Starting HTTP server on %s", url)
	log.Printf("Serving: %s", absPath)
	log.Printf("Press Ctrl+C to stop the server")

	// Try to open browser
	go func() {
		time.Sleep(500 * time.Millisecond)
		openURL(url)
	}()

	// Setup HTTP handler
	mux := http.NewServeMux()
	// Serve static assets (JS, CSS) from embedded filesystem
	mux.Handle("/static/", http.StripPrefix("/static/", staticserve.GetStaticHandler()))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, absPath)
	})

	// Start server using the already-open listener to avoid TOCTOU port races
	server := &http.Server{Handler: mux}
	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// https://stackoverflow.com/questions/39320371/how-start-web-server-to-open-page-in-browser-in-golang
// openURL opens the specified URL in the default browser of the user.
func openURL(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // "linux", "freebsd", "openbsd", "netbsd"
		// Check if running under WSL
		if isWSL() {
			// Use 'cmd.exe /c start' to open the URL in the default Windows browser
			cmd = "cmd.exe"
			args = []string{"/c", "start", url}
		} else {
			// Use xdg-open on native Linux environments
			cmd = "xdg-open"
			args = []string{url}
		}
	}
	return exec.Command(cmd, args...).Start()
}

// isWSL checks if the Go program is running inside Windows Subsystem for Linux
func isWSL() bool {
	releaseData, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(releaseData)), "microsoft")
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// sanitizeInitialMessage strips trailers and whitespace from a prefilled commit message
// and drops the message entirely if only trailers remain.
func sanitizeInitialMessage(msg string) string {
	trimmed := strings.TrimSpace(msg)
	if trimmed == "" {
		return ""
	}

	lines := strings.Split(trimmed, "\n")
	filtered := make([]string, 0, len(lines))

	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		if strings.HasPrefix(clean, "LiveReview Pre-Commit Check:") {
			continue
		}
		if strings.HasPrefix(clean, "#") {
			// Drop git template comment lines for prefill cleanliness
			continue
		}
		filtered = append(filtered, line)
	}

	result := strings.TrimSpace(strings.Join(filtered, "\n"))
	return result
}

// openTTY opens the controlling terminal for reading.
// On Unix this is /dev/tty; on Windows it is CONIN$ (the console input buffer).
func openTTY() (*os.File, error) {
	return input.OpenTTY()
}

// persistCommitMessage writes the desired commit message to a temporary file that the commit-msg hook will consume.
func persistCommitMessage(commitMsgPath, message string) error {
	if commitMsgPath == "" {
		return nil
	}

	trimmed := strings.TrimRight(message, "\r\n")
	if strings.TrimSpace(trimmed) == "" {
		return clearCommitMessageFile(commitMsgPath)
	}

	normalized := trimmed + "\n"
	return storage.WriteFile(commitMsgPath, []byte(normalized), 0600)
}

// clearCommitMessageFile removes any pending commit-message override file.
func clearCommitMessageFile(commitMsgPath string) error {
	if commitMsgPath == "" {
		return nil
	}

	if err := storage.RemoveCommitMessageOverrideFile(commitMsgPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// persistPushRequest creates a marker file to request a post-commit push.
func persistPushRequest(commitMsgPath string) error {
	if commitMsgPath == "" {
		return nil
	}

	pushPath := filepath.Join(filepath.Dir(commitMsgPath), pushRequestFile)
	return storage.WriteFile(pushPath, []byte("push"), 0600)
}

// clearPushRequest removes any pending push request marker.
func clearPushRequest(commitMsgPath string) error {
	if commitMsgPath == "" {
		return nil
	}

	pushPath := filepath.Join(filepath.Dir(commitMsgPath), pushRequestFile)
	if err := storage.RemoveCommitPushRequestFile(pushPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// readCommitMessageFromRequest extracts an optional commit message from a JSON request body.
func readCommitMessageFromRequest(r *http.Request) string {
	if r.Body == nil {
		return ""
	}

	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil || len(body) == 0 {
		return ""
	}

	var payload struct {
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}

	return sanitizeUserMessage(payload.Message)
}

type draftUpdateRequest struct {
	Message         string `json:"message"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func readDraftUpdateFromRequest(r *http.Request) (draftUpdateRequest, error) {
	var req draftUpdateRequest
	if r.Body == nil {
		return req, nil
	}
	defer r.Body.Close()

	body, err := io.ReadAll(io.LimitReader(r.Body, 128*1024))
	if err != nil {
		return req, err
	}
	if len(body) == 0 {
		return req, nil
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return req, err
	}
	req.Message = sanitizeUserMessage(req.Message)
	return req, nil
}

func sanitizeUserMessage(msg string) string {
	msg = strings.TrimRight(msg, "\r\n")
	msg = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == '\r' {
			return r
		}
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, msg)
	return msg
}

func writeDraftSnapshot(w http.ResponseWriter, snap draftSnapshot) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(snap); err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
	}
}

func handleDraftGet(w http.ResponseWriter, r *http.Request, hub *draftHub) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeDraftSnapshot(w, hub.Snapshot())
}

func handleDraftUpdate(w http.ResponseWriter, r *http.Request, hub *draftHub, source draftSource) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	req, err := readDraftUpdateFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	snap, updateErr := hub.Update(req.Message, source, req.ExpectedVersion)
	if updateErr != nil {
		if errors.Is(updateErr, ErrDraftFrozen) {
			http.Error(w, updateErr.Error(), http.StatusConflict)
			return
		}
		if errors.Is(updateErr, ErrDraftStaleVersion) {
			http.Error(w, updateErr.Error(), http.StatusConflict)
			return
		}
		http.Error(w, updateErr.Error(), http.StatusBadRequest)
		return
	}
	writeDraftSnapshot(w, snap)
}

func handleDraftEvents(w http.ResponseWriter, r *http.Request, hub *draftHub) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	updates, unsubscribe := hub.Subscribe()
	defer unsubscribe()

	for {
		select {
		case <-r.Context().Done():
			return
		case snap, ok := <-updates:
			if !ok {
				return
			}
			payload, err := json.Marshal(snap)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func openDraftInEditor(initial string) (string, error) {
	tmp, err := os.CreateTemp("", "lrc-draft-*.txt")
	if err != nil {
		return "", err
	}
	path := tmp.Name()
	defer os.Remove(path)

	if _, err := tmp.WriteString(initial); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	cmdSpec := strings.TrimSpace(os.Getenv("LRC_FALLBACK_EDITOR"))
	if cmdSpec == "" {
		cmdSpec = strings.TrimSpace(os.Getenv("VISUAL"))
	}
	if cmdSpec == "" {
		cmdSpec = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if cmdSpec == "" {
		cmdSpec = "vi"
	}

	parts := strings.Fields(cmdSpec)
	if len(parts) == 0 {
		parts = []string{"vi"}
	}
	args := append(parts[1:], path)
	cmd := exec.Command(parts[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return sanitizeUserMessage(string(body)), nil
}

func handleDraftEditor(w http.ResponseWriter, r *http.Request, hub *draftHub) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	current := hub.Snapshot()
	edited, err := openDraftInEditor(current.Text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	snap, updateErr := hub.Update(edited, draftSourceEditor, current.Version)
	if updateErr != nil {
		if errors.Is(updateErr, ErrDraftFrozen) || errors.Is(updateErr, ErrDraftStaleVersion) {
			http.Error(w, updateErr.Error(), http.StatusConflict)
			return
		}
		http.Error(w, updateErr.Error(), http.StatusBadRequest)
		return
	}
	writeDraftSnapshot(w, snap)
}

func handleReviewEventsProxy(w http.ResponseWriter, r *http.Request, config Config, reviewID string, verbose bool) {
	backendURL := network.ReviewProxyRequestURL(config.APIURL, r.URL.Path, r.URL.RawQuery)

	if verbose {
		log.Printf("Proxying %s request to: %s", r.Method, backendURL)
		log.Printf("Using API key: %s...", config.APIKey[:min(10, len(config.APIKey))])
	}

	var reqBody []byte
	if r.Body != nil {
		const maxProxyBodyBytes = 8 << 20 // 8 MiB
		readBody, readErr := io.ReadAll(io.LimitReader(r.Body, maxProxyBodyBytes+1))
		if readErr != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		if len(readBody) > maxProxyBodyBytes {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		reqBody = readBody
	}

	client := network.NewReviewProxyClient(10 * time.Second)
	resp, err := network.ReviewProxyRequest(client, r.Method, config.APIURL, r.URL.Path, r.URL.RawQuery, reqBody, config.APIKey)
	if err != nil {
		if verbose {
			log.Printf("Proxy error: %v", err)
		}
		http.Error(w, "Failed to fetch events", http.StatusBadGateway)
		return
	}
	if verbose {
		log.Printf("Backend response status: %d", resp.StatusCode)
	}

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)

	if verbose && resp.StatusCode != http.StatusOK {
		log.Printf("Error response body: %s", string(resp.Body))
	}
	if _, err := io.Copy(w, bytes.NewReader(resp.Body)); err != nil && verbose {
		log.Printf("failed to write proxy response body: %v", err)
	}
}

func buildDecisionMetadata(reviewID, account, title, reviewURL string) []string {
	metadata := make([]string, 0, 4)
	if strings.TrimSpace(reviewID) != "" {
		metadata = append(metadata, fmt.Sprintf("Review ID: %s", reviewID))
	}
	if strings.TrimSpace(account) != "" {
		metadata = append(metadata, fmt.Sprintf("Account: %s", account))
	}
	if strings.TrimSpace(title) != "" {
		metadata = append(metadata, fmt.Sprintf("Title: %s", title))
	}
	if strings.TrimSpace(reviewURL) != "" {
		metadata = append(metadata, fmt.Sprintf("Review link: %s", reviewURL))
	}
	return metadata
}

// serveHTMLInteractive serves HTML and waits for user decision
// Returns decision details (code: 0 commit, 1 abort, 2 skip-from-terminal, 3 skip-from-HTML)
// skipBrowserOpen: set to true if browser is already open (e.g., from progressive loading)
func serveHTMLInteractive(htmlPath string, port int, ln net.Listener, initialMsg string, metadata []string, skipBrowserOpen bool) (int, string, bool, error) {
	absPath, err := filepath.Abs(htmlPath)
	if err != nil {
		return 1, "", false, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); err != nil {
		return 1, "", false, fmt.Errorf("HTML file not found: %w", err)
	}

	url := fmt.Sprintf("http://localhost:%d", port)
	fmt.Printf("\n")
	fmt.Printf("🌐 Review available at: %s\n", highlightURL(url))
	fmt.Printf("\n")

	// Open browser only if not already open
	if !skipBrowserOpen {
		go func() {
			time.Sleep(500 * time.Millisecond)
			openURL(url)
		}()
	}

	// Setup HTTP handler
	mux := http.NewServeMux()
	// Serve static assets (JS, CSS) from embedded filesystem
	mux.Handle("/static/", http.StripPrefix("/static/", staticserve.GetStaticHandler()))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, absPath)
	})

	type precommitDecision struct {
		code    int
		message string
		push    bool
	}

	runtimeDecision := decisionruntime.New(decisionflow.PhaseReviewComplete)
	draftState := newDraftHub(initialMsg)

	decisionChan := make(chan precommitDecision, 1)
	var decideOnce sync.Once
	decide := func(code int, message string, push bool) {
		decideOnce.Do(func() {
			decisionChan <- precommitDecision{code: code, message: message, push: push}
		})
	}
	submit := func(source decisionruntime.Source, code int, message string, push bool) bool {
		outcome := runtimeDecision.TryDecide(decisionruntime.Decision{
			Source:  source,
			Code:    code,
			Message: message,
			Push:    push,
		})
		if outcome.Err != nil || !outcome.Accepted {
			return false
		}
		chosen, ok := runtimeDecision.Chosen()
		if !ok {
			return false
		}
		draftState.Freeze()
		decide(chosen.Code, chosen.Message, chosen.Push)
		return true
	}
	handleDecision := func(w http.ResponseWriter, code int, message string, push bool) {
		outcome := runtimeDecision.TryDecide(decisionruntime.Decision{
			Source:  decisionruntime.SourceWeb,
			Code:    code,
			Message: message,
			Push:    push,
		})
		if outcome.Err != nil {
			if errors.Is(outcome.Err, decisionruntime.ErrAlreadyResolved) {
				http.Error(w, outcome.Err.Error(), http.StatusConflict)
				return
			}
			reqErr, ok := outcome.Err.(*decisionflow.RequestError)
			if !ok {
				http.Error(w, outcome.Err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, reqErr.Error(), reqErr.StatusCode())
			return
		}
		chosen, ok := runtimeDecision.Chosen()
		if !ok {
			http.Error(w, "decision resolution failed", http.StatusConflict)
			return
		}
		draftState.Freeze()
		decide(chosen.Code, chosen.Message, chosen.Push)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}

	mux.HandleFunc("/api/draft", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handleDraftGet(w, r, draftState)
			return
		}
		if r.Method == http.MethodPost {
			handleDraftUpdate(w, r, draftState, draftSourceWeb)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/api/draft/events", func(w http.ResponseWriter, r *http.Request) {
		handleDraftEvents(w, r, draftState)
	})

	// Pre-commit action endpoints (HTML buttons call these)
	mux.HandleFunc("/commit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		msg := readCommitMessageFromRequest(r)
		if snap, err := draftState.Update(msg, draftSourceWeb, 0); err == nil {
			msg = snap.Text
		}
		handleDecision(w, decisionflow.DecisionCommit, msg, false)
	})

	mux.HandleFunc("/commit-push", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		msg := readCommitMessageFromRequest(r)
		if snap, err := draftState.Update(msg, draftSourceWeb, 0); err == nil {
			msg = snap.Text
		}
		handleDecision(w, decisionflow.DecisionCommit, msg, true)
	})

	mux.HandleFunc("/skip", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		msg := readCommitMessageFromRequest(r)
		if snap, err := draftState.Update(msg, draftSourceWeb, 0); err == nil {
			msg = snap.Text
		}
		handleDecision(w, decisionflow.DecisionSkip, msg, false)
	})

	mux.HandleFunc("/vouch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		msg := readCommitMessageFromRequest(r)
		if snap, err := draftState.Update(msg, draftSourceWeb, 0); err == nil {
			msg = snap.Text
		}
		handleDecision(w, decisionflow.DecisionVouch, msg, false)
	})

	mux.HandleFunc("/abort", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		draftState.Freeze()
		handleDecision(w, decisionflow.DecisionAbort, "", false)
	})

	mux.HandleFunc("/handoff", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		handleDecision(w, decisionflow.DecisionHandoff, string(body), false)
	})

	// Start server in background using the already-open listener
	server := &http.Server{
		Handler: mux,
	}

	serverReady := make(chan bool, 1)
	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	// Give server a moment to start
	go func() {
		time.Sleep(200 * time.Millisecond)
		serverReady <- true
	}()

	<-serverReady

	// Wait for decision: terminal Bubble Tea, Ctrl-C signal, or HTML buttons
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	prompt := decisionPrompt{
		Title:                   "Review complete. Choose action",
		Description:             "Use terminal keys or web UI buttons.",
		Metadata:                metadata,
		InitialText:             initialMsg,
		FocusMessage:            true,
		AllowCommit:             true,
		AllowPush:               true,
		AllowSkip:               true,
		AllowVouch:              true,
		AllowAbort:              true,
		RequireMessageForCommit: true,
		RequireMessageForSkip:   true,
		RequireMessageForVouch:  true,
	}
	publishTerminalDraft := func(text string) int64 {
		snap, err := draftState.Update(text, draftSourceTerminal, 0)
		if err != nil {
			return 0
		}
		return snap.Version
	}
	openEditor := func() (string, int64, error) {
		current := draftState.Snapshot()
		edited, err := openDraftInEditor(current.Text)
		if err != nil {
			return "", 0, err
		}
		snap, err := draftState.Update(edited, draftSourceEditor, current.Version)
		if err != nil {
			return "", 0, err
		}
		return snap.Text, snap.Version, nil
	}
	tuiDecisionCh, _, setTUIDraft, stopTUI, tuiDone := startTerminalDecisionBubbleTea(prompt, publishTerminalDraft, openEditor)
	updates, unsubscribe := draftState.Subscribe()
	defer unsubscribe()
	go func() {
		for snap := range updates {
			setTUIDraft(snap.Text, snap.Version)
		}
	}()

	go func() {
		<-sigChan
		submit(decisionruntime.SourceSignal, decisionflow.DecisionAbort, "", false)
	}()

	go func() {
		for d := range tuiDecisionCh {
			submit(decisionruntime.SourceTerminal, d.Code, d.Message, d.Push)
		}
	}()

	// Wait for any decision source
	decision := <-decisionChan
	stopTUI()
	<-tuiDone

	switch decision.code {
	case decisionflow.DecisionCommit:
		syncedPrintln("\n✅ Proceeding with commit")
	case decisionflow.DecisionSkip:
		syncedPrintln("\n⏭️  Review skipped, proceeding with commit")
	case decisionflow.DecisionVouch:
		syncedPrintln("\n✅ Vouched, proceeding with commit")
	case decisionflow.DecisionAbort:
		syncedPrintln("\n❌ Commit aborted by user")
	}
	syncedPrintln()
	server.Close()
	return decision.code, decision.message, decision.push, nil
}

// =============================================================================
