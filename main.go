package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

// Version information (set via ldflags during build)
const appVersion = "v0.1.29" // Semantic version - bump this for releases

var (
	version   = appVersion // Can be overridden via ldflags
	buildTime = "unknown"
	gitCommit = "unknown"

	// Global review state for the web UI API
	currentReviewState *ReviewState
	reviewStateMu      sync.RWMutex
)

// Decision codes for interactive review flow.
const (
	decisionCommit  = 0 // proceed with commit
	decisionAbort   = 1 // abort commit
	decisionSkip    = 2 // skip review, proceed with commit
	decisionSkipWeb = 3 // skip requested from web UI, abort commit
	decisionVouch   = 4 // vouch for changes, proceed with commit
)

// diffReviewRequest models the POST payload to /api/v1/diff-review
type diffReviewRequest struct {
	DiffZipBase64 string `json:"diff_zip_base64"`
	RepoName      string `json:"repo_name"`
}

// diffReviewResponse models the response from GET /api/v1/diff-review/:id
type diffReviewResponse struct {
	Status       string                 `json:"status"`
	Summary      string                 `json:"summary,omitempty"`
	Files        []diffReviewFileResult `json:"files,omitempty"`
	Message      string                 `json:"message,omitempty"`
	FriendlyName string                 `json:"friendly_name,omitempty"`
}

type diffReviewCreateResponse struct {
	ReviewID     string `json:"review_id"`
	Status       string `json:"status"`
	FriendlyName string `json:"friendly_name,omitempty"`
	UserEmail    string `json:"user_email,omitempty"`
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API returned status %d: %s", e.StatusCode, e.Body)
}

type diffReviewFileResult struct {
	FilePath string              `json:"file_path"`
	Hunks    []diffReviewHunk    `json:"hunks"`
	Comments []diffReviewComment `json:"comments"`
}

type diffReviewHunk struct {
	OldStartLine int    `json:"old_start_line"`
	OldLineCount int    `json:"old_line_count"`
	NewStartLine int    `json:"new_start_line"`
	NewLineCount int    `json:"new_line_count"`
	Content      string `json:"content"`
}

type diffReviewComment struct {
	Line     int    `json:"line"`
	Content  string `json:"content"`
	Severity string `json:"severity"`
	Category string `json:"category"`
}

const (
	defaultAPIURL       = "http://localhost:8888"
	defaultPollInterval = 2 * time.Second
	defaultTimeout      = 5 * time.Minute
	defaultOutputFormat = "pretty"
	commitMessageFile   = "livereview_commit_message"
	editorWrapperScript = "lrc_editor.sh"
	editorBackupFile    = ".lrc_editor_backup"
	pushRequestFile     = "livereview_push_request"

	// B2 constants for self-update (read-only credentials)
	b2KeyID    = "00536b4c5851afd0000000006"
	b2AppKey   = "K005DV+hNk6/fdQr8oXHmRsdo8U2YAU"
	b2BucketID = "REDACTED_B2_BUCKET_ID"
	b2Prefix   = "lrc"
	b2AuthURL  = "https://api.backblazeb2.com/b2api/v2/b2_authorize_account"
)

// highlightURL adds ANSI color to make served links stand out in terminals.
func highlightURL(url string) string {
	return "\033[36m" + url + "\033[0m"
}

func buildReviewURL(apiURL, reviewID string) string {
	base := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(apiURL, "/"), "/api"), "/api/v1")
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s/#/reviews/%s", base, reviewID)
}

var baseFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    "repo-name",
		Usage:   "repository name (defaults to current directory basename)",
		EnvVars: []string{"LRC_REPO_NAME"},
	},
	&cli.BoolFlag{
		Name:    "staged",
		Usage:   "use staged changes instead of working tree",
		EnvVars: []string{"LRC_STAGED"},
	},
	&cli.StringFlag{
		Name:    "range",
		Usage:   "git range for staged/working diff override (e.g., HEAD~1..HEAD)",
		EnvVars: []string{"LRC_RANGE"},
	},
	&cli.StringFlag{
		Name:    "commit",
		Usage:   "review a specific commit or commit range (e.g., HEAD, HEAD~1, HEAD~3..HEAD, abc123)",
		EnvVars: []string{"LRC_COMMIT"},
	},
	&cli.StringFlag{
		Name:    "diff-file",
		Usage:   "path to pre-generated diff file",
		EnvVars: []string{"LRC_DIFF_FILE"},
	},
	&cli.StringFlag{
		Name:    "api-url",
		Value:   defaultAPIURL,
		Usage:   "LiveReview API base URL",
		EnvVars: []string{"LRC_API_URL"},
	},
	&cli.StringFlag{
		Name:    "api-key",
		Usage:   "API key for authentication (can be set in ~/.lrc.toml or env var)",
		EnvVars: []string{"LRC_API_KEY"},
	},
	&cli.StringFlag{
		Name:    "output",
		Value:   defaultOutputFormat,
		Usage:   "output format: pretty or json",
		EnvVars: []string{"LRC_OUTPUT"},
	},
	&cli.StringFlag{
		Name:    "save-html",
		Usage:   "save formatted HTML output (GitHub-style review) to this file",
		EnvVars: []string{"LRC_SAVE_HTML"},
	},
	&cli.BoolFlag{
		Name:    "serve",
		Usage:   "start HTTP server to serve the HTML output (auto-creates HTML when omitted)",
		EnvVars: []string{"LRC_SERVE"},
	},
	&cli.IntFlag{
		Name:    "port",
		Usage:   "port for HTTP server (used with --serve)",
		Value:   8000,
		EnvVars: []string{"LRC_PORT"},
	},
	&cli.BoolFlag{
		Name:    "verbose",
		Usage:   "enable verbose output",
		EnvVars: []string{"LRC_VERBOSE"},
	},
	&cli.BoolFlag{
		Name:    "precommit",
		Usage:   "pre-commit mode: interactive prompts for commit decision (Ctrl-C=abort, Ctrl-S=skip+commit, Ctrl-V=vouch+commit, Enter=commit)",
		Value:   false,
		EnvVars: []string{"LRC_PRECOMMIT"},
	},
	&cli.BoolFlag{
		Name:    "skip",
		Usage:   "mark review as skipped and write attestation without contacting the API",
		EnvVars: []string{"LRC_SKIP"},
	},
	&cli.BoolFlag{
		Name:    "force",
		Usage:   "force rerun by removing existing attestation/hash for current tree",
		EnvVars: []string{"LRC_FORCE"},
	},
	&cli.BoolFlag{
		Name:    "vouch",
		Usage:   "vouch for changes manually without running AI review (records attestation with coverage stats from prior iterations)",
		EnvVars: []string{"LRC_VOUCH"},
	},
}

var debugFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    "diff-source",
		Usage:   "diff source: working, staged, range, or file (debug override)",
		EnvVars: []string{"LRC_DIFF_SOURCE"},
		Hidden:  true,
	},
	&cli.DurationFlag{
		Name:    "poll-interval",
		Value:   defaultPollInterval,
		Usage:   "interval between status polls",
		EnvVars: []string{"LRC_POLL_INTERVAL"},
	},
	&cli.DurationFlag{
		Name:    "timeout",
		Value:   defaultTimeout,
		Usage:   "maximum time to wait for review completion",
		EnvVars: []string{"LRC_TIMEOUT"},
	},
	&cli.StringFlag{
		Name:    "save-bundle",
		Usage:   "save the base64-encoded bundle to this file for inspection before sending",
		EnvVars: []string{"LRC_SAVE_BUNDLE"},
	},
	&cli.StringFlag{
		Name:    "save-json",
		Usage:   "save the JSON response to this file after completion",
		EnvVars: []string{"LRC_SAVE_JSON"},
	},
	&cli.StringFlag{
		Name:    "save-text",
		Usage:   "save formatted text output with comment markers to this file",
		EnvVars: []string{"LRC_SAVE_TEXT"},
	},
}

func main() {
	app := &cli.App{
		Name:    "lrc",
		Usage:   "LiveReview CLI - submit local diffs for AI review",
		Version: version,
		Flags:   baseFlags,
		Commands: []*cli.Command{
			{
				Name:    "review",
				Aliases: []string{"r"},
				Usage:   "Run a review with sensible defaults",
				Flags:   baseFlags,
				Action:  runReviewSimple,
			},
			{
				Name:   "review-debug",
				Usage:  "Run a review with advanced debug options",
				Flags:  append(baseFlags, debugFlags...),
				Action: runReviewDebug,
			},
			{
				Name:  "hooks",
				Usage: "Manage LiveReview Git hook integration (global dispatcher)",
				Subcommands: []*cli.Command{
					{
						Name:  "install",
						Usage: "Install global LiveReview hook dispatchers (uses core.hooksPath)",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "path",
								Usage: "custom hooksPath (defaults to core.hooksPath or ~/.git-hooks)",
							},
							&cli.BoolFlag{
								Name:  "local",
								Usage: "install into the current repo hooks path (respects core.hooksPath)",
							},
						},
						Action: runHooksInstall,
					},
					{
						Name:  "uninstall",
						Usage: "Remove LiveReview hook dispatchers and managed scripts",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "local",
								Usage: "uninstall from the current repo hooks path",
							},
						},
						Action: runHooksUninstall,
					},
					{
						Name:   "enable",
						Usage:  "Enable LiveReview hooks for the current repository",
						Action: runHooksEnable,
					},
					{
						Name:   "disable",
						Usage:  "Disable LiveReview hooks for the current repository",
						Action: runHooksDisable,
					},
					{
						Name:   "status",
						Usage:  "Show LiveReview hook status for the current repository",
						Action: runHooksStatus,
					},
				},
			},
			{
				Name:   "install-hooks",
				Usage:  "Install LiveReview hooks (deprecated; use 'lrc hooks install')",
				Hidden: true,
				Action: runHooksInstall,
			},
			{
				Name:   "uninstall-hooks",
				Usage:  "Uninstall LiveReview hooks (deprecated; use 'lrc hooks uninstall')",
				Hidden: true,
				Action: runHooksUninstall,
			},
			{
				Name:  "version",
				Usage: "Show version information",
				Action: func(c *cli.Context) error {
					fmt.Printf("lrc version %s\n", version)
					fmt.Printf("  Build time: %s\n", buildTime)
					fmt.Printf("  Git commit: %s\n", gitCommit)
					return nil
				},
			},
			{
				Name:    "self-update",
				Aliases: []string{"update"},
				Usage:   "Update lrc to the latest version",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "check",
						Usage: "Only check for updates without installing",
					},
					&cli.BoolFlag{
						Name:  "force",
						Usage: "Force reinstall even if already up-to-date",
					},
				},
				Action: runSelfUpdate,
			},
			{
				Name:   "review-cleanup",
				Usage:  "Clean up review session history for the current branch (called by post-commit hook)",
				Hidden: true,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "verbose",
						Usage: "enable verbose output",
					},
				},
				Action: func(c *cli.Context) error {
					return runReviewDBCleanup(c.Bool("verbose"))
				},
			},
			{
				Name:   "attestation-trailer",
				Usage:  "Output the commit trailer for the current attestation (called by commit-msg hook)",
				Hidden: true,
				Action: runAttestationTrailer,
			},
			{
				Name:   "setup",
				Usage:  "Guided onboarding — authenticate with Hexmos and configure LiveReview + AI",
				Action: runSetup,
			},
		},
		Action: runReviewSimple,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

