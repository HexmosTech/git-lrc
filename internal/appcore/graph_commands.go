package appcore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/HexmosTech/blastradius/client"
	"github.com/HexmosTech/git-lrc/internal/graphengine"
	"github.com/urfave/cli/v2"
)

// runGraphInstall downloads and installs the codebase-memory-mcp engine
// binary into ~/.lrc/bin. It only ever writes that one binary - no PATH
// edits, no agent-config changes (and it never invokes the vendor's own
// `install` subcommand, which does modify agent configs).
func runGraphInstall(c *cli.Context) error {
	fmt.Printf("Installing %s %s into lrc's bin directory...\n", graphengine.BinaryName, graphengine.PinnedVersion)

	lastPercent := -1
	progress := func(downloaded, total int64) {
		if total <= 0 {
			return
		}
		percent := int(downloaded * 100 / total)
		// Only print every 10% so non-interactive logs stay readable.
		if percent/10 > lastPercent/10 {
			lastPercent = percent
			fmt.Printf("  downloading... %d%%\n", percent)
		}
	}

	res, err := graphengine.Install(graphengine.InstallOptions{
		Force:    c.Bool("force"),
		Progress: progress,
	})
	if err != nil {
		return fmt.Errorf("graph engine install failed: %w", err)
	}
	if res.Skipped {
		fmt.Printf("Already installed: %s (version %s)\n", res.Path, res.Version)
		return nil
	}
	fmt.Printf("Installed %s (version %s)\n", res.Path, res.Version)
	return nil
}

func runGraphStatus(c *cli.Context) error {
	binPath, err := graphengine.Resolve()
	if errors.Is(err, graphengine.ErrNotInstalled) {
		fmt.Println("Graph engine: not installed")
		fmt.Println("Run `lrc graph install` to enable blast-radius scoring.")
		return nil
	}
	if err != nil {
		return err
	}

	version, verErr := graphengine.InstalledVersion(binPath)
	if verErr != nil {
		fmt.Printf("Graph engine: %s (version check failed: %v)\n", binPath, verErr)
		return nil
	}
	fmt.Printf("Graph engine: %s (version %s)\n", binPath, version)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	engineClient := &client.Client{Binary: binPath, Project: "-"}
	projects, listErr := engineClient.ListProjects(ctx)
	if listErr != nil {
		fmt.Printf("Indexed projects: unavailable (%v)\n", listErr)
		return nil
	}
	if len(projects) == 0 {
		fmt.Println("Indexed projects: none (the first `lrc review` in a repo indexes it automatically)")
		return nil
	}
	fmt.Printf("Indexed projects (%d):\n", len(projects))
	for _, p := range projects {
		fmt.Printf("  %s  (%d nodes, %d edges)  %s\n", p.Name, p.Nodes, p.Edges, p.RootPath)
	}
	return nil
}

func runGraphUninstall(c *cli.Context) error {
	if err := graphengine.Uninstall(); err != nil {
		return err
	}
	fmt.Println("Removed the lrc-managed graph engine binary (user installs on PATH are untouched).")
	return nil
}

func RunGraphInstall(c *cli.Context) error   { return runGraphInstall(c) }
func RunGraphStatus(c *cli.Context) error    { return runGraphStatus(c) }
func RunGraphUninstall(c *cli.Context) error { return runGraphUninstall(c) }
