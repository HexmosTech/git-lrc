package appcore

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/HexmosTech/git-lrc/internal/graphengine"
	"github.com/HexmosTech/git-lrc/storage"
	"github.com/urfave/cli/v2"
)

type uninstallMode string

const (
	uninstallModeMinimal  uninstallMode = "minimal"
	uninstallModeStandard uninstallMode = "standard"
	uninstallModeDeep     uninstallMode = "deep"
)

func runUninstall(c *cli.Context) error {
	mode, err := parseUninstallMode(c.String("mode"))
	if err != nil {
		return err
	}

	if c.Bool("remove-config") && c.Bool("keep-config") {
		return fmt.Errorf("cannot use --remove-config and --keep-config together")
	}
	if c.Bool("remove-shell-integration") && c.Bool("keep-shell-integration") {
		return fmt.Errorf("cannot use --remove-shell-integration and --keep-shell-integration together")
	}

	dryRun := c.Bool("dry-run")
	nonInteractive := c.Bool("yes")
	binariesOnly := c.Bool("binaries-only")
	removeHooks := !c.Bool("keep-hooks")

	if binariesOnly {
		removeHooks = false
	}

	removeShell, err := resolveShellCleanupDecision(c, mode, nonInteractive, binariesOnly)
	if err != nil {
		return err
	}

	removeConfig, err := resolveConfigCleanupDecision(c, nonInteractive, binariesOnly)
	if err != nil {
		return err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to determine home directory: %w", err)
	}

	binaries, shellArtifacts := detectUninstallArtifacts(homeDir)
	// The lrc-managed graph engine binary (~/.lrc/bin) counts as an lrc
	// binary; a user-installed codebase-memory-mcp elsewhere on PATH is
	// never touched.
	if enginePath, engineErr := graphengine.InstalledBinaryPath(); engineErr == nil {
		binaries = append(binaries, enginePath)
	}
	configFile := filepath.Join(homeDir, ".lrc.toml")

	fmt.Printf("Running uninstall (mode: %s)\n", mode)
	if dryRun {
		fmt.Println("Dry-run mode: no files will be modified.")
	}

	if removeHooks {
		if dryRun {
			fmt.Println("Would run: lrc hooks uninstall")
		} else {
			if err := runHooksUninstall(c); err != nil {
				fmt.Printf("⚠️  Warning: hooks uninstall failed: %v\n", err)
			}
		}
	} else {
		fmt.Println("Keeping hooks integration.")
	}

	if removeShell {
		if runtime.GOOS == "windows" {
			if err := cleanupWindowsShellArtifacts(shellArtifacts, dryRun); err != nil {
				fmt.Printf("⚠️  Warning: shell integration cleanup had issues: %v\n", err)
			}
		} else {
			if err := cleanupShellIntegration(homeDir, shellArtifacts.EnvFile, shellArtifacts.EnvDir, dryRun); err != nil {
				fmt.Printf("⚠️  Warning: shell integration cleanup had issues: %v\n", err)
			}
		}
	} else {
		if runtime.GOOS == "windows" {
			fmt.Println("Keeping shell integration (WindowsApps shims).")
		} else {
			fmt.Println("Keeping shell integration (~/.lrc/env and startup file entries).")
		}
	}

	if removeConfig {
		removed, err := storage.RemoveFileIfExists(configFile, dryRun)
		if err != nil {
			fmt.Printf("⚠️  Warning: failed to remove config %s: %v\n", configFile, err)
		} else if removed {
			printRemovedOrWouldRemove(configFile, dryRun)
		}
	} else {
		fmt.Println("Keeping config (~/.lrc.toml).")
	}

	for _, path := range binaries {
		removed, err := storage.RemoveFileIfExists(path, dryRun)
		if err != nil {
			fmt.Printf("⚠️  Warning: failed to remove binary %s: %v\n", path, err)
			if runtime.GOOS == "windows" {
				fmt.Printf("   On Windows, remove it manually after closing shells using: Remove-Item -Force \"%s\"\n", path)
			}
			continue
		}
		if removed {
			printRemovedOrWouldRemove(path, dryRun)
		}
	}

	fmt.Println("Uninstall flow complete.")
	return nil
}

type shellArtifacts struct {
	EnvDir           string
	EnvFile          string
	WindowsAppsShims []string
}

