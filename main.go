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
	"github.com/HexmosTech/git-lrc/internal/selfupdate"
	"github.com/urfave/cli/v2"
)

const appVersion = "v0.3.4"

var (
	version    = appVersion
	buildTime  = "unknown"
	gitCommit  = "unknown"
	reviewMode = "prod"
)

var baseFlags = []cli.Flag{
	&cli.StringFlag{Name: "repo-name", Usage: "repository name (defaults to current directory basename)", EnvVars: []string{"LRC_REPO_NAME"}},
	&cli.BoolFlag{Name: "staged", Usage: "use staged changes instead of working tree", EnvVars: []string{"LRC_STAGED"}},
	&cli.StringFlag{Name: "range", Usage: "git range for staged/working diff override (e.g., HEAD~1..HEAD)", EnvVars: []string{"LRC_RANGE"}},
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
	&cli.BoolFlag{Name: "skip", Usage: "mark review as skipped and write attestation without contacting the API", EnvVars: []string{"LRC_SKIP"}},
	&cli.BoolFlag{Name: "force", Usage: "force rerun by removing existing attestation/hash for current tree", EnvVars: []string{"LRC_FORCE"}},
	&cli.BoolFlag{Name: "vouch", Usage: "vouch for changes manually without running AI review (records attestation with coverage stats from prior iterations)", EnvVars: []string{"LRC_VOUCH"}},
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
	appui.SetBuildInfo(version, buildTime, gitCommit)
	appcore.Configure(version, reviewMode)

	app := cmdapp.BuildApp(version, buildTime, gitCommit, baseFlags, debugFlags, cmdapp.Handlers{
		RunReviewSimple:       runReviewSimple,
		RunReviewDebug:        runReviewDebug,
		RunUninstall:          appcore.RunUninstall,
		RunHooksInstall:       appcore.RunHooksInstall,
		RunHooksUninstall:     appcore.RunHooksUninstall,
		RunHooksEnable:        appcore.RunHooksEnable,
		RunHooksDisable:       appcore.RunHooksDisable,
		RunHooksStatus:        appcore.RunHooksStatus,
		RunSelfUpdate:         selfupdate.RunSelfUpdate,
		RunReviewCleanup:      func(c *cli.Context) error { return reviewdb.RunReviewDBCleanup(c.Bool("verbose")) },
		RunAttestationTrailer: appcore.RunAttestationTrailer,
		RunSetup:              appui.RunSetup,
		RunUI:                 appui.RunUI,
		RunUsageInspect:       appcore.RunUsageInspect,
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