type reviewOptions struct {
	repoName     string
	diffSource   string
	rangeVal     string
	commitVal    string
	diffFile     string
	apiURL       string
	apiKey       string
	pollInterval time.Duration
	timeout      time.Duration
	output       string
	saveBundle   string
	saveJSON     string
	saveText     string
	saveHTML     string
	serve        bool
	port         int
	verbose      bool
	precommit    bool
	skip         bool
	force        bool
	vouch        bool
	initialMsg   string
}

func runReviewSimple(c *cli.Context) error {
	opts, err := buildOptionsFromContext(c, false)
	if err != nil {
		return err
	}
	return runReviewWithOptions(opts)
}

func runReviewDebug(c *cli.Context) error {
	opts, err := buildOptionsFromContext(c, true)
	if err != nil {
		return err
	}
	return runReviewWithOptions(opts)
}

func buildOptionsFromContext(c *cli.Context, includeDebug bool) (reviewOptions, error) {
	// Get initial commit message from file or environment variable
	initialMsg := ""
	if msgFile := os.Getenv("LRC_INITIAL_MESSAGE_FILE"); msgFile != "" {
		if data, err := os.ReadFile(msgFile); err == nil {
			initialMsg = strings.TrimRight(string(data), "\r\n")
		}
	} else {
		initialMsg = strings.TrimRight(os.Getenv("LRC_INITIAL_MESSAGE"), "\r\n")
	}

	opts := reviewOptions{
		repoName:   c.String("repo-name"),
		rangeVal:   c.String("range"),
		commitVal:  c.String("commit"),
		diffFile:   c.String("diff-file"),
		apiURL:     c.String("api-url"),
		apiKey:     c.String("api-key"),
		output:     c.String("output"),
		saveHTML:   c.String("save-html"),
		serve:      c.Bool("serve"),
		port:       c.Int("port"),
		verbose:    c.Bool("verbose"),
		precommit:  c.Bool("precommit"),
		skip:       c.Bool("skip"),
		force:      c.Bool("force"),
		vouch:      c.Bool("vouch"),
		saveJSON:   c.String("save-json"),
		saveText:   c.String("save-text"),
		initialMsg: initialMsg,
	}

	if opts.skip || opts.vouch {
		opts.precommit = false
	}
	if opts.skip && opts.vouch {
		return reviewOptions{}, fmt.Errorf("cannot use --skip and --vouch together")
	}

	staged := c.Bool("staged")
	diffSource := c.String("diff-source")

	if opts.diffFile != "" {
		diffSource = "file"
	} else if opts.commitVal != "" {
		diffSource = "commit"
		// Commit mode is for post-commit reviews - disable precommit/skip features
		opts.precommit = false
		opts.skip = false
		// Auto-enable serve mode for post-commit reviews (user can view in browser)
		// Only if not explicitly set by user via flags
		if !c.IsSet("serve") && !c.IsSet("save-html") {
			opts.serve = true
		}
	} else if opts.rangeVal != "" {
		diffSource = "range"
	} else if staged {
		diffSource = "staged"
	}

	if diffSource == "" {
		diffSource = "staged"
	}

	opts.diffSource = diffSource

	if includeDebug {
		opts.pollInterval = c.Duration("poll-interval")
		opts.timeout = c.Duration("timeout")
		opts.saveBundle = c.String("save-bundle")
	} else {
		opts.pollInterval = defaultPollInterval
		opts.timeout = defaultTimeout
	}

	if opts.apiURL == "" {
		opts.apiURL = defaultAPIURL
	}

	if opts.output == "" {
		opts.output = defaultOutputFormat
	}

	return opts, nil
}

// applyDefaultHTMLServe enables HTML saving/serving when the user runs with defaults.
// It only triggers when no HTML path or serve flag was provided and the output format is the default.
func applyDefaultHTMLServe(opts *reviewOptions) (string, error) {
	// If HTML path already set or output format is not HTML, nothing to do
	if opts.saveHTML != "" || opts.output != defaultOutputFormat {
		return opts.saveHTML, nil
	}

	// Serve is enabled but no HTML path - create temp file
	if opts.serve {
		tmpFile, err := os.CreateTemp("", "lrc-review-*.html")
		if err != nil {
			return "", fmt.Errorf("failed to create temporary HTML file: %w", err)
		}

		if err := tmpFile.Close(); err != nil {
			return "", fmt.Errorf("failed to prepare temporary HTML file: %w", err)
		}

		opts.saveHTML = tmpFile.Name()
		return opts.saveHTML, nil
	}

	return "", nil
}

// pickServePort tries the requested port, then increments by 1 up to maxTries to find a free port.
// It returns the listener itself (kept open) to avoid TOCTOU races where another
// process grabs the port between the check and the actual server start.
//
// On Windows, 0.0.0.0:<port> and 127.0.0.1:<port> are treated as separate bindings,
// so we must check both to detect if a port is truly occupied. On Linux/Mac,
// binding 0.0.0.0 already conflicts with 127.0.0.1, so a single check suffices.
func pickServePort(preferredPort, maxTries int) (net.Listener, int, error) {
	for i := 0; i < maxTries; i++ {
		candidate := preferredPort + i

		if runtime.GOOS == "windows" {
			// On Windows, check both addresses. If either is occupied, the port is busy.
			lnLocal, errLocal := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", candidate))
			lnAll, errAll := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", candidate))

			if errLocal != nil || errAll != nil {
				// Port is busy on at least one address — close whichever succeeded
				if lnLocal != nil {
					lnLocal.Close()
				}
				if lnAll != nil {
					lnAll.Close()
				}
				continue
			}

			// Both succeeded — port is free. Close the 0.0.0.0 listener,
			// keep 127.0.0.1 (lrc only serves on localhost).
			lnAll.Close()
			return lnLocal, candidate, nil
		}

		// Linux/Mac: single bind on all interfaces is sufficient
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", candidate))
		if err == nil {
			return ln, candidate, nil
		}
	}

	return nil, 0, fmt.Errorf("no available port found starting from %d", preferredPort)
}

