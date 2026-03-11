package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	hooksvc "github.com/HexmosTech/git-lrc/hooks"
	"github.com/urfave/cli/v2"
)

type hooksMeta = hooksvc.Meta

func defaultGlobalHooksPath() (string, error) {
	return hooksvc.DefaultGlobalHooksPath(defaultGlobalHooksDir)
}

func currentHooksPath() (string, error) {
	return hooksvc.CurrentHooksPath()
}

func currentLocalHooksPath(repoRoot string) (string, error) {
	return hooksvc.CurrentLocalHooksPath(repoRoot)
}

func resolveRepoHooksPath(repoRoot string) (string, error) {
	return hooksvc.ResolveRepoHooksPath(repoRoot)
}

func setGlobalHooksPath(path string) error {
	return hooksvc.SetGlobalHooksPath(path)
}

func unsetGlobalHooksPath() error {
	return hooksvc.UnsetGlobalHooksPath()
}

func hooksMetaPath(hooksPath string) string {
	return hooksvc.MetaPath(hooksPath, hooksMetaFilename)
}

func writeHooksMeta(hooksPath string, meta hooksMeta) {
	hooksvc.WriteMeta(hooksPath, hooksMetaFilename, meta)
}

func readHooksMeta(hooksPath string) (*hooksMeta, error) {
	return hooksvc.ReadMeta(hooksPath, hooksMetaFilename)
}

func removeHooksMeta(hooksPath string) error {
	return hooksvc.RemoveMeta(hooksPath, hooksMetaFilename)
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
	var prevGlobalPath string
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
		prevGlobalPath, _ = currentHooksPath()
		currentPath := prevGlobalPath
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
		writeHooksMeta(absHooksPath, hooksMeta{Path: absHooksPath, PrevPath: prevGlobalPath, SetByLRC: setConfig})
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
	requestedPath := strings.TrimSpace(c.String("path"))
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
		if requestedPath != "" {
			hooksPath = requestedPath
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
	}

	absHooksPath, err := filepath.Abs(hooksPath)
	if err != nil {
		return fmt.Errorf("failed to resolve hooks path: %w", err)
	}

	currentGlobalPath, _ := currentHooksPath()

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
	_ = os.RemoveAll(filepath.Join(absHooksPath, ".lrc_backups"))
	if !localUninstall {
		_ = removeHooksMeta(absHooksPath)
	}

	if !localUninstall {
		restoredHooksPath := false

		if meta != nil && meta.SetByLRC {
			if meta.PrevPath == "" {
				if err := unsetGlobalHooksPath(); err != nil {
					fmt.Printf("⚠️  Warning: failed to unset core.hooksPath: %v\n", err)
				} else {
					fmt.Println("✅ Unset core.hooksPath (was set by lrc)")
					restoredHooksPath = true
				}
			} else {
				if err := setGlobalHooksPath(meta.PrevPath); err != nil {
					fmt.Printf("⚠️  Warning: failed to restore core.hooksPath to %s: %v\n", meta.PrevPath, err)
				} else {
					fmt.Printf("✅ Restored core.hooksPath to %s\n", meta.PrevPath)
					restoredHooksPath = true
				}
			}
		} else if meta == nil && currentGlobalPath != "" && pathsEqual(currentGlobalPath, absHooksPath) {
			if err := unsetGlobalHooksPath(); err != nil {
				fmt.Printf("⚠️  Warning: failed to unset core.hooksPath: %v\n", err)
			} else {
				fmt.Println("✅ Unset core.hooksPath (was pointing to uninstalled hooks dir)")
				restoredHooksPath = true
			}
		}

		postPath, _ := currentHooksPath()
		if postPath != "" && pathsEqual(postPath, absHooksPath) && !restoredHooksPath {
			fmt.Printf("⚠️  Warning: core.hooksPath is still set to %s\n", postPath)
			fmt.Println("   This may prevent repo-local hooks from working.")
			fmt.Println("   Run: git config --global --unset core.hooksPath")
		}
	}

	if !localUninstall {
		cleanEmptyHooksDir(absHooksPath)
	}

	if removed > 0 {
		fmt.Printf("✅ Removed LiveReview sections from %d hook(s) at %s\n", removed, absHooksPath)
	} else {
		fmt.Printf("ℹ️  No LiveReview sections found in %s\n", absHooksPath)
	}

	return nil
}

