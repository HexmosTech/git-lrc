package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

// reviewCommandDescription documents the most common diff sources, including
// how to review a feature branch before merging (PR-style review).
const reviewCommandDescription = `By default, reviews staged changes (git diff --staged).

Common diff sources:

   lrc review                          # staged changes (default)
   lrc review --staged=false           # working tree changes (unstaged)
   lrc review --commit HEAD            # the most recent commit
   lrc review --commit HEAD~3..HEAD    # the last 3 commits

Reviewing a branch before merging (PR-style review):

   lrc review --range main...my-feature

   Three dots (...) compare against the merge base: you get exactly the
   changes introduced by my-feature since it diverged from main, even if
   main has moved on since. This is what GitHub/GitLab show in a PR diff,
   and is almost always what you want.

   Two dots (main..my-feature) is a direct diff between the two tips,
   which also includes any commits already on main that my-feature
   hasn't picked up yet.

--range and --commit (with a "..." or ".." range) are read-only: they
review a diff between existing refs, open a browsable HTML report, and
do not write a commit attestation or offer to commit/push.`

// Handlers contains injected command actions so CLI wiring can live outside main.
type Handlers struct {
	RunReviewSimple                 cli.ActionFunc
	RunReviewDebug                  cli.ActionFunc
	RunEnsure                       cli.ActionFunc
	RunUninstall                    cli.ActionFunc
	RunHooksInstall                 cli.ActionFunc
	RunHooksUninstall               cli.ActionFunc
	RunHooksEnable                  cli.ActionFunc
	RunHooksDisable                 cli.ActionFunc
	RunHooksStatus                  cli.ActionFunc
	RunSelfUpdate                   cli.ActionFunc
	RunReviewCleanup                cli.ActionFunc
	RunAttestationTrailer           cli.ActionFunc
	RunSetup                        cli.ActionFunc
	RunUI                           cli.ActionFunc
	RunUsageInspect                 cli.ActionFunc
	RunInternalClaudePreToolUse     cli.ActionFunc
	RunInternalClaudeRunCommit      cli.ActionFunc
	RunInternalClaudeSetupStart     cli.ActionFunc
	RunInternalClaudeSetupWorker    cli.ActionFunc
	RunInternalClaudeSetupSubmitKey cli.ActionFunc
	RunInternalClaudeSetupStatus    cli.ActionFunc
	RunRemoveAttestation            cli.ActionFunc
	RunConfigInit                   cli.ActionFunc
	RunConfigCheck                  cli.ActionFunc
	RunConfigPreview                cli.ActionFunc
	RunQuery                        cli.ActionFunc
	RunQueryAdd                     cli.ActionFunc
	RunQueryList                    cli.ActionFunc
	RunQueryView                    cli.ActionFunc
	RunQueryDelete                  cli.ActionFunc
}