func runReviewWithOptions(opts reviewOptions) error {
	verbose := opts.verbose
	var tempHTMLPath string
	var commitMsgPath string
	attestationAction := ""
	attestationWritten := false
	initialMsg := sanitizeInitialMessage(opts.initialMsg)

	// Determine if this is a post-commit review (reviewing already-committed code, read-only)
	// vs a pre-commit review (reviewing staged changes before commit, can commit from UI)
	// When --commit flag is used, we're always reviewing historical commits (read-only mode)
	isPostCommitReview := opts.diffSource == "commit"

	// Interactive flow (Web UI with commit actions) is the default when --serve is enabled
	// BUT: disable interactive actions when reviewing historical commits (isPostCommitReview)
	// Skip interactive mode if explicitly using --skip, not serving, or reviewing history
	useInteractive := !opts.skip && opts.serve && !isPostCommitReview

	// Short-circuit skip: collect diff for coverage tracking, write attestation, exit
	if opts.skip {
		attestationAction = "skipped"
		var cov coverageResult
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
				cov, covErr = recordAndComputeCoverage("skipped", parsedFiles, "", verbose)
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
	if opts.vouch {
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
		cov, _ := recordAndComputeCoverage("vouched", parsedFiles, "", verbose)
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

	if opts.precommit {
		gitDir, err := resolveGitDir()
		if err != nil {
			return fmt.Errorf("precommit mode requires a git repository: %w", err)
		}
		commitMsgPath = filepath.Join(gitDir, commitMessageFile)
		_ = clearCommitMessageFile(commitMsgPath)
	}

	// Handle --force: delete existing attestation if present
	// Skip attestation logic for post-commit reviews
	if !isPostCommitReview {
		if opts.force {
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

	// Load configuration from config file or overrides
	config, err := loadConfigValues(opts.apiKey, opts.apiURL, verbose)
	if err != nil {
		return err
	}

	// Determine repo name
	repoName := opts.repoName
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

	var result *diffReviewResponse

	// Collect diff
	diffContent, err := collectDiffWithOptions(opts)
	if err != nil {
		return fmt.Errorf("failed to collect diff: %w", err)
	}

	if len(diffContent) == 0 {
		return fmt.Errorf("no diff content collected")
	}

	if verbose {
		log.Printf("Collected %d bytes of diff content", len(diffContent))
	}

	// Create ZIP archive
	zipData, err := createZipArchive(diffContent)
	if err != nil {
		return fmt.Errorf("failed to create zip archive: %w", err)
	}

	if verbose {
		log.Printf("Created ZIP archive: %d bytes", len(zipData))
	}

	// Base64 encode
	base64Diff := base64.StdEncoding.EncodeToString(zipData)

	// Save bundle if requested
	if bundlePath := opts.saveBundle; bundlePath != "" {
		if err := saveBundleForInspection(bundlePath, diffContent, zipData, base64Diff, verbose); err != nil {
			return fmt.Errorf("failed to save bundle: %w", err)
		}
	}

	// Submit review
	submitResp, err := submitReview(config.APIURL, config.APIKey, base64Diff, repoName, verbose)
	if err != nil {
		// Handle 413 Request Entity Too Large - prompt user to skip if interactive
		var apiErr *APIError
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
		return fmt.Errorf("failed to submit review: %w", err)
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
	var progressiveDecideOnce sync.Once

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

	// In precommit mode, ensure unbuffered output
	if opts.precommit {
		// Force flush and set unbuffered
		os.Stdout.Sync()
		os.Stderr.Sync()
	}

	// Track CLI usage (best-effort, non-blocking)
	go trackCLIUsage(config.APIURL, config.APIKey, verbose)

	// Generate and serve skeleton HTML immediately if --serve is enabled
	// Auto-enable serve when no HTML path specified and not in post-commit mode
	autoServeEnabled := !opts.serve && opts.saveHTML == "" && !isPostCommitReview
	if autoServeEnabled {
		opts.serve = true
	}

	// Recalculate useInteractive now that opts.serve may have been auto-enabled
	// This is critical for Case 1 (hook-based terminal invocation) where serve is auto-enabled
	// and we need the interactive flow with commit/push/skip options
	useInteractive = !opts.skip && opts.serve && !isPostCommitReview

	if opts.serve {
		// Parse the diff content to generate file structures for immediate display
		filesFromDiff, parseErr := parseDiffToFiles(diffContent)
		if parseErr != nil && verbose {
			log.Printf("Warning: failed to parse diff for skeleton HTML: %v", parseErr)
		}

		// Initialize global review state for API-based UI
		reviewStateMu.Lock()
		currentReviewState = NewReviewState(reviewID, filesFromDiff, useInteractive, isPostCommitReview, initialMsg, config.APIURL)
		reviewStateMu.Unlock()

		// Start serving immediately in background
		serveListener, selectedPort, err := pickServePort(opts.port, 10)
		if err != nil {
			return fmt.Errorf("failed to find available port: %w", err)
		}
		if selectedPort != opts.port {
			fmt.Printf("Port %d is busy; serving on %d instead.\n", opts.port, selectedPort)
			opts.port = selectedPort
		}

		serveURL := fmt.Sprintf("http://localhost:%d", opts.port)
		fmt.Printf("\n🌐 Review available at: %s\n", highlightURL(serveURL))
		fmt.Printf("   Comments will appear progressively as review runs\n\n")

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

		// Start server in background
		go func() {
			mux := http.NewServeMux()
			// Serve static assets (JS, CSS) from embedded filesystem
			mux.Handle("/static/", http.StripPrefix("/static/", getStaticHandler()))

			// Serve index.html from embedded filesystem (no file on disk needed)
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				htmlBytes, err := staticFiles.ReadFile("static/index.html")
				if err != nil {
					http.Error(w, "Failed to load page", http.StatusInternalServerError)
					return
				}
				w.Write(htmlBytes)
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

			// Functional commit handlers that work with the decision channel
			mux.HandleFunc("/commit", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				msg := readCommitMessageFromRequest(r)
				progressiveDecide(decisionCommit, msg, false)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			})
			mux.HandleFunc("/commit-push", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				msg := readCommitMessageFromRequest(r)
				progressiveDecide(decisionCommit, msg, true)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			})
			mux.HandleFunc("/skip", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				progressiveDecide(decisionSkipWeb, "", false)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			})
			// Proxy endpoint for review-events API to avoid CORS
			mux.HandleFunc("/api/v1/diff-review/", func(w http.ResponseWriter, r *http.Request) {
				// Forward request to backend API with authentication
				backendURL := config.APIURL + r.URL.Path
				if r.URL.RawQuery != "" {
					backendURL += "?" + r.URL.RawQuery
				}

				if verbose {
					log.Printf("Proxying %s request to: %s", r.Method, backendURL)
					log.Printf("Using API key: %s...", config.APIKey[:min(10, len(config.APIKey))])
				}

				// Forward the actual HTTP method (GET, POST, PUT, etc)
				req, err := http.NewRequest(r.Method, backendURL, r.Body)
				if err != nil {
					http.Error(w, "Failed to create request", http.StatusInternalServerError)
					return
				}
				req.Header.Set("X-API-Key", config.APIKey)

				client := &http.Client{Timeout: 10 * time.Second}
				resp, err := client.Do(req)
				if err != nil {
					if verbose {
						log.Printf("Proxy error: %v", err)
					}
					http.Error(w, "Failed to fetch events", http.StatusBadGateway)
					return
				}
				defer resp.Body.Close()

				if verbose {
					log.Printf("Backend response status: %d", resp.StatusCode)
				}

				// Copy response headers
				for key, values := range resp.Header {
					for _, value := range values {
						w.Header().Add(key, value)
					}
				}
				w.WriteHeader(resp.StatusCode)

				// Copy response body
				bodyBytes, err := io.ReadAll(resp.Body)
				if err != nil && verbose {
					log.Printf("Error reading response: %v", err)
				}
				if verbose && resp.StatusCode != 200 {
					log.Printf("Error response body: %s", string(bodyBytes))
				}
				w.Write(bodyBytes)
			})
			server := &http.Server{
				Handler: mux,
			}
			if err := server.Serve(serveListener); err != nil && err != http.ErrServerClosed {
				if verbose {
					log.Printf("Background server error: %v", err)
				}
			}
		}()
		time.Sleep(100 * time.Millisecond) // Give server time to start
	}

	// For post-commit reviews, just poll and get results without interactive flow
	if isPostCommitReview {
		var pollErr error
		result, pollErr = pollReview(config.APIURL, config.APIKey, reviewID, opts.pollInterval, opts.timeout, verbose)
		if pollErr != nil {
			// If progressive loading is active, don't crash - keep server running to show error
			if progressiveLoadingActive {
				fmt.Printf("\n⚠️  Review failed: %v\n", pollErr)
				fmt.Printf("   Error details available in browser at: http://localhost:%d\n", opts.port)
				fmt.Printf("   Press Ctrl-C to exit\n\n")
				// Create result with error so HTML can display it
				result = &diffReviewResponse{
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
		// No attestation for post-commit reviews
	}

	// Interactive path (default): set up decision channels for Ctrl-C / Ctrl-S and poll
	decisionCode := -1
	if useInteractive {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		decisionChan := make(chan int, 1)
		stopCtrlS := make(chan struct{})
		var stopCtrlSOnce sync.Once
		stopCtrlSFn := func() { stopCtrlSOnce.Do(func() { close(stopCtrlS) }) }

		// Ctrl-C -> abort commit
		go func() {
			<-sigChan
			decisionChan <- decisionAbort
		}()

		// Ctrl-S -> skip review but still commit; Ctrl-C captured in raw mode fallback
		go func() {
			code, err := handleCtrlKeyWithCancel(stopCtrlS)
			if err == nil && code != 0 {
				decisionChan <- code
			}
		}()

		fmt.Println("💡 Press Ctrl-C to abort, Ctrl-S to skip, or Ctrl-V to vouch and commit")
		fmt.Println("")
		os.Stdout.Sync()

		// Poll concurrently and race with decisions
		var pollResult *diffReviewResponse
		var pollErr error
		pollDone := make(chan struct{})
		go func() {
			pollResult, pollErr = pollReview(config.APIURL, config.APIKey, reviewID, opts.pollInterval, opts.timeout, verbose)
			close(pollDone)
		}()

		var pollFinished bool
		select {
		case decisionCode = <-decisionChan:
			stopCtrlSFn()
		case <-pollDone:
			pollFinished = true
		}

		if pollFinished {
			// Prefer a user decision if it arrives within a short grace window after poll finishes
			select {
			case decisionCode = <-decisionChan:
				// got user decision
			case <-time.After(300 * time.Millisecond):
				// no decision quickly; proceed with poll result
			}
			stopCtrlSFn()
			if pollErr != nil {
				// If progressive loading is active, don't crash - let server keep running to show error
				if progressiveLoadingActive {
					fmt.Printf("\n⚠️  Review failed: %v\n", pollErr)
					fmt.Printf("   Error details available in browser at: http://localhost:%d\n\n", opts.port)
					// Create empty result - error will be delivered via completion event, not in Summary
					result = &diffReviewResponse{
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
					if reviewURL != "" {
						return fmt.Errorf("failed to poll review (see %s): %w", reviewURL, pollErr)
					}
					return fmt.Errorf("failed to poll review: %w", pollErr)
				}
			} else {
				result = pollResult
				// Update review state with final result
				reviewStateMu.Lock()
				if currentReviewState != nil {
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
			switch decisionCode {
			case decisionAbort:
				fmt.Println("\n❌ Review and commit aborted by user")
				fmt.Println()
				return cli.Exit("", decisionAbort)
			case decisionSkip:
				fmt.Println("\n⏭️  Review skipped, proceeding with commit")
				if err := ensureAttestation("skipped", verbose, &attestationWritten); err != nil {
					return err
				}
				fmt.Println()
				return cli.Exit("", decisionSkip)
			case decisionSkipWeb:
				fmt.Println("\n⏭️  Skip requested from review page; aborting commit")
				fmt.Println()
				return cli.Exit("", decisionSkipWeb)
			case decisionVouch:
				fmt.Println("\n✅ Vouched — proceeding with commit")
				if err := recordCoverageAndAttest("vouched", diffContent, reviewID, verbose, &attestationWritten); err != nil {
					fmt.Fprintf(os.Stderr, "Error: vouch failed: %v\n", err)
					return cli.Exit("", decisionAbort)
				}
				fmt.Println()
				return cli.Exit("", decisionSkip) // exit code 2: proceed with commit
			}
		}
	}

	// Apply default HTML serve for interactive/non-post-commit reviews
	if !isPostCommitReview {
		autoHTMLPath, err := applyDefaultHTMLServe(&opts)
		if err != nil {
			return err
		}
		tempHTMLPath = autoHTMLPath
	}

	// Clean up temp HTML file on exit
	if tempHTMLPath != "" {
		defer func() {
			if err := os.Remove(tempHTMLPath); err == nil {
				if verbose {
					log.Printf("Removed temporary HTML file: %s", tempHTMLPath)
				}
			} else if verbose {
				log.Printf("Could not remove temporary HTML file %s: %v", tempHTMLPath, err)
			}
		}()
	}

	// Save JSON response if requested
	if jsonPath := opts.saveJSON; jsonPath != "" {
		if err := saveJSONResponse(jsonPath, result, verbose); err != nil {
			return fmt.Errorf("failed to save JSON response: %w", err)
		}
	}

	// Save formatted text output if requested
	if textPath := opts.saveText; textPath != "" {
		if err := saveTextOutput(textPath, result, verbose); err != nil {
			return fmt.Errorf("failed to save text output: %w", err)
		}
	}

	// Save HTML output if requested
	// Skip if progressive loading is active - the browser already has the skeleton HTML
	// and will receive error/completion via the events API
	if htmlPath := opts.saveHTML; htmlPath != "" && !progressiveLoadingActive {
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
	if opts.serve {
		htmlPath := opts.saveHTML

		// Only pick a new port if progressive loading is NOT active (server not already running)
		var nonProgressiveListener net.Listener
		if !progressiveLoadingActive {
			var selectedPort int
			var err error
			nonProgressiveListener, selectedPort, err = pickServePort(opts.port, 10)
			if err != nil {
				return fmt.Errorf("failed to find available port: %w", err)
			}
			if selectedPort != opts.port {
				fmt.Printf("Port %d is busy; serving on %d instead.\n", opts.port, selectedPort)
				opts.port = selectedPort
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
				// Progressive loading active - server already running on opts.port
				fmt.Printf("\n📋 Review complete. Choose action:\n")
				fmt.Printf("   [Enter]  Continue with commit\n")
				fmt.Printf("   [Ctrl-C] Abort commit\n")
				fmt.Printf("   Or use the web UI buttons\n\n")

				// Set up terminal input handlers that call progressiveDecide
				sigChan := make(chan os.Signal, 1)
				signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

				go func() {
					<-sigChan
					progressiveDecide(decisionAbort, "", false) // abort
				}()

				go func() {
					tty, err := openTTY()
					if err != nil {
						// Cannot open terminal (e.g. no console attached).
						// Don't abort — let the web UI buttons be the decision source.
						return
					}
					defer tty.Close()

					// Prompt for commit message if empty
					msg := initialMsg
					if strings.TrimSpace(msg) == "" {
						// Write prompt to stdout so user can see it
						fmt.Print("Enter commit message: ")
						os.Stdout.Sync()
						reader := bufio.NewReader(tty)
						input, err := reader.ReadString('\n')
						if err != nil {
							progressiveDecide(decisionAbort, "", false) // abort on read error
							return
						}
						msg = strings.TrimSpace(input)
					} else {
						// Just wait for Enter if we have initial message
						reader := bufio.NewReader(tty)
						_, err := reader.ReadString('\n')
						if err != nil {
							progressiveDecide(decisionAbort, "", false) // abort on read error
							return
						}
					}
					progressiveDecide(decisionCommit, msg, false) // commit with entered/initial message
				}()

				// Wait for decision from either HTTP endpoint or terminal
				decision := <-progressiveDecisionChan

				if opts.precommit {
					os.Exit(decision.code)
				}

				switch decision.code {
				case decisionAbort:
					fmt.Println("\n❌ Commit aborted by user")
					return cli.Exit("", decision.code)
				case decisionCommit:
					finalMsg := strings.TrimSpace(decision.message)
					if finalMsg == "" {
						finalMsg = strings.TrimSpace(initialMsg)
					}
					if finalMsg == "" {
						return fmt.Errorf("commit message is required for commit/commit+push")
					}
					if err := runCommitAndMaybePush(finalMsg, decision.push, verbose); err != nil {
						return err
					}
					return nil
				}
			} else {
				// No progressive loading - use normal serveHTMLInteractive
				code, msg, push, err := serveHTMLInteractive(htmlPath, opts.port, nonProgressiveListener, initialMsg, false)
				if err != nil {
					return err
				}

				if opts.precommit {
					// Hook path: persist commit message/push request for downstream hooks and exit with hook code
					if commitMsgPath != "" {
						if code == 0 {
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

					if code == 0 && push {
						if err := persistPushRequest(commitMsgPath); err != nil {
							fmt.Fprintf(os.Stderr, "Warning: failed to store push request: %v\n", err)
						}
					} else {
						_ = clearPushRequest(commitMsgPath)
					}

					os.Exit(code)
				}

				// Non-hook interactive: execute commit (and optional push) directly
				switch code {
				case 1:
					return cli.Exit("", code)
				case 2:
					// Skip review but proceed with commit: honor skip by writing attestation already handled above; no commit performed here.
					return nil
				case 3:
					return cli.Exit("", code)
				case 0:
					finalMsg := strings.TrimSpace(msg)
					if finalMsg == "" {
						finalMsg = strings.TrimSpace(initialMsg)
					}
					if finalMsg == "" {
						return fmt.Errorf("commit message is required for commit/commit+push")
					}
					if err := runCommitAndMaybePush(finalMsg, push, verbose); err != nil {
						return err
					}
					return nil
				}
			}
		}

		// Non-interactive serve: just host HTML (skip if progressive loading was active - server already running)
		if !progressiveLoadingActive {
			serveURL := fmt.Sprintf("http://localhost:%d", opts.port)
			fmt.Printf("Serving HTML review at: %s\n", highlightURL(serveURL))
			if err := serveHTML(htmlPath, opts.port, nonProgressiveListener); err != nil {
				return fmt.Errorf("failed to serve HTML: %w", err)
			}
		} else {
			// Progressive loading is active - server is already running in background goroutine
			// We need to block and wait for Ctrl-C so the server keeps running
			if isPostCommitReview {
				fmt.Printf("\n📖 Viewing historical commit review.\n")
			} else {
				fmt.Printf("\n📋 Review in progress.\n")
			}
			fmt.Printf("   Press Ctrl-C to exit.\n\n")
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
			<-sigChan
			fmt.Println("\nExiting...")
			return nil
		}
	}

	// Render result to stdout (skip in interactive mode or when serving - handled by UI)
	if !useInteractive && !opts.serve {
		if err := renderResult(result, opts.output); err != nil {
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

func collectDiffWithOptions(opts reviewOptions) ([]byte, error) {
	diffSource := opts.diffSource
	verbose := opts.verbose

	switch diffSource {
	case "staged":
		if verbose {
			log.Println("Collecting staged changes...")
		}
		return runGitCommand("git", "diff", "--staged")

	case "working":
		if verbose {
			log.Println("Collecting working tree changes...")
		}
		return runGitCommand("git", "diff")

	case "commit":
		commitVal := opts.commitVal
		if commitVal == "" {
			return nil, fmt.Errorf("--commit is required when diff-source=commit")
		}
		if verbose {
			log.Printf("Collecting diff for commit: %s", commitVal)
		}
		// Check if it's a range (contains .. or ...)
		if strings.Contains(commitVal, "..") {
			// It's a commit range, use git diff
			return runGitCommand("git", "diff", commitVal)
		}
		// Single commit, use git show to get the commit's changes
		return runGitCommand("git", "show", "--format=", commitVal)

	case "range":
		rangeVal := opts.rangeVal
		if rangeVal == "" {
			return nil, fmt.Errorf("--range is required when diff-source=range")
		}
		if verbose {
			log.Printf("Collecting diff for range: %s", rangeVal)
		}
		return runGitCommand("git", "diff", rangeVal)

	case "file":
		filePath := opts.diffFile
		if filePath == "" {
			return nil, fmt.Errorf("--diff-file is required when diff-source=file")
		}
		if verbose {
			log.Printf("Reading diff from file: %s", filePath)
		}
		return os.ReadFile(filePath)

	default:
		return nil, fmt.Errorf("invalid diff-source: %s (must be staged, working, commit, range, or file)", diffSource)
	}
}

func runGitCommand(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git command failed: %s\nstderr: %s", err, string(exitErr.Stderr))
		}
		return nil, err
	}
	return output, nil
}

// runCommitAndMaybePush commits the staged changes (bypassing hooks) and optionally pushes with safety checks.
func runCommitAndMaybePush(message string, push bool, verbose bool) error {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return fmt.Errorf("commit message cannot be empty")
	}

	commitCmd := exec.Command("git", "commit", "--no-verify", "-m", msg)
	commitCmd.Stdout = os.Stdout
	commitCmd.Stderr = os.Stderr
	// Set env var to prevent hook recursion (prepare-commit-msg still runs with --no-verify)
	commitCmd.Env = append(os.Environ(), "LRC_SKIP_REVIEW=1")
	if verbose {
		log.Printf("Running git commit (no-verify, LRC_SKIP_REVIEW=1)")
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
		fmt.Println("Skipping push – no upstream configured")
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
	fetchCmd := exec.Command("git", "fetch", "--prune", remote)
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
	pushCmd := exec.Command("git", "push", remote, "HEAD:"+branch)
	pushCmd.Stdout = os.Stdout
	pushCmd.Stderr = os.Stderr
	if err := pushCmd.Run(); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}

	fmt.Println("✅ Push complete")
	return nil
}

type attestationPayload struct {
	Action           string  `json:"action"`
	Iterations       int     `json:"iterations"`
	PriorAICovPct    float64 `json:"prior_ai_coverage_pct"`
	PriorReviewCount int     `json:"prior_review_count"`
}

func ensureAttestation(action string, verbose bool, written *bool) error {
	return ensureAttestationFull(attestationPayload{Action: action}, verbose, written)
}

// recordCoverageAndAttest parses the diff, records a review session with coverage stats,
// and writes a full attestation. Used by both the "reviewed" and "vouched" interactive paths.
func recordCoverageAndAttest(action string, diffContent []byte, reviewID string, verbose bool, attestationWritten *bool) error {
	parsedFiles, parseErr := parseDiffToFiles(diffContent)
	if parseErr != nil {
		return fmt.Errorf("could not parse diff for coverage tracking: %w", parseErr)
	}
	cov, covErr := recordAndComputeCoverage(action, parsedFiles, reviewID, verbose)
	if covErr != nil {
		return fmt.Errorf("coverage computation failed: %w", covErr)
	}
	// Iterations may be 0 when this is the first session on the branch
	// (no prior history); ensure at least 1 so attestation reflects the current action.
	if cov.Iterations == 0 {
		cov.Iterations = 1
	}
	return ensureAttestationFull(attestationPayload{
		Action:           action,
		Iterations:       cov.Iterations,
		PriorAICovPct:    cov.PriorAICovPct,
		PriorReviewCount: cov.PriorReviewCount,
	}, verbose, attestationWritten)
}

func ensureAttestationFull(payload attestationPayload, verbose bool, written *bool) error {
	if written != nil && *written {
		return nil
	}
	if strings.TrimSpace(payload.Action) == "" {
		return nil
	}

	path, err := writeAttestationFullForCurrentTree(payload)
	if err != nil {
		return fmt.Errorf("failed to write attestation: %w", err)
	}
	if verbose {
		log.Printf("Attestation written: %s (action=%s, iter:%d, coverage:%.0f%%)",
			path, payload.Action, payload.Iterations, payload.PriorAICovPct)
	}
	if written != nil {
		*written = true
	}
	return nil
}

// existingAttestationAction returns the attestation action for the current tree, if present.
func existingAttestationAction() (string, error) {
	treeHash, err := currentTreeHash()
	if err != nil {
		return "", err
	}
	if treeHash == "" {
		return "", nil
	}

	gitDir, err := resolveGitDir()
	if err != nil {
		return "", err
	}

	attestPath := filepath.Join(gitDir, "lrc", "attestations", fmt.Sprintf("%s.json", treeHash))
	data, err := os.ReadFile(attestPath)
	if err != nil {
		return "", nil // not present
	}

	var payload attestationPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", nil
	}

	return strings.TrimSpace(payload.Action), nil
}

// readCurrentAttestation reads and parses the full attestation payload for the current tree.
func readCurrentAttestation() (*attestationPayload, error) {
	treeHash, err := currentTreeHash()
	if err != nil {
		return nil, err
	}
	if treeHash == "" {
		return nil, nil
	}

	gitDir, err := resolveGitDir()
	if err != nil {
		return nil, err
	}

	attestPath := filepath.Join(gitDir, "lrc", "attestations", fmt.Sprintf("%s.json", treeHash))
	data, err := os.ReadFile(attestPath)
	if err != nil {
		return nil, nil // not present
	}

	var payload attestationPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("malformed attestation JSON: %w", err)
	}

	return &payload, nil
}

// runAttestationTrailer outputs the formatted commit trailer from the current
// attestation. Called by the commit-msg hook to avoid fragile sed JSON parsing.
// Outputs nothing (and exits 0) if no attestation is present.
func runAttestationTrailer(c *cli.Context) error {
	payload, err := readCurrentAttestation()
	if err != nil {
		return err
	}
	if payload == nil || strings.TrimSpace(payload.Action) == "" {
		return nil // no attestation — hook will fall back to legacy
	}

	// Map action to trailer value
	var trailerVal string
	switch payload.Action {
	case "reviewed":
		trailerVal = "ran"
	case "skipped":
		trailerVal = "skipped"
	case "vouched":
		trailerVal = "vouched"
	default:
		trailerVal = payload.Action
	}

	// Append iteration and coverage info if available
	if payload.Iterations > 0 {
		covPct := int(payload.PriorAICovPct + 0.5) // round to nearest int
		trailerVal = fmt.Sprintf("%s (iter:%d, coverage:%d%%)", trailerVal, payload.Iterations, covPct)
	}

	fmt.Printf("LiveReview Pre-Commit Check: %s", trailerVal)
	return nil
}

func writeAttestationForCurrentTree(action string) (string, error) {
	return writeAttestationFullForCurrentTree(attestationPayload{Action: action})
}

func writeAttestationFullForCurrentTree(payload attestationPayload) (string, error) {
	if strings.TrimSpace(payload.Action) == "" {
		return "", fmt.Errorf("attestation action cannot be empty")
	}

	treeHash, err := currentTreeHash()
	if err != nil {
		return "", fmt.Errorf("failed to compute tree hash: %w", err)
	}
	if treeHash == "" {
		return "", fmt.Errorf("empty tree hash")
	}

	gitDir, err := resolveGitDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve git dir: %w", err)
	}
	if !filepath.IsAbs(gitDir) {
		gitDir, err = filepath.Abs(gitDir)
		if err != nil {
			return "", fmt.Errorf("failed to absolutize git dir: %w", err)
		}
	}

	attestDir := filepath.Join(gitDir, "lrc", "attestations")
	if err := os.MkdirAll(attestDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create attestation directory: %w", err)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal attestation: %w", err)
	}

	tmpFile, err := os.CreateTemp(attestDir, fmt.Sprintf("%s.*.json", treeHash))
	if err != nil {
		return "", fmt.Errorf("failed to create temp attestation file: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write attestation: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("failed to finalize attestation: %w", err)
	}

	target := filepath.Join(attestDir, fmt.Sprintf("%s.json", treeHash))
	if err := os.Rename(tmpFile.Name(), target); err != nil {
		return "", fmt.Errorf("failed to move attestation into place: %w", err)
	}

	return target, nil
}

func deleteAttestationForCurrentTree() error {
	treeHash, err := currentTreeHash()
	if err != nil {
		return fmt.Errorf("failed to compute tree hash: %w", err)
	}
	if treeHash == "" {
		return nil
	}

	gitDir, err := resolveGitDir()
	if err != nil {
		return fmt.Errorf("failed to resolve git dir: %w", err)
	}

	attestPath := filepath.Join(gitDir, "lrc", "attestations", fmt.Sprintf("%s.json", treeHash))
	if err := os.Remove(attestPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to delete attestation %s: %w", attestPath, err)
	}

	return nil
}

func currentTreeHash() (string, error) {
	out, err := runGitCommand("git", "write-tree")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// resolveGitDir returns the absolute path to the repository's .git directory.
func resolveGitDir() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to locate git directory: %w", err)
	}

	gitDir := strings.TrimSpace(string(out))
	if gitDir == "" {
		return "", fmt.Errorf("git directory path is empty")
	}

	if filepath.IsAbs(gitDir) {
		return gitDir, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to resolve working directory: %w", err)
	}

	return filepath.Join(cwd, gitDir), nil
}

func createZipArchive(diffContent []byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	fileWriter, err := zipWriter.Create("diff.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to create zip entry: %w", err)
	}

	if _, err := fileWriter.Write(diffContent); err != nil {
		return nil, fmt.Errorf("failed to write to zip: %w", err)
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// formatJSONParseError creates a helpful error message when JSON parsing fails.
// It includes hints about common causes like wrong API URL/port.
func formatJSONParseError(body []byte, contentType string, parseErr error) error {
	bodyStr := string(body)
	preview := bodyStr
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}

	// Check if the response looks like HTML (common when hitting frontend instead of API)
	if strings.HasPrefix(strings.TrimSpace(bodyStr), "<") || strings.Contains(contentType, "text/html") {
		return fmt.Errorf("received HTML instead of JSON (Content-Type: %s).\n"+
			"This usually means api_url in ~/.lrc.toml points to the frontend UI instead of the API.\n"+
			"Check that api_url uses the correct port (default API port is 8888, not 8081).\n"+
			"Response preview: %s", contentType, preview)
	}

	// Generic JSON parse error with body preview
	return fmt.Errorf("failed to parse response as JSON: %w\nContent-Type: %s\nResponse preview: %s",
		parseErr, contentType, preview)
}

func submitReview(apiURL, apiKey, base64Diff, repoName string, verbose bool) (diffReviewCreateResponse, error) {
	endpoint := strings.TrimSuffix(apiURL, "/") + "/api/v1/diff-review"

	payload := diffReviewRequest{
		DiffZipBase64: base64Diff,
		RepoName:      repoName,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return diffReviewCreateResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return diffReviewCreateResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	if verbose {
		log.Printf("POST %s", endpoint)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return diffReviewCreateResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return diffReviewCreateResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")

	if resp.StatusCode != http.StatusOK {
		return diffReviewCreateResponse{}, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var result diffReviewCreateResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return diffReviewCreateResponse{}, formatJSONParseError(body, contentType, err)
	}

	if result.ReviewID == "" {
		return diffReviewCreateResponse{}, fmt.Errorf("review_id not found in response")
	}

	return result, nil
}

// trackCLIUsage sends a telemetry ping to the backend to track CLI usage
// This is a best-effort call and failures are silently ignored
func trackCLIUsage(apiURL, apiKey string, verbose bool) {
	endpoint := strings.TrimSuffix(apiURL, "/") + "/api/v1/diff-review/cli-used"

	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		if verbose {
			log.Printf("Failed to create telemetry request: %v", err)
		}
		return
	}

	req.Header.Set("X-API-Key", apiKey)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		if verbose {
			log.Printf("Failed to send telemetry: %v", err)
		}
		return
	}
	defer resp.Body.Close()

	if verbose && resp.StatusCode == http.StatusOK {
		log.Println("CLI usage tracked successfully")
	}
}

func pollReview(apiURL, apiKey, reviewID string, pollInterval, timeout time.Duration, verbose bool) (*diffReviewResponse, error) {
	endpoint := strings.TrimSuffix(apiURL, "/") + "/api/v1/diff-review/" + reviewID
	deadline := time.Now().Add(timeout)
	start := time.Now()
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	fmt.Printf("Waiting for review completion (poll every %s, timeout %s)...\n", pollInterval, timeout)
	os.Stdout.Sync()

	if verbose {
		log.Printf("Polling for review completion (timeout: %v)...", timeout)
	}

	for time.Now().Before(deadline) {
		req, err := http.NewRequest("GET", endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("X-API-Key", apiKey)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		contentType := resp.Header.Get("Content-Type")

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
		}

		var result diffReviewResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, formatJSONParseError(body, contentType, err)
		}

		statusLine := fmt.Sprintf("Status: %s | elapsed: %s", result.Status, time.Since(start).Truncate(time.Second))
		if isTTY {
			fmt.Printf("\r%-80s", statusLine)
			os.Stdout.Sync() // Force flush for real-time updates and clear prior text
		} else {
			fmt.Println(statusLine)
		}
		if verbose {
			log.Printf("%s", statusLine)
		}

		if result.Status == "completed" {
			if isTTY {
				fmt.Printf("\r%-80s\n", statusLine)
			}
			return &result, nil
		}

		if result.Status == "failed" {
			if isTTY {
				fmt.Printf("\r%-80s\n", statusLine)
			}
			// Return the result with error info instead of just an error
			// This allows progressive loading to display error details in the UI
			reason := strings.TrimSpace(result.Message)
			if reason == "" {
				reason = "no additional details provided"
			}
			result.Summary = fmt.Sprintf("Review failed: %s", reason)
			return &result, fmt.Errorf("review failed: %s", reason)
		}

		time.Sleep(pollInterval)
	}

	fmt.Println()
	return nil, fmt.Errorf("timeout waiting for review completion")
}

func renderResult(result *diffReviewResponse, format string) error {
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

func renderPretty(result *diffReviewResponse) error {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("LIVEREVIEW RESULTS")
	fmt.Println(strings.Repeat("=", 80))

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

func countTotalComments(files []diffReviewFileResult) int {
	total := 0
	for _, file := range files {
		total += len(file.Comments)
	}
	return total
}

// Config holds the CLI configuration
type Config struct {
	APIKey string
	APIURL string
}

// loadConfigValues attempts to load configuration from ~/.lrc.toml, then applies CLI/env overrides
func loadConfigValues(apiKeyOverride, apiURLOverride string, verbose bool) (*Config, error) {
	config := &Config{}

	// Try to load from config file first
	homeDir, err := os.UserHomeDir()
	var k *koanf.Koanf
	if err == nil {
		configPath := filepath.Join(homeDir, ".lrc.toml")
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
	if apiURLOverride != "" && apiURLOverride != defaultAPIURL {
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
		config.APIURL = defaultAPIURL
		if verbose {
			log.Printf("Using default API URL: %s", config.APIURL)
		}
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

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return err
	}

	if verbose {
		log.Printf("Bundle saved to: %s (%d bytes)", path, buf.Len())
	}

	return nil
}

// saveJSONResponse saves the raw JSON response to a file
func saveJSONResponse(path string, result *diffReviewResponse, verbose bool) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	if verbose {
		log.Printf("JSON response saved to: %s (%d bytes)", path, len(data))
	}

	return nil
}

// saveTextOutput saves formatted text output with special markers for easy comment navigation
func saveTextOutput(path string, result *diffReviewResponse, verbose bool) error {
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
			commentsByLine := make(map[int][]diffReviewComment)
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

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return err
	}

	if verbose {
		log.Printf("Text output saved to: %s (%d bytes)", path, buf.Len())
		log.Printf("Search for '%s' in the file to navigate between comments", commentMarker)
	}

	return nil
}

// renderHunkWithComments renders a diff hunk with line numbers and inline comments
func renderHunkWithComments(buf *bytes.Buffer, hunk diffReviewHunk, commentsByLine map[int][]diffReviewComment, marker string) {
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
func parseDiffToFiles(diffContent []byte) ([]diffReviewFileResult, error) {
	if len(diffContent) == 0 {
		return nil, fmt.Errorf("empty diff content")
	}

	var files []diffReviewFileResult
	diffStr := string(diffContent)
	// Handle both LF (\n) and CRLF (\r\n) line endings for cross-platform compatibility
	lines := strings.FieldsFunc(diffStr, func(r rune) bool {
		return r == '\n' || r == '\r'
	})

	var currentFile *diffReviewFileResult
	var currentHunk *diffReviewHunk
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

			currentFile = &diffReviewFileResult{
				FilePath: filePath,
				Hunks:    []diffReviewHunk{},
				Comments: []diffReviewComment{},
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

				currentHunk = &diffReviewHunk{
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

func saveHTMLOutput(path string, result *diffReviewResponse, verbose bool, interactive bool, isPostCommitReview bool, initialMsg, reviewID, apiURL, apiKey string) error {
	// Prepare template data
	data := prepareHTMLData(result, interactive, isPostCommitReview, initialMsg, reviewID, apiURL, apiKey)

	// Render HTML using template
	htmlContent, err := renderHTMLTemplate(data)
	if err != nil {
		return fmt.Errorf("failed to render HTML template: %w", err)
	}

	// Write to file
	if err := os.WriteFile(path, []byte(htmlContent), 0644); err != nil {
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
	mux.Handle("/static/", http.StripPrefix("/static/", getStaticHandler()))
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
	if runtime.GOOS == "windows" {
		return os.OpenFile("CONIN$", os.O_RDWR, 0)
	}
	return os.Open("/dev/tty")
}

// handleCtrlKeyWithCancel sets up raw terminal mode to detect Ctrl-S (skip), Ctrl-V (vouch), and Ctrl-C (abort).
// Returns a decision code constant or 0 on cancellation/failure.
func handleCtrlKeyWithCancel(stop <-chan struct{}) (int, error) {
	tty, err := openTTY()
	if err != nil {
		return 0, err
	}
	defer tty.Close()

	// Set terminal to raw mode
	fd := int(tty.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return 0, err
	}

	// Ensure restoration on exit
	defer term.Restore(fd, oldState)

	buf := make([]byte, 1)
	codeChan := make(chan int, 1)
	errChan := make(chan error, 1)

	go func() {
		for {
			n, err := tty.Read(buf)
			if err != nil || n == 0 {
				errChan <- err
				return
			}
			switch buf[0] {
			case 0x03: // Ctrl-C (ETX)
				codeChan <- decisionAbort
				return
			case 0x13: // Ctrl-S (XOFF)
				codeChan <- decisionSkip
				return
			case 0x16: // Ctrl-V (SYN)
				codeChan <- decisionVouch
				return
			}
		}
	}()

	select {
	case code := <-codeChan:
		return code, nil
	case err := <-errChan:
		return 0, err
	case <-stop:
		return 0, fmt.Errorf("cancelled")
	}
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
	return os.WriteFile(commitMsgPath, []byte(normalized), 0600)
}

// clearCommitMessageFile removes any pending commit-message override file.
func clearCommitMessageFile(commitMsgPath string) error {
	if commitMsgPath == "" {
		return nil
	}

	if err := os.Remove(commitMsgPath); err != nil && !os.IsNotExist(err) {
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
	return os.WriteFile(pushPath, []byte("push"), 0600)
}

// clearPushRequest removes any pending push request marker.
func clearPushRequest(commitMsgPath string) error {
	if commitMsgPath == "" {
		return nil
	}

	pushPath := filepath.Join(filepath.Dir(commitMsgPath), pushRequestFile)
	if err := os.Remove(pushPath); err != nil && !os.IsNotExist(err) {
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

	// Sanitize message: remove null bytes and control characters (except newlines/tabs)
	msg := strings.TrimRight(payload.Message, "\r\n")
	msg = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == '\r' {
			return r // Allow newlines and tabs
		}
		if r < 32 || r == 127 {
			return -1 // Remove control characters and DEL
		}
		return r
	}, msg)

	return msg
}

// serveHTMLInteractive serves HTML and waits for user decision
// Returns decision details (code: 0 commit, 1 abort, 2 skip-from-terminal, 3 skip-from-HTML)
// skipBrowserOpen: set to true if browser is already open (e.g., from progressive loading)
func serveHTMLInteractive(htmlPath string, port int, ln net.Listener, initialMsg string, skipBrowserOpen bool) (int, string, bool, error) {
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
	mux.Handle("/static/", http.StripPrefix("/static/", getStaticHandler()))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, absPath)
	})

	type precommitDecision struct {
		code    int
		message string
		push    bool
	}

	decisionChan := make(chan precommitDecision, 1)
	var decideOnce sync.Once
	decide := func(code int, message string, push bool) {
		decideOnce.Do(func() {
			decisionChan <- precommitDecision{code: code, message: message, push: push}
		})
	}

	// Pre-commit action endpoints (HTML buttons call these)
	mux.HandleFunc("/commit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		msg := readCommitMessageFromRequest(r)
		decide(decisionCommit, msg, false)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/commit-push", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		msg := readCommitMessageFromRequest(r)
		decide(decisionCommit, msg, true)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/skip", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		decide(decisionSkipWeb, "", false)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
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

	// Wait for decision: Enter, Ctrl-C, HTML buttons
	// Set up signal handling for Ctrl-C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		decide(1, "", false)
	}()

	// Read from terminal directly to avoid stdin issues in git hooks (Enter fallback, cooked mode)
	go func() {
		tty, err := openTTY()
		if err != nil {
			fmt.Println("Warning: Could not open terminal, auto-proceeding")
			time.Sleep(2 * time.Second)
			decide(decisionCommit, initialMsg, false)
			return
		}
		defer tty.Close()

		reader := bufio.NewReader(tty)

		fmt.Printf("📋 Review complete. Choose action:\n")
		fmt.Printf("   [Enter]  Continue with commit\n")
		fmt.Printf("   [Ctrl-C] Abort commit\n")
		fmt.Printf("\nOptional: type a new commit message and press Enter to use it (leave blank to keep Git's message).\n")
		if strings.TrimSpace(initialMsg) != "" {
			fmt.Printf("(current message): %s\n", initialMsg)
		}
		fmt.Printf("> ")
		os.Stdout.Sync()

		typedMessage, _ := reader.ReadString('\n')
		typedMessage = strings.TrimRight(strings.TrimRight(typedMessage, "\n"), "\r")
		if strings.TrimSpace(typedMessage) == "" {
			typedMessage = initialMsg
		}

		fmt.Printf("\n[Enter] Continue with commit\n")
		fmt.Printf("[Ctrl-C] Abort commit\n")
		fmt.Printf("\nYour choice: ")
		os.Stdout.Sync()

		_, err = reader.ReadString('\n')
		if err != nil {
			decide(decisionCommit, typedMessage, false)
			return
		}
		decide(decisionCommit, typedMessage, false)
	}()

	// Wait for any decision source
	decision := <-decisionChan

	switch decision.code {
	case decisionCommit:
		fmt.Println("\n✅ Proceeding with commit")
	case decisionSkip:
		fmt.Println("\n⏭️  Review skipped from terminal; proceeding with commit")
	case decisionSkipWeb:
		fmt.Println("\n⏭️  Skip requested from review page; aborting commit")
	case decisionAbort:
		fmt.Println("\n❌ Commit aborted by user")
	}
	fmt.Println()
	server.Close()
	return decision.code, decision.message, decision.push, nil
}

// =============================================================================
// GIT HOOK MANAGEMENT
// =============================================================================

const (
	lrcMarkerBegin        = "# BEGIN lrc managed section - DO NOT EDIT"
	lrcMarkerEnd          = "# END lrc managed section"
	defaultGlobalHooksDir = ".git-hooks"
	hooksMetaFilename     = ".lrc-hooks-meta.json"
)

var managedHooks = []string{"pre-commit", "prepare-commit-msg", "commit-msg", "post-commit"}

type hooksMeta struct {
	Path     string `json:"path"`
	PrevPath string `json:"prev_path,omitempty"`
	SetByLRC bool   `json:"set_by_lrc"`
}

func defaultGlobalHooksPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, defaultGlobalHooksDir), nil
}

func currentHooksPath() (string, error) {
	cmd := exec.Command("git", "config", "--global", "--get", "core.hooksPath")
	out, err := cmd.Output()
	if err != nil {
		return "", nil
	}

	return strings.TrimSpace(string(out)), nil
}

func currentLocalHooksPath(repoRoot string) (string, error) {
	cmd := exec.Command("git", "config", "--local", "--get", "core.hooksPath")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", nil
	}

	return strings.TrimSpace(string(out)), nil
}

func resolveRepoHooksPath(repoRoot string) (string, error) {
	localPath, _ := currentLocalHooksPath(repoRoot)
	if localPath == "" {
		return filepath.Join(repoRoot, ".git", "hooks"), nil
	}
	if filepath.IsAbs(localPath) {
		return localPath, nil
	}
	return filepath.Join(repoRoot, localPath), nil
}

func setGlobalHooksPath(path string) error {
	cmd := exec.Command("git", "config", "--global", "core.hooksPath", path)
	return cmd.Run()
}

func unsetGlobalHooksPath() error {
	cmd := exec.Command("git", "config", "--global", "--unset", "core.hooksPath")
	return cmd.Run()
}

func hooksMetaPath(hooksPath string) string {
	return filepath.Join(hooksPath, hooksMetaFilename)
}

func writeHooksMeta(hooksPath string, meta hooksMeta) {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return
	}

	_ = os.MkdirAll(hooksPath, 0755)
	_ = os.WriteFile(hooksMetaPath(hooksPath), data, 0644)
}

func readHooksMeta(hooksPath string) (*hooksMeta, error) {
	data, err := os.ReadFile(hooksMetaPath(hooksPath))
	if err != nil {
		return nil, err
	}

	var meta hooksMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

func removeHooksMeta(hooksPath string) error {
	return os.Remove(hooksMetaPath(hooksPath))
}

func writeManagedHookScripts(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	scripts := map[string]string{
		"pre-commit":         generatePreCommitHook(),
		"prepare-commit-msg": generatePrepareCommitMsgHook(),
		"commit-msg":         generateCommitMsgHook(),
		"post-commit":        generatePostCommitHook(),
	}

	for name, content := range scripts {
		path := filepath.Join(dir, name)
		script := "#!/bin/sh\n" + content
		if err := os.WriteFile(path, []byte(script), 0755); err != nil {
			return fmt.Errorf("failed to write managed hook %s: %w", name, err)
		}
	}

	return nil
}

// runHooksInstall installs dispatchers and managed hook scripts under either global core.hooksPath or the current repo hooks path when --local is used
func runHooksInstall(c *cli.Context) error {
	localInstall := c.Bool("local")
	requestedPath := strings.TrimSpace(c.String("path"))
	var hooksPath string
	setConfig := false

	if localInstall {
		if !isGitRepository() {
			return fmt.Errorf("not in a git repository (no .git directory found)")
		}

		gitDir, err := resolveGitDir()
		if err != nil {
			return err
		}
		repoRoot := filepath.Dir(gitDir)
		hooksPath, err = resolveRepoHooksPath(repoRoot)
		if err != nil {
			return err
		}
	} else {
		currentPath, _ := currentHooksPath()
		defaultPath, err := defaultGlobalHooksPath()
		if err != nil {
			return fmt.Errorf("failed to determine default hooks path: %w", err)
		}

		hooksPath = requestedPath
		if hooksPath == "" {
			if currentPath != "" {
				hooksPath = currentPath
			} else {
				hooksPath = defaultPath
			}
		}

		if currentPath == "" {
			setConfig = true
		} else if requestedPath != "" && requestedPath != currentPath {
			setConfig = true
		}
	}

	absHooksPath, err := filepath.Abs(hooksPath)
	if err != nil {
		return fmt.Errorf("failed to resolve hooks path: %w", err)
	}

	if !localInstall && setConfig {
		if err := setGlobalHooksPath(absHooksPath); err != nil {
			return fmt.Errorf("failed to set core.hooksPath: %w", err)
		}
	}

	if err := os.MkdirAll(absHooksPath, 0755); err != nil {
		return fmt.Errorf("failed to create hooks path %s: %w", absHooksPath, err)
	}

	managedDir := filepath.Join(absHooksPath, "lrc")
	backupDir := filepath.Join(absHooksPath, ".lrc_backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	if err := writeManagedHookScripts(managedDir); err != nil {
		return err
	}

	for _, hookName := range managedHooks {
		hookPath := filepath.Join(absHooksPath, hookName)
		dispatcher := generateDispatcherHook(hookName)
		if err := installHook(hookPath, dispatcher, hookName, backupDir, true); err != nil {
			return fmt.Errorf("failed to install dispatcher for %s: %w", hookName, err)
		}
	}

	if !localInstall {
		writeHooksMeta(absHooksPath, hooksMeta{Path: absHooksPath, PrevPath: hooksPath, SetByLRC: setConfig})
	}
	_ = cleanOldBackups(backupDir, 5)

	if localInstall {
		fmt.Printf("✅ LiveReview hooks installed in repo path: %s\n", absHooksPath)
	} else {
		fmt.Printf("✅ LiveReview global hooks installed at %s\n", absHooksPath)
	}
	fmt.Println("Dispatchers will chain repo-local hooks when present.")
	fmt.Println("Use 'lrc hooks disable' in a repo to bypass LiveReview hooks there.")

	return nil
}

// runHooksUninstall removes lrc-managed sections from dispatchers and managed scripts (global or local)
func runHooksUninstall(c *cli.Context) error {
	localUninstall := c.Bool("local")
	var hooksPath string

	if localUninstall {
		if !isGitRepository() {
			return fmt.Errorf("not in a git repository (no .git directory found)")
		}
		gitDir, err := resolveGitDir()
		if err != nil {
			return err
		}
		repoRoot := filepath.Dir(gitDir)
		hooksPath, err = resolveRepoHooksPath(repoRoot)
		if err != nil {
			return err
		}
	} else {
		hooksPath, _ = currentHooksPath()
		if hooksPath == "" {
			var err error
			hooksPath, err = defaultGlobalHooksPath()
			if err != nil {
				return fmt.Errorf("failed to determine hooks path: %w", err)
			}
		}
	}

	absHooksPath, err := filepath.Abs(hooksPath)
	if err != nil {
		return fmt.Errorf("failed to resolve hooks path: %w", err)
	}

	var meta *hooksMeta
	if !localUninstall {
		meta, _ = readHooksMeta(absHooksPath)
	}
	removed := 0
	for _, hookName := range managedHooks {
		hookPath := filepath.Join(absHooksPath, hookName)
		if err := uninstallHook(hookPath, hookName); err != nil {
			fmt.Printf("⚠️  Warning: failed to uninstall %s: %v\n", hookName, err)
		} else {
			removed++
		}
	}

	_ = os.RemoveAll(filepath.Join(absHooksPath, "lrc"))
	_ = cleanOldBackups(filepath.Join(absHooksPath, ".lrc_backups"), 5)
	if !localUninstall {
		_ = removeHooksMeta(absHooksPath)
	}

	if !localUninstall && meta != nil && meta.SetByLRC && meta.Path == absHooksPath {
		if meta.PrevPath == "" {
			_ = unsetGlobalHooksPath()
		} else {
			_ = setGlobalHooksPath(meta.PrevPath)
		}
	}

	if removed > 0 {
		fmt.Printf("✅ Removed LiveReview sections from %d hook(s) at %s\n", removed, absHooksPath)
	} else {
		fmt.Printf("ℹ️  No LiveReview sections found in %s\n", absHooksPath)
	}

	return nil
}

func runHooksDisable(c *cli.Context) error {
	gitDir, err := resolveGitDir()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	lrcDir := filepath.Join(gitDir, "lrc")
	if err := os.MkdirAll(lrcDir, 0755); err != nil {
		return fmt.Errorf("failed to create lrc directory: %w", err)
	}

	marker := filepath.Join(lrcDir, "disabled")
	if err := os.WriteFile(marker, []byte("disabled\n"), 0644); err != nil {
		return fmt.Errorf("failed to write disable marker: %w", err)
	}

	fmt.Println("🔕 LiveReview hooks disabled for this repository")
	return nil
}

func runHooksEnable(c *cli.Context) error {
	gitDir, err := resolveGitDir()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	marker := filepath.Join(gitDir, "lrc", "disabled")
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove disable marker: %w", err)
	}

	fmt.Println("🔔 LiveReview hooks enabled for this repository")
	return nil
}

func hookHasManagedSection(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	return strings.Contains(string(content), lrcMarkerBegin)
}

func runHooksStatus(c *cli.Context) error {
	hooksPath, _ := currentHooksPath()
	defaultPath, _ := defaultGlobalHooksPath()
	if hooksPath == "" {
		hooksPath = defaultPath
	}

	absHooksPath, err := filepath.Abs(hooksPath)
	if err != nil {
		return fmt.Errorf("failed to resolve hooks path: %w", err)
	}

	gitDir, gitErr := resolveGitDir()
	repoDisabled := false
	if gitErr == nil {
		repoDisabled = fileExists(filepath.Join(gitDir, "lrc", "disabled"))
	}

	fmt.Printf("hooksPath: %s\n", absHooksPath)
	if cfg, _ := currentHooksPath(); cfg != "" {
		fmt.Printf("core.hooksPath: %s\n", cfg)
	} else {
		fmt.Println("core.hooksPath: not set (using repo default unless dispatcher present)")
	}

	if gitErr == nil {
		fmt.Printf("repo: %s\n", filepath.Dir(gitDir))
		if repoDisabled {
			fmt.Println("status: disabled via .git/lrc/disabled")
		} else {
			fmt.Println("status: enabled")
		}
	} else {
		fmt.Println("repo: not detected")
	}

	for _, hookName := range managedHooks {
		hookPath := filepath.Join(absHooksPath, hookName)
		fmt.Printf("%s: ", hookName)
		if hookHasManagedSection(hookPath) {
			fmt.Println("LiveReview dispatcher present")
		} else if fileExists(hookPath) {
			fmt.Println("custom hook (no LiveReview block)")
		} else {
			fmt.Println("missing")
		}
	}

	return nil
}

// isGitRepository checks if current directory is in a git repository
func isGitRepository() bool {
	_, err := os.Stat(".git")
	return err == nil
}

// installHook installs or updates a hook with lrc managed section
func installHook(hookPath, lrcSection, hookName, backupDir string, force bool) error {
	timestamp := time.Now().Format("20060102_150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("%s.%s", hookName, timestamp))

	// Check if hook file exists
	existingContent, err := os.ReadFile(hookPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing hook: %w", err)
	}

	if len(existingContent) == 0 {
		// No existing hook - create new file with just lrc section
		content := "#!/bin/sh\n" + lrcSection
		if err := os.WriteFile(hookPath, []byte(content), 0755); err != nil {
			return fmt.Errorf("failed to write hook: %w", err)
		}
		fmt.Printf("✅ Created %s\n", hookName)
		return nil
	}

	// Existing hook found - create backup
	if err := os.WriteFile(backupPath, existingContent, 0644); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	fmt.Printf("📁 Backup created: %s\n", backupPath)

	// Check if lrc section already exists
	contentStr := string(existingContent)
	if strings.Contains(contentStr, lrcMarkerBegin) {
		if !force {
			fmt.Printf("ℹ️  %s already has lrc section (use --force=false to skip updating)\n", hookName)
			return nil
		}
		// Replace existing lrc section
		newContent := replaceLrcSection(contentStr, lrcSection)
		if err := os.WriteFile(hookPath, []byte(newContent), 0755); err != nil {
			return fmt.Errorf("failed to update hook: %w", err)
		}
		fmt.Printf("✅ Updated %s (replaced lrc section)\n", hookName)
		return nil
	}

	// No lrc section - append it
	var newContent string
	if !strings.HasPrefix(contentStr, "#!/") {
		// No shebang - add one
		newContent = "#!/bin/sh\n" + lrcSection + "\n" + contentStr
	} else {
		// Has shebang - insert after first line
		lines := strings.SplitN(contentStr, "\n", 2)
		if len(lines) == 1 {
			newContent = lines[0] + "\n" + lrcSection
		} else {
			newContent = lines[0] + "\n" + lrcSection + "\n" + lines[1]
		}
	}

	if err := os.WriteFile(hookPath, []byte(newContent), 0755); err != nil {
		return fmt.Errorf("failed to write hook: %w", err)
	}
	fmt.Printf("✅ Updated %s (added lrc section)\n", hookName)

	return nil
}

// uninstallHook removes lrc-managed section from a hook file
func uninstallHook(hookPath, hookName string) error {
	content, err := os.ReadFile(hookPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Hook doesn't exist, nothing to do
		}
		return fmt.Errorf("failed to read hook: %w", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, lrcMarkerBegin) {
		// No lrc section found
		return nil
	}

	// Remove lrc section
	newContent := removeLrcSection(contentStr)

	// If file is now empty or only has shebang, delete it
	trimmed := strings.TrimSpace(newContent)
	if trimmed == "" || trimmed == "#!/bin/sh" {
		if err := os.Remove(hookPath); err != nil {
			return fmt.Errorf("failed to remove hook file: %w", err)
		}
		fmt.Printf("🗑️  Removed %s (was empty after removing lrc section)\n", hookName)
		return nil
	}

	// Write cleaned content back
	if err := os.WriteFile(hookPath, []byte(newContent), 0755); err != nil {
		return fmt.Errorf("failed to write hook: %w", err)
	}
	fmt.Printf("✅ Removed lrc section from %s\n", hookName)

	return nil
}

// installEditorWrapper sets core.editor to an lrc-managed wrapper that injects
// the precommit-provided message when available and falls back to the user's editor.
func installEditorWrapper(gitDir string) error {
	repoRoot := filepath.Dir(gitDir)
	scriptPath := filepath.Join(gitDir, editorWrapperScript)
	backupPath := filepath.Join(gitDir, editorBackupFile)

	// Backup existing core.editor if set
	currentEditor, _ := readGitConfig(repoRoot, "core.editor")
	if currentEditor != "" {
		_ = os.WriteFile(backupPath, []byte(currentEditor), 0600)
	}

	script := fmt.Sprintf(`#!/bin/sh
set -e

OVERRIDE_FILE="%s"

if [ -f "$OVERRIDE_FILE" ] && [ -s "$OVERRIDE_FILE" ]; then
    cat "$OVERRIDE_FILE" > "$1"
    exit 0
fi

if [ -n "$LRC_FALLBACK_EDITOR" ]; then
    exec $LRC_FALLBACK_EDITOR "$@"
fi

if [ -n "$VISUAL" ]; then
    exec "$VISUAL" "$@"
fi

if [ -n "$EDITOR" ]; then
    exec "$EDITOR" "$@"
fi

exec vi "$@"
`, filepath.Join(gitDir, commitMessageFile))

	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("failed to write editor wrapper: %w", err)
	}

	if err := setGitConfig(repoRoot, "core.editor", scriptPath); err != nil {
		return fmt.Errorf("failed to set core.editor: %w", err)
	}

	return nil
}

// uninstallEditorWrapper restores the previous editor (if backed up) and removes wrapper files.
func uninstallEditorWrapper(gitDir string) error {
	repoRoot := filepath.Dir(gitDir)
	scriptPath := filepath.Join(gitDir, editorWrapperScript)
	backupPath := filepath.Join(gitDir, editorBackupFile)

	if data, err := os.ReadFile(backupPath); err == nil {
		value := strings.TrimSpace(string(data))
		if value != "" {
			_ = setGitConfig(repoRoot, "core.editor", value)
		}
	} else {
		// No backup; remove config if set
		_ = unsetGitConfig(repoRoot, "core.editor")
	}

	_ = os.Remove(scriptPath)
	_ = os.Remove(backupPath)

	return nil
}

// readGitConfig reads a single git config key from the repository root.
func readGitConfig(repoRoot, key string) (string, error) {
	cmd := exec.Command("git", "config", "--get", key)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// setGitConfig sets a git config key in the given repository.
func setGitConfig(repoRoot, key, value string) error {
	cmd := exec.Command("git", "config", key, value)
	cmd.Dir = repoRoot
	return cmd.Run()
}

// unsetGitConfig removes a git config key in the given repository.
func unsetGitConfig(repoRoot, key string) error {
	cmd := exec.Command("git", "config", "--unset", key)
	cmd.Dir = repoRoot
	return cmd.Run()
}

// replaceLrcSection replaces the lrc-managed section in hook content
func replaceLrcSection(content, newSection string) string {
	start := strings.Index(content, lrcMarkerBegin)
	if start == -1 {
		return content
	}

	end := strings.Index(content[start:], lrcMarkerEnd)
	if end == -1 {
		return content
	}
	end += start + len(lrcMarkerEnd)

	// Find end of line after marker
	if end < len(content) && content[end] == '\n' {
		end++
	}

	return content[:start] + newSection + "\n" + content[end:]
}

// removeLrcSection removes the lrc-managed section from hook content
func removeLrcSection(content string) string {
	start := strings.Index(content, lrcMarkerBegin)
	if start == -1 {
		return content
	}

	end := strings.Index(content[start:], lrcMarkerEnd)
	if end == -1 {
		return content
	}
	end += start + len(lrcMarkerEnd)

	// Find end of line after marker
	if end < len(content) && content[end] == '\n' {
		end++
	}

	// Remove the section, preserving content before and after
	return content[:start] + content[end:]
}

// generatePreCommitHook generates the pre-commit hook script
func generatePreCommitHook() string {
	return renderHookTemplate("hooks/pre-commit.sh", map[string]string{
		hookMarkerBeginPlaceholder: lrcMarkerBegin,
		hookMarkerEndPlaceholder:   lrcMarkerEnd,
		hookVersionPlaceholder:     version,
	})
}

// generatePrepareCommitMsgHook generates the prepare-commit-msg hook script
func generatePrepareCommitMsgHook() string {
	return renderHookTemplate("hooks/prepare-commit-msg.sh", map[string]string{
		hookMarkerBeginPlaceholder: lrcMarkerBegin,
		hookMarkerEndPlaceholder:   lrcMarkerEnd,
		hookVersionPlaceholder:     version,
	})
}

// generateCommitMsgHook generates the commit-msg hook script
func generateCommitMsgHook() string {
	return renderHookTemplate("hooks/commit-msg.sh", map[string]string{
		hookMarkerBeginPlaceholder:       lrcMarkerBegin,
		hookMarkerEndPlaceholder:         lrcMarkerEnd,
		hookVersionPlaceholder:           version,
		hookCommitMessageFilePlaceholder: commitMessageFile,
	})
}

// generatePostCommitHook runs a safe pull (ff-only) and push when requested.
func generatePostCommitHook() string {
	return renderHookTemplate("hooks/post-commit.sh", map[string]string{
		hookMarkerBeginPlaceholder:     lrcMarkerBegin,
		hookMarkerEndPlaceholder:       lrcMarkerEnd,
		hookVersionPlaceholder:         version,
		hookPushRequestFilePlaceholder: pushRequestFile,
	})
}

func generateDispatcherHook(hookName string) string {
	return renderHookTemplate("hooks/dispatcher.sh", map[string]string{
		hookMarkerBeginPlaceholder: lrcMarkerBegin,
		hookMarkerEndPlaceholder:   lrcMarkerEnd,
		hookVersionPlaceholder:     version,
		hookNamePlaceholder:        hookName,
	})
}

// cleanOldBackups removes old backup files, keeping only the last N
func cleanOldBackups(backupDir string, keepLast int) error {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// Group backups by hook name
	backupsByHook := make(map[string][]os.DirEntry)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Extract hook name (before first dot)
		parts := strings.SplitN(name, ".", 2)
		if len(parts) == 2 {
			hookName := parts[0]
			backupsByHook[hookName] = append(backupsByHook[hookName], entry)
		}
	}

	// For each hook, keep only the last N backups
	for hookName, backups := range backupsByHook {
		if len(backups) <= keepLast {
			continue
		}

		// Sort by name (which includes timestamp)
		// Oldest first
		for i := 0; i < len(backups)-keepLast; i++ {
			oldPath := filepath.Join(backupDir, backups[i].Name())
			if err := os.Remove(oldPath); err != nil {
				log.Printf("Warning: failed to remove old backup %s: %v", oldPath, err)
			} else {
				log.Printf("Removed old backup: %s", backups[i].Name())
			}
		}
		log.Printf("Cleaned up old %s backups (kept last %d)", hookName, keepLast)
	}

	return nil
}

// =============================================================================
// SELF-UPDATE FUNCTIONALITY
// =============================================================================

// Pre-compiled regexes for version parsing
var (
	semverRe        = regexp.MustCompile(`v?(\d+)\.(\d+)\.(\d+)`)
	b2VersionPathRe = regexp.MustCompile(`^lrc/(v\d+\.\d+\.\d+)/`)
)

// b2AuthResponse models the B2 authorization response
type b2AuthResponse struct {
	AuthorizationToken string `json:"authorizationToken"`
	APIURL             string `json:"apiUrl"`
}

// b2ListRequest models the B2 list files request
type b2ListRequest struct {
	BucketID      string `json:"bucketId"`
	StartFileName string `json:"startFileName"`
	Prefix        string `json:"prefix"`
	MaxFileCount  int    `json:"maxFileCount"`
}

// b2ListResponse models the B2 list files response
type b2ListResponse struct {
	Files []struct {
		FileName string `json:"fileName"`
	} `json:"files"`
}

// semverParse extracts major, minor, patch from a version string like "v0.1.14"
func semverParse(v string) (int, int, int, bool) {
	match := semverRe.FindStringSubmatch(strings.TrimSpace(v))
	if match == nil {
		return 0, 0, 0, false
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	return major, minor, patch, true
}

// semverCompare compares two version strings, returns:
// 1 if a > b, -1 if a < b, 0 if equal, error if parsing fails
func semverCompare(a, b string) (int, error) {
	a1, a2, a3, okA := semverParse(a)
	b1, b2, b3, okB := semverParse(b)
	if !okA {
		return 0, fmt.Errorf("invalid version format: %q", a)
	}
	if !okB {
		return 0, fmt.Errorf("invalid version format: %q", b)
	}
	if a1 != b1 {
		if a1 > b1 {
			return 1, nil
		}
		return -1, nil
	}
	if a2 != b2 {
		if a2 > b2 {
			return 1, nil
		}
		return -1, nil
	}
	if a3 != b3 {
		if a3 > b3 {
			return 1, nil
		}
		return -1, nil
	}
	return 0, nil
}

// fetchLatestVersionFromB2 queries B2 to find the latest lrc version
func fetchLatestVersionFromB2() (string, error) {
	// Step 1: Authorize with B2
	authReq, err := http.NewRequest("GET", b2AuthURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create auth request: %w", err)
	}
	authReq.SetBasicAuth(b2KeyID, b2AppKey)

	client := &http.Client{Timeout: 30 * time.Second}
	authResp, err := client.Do(authReq)
	if err != nil {
		return "", fmt.Errorf("B2 auth request failed: %w", err)
	}
	defer authResp.Body.Close()

	if authResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(authResp.Body)
		return "", fmt.Errorf("B2 auth failed with status %d: %s", authResp.StatusCode, string(body))
	}

	var authData b2AuthResponse
	if err := json.NewDecoder(authResp.Body).Decode(&authData); err != nil {
		return "", fmt.Errorf("failed to decode B2 auth response: %w", err)
	}

	// Step 2: List files in the lrc/ prefix
	listURL := authData.APIURL + "/b2api/v2/b2_list_file_names"
	listReqBody := b2ListRequest{
		BucketID:      b2BucketID,
		StartFileName: b2Prefix + "/",
		Prefix:        b2Prefix + "/",
		MaxFileCount:  1000,
	}
	listBodyBytes, err := json.Marshal(listReqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal list request: %w", err)
	}

	listReq, err := http.NewRequest("POST", listURL, bytes.NewReader(listBodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create list request: %w", err)
	}
	listReq.Header.Set("Authorization", authData.AuthorizationToken)
	listReq.Header.Set("Content-Type", "application/json")

	listResp, err := client.Do(listReq)
	if err != nil {
		return "", fmt.Errorf("B2 list request failed: %w", err)
	}
	defer listResp.Body.Close()

	if listResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(listResp.Body)
		return "", fmt.Errorf("B2 list failed with status %d: %s", listResp.StatusCode, string(body))
	}

	var listData b2ListResponse
	if err := json.NewDecoder(listResp.Body).Decode(&listData); err != nil {
		return "", fmt.Errorf("failed to decode B2 list response: %w", err)
	}

	// Step 3: Extract versions and find the latest
	seen := make(map[string]bool)
	var latestVersion string

	for _, f := range listData.Files {
		match := b2VersionPathRe.FindStringSubmatch(f.FileName)
		if match != nil {
			v := match[1]
			if !seen[v] {
				seen[v] = true
				if latestVersion == "" {
					latestVersion = v
				} else if cmp, err := semverCompare(v, latestVersion); err == nil && cmp > 0 {
					latestVersion = v
				}
			}
		}
	}

	if latestVersion == "" {
		return "", fmt.Errorf("no versions found in B2 bucket")
	}

	return latestVersion, nil
}

// platformInstallCommand returns the appropriate installer command for the current platform
func platformInstallCommand() string {
	if runtime.GOOS == "windows" {
		return `powershell -Command "iwr -useb https://hexmos.com/lrc-install.ps1 | iex"`
	}
	return "curl -fsSL https://hexmos.com/lrc-install.sh | sudo bash"
}

// ANSI color codes for terminal output
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// runSelfUpdate handles the self-update command
func runSelfUpdate(c *cli.Context) error {
	checkOnly := c.Bool("check")
	force := c.Bool("force")

	fmt.Printf("Current version: %s%s%s\n", colorCyan, version, colorReset)
	fmt.Println("Checking for updates...")

	latestVersion, err := fetchLatestVersionFromB2()
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	fmt.Printf("Latest version:  %s%s%s\n", colorCyan, latestVersion, colorReset)

	cmp, err := semverCompare(version, latestVersion)
	if err != nil {
		return fmt.Errorf("failed to compare versions: %w", err)
	}
	if cmp >= 0 && !force {
		fmt.Printf("\n%s✓ lrc is already up to date!%s\n", colorGreen, colorReset)
		return nil
	}

	if cmp >= 0 && force {
		fmt.Printf("\n%sForce reinstall requested%s\n", colorYellow, colorReset)
	} else {
		fmt.Printf("\n%s⬆ Update available: %s → %s%s\n", colorYellow, version, latestVersion, colorReset)
	}

	if checkOnly {
		fmt.Println("\nRun 'lrc self-update' (without --check) to install the update.")
		return nil
	}

	// Warn about sudo requirement on non-Windows platforms
	if runtime.GOOS != "windows" {
		fmt.Printf("\n%s%s⚠ NOTE: The installer will use 'sudo' and may prompt for your password.%s\n\n",
			colorBold, colorYellow, colorReset)
	}

	// Get the installer command
	installCmd := platformInstallCommand()
	fmt.Printf("Running installer: %s\n\n", installCmd)

	// Execute the installer
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-Command", installCmd)
	} else {
		cmd = exec.Command("bash", "-c", installCmd)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installer failed: %w", err)
	}

	fmt.Printf("\n%s✓ Update complete! Run 'lrc version' to verify.%s\n", colorGreen, colorReset)
	return nil
}