// pathsEqual compares two filesystem paths robustly, resolving symlinks
func pathsEqual(a, b string) bool {
	return hooksvc.PathsEqual(a, b)
}

// cleanEmptyHooksDir removes the hooks directory if it's empty or contains only lrc artifacts
func cleanEmptyHooksDir(dir string) {
	hooksvc.CleanEmptyHooksDir(dir)
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
	return hooksvc.HookHasManagedSection(path, lrcMarkerBegin)
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
	return hooksvc.InstallHook(hookPath, lrcSection, hookName, backupDir, lrcMarkerBegin, lrcMarkerEnd, force)
}

// uninstallHook removes lrc-managed section from a hook file
func uninstallHook(hookPath, hookName string) error {
	return hooksvc.UninstallHook(hookPath, hookName, lrcMarkerBegin, lrcMarkerEnd)
}

// installEditorWrapper sets core.editor to an lrc-managed wrapper that injects
// the precommit-provided message when available and falls back to the user's editor.
func installEditorWrapper(gitDir string) error {
	repoRoot := filepath.Dir(gitDir)
	scriptPath := filepath.Join(gitDir, editorWrapperScript)
	backupPath := filepath.Join(gitDir, editorBackupFile)

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
	return hooksvc.ReplaceManagedSection(content, newSection, lrcMarkerBegin, lrcMarkerEnd)
}

// removeLrcSection removes the lrc-managed section from hook content
func removeLrcSection(content string) string {
	return hooksvc.RemoveManagedSection(content, lrcMarkerBegin, lrcMarkerEnd)
}

// generatePreCommitHook generates the pre-commit hook script
func generatePreCommitHook() string {
	return hooksvc.GeneratePreCommitHook(hooksvc.TemplateConfig{
		MarkerBegin: lrcMarkerBegin,
		MarkerEnd:   lrcMarkerEnd,
		Version:     version,
	})
}

// generatePrepareCommitMsgHook generates the prepare-commit-msg hook script
func generatePrepareCommitMsgHook() string {
	return hooksvc.GeneratePrepareCommitMsgHook(hooksvc.TemplateConfig{
		MarkerBegin: lrcMarkerBegin,
		MarkerEnd:   lrcMarkerEnd,
		Version:     version,
	})
}

// generateCommitMsgHook generates the commit-msg hook script
func generateCommitMsgHook() string {
	return hooksvc.GenerateCommitMsgHook(hooksvc.TemplateConfig{
		MarkerBegin:       lrcMarkerBegin,
		MarkerEnd:         lrcMarkerEnd,
		Version:           version,
		CommitMessageFile: commitMessageFile,
	})
}

// generatePostCommitHook runs a safe pull (ff-only) and push when requested.
func generatePostCommitHook() string {
	return hooksvc.GeneratePostCommitHook(hooksvc.TemplateConfig{
		MarkerBegin:     lrcMarkerBegin,
		MarkerEnd:       lrcMarkerEnd,
		Version:         version,
		PushRequestFile: pushRequestFile,
	})
}

func generateDispatcherHook(hookName string) string {
	return hooksvc.GenerateDispatcherHook(hookName, hooksvc.TemplateConfig{
		MarkerBegin: lrcMarkerBegin,
		MarkerEnd:   lrcMarkerEnd,
		Version:     version,
	})
}

// cleanOldBackups removes old backup files, keeping only the last N
func cleanOldBackups(backupDir string, keepLast int) error {
	return hooksvc.CleanOldBackups(backupDir, keepLast)
}
