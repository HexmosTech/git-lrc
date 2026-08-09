package main

import (
	"errors"
	"fmt"
	"os"

	cmdapp "github.com/HexmosTech/git-lrc/cmd"
	"github.com/HexmosTech/git-lrc/internal/appcore"
	"github.com/HexmosTech/git-lrc/internal/appui"
	"github.com/HexmosTech/git-lrc/internal/reviewdb"
	"github.com/HexmosTech/git-lrc/internal/reviewopts"
	"github.com/HexmosTech/git-lrc/internal/reviewquery"
	"github.com/HexmosTech/git-lrc/internal/selfupdate"
	"github.com/urfave/cli/v2"
)

const appVersion = "v0.6.7"

var (
	version    = appVersion
	buildTime  = "unknown"
	gitCommit  = "unknown"
	reviewMode = "prod"
)

var baseFlags = []cli.Flag{
	&cli.StringFlag{Name: "repo-name", Usage: "repository name (defaults to current directory basename)", EnvVars: []string{"LRC_REPO_NAME"}},
	&cli.BoolFlag{Name: "staged", Usage: "use staged changes instead of working tree", EnvVars: []string{"LRC_STAGED"}},
	&cli.StringFlag{Name: "range", Usage: "review a diff between two refs, e.g. for a PR before merging: 'main...my-feature' (changes on my-feature since it diverged from main). Read-only: skips commit/attestation.", EnvVars: []string{"LRC_RANGE"}},
	&cli.StringFlag{Name: "commit", Usage: "review a specific commit or commit range (e.g., HEAD, HEAD~1, HEAD~3..HEAD, abc123)", EnvVars: []string{"LRC_COMMIT"}},
	&cli.StringFlag{Name: "diff-file", Usage: "path to pre-generated diff file", EnvVars: []string{"LRC_DIFF_FILE"}},
	&cli.StringFlag{Name: "api-url", Value: reviewopts.DefaultAPIURL, Usage: "LiveReview API base URL", EnvVars: []string{"LRC_API_URL"}},
	&cli.StringFlag{Name: "api-key", Usage: "API key for authentication (can be set in ~/.lrc.toml or env var)", EnvVars: []string{"LRC_API_KEY"}},
	&cli.StringFlag{Name: "output", Value: reviewopts.DefaultOutputFormat, Usage: "output format: pretty or json", EnvVars: []string{"LRC_OUTPUT"}},
	&cli.StringFlag{Name: "save-html", Usage: "save formatted HTML output (GitHub-style review) to this file", EnvVars: []string{"LRC_SAVE_HTML"}},
	&cli.BoolFlag{Name: "serve", Usage: "start HTTP server to serve the HTML output (auto-creates HTML when omitted)", EnvVars: []string{"LRC_SERVE"}},
	&cli.IntFlag{Name: "port", Usage: "port for HTTP server (used with --serve)", Value: 8000, EnvVars: []string{"LRC_PORT"}},
	&cli.BoolFlag{Name: "verbose", Usage: "enable verbose output", EnvVars: []string{"LRC_VERBOSE"}},
	&cli.BoolFlag{Name: "precommit", Usage: "pre-commit mode: interactive prompts for commit decision (Ctrl-C=abort, Ctrl-S=skip+commit, Ctrl-V=vouch+commit, Enter=commit)", Value: false, EnvVars: []string{"LRC_PRECOMMIT"}},
	&cli.BoolFlag{Name: "blocking-review", Usage: "launch the decision-capable web review UI and block until a proceed or abort decision is made", EnvVars: []string{"LRC_BLOCKING_REVIEW"}},
	&cli.DurationFlag{Name: "blocking-review-timeout", Value: reviewopts.DefaultBlockingReviewTimeout, Usage: "maximum total time blocking review mode may hold the caller before aborting", EnvVars: []string{"LRC_BLOCKING_REVIEW_TIMEOUT"}},
	&cli.BoolFlag{Name: "skip", Usage: "mark review as skipped and write attestation without contacting the API", EnvVars: []string{"LRC_SKIP"}},
	&cli.BoolFlag{Name: "force", Usage: "force rerun by removing existing attestation/hash for current tree", EnvVars: []string{"LRC_FORCE"}},
	&cli.BoolFlag{Name: "vouch", Usage: "vouch for changes manually without running AI review (records attestation with coverage stats from prior iterations)", EnvVars: []string{"LRC_VOUCH"}},
	&cli.BoolFlag{Name: "blast-radius", Value: true, Usage: "score hunks by blast radius using the local graph engine (default on; auto-indexes the current repo, silently skipped when the engine isn't installed - see `lrc graph install`); disable with --blast-radius=false", EnvVars: []string{"LRC_BLAST_RADIUS"}},
	&cli.StringFlag{Name: "blast-radius-project", Usage: "override the codebase-memory-mcp project name to score against (default: auto-derived from the repo root; see `lrc graph status`)", EnvVars: []string{"LRC_BLAST_RADIUS_PROJECT"}},
	&cli.BoolFlag{Name: "sort-by-blast-radius", Usage: "reorder hunks within each file by descending blast-radius score (implies --blast-radius)", EnvVars: []string{"LRC_SORT_BY_BLAST_RADIUS"}},
}