// BuildApp constructs the full CLI app with all command wiring.
func BuildApp(version, buildTime, gitCommit, reviewMode string, baseFlags, debugFlags []cli.Flag, h Handlers) *cli.App {
	return &cli.App{
		Name:    "lrc",
		Usage:   "LiveReview CLI - submit local diffs for AI review",
		Version: version,
		Flags:   baseFlags,
		Commands: []*cli.Command{
			{
				Name:  "ensure",
				Usage: "Check whether LiveReview auth and at least one AI connector are ready for review",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "api-url",
						Aliases: []string{"base-url"},
						Usage:   "override LiveReview API base URL for readiness check",
					},
				},
				Action: h.RunEnsure,
			},
			{
				Name:  "uninstall",
				Usage: "Uninstall lrc from your user environment",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "mode",
						Value: "standard",
						Usage: "uninstall mode: minimal, standard, deep",
					},
					&cli.BoolFlag{
						Name:  "yes",
						Usage: "run non-interactively using defaults and explicit flags",
					},
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "show what would be removed without making changes",
					},
					&cli.BoolFlag{
						Name:  "binaries-only",
						Usage: "remove only lrc and git-lrc binaries",
					},
					&cli.BoolFlag{
						Name:  "keep-hooks",
						Usage: "keep hook integration (skip 'lrc hooks uninstall')",
					},
					&cli.BoolFlag{
						Name:  "remove-config",
						Usage: "remove ~/.lrc.toml",
					},
					&cli.BoolFlag{
						Name:  "keep-config",
						Usage: "keep ~/.lrc.toml",
					},
					&cli.BoolFlag{
						Name:  "remove-shell-integration",
						Usage: "remove ~/.lrc/env and installer-added shell startup lines",
					},
					&cli.BoolFlag{
						Name:  "keep-shell-integration",
						Usage: "keep ~/.lrc/env and shell startup lines",
					},
				},
				Action: h.RunUninstall,
			},
			{
				Name:        "review",
				Aliases:     []string{"r"},
				Usage:       "Run a review with sensible defaults",
				Description: reviewCommandDescription,
				Flags:       baseFlags,
				Action:      h.RunReviewSimple,
			},
			{
				Name:        "review-debug",
				Usage:       "Run a review with advanced debug options",
				Description: reviewCommandDescription,
				Flags:       append(baseFlags, debugFlags...),
				Action:      h.RunReviewDebug,
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
								Name:  "surface",
								Usage: "hook surface to install: all, git, or claude",
							},
							&cli.StringFlag{
								Name:  "path",
								Usage: "custom hooksPath (defaults to core.hooksPath or ~/.git-hooks)",
							},
							&cli.BoolFlag{
								Name:  "local",
								Usage: "install into the current repo hooks path (respects core.hooksPath)",
							},
						},
						Action: h.RunHooksInstall,
					},
					{
						Name:  "uninstall",
						Usage: "Remove LiveReview hook dispatchers and managed scripts",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "surface",
								Usage: "hook surface to uninstall: all, git, or claude",
							},
							&cli.BoolFlag{
								Name:  "local",
								Usage: "uninstall from the current repo hooks path",
							},
							&cli.StringFlag{
								Name:  "path",
								Usage: "target a specific hooksPath directory for uninstall",
							},
						},
						Action: h.RunHooksUninstall,
					},
					{
						Name:  "enable",
						Usage: "Enable LiveReview hooks for the current repository",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "surface",
								Usage: "hook surface to target: all, git, or claude",
							},
						},
						Action: h.RunHooksEnable,
					},
					{
						Name:  "disable",
						Usage: "Disable LiveReview hooks for the current repository",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "surface",
								Usage: "hook surface to target: all, git, or claude",
							},
						},
						Action: h.RunHooksDisable,
					},
					{
						Name:  "status",
						Usage: "Show LiveReview hook status for the current repository",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "surface",
								Usage: "hook surface to target: all, git, or claude",
							},
						},
						Action: h.RunHooksStatus,
					},
				},
			},
			{
				Name:   "install-hooks",
				Usage:  "Install LiveReview hooks (deprecated; use 'lrc hooks install')",
				Hidden: true,
				Action: h.RunHooksInstall,
			},
			{
				Name:   "uninstall-hooks",
				Usage:  "Uninstall LiveReview hooks (deprecated; use 'lrc hooks uninstall')",
				Hidden: true,
				Action: h.RunHooksUninstall,
			},
			{
				Name:  "version",
				Usage: "Show version information",
				Action: func(c *cli.Context) error {
					fmt.Printf("lrc version %s\n", version)
					fmt.Printf("  Build time: %s\n", buildTime)
					fmt.Printf("  Git commit: %s\n", gitCommit)
					fmt.Printf("  Review mode: %s\n", reviewMode)
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
						Usage: "Force recovery by terminating another active lrc self-update process, then continue update",
					},
				},
				Action: h.RunSelfUpdate,
			},
			{
				Name:  "usage",
				Usage: "Inspect plan and quota usage",
				Subcommands: []*cli.Command{
					{
						Name:  "inspect",
						Usage: "Fetch and display current quota envelope for selected org",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "api-url", Usage: "override LiveReview API base URL"},
							&cli.StringFlag{Name: "output", Value: "pretty", Usage: "output format: pretty or json"},
							&cli.BoolFlag{Name: "verbose", Usage: "enable verbose output"},
						},
						Action: h.RunUsageInspect,
					},
				},
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
				Action: h.RunReviewCleanup,
			},
			{
				Name:   "attestation-trailer",
				Usage:  "Output the commit trailer for the current attestation (called by commit-msg hook)",
				Hidden: true,
				Action: h.RunAttestationTrailer,
			},
			{
				Name:  "setup",
				Usage: "Guided onboarding — authenticate with Hexmos and configure LiveReview + AI",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "api-url",
						Aliases: []string{"base-url"},
						Usage:   "override LiveReview API base URL for setup",
					},
					&cli.BoolFlag{
						Name:  "yes",
						Usage: "run non-interactively; requires explicit --keep-api-url or --replace-api-url when config already exists",
					},
					&cli.BoolFlag{
						Name:  "keep-api-url",
						Usage: "when config exists, preserve existing api_url",
					},
					&cli.BoolFlag{
						Name:  "replace-api-url",
						Usage: "when config exists, replace api_url with setup target URL",
					},
				},
				Action: h.RunSetup,
			},
			{
				Name:   "ui",
				Usage:  "Open local web UI to manage your git-lrc",
				Action: h.RunUI,
			},
			{
				Name:   "remove-attestation",
				Usage:  "Remove the attestation for the current staged tree",
				Action: h.RunRemoveAttestation,
			},
			{
				Name:  "config",
				Usage: "Manage .lrc/ repository rules configuration",
				Subcommands: []*cli.Command{
					{
						Name:   "init",
						Usage:  "Scaffold the .lrc/ directory structure",
						Action: h.RunConfigInit,
					},
					{
						Name:   "check",
						Usage:  "Validate .lrc/ rules and structure (offline)",
						Action: h.RunConfigCheck,
					},
					{
						Name:   "preview",
						Usage:  "Show the rules bundle LiveReview will use (offline)",
						Action: h.RunConfigPreview,
					},
				},
			},
			{
				Name:  "query",
				Usage: "Query LiveReview history with SQL or a saved alias (e.g. 'lrc query stats')",
				Description: `Builds an in-memory SQLite table of this repo's review history (parsed
from the 'LiveReview Pre-Commit Check' commit trailers) and runs SQL — or a
saved alias — against it. Output as a table or, with --json, machine-readable.

TABLE: review_log (one row per commit)
   hash         TEXT     full commit hash
   short_hash   TEXT     abbreviated hash
   author       TEXT     commit author name
   email        TEXT     commit author email
   date         TEXT     author date, ISO-8601 (sortable, e.g. 2026-06-17T10:30:00Z)
   branch       TEXT     branch the query ran from
   subject      TEXT     commit subject (first line)
   action       TEXT     'reviewed' | 'vouched' | 'skipped' | 'none'
   iterations   INTEGER  review iterations (0 if none)
   coverage     INTEGER  review coverage percent 0-100 (0 if none)

ALIASES: built-in (stats, by-author, recent) plus your own. Manage them with
'lrc query add|list|view|delete'. User aliases are saved in ~/.lrc/queries.toml:

   [queries]
   skipped = "SELECT date, subject FROM review_log WHERE action='skipped'"
   my-cov  = "SELECT ROUND(AVG(coverage),1) FROM review_log WHERE action='reviewed'"

EXAMPLES
   lrc query stats                        # run a built-in alias
   lrc query stats --json                 # same data, as JSON
   lrc query list                         # show all aliases + a preview
   lrc query view stats                   # show an alias's full SQL

   # Was a specific commit reviewed? (incident forensics)
   lrc query "SELECT short_hash, action, iterations, coverage FROM review_log WHERE hash LIKE 'a1b2c3%'"

   # Per-author review effort
   lrc query "SELECT author, COUNT(*) AS commits, SUM(action='reviewed') AS reviewed FROM review_log GROUP BY author ORDER BY commits DESC"

   # Save and reuse your own query
   lrc query add skipped "SELECT date, subject FROM review_log WHERE action='skipped'"
   lrc query skipped --json

   # Bound the scan on huge repos (Linux kernel = ~1.5M commits)
   lrc query stats --from "2024-01-01" --to "2024-12-31"
   lrc query stats --range main...feature   # just this PR's commits`,
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "json", Usage: "output machine-readable JSON"},
					&cli.StringFlag{Name: "from", Usage: "only scan commits since this git date (e.g. 2024-01-01, '2 weeks ago') — bounds large repos"},
					&cli.StringFlag{Name: "to", Usage: "only scan commits until this git date"},
					&cli.StringFlag{Name: "range", Usage: "only scan a ref range, e.g. main...feature (per-PR stats)"},
				},
				Action: h.RunQuery,
				Subcommands: []*cli.Command{
					{
						Name:      "add",
						Usage:     "Save a query alias: lrc query add <name> \"<sql>\"",
						ArgsUsage: "<name> \"<sql>\"",
						Action:    h.RunQueryAdd,
					},
					{
						Name:   "list",
						Usage:  "List saved and built-in query aliases",
						Action: h.RunQueryList,
					},
					{
						Name:   "view",
						Usage:  "Print the SQL behind an alias",
						Action: h.RunQueryView,
					},
					{
						Name:   "delete",
						Usage:  "Delete a saved alias",
						Action: h.RunQueryDelete,
					},
				},
			},
			{
				Name:   "internal",
				Usage:  "Internal back-office commands (not for direct use)",
				Hidden: true,
				Subcommands: []*cli.Command{
					{
						Name:   "claude",
						Usage:  "Claude Code integration commands",
						Hidden: true,
						Subcommands: []*cli.Command{
							{
								Name:   "pre-tool-use",
								Usage:  "PreToolUse hook handler — intercepts git commits for the LiveReview gate",
								Hidden: true,
								Action: h.RunInternalClaudePreToolUse,
							},
							{
								Name:   "run-commit",
								Usage:  "Runs lrc review then the original git commit (invoked by pre-tool-use rewrite)",
								Hidden: true,
								Flags: []cli.Flag{
									&cli.StringFlag{
										Name:     "encoded",
										Usage:    "base64-encoded JSON payload from pre-tool-use",
										Required: true,
									},
								},
								Action: h.RunInternalClaudeRunCommit,
							},
							{
								Name:   "setup",
								Usage:  "Manage the lrc setup session (replaces setup-session.py)",
								Hidden: true,
								Subcommands: []*cli.Command{
									{
										Name:   "start",
										Usage:  "Start a background lrc setup session",
										Hidden: true,
										Action: h.RunInternalClaudeSetupStart,
									},
									{
										Name:   "worker",
										Usage:  "Background worker — runs lrc setup and manages the key handoff",
										Hidden: true,
										Action: h.RunInternalClaudeSetupWorker,
									},
									{
										Name:   "submit-key",
										Usage:  "Submit the Gemini API key to a running setup session",
										Hidden: true,
										Flags: []cli.Flag{
											&cli.StringFlag{
												Name:     "key",
												Usage:    "Gemini API key to submit",
												Required: true,
											},
										},
										Action: h.RunInternalClaudeSetupSubmitKey,
									},
									{
										Name:   "status",
										Usage:  "Print the current setup session status",
										Hidden: true,
										Action: h.RunInternalClaudeSetupStatus,
									},
								},
							},
						},
					},
				},
			},
		},
		Action: h.RunReviewSimple,
	}
}