func detectUninstallArtifacts(homeDir string) ([]string, shellArtifacts) {
	if runtime.GOOS == "windows" {
		localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		if localAppData == "" {
			localAppData = filepath.Join(homeDir, "AppData", "Local")
		}

		installDir := filepath.Join(localAppData, "Programs", "lrc")
		windowsApps := filepath.Join(localAppData, "Microsoft", "WindowsApps")

		binaries := []string{
			filepath.Join(installDir, "lrc.exe"),
			filepath.Join(installDir, "git-lrc.exe"),
		}

		artifacts := shellArtifacts{
			WindowsAppsShims: []string{
				filepath.Join(windowsApps, "lrc.cmd"),
				filepath.Join(windowsApps, "git-lrc.cmd"),
				filepath.Join(windowsApps, "git-lrc.exe"),
			},
		}

		return binaries, artifacts
	}

	installDir := filepath.Join(homeDir, ".local", "bin")
	binaries := []string{
		filepath.Join(installDir, "lrc"),
		filepath.Join(installDir, "git-lrc"),
	}

	envDir := filepath.Join(homeDir, ".lrc")
	artifacts := shellArtifacts{
		EnvDir:  envDir,
		EnvFile: filepath.Join(envDir, "env"),
	}

	return binaries, artifacts
}

func parseUninstallMode(raw string) (uninstallMode, error) {
	mode := uninstallMode(strings.ToLower(strings.TrimSpace(raw)))
	switch mode {
	case uninstallModeMinimal, uninstallModeStandard, uninstallModeDeep:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid --mode value %q (expected: minimal, standard, deep)", raw)
	}
}

func resolveShellCleanupDecision(c *cli.Context, mode uninstallMode, nonInteractive, binariesOnly bool) (bool, error) {
	if binariesOnly {
		return false, nil
	}
	if c.Bool("remove-shell-integration") {
		return true, nil
	}
	if c.Bool("keep-shell-integration") {
		return false, nil
	}

	defaultRemoveShell := mode == uninstallModeStandard || mode == uninstallModeDeep
	if nonInteractive {
		return defaultRemoveShell, nil
	}

	prompt := "Remove shell integration (~/.lrc/env and installer-added startup lines)?"
	if defaultRemoveShell {
		return promptYesNo(prompt, true)
	}
	return promptYesNo(prompt, false)
}

func resolveConfigCleanupDecision(c *cli.Context, nonInteractive, binariesOnly bool) (bool, error) {
	if binariesOnly {
		return false, nil
	}
	if c.Bool("remove-config") {
		return true, nil
	}
	if c.Bool("keep-config") {
		return false, nil
	}

	if nonInteractive {
		return false, nil
	}

	return promptYesNo("Remove ~/.lrc.toml (contains API credentials)?", false)
}

func promptYesNo(question string, defaultYes bool) (bool, error) {
	if !isInteractiveStdin() {
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
		return false, err
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

func isInteractiveStdin() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func cleanupShellIntegration(homeDir, envFile, envDir string, dryRun bool) error {
	rcFiles := []string{
		filepath.Join(homeDir, ".profile"),
		filepath.Join(homeDir, ".bashrc"),
		filepath.Join(homeDir, ".bash_profile"),
		filepath.Join(homeDir, ".zshenv"),
		filepath.Join(homeDir, ".zshrc"),
	}

	for _, rc := range rcFiles {
		changed, err := storage.RemoveLRCInstallerShellSourceLines(rc, dryRun)
		if err != nil {
			fmt.Printf("⚠️  Warning: failed updating %s: %v\n", rc, err)
			continue
		}
		if changed {
			if dryRun {
				fmt.Printf("Would update %s\n", rc)
			} else {
				fmt.Printf("Removed lrc startup lines from %s\n", rc)
			}
		}
	}

	fishConf := filepath.Join(homeDir, ".config", "fish", "conf.d", "lrc.fish")
	if fishRemoved, err := storage.RemoveManagedFishLRCConfig(fishConf, dryRun); err != nil {
		fmt.Printf("⚠️  Warning: failed updating fish integration %s: %v\n", fishConf, err)
	} else if fishRemoved {
		if dryRun {
			fmt.Printf("Would remove %s\n", fishConf)
		} else {
			fmt.Printf("Removed %s\n", fishConf)
		}
	}

	envRemoved, err := storage.RemoveFileIfExists(envFile, dryRun)
	if err != nil {
		fmt.Printf("⚠️  Warning: failed to remove %s: %v\n", envFile, err)
	} else if envRemoved {
		printRemovedOrWouldRemove(envFile, dryRun)
	}

	if dryRun {
		return nil
	}

	if dirRemoved, err := storage.RemoveDirIfEmptyIfExists(envDir, false); err == nil && dirRemoved {
		fmt.Printf("Removed %s\n", envDir)
	}

	return nil
}

func cleanupWindowsShellArtifacts(artifacts shellArtifacts, dryRun bool) error {
	for _, shim := range artifacts.WindowsAppsShims {
		removed, err := storage.RemoveFileIfExists(shim, dryRun)
		if err != nil {
			fmt.Printf("⚠️  Warning: failed to remove WindowsApps shim %s: %v\n", shim, err)
			continue
		}
		if removed {
			printRemovedOrWouldRemove(shim, dryRun)
		}
	}

	return nil
}

func printRemovedOrWouldRemove(path string, dryRun bool) {
	if dryRun {
		fmt.Printf("Would remove %s\n", path)
		return
	}
	fmt.Printf("Removed %s\n", path)
}
