package cmd

import (
	"fmt"
	"io"

	"github.com/urfave/cli/v2"
)

func init() {
	// Override the default HelpPrinter to write directly to the provided
	// io.Writer using fmt.Fprint.  This avoids any OS-specific pager or
	// man-page lookup and ensures --help works identically on Windows,
	// macOS, and Linux.
	// HelpPrinterCustom renders the help template directly into w via
	// text/template.Execute — no external pager, man-page, or shell command
	// is invoked, which is what makes this safe on Windows/PowerShell.
	cli.HelpPrinter = func(w io.Writer, templ string, data interface{}) {
		cli.HelpPrinterCustom(w, templ, data, nil)
	}
}

// Handlers contains injected command actions so CLI wiring can live outside main.
type Handlers struct {
	RunReviewSimple       cli.ActionFunc
	RunReviewDebug        cli.ActionFunc
	RunUninstall          cli.ActionFunc
	RunHooksInstall       cli.ActionFunc
	RunHooksUninstall     cli.ActionFunc
	RunHooksEnable        cli.ActionFunc
	RunHooksDisable       cli.ActionFunc
	RunHooksStatus        cli.ActionFunc
	RunSelfUpdate         cli.ActionFunc
	RunReviewCleanup      cli.ActionFunc
	RunAttestationTrailer cli.ActionFunc
	RunSetup              cli.ActionFunc
	RunUI                 cli.ActionFunc
	RunUsageInspect       cli.ActionFunc
}

// BuildApp constructs the full CLI app with all command wiring.
func BuildApp(version, buildTime, gitCommit string, baseFlags, debugFlags []cli.Flag, h Handlers) *cli.App {
	return &cli.App{
		Name:    "lrc",
		Usage:   "LiveReview CLI - submit local diffs for AI review",
		Version: version,
		Flags:   baseFlags,
		Commands: []*cli.Command{
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
				Name:    "review",
				Aliases: []string{"r"},
				Usage:   "Run a review with sensible defaults",
				Flags:   baseFlags,
				Action:  h.RunReviewSimple,
			},
			{
				Name:   "review-debug",
				Usage:  "Run a review with advanced debug options",
				Flags:  append(baseFlags, debugFlags...),
				Action: h.RunReviewDebug,
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
						Action: h.RunHooksInstall,
					},
					{
						Name:  "uninstall",
						Usage: "Remove LiveReview hook dispatchers and managed scripts",
						Flags: []cli.Flag{
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
						Name:   "enable",
						Usage:  "Enable LiveReview hooks for the current repository",
						Action: h.RunHooksEnable,
					},
					{
						Name:   "disable",
						Usage:  "Disable LiveReview hooks for the current repository",
						Action: h.RunHooksDisable,
					},
					{
						Name:   "status",
						Usage:  "Show LiveReview hook status for the current repository",
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
		},
		Action: h.RunReviewSimple,
	}
}