var debugFlags = []cli.Flag{
	&cli.StringFlag{Name: "diff-source", Usage: "diff source: working, staged, range, or file (debug override)", EnvVars: []string{"LRC_DIFF_SOURCE"}, Hidden: true},
	&cli.DurationFlag{Name: "poll-interval", Value: reviewopts.DefaultPollInterval, Usage: "interval between status polls", EnvVars: []string{"LRC_POLL_INTERVAL"}},
	&cli.DurationFlag{Name: "timeout", Value: reviewopts.DefaultTimeout, Usage: "maximum time to wait for review completion", EnvVars: []string{"LRC_TIMEOUT"}},
	&cli.StringFlag{Name: "save-bundle", Usage: "save the base64-encoded bundle to this file for inspection before sending", EnvVars: []string{"LRC_SAVE_BUNDLE"}},
	&cli.StringFlag{Name: "save-json", Usage: "save the JSON response to this file after completion", EnvVars: []string{"LRC_SAVE_JSON"}},
	&cli.StringFlag{Name: "save-text", Usage: "save formatted text output with comment markers to this file", EnvVars: []string{"LRC_SAVE_TEXT"}},
}

func main() {
	selfupdate.SetVersion(version)
	// Self-healing backstop, independent of self-update's own sync step:
	// runs on every invocation (any subcommand, either binary name) so a
	// git-lrc binary that drifted out of sync with lrc - however that
	// happened - gets repaired on the very next command rather than staying
	// silently stale. Cheap in the common case (just a size comparison via
	// os.Stat); only reads/copies file content when a mismatch is found.
	if err := selfupdate.EnsureGitLRCBinarySynced(false); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not resync git-lrc binary: %v\n", err)
	}
	// Opportunistic, cheap (single indexed COUNT query) check for pending
	// offline commit-sync items; only spawns a detached background worker
	// when there's actually something due. Runs on every invocation, any
	// repo, so a machine that was offline when a commit's post-commit hook
	// fired still catches up on the next `lrc` command run anywhere.
	//
	// Backgrounded (mirrors selfupdate's own auto-update check goroutine
	// below) so this can never add latency to the foreground command: the
	// SQLite busy-timeout on ~/.lrc/sync-queue.db is 5s, and without this a
	// lock collision with a concurrently-running flush worker could stall
	// every single `lrc` invocation by that much. Fire-and-forget is
	// correct here -- if the process exits before this completes, the next
	// invocation (or the post-commit-triggered worker) picks it up.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "warning: opportunistic sync check panicked: %v\n", r)
			}
		}()
		appcore.TriggerOpportunisticSyncFlush()
	}()
	appui.SetBuildInfo(version, buildTime, gitCommit)
	appcore.Configure(version, reviewMode)

	app := cmdapp.BuildApp(version, buildTime, gitCommit, reviewMode, baseFlags, debugFlags, cmdapp.Handlers{
		RunReviewSimple:                 runReviewSimple,
		RunReviewDebug:                  runReviewDebug,
		RunEnsure:                       appui.RunEnsure,
		RunUninstall:                    appcore.RunUninstall,
		RunHooksInstall:                 appcore.RunHooksInstall,
		RunHooksUninstall:               appcore.RunHooksUninstall,
		RunGraphInstall:                 appcore.RunGraphInstall,
		RunGraphStatus:                  appcore.RunGraphStatus,
		RunGraphUninstall:               appcore.RunGraphUninstall,
		RunHooksEnable:                  appcore.RunHooksEnable,
		RunHooksDisable:                 appcore.RunHooksDisable,
		RunHooksStatus:                  appcore.RunHooksStatus,
		RunSelfUpdate:                   selfupdate.RunSelfUpdate,
		RunReviewCleanup:                func(c *cli.Context) error { return reviewdb.RunReviewDBCleanup(c.Bool("verbose")) },
		RunAttestationTrailer:           appcore.RunAttestationTrailer,
		RunSetup:                        appui.RunSetup,
		RunOnboard:                      appui.RunOnboard,
		RunUI:                           appui.RunUI,
		RunUsageInspect:                 appcore.RunUsageInspect,
		RunInternalClaudePreToolUse:     appcore.RunInternalClaudePreToolUse,
		RunInternalClaudeRunCommit:      appcore.RunInternalClaudeRunCommit,
		RunInternalClaudeSetupStart:     appcore.RunInternalClaudeSetupStart,
		RunInternalClaudeSetupWorker:    appcore.RunInternalClaudeSetupWorker,
		RunInternalClaudeSetupSubmitKey: appcore.RunInternalClaudeSetupSubmitKey,
		RunInternalClaudeSetupStatus:    appcore.RunInternalClaudeSetupStatus,
		RunRemoveAttestation:            appcore.RunRemoveAttestation,
		RunConfigInit:                   appcore.RunConfigInit,
		RunConfigCheck:                  appcore.RunConfigCheck,
		RunConfigPreview:                appcore.RunConfigPreview,
		RunQuery:                        reviewquery.RunQuery,
		RunQueryAdd:                     reviewquery.RunQueryAdd,
		RunQueryList:                    reviewquery.RunQueryList,
		RunQueryView:                    reviewquery.RunQueryView,
		RunQueryDelete:                  reviewquery.RunQueryDelete,
		RunCoverage:                     appcore.RunCoverage,
		RunSyncEnqueue:                  appcore.RunSyncEnqueue,
		RunSyncFlushWorker:              appcore.RunSyncFlushWorker,
		RunSyncFlush:                    appcore.RunSyncFlush,
		RunSyncStatus:                   appcore.RunSyncStatus,
		RunSyncList:                     appcore.RunSyncList,
		RunSyncForget:                   appcore.RunSyncForget,
	})

	if err := app.Run(os.Args); err != nil {
		if errors.Is(err, appcore.ErrAuthHandled) {
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runReviewSimple(c *cli.Context) error {
	opts, err := reviewopts.BuildFromContext(c, false)
	if err != nil {
		return err
	}
	return appcore.RunReviewWithOptions(opts)
}

func runReviewDebug(c *cli.Context) error {
	opts, err := reviewopts.BuildFromContext(c, true)
	if err != nil {
		return err
	}
	return appcore.RunReviewWithOptions(opts)
}
