package appcore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gitops "github.com/HexmosTech/git-lrc/gitops"
	hooksvc "github.com/HexmosTech/git-lrc/hooks"
	"github.com/HexmosTech/git-lrc/internal/reviewapi"
	"github.com/HexmosTech/git-lrc/storage"
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

func resolveRepoHooksPath(repoRoot, gitCommonDir string) (string, error) {
	return hooksvc.ResolveRepoHooksPath(repoRoot, gitCommonDir)
}

func resolveEffectiveHooksPath(repoRoot, gitCommonDir string) (string, error) {
	return hooksvc.ResolveEffectiveHooksPath(repoRoot, gitCommonDir)
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

func writeHooksMeta(hooksPath string, meta hooksMeta) error {
	return hooksvc.WriteMeta(hooksPath, hooksMetaFilename, meta)
}

func readHooksMeta(hooksPath string) (*hooksMeta, error) {
	return hooksvc.ReadMeta(hooksPath, hooksMetaFilename)
}

func removeHooksMeta(hooksPath string) error {
	return hooksvc.RemoveMeta(hooksPath, hooksMetaFilename)
}

func writeManagedHookScripts(dir string) error {
	return hooksvc.WriteManagedHookScripts(dir, hooksvc.TemplateConfig{
		MarkerBegin:       lrcMarkerBegin,
		MarkerEnd:         lrcMarkerEnd,
		Version:           version,
		CommitMessageFile: commitMessageFile,
		PushRequestFile:   pushRequestFile,
	})
}

func resolveRepoContext() (repoRoot, gitDir, gitCommonDir string, err error) {
	repoRoot, err = reviewapi.ResolveRepoRoot()
	if err != nil {
		return "", "", "", err
	}
	gitDir, err = reviewapi.ResolveGitDir()
	if err != nil {
		return "", "", "", err
	}

	gitCommonDir, err = reviewapi.ResolveGitCommonDir()
	if err != nil {
		return "", "", "", err
	}

	return repoRoot, gitDir, gitCommonDir, nil
}

func parseHookInstallSurface(raw string, local bool) (hookSurface, error) {
	if strings.TrimSpace(raw) == "" {
		if local {
			return hookSurfaceGit, nil
		}
		return hookSurfaceAll, nil
	}

	surface, err := parseHookSurface(raw)
	if err != nil {
		return "", err
	}
	if local && surface != hookSurfaceGit {
		return "", fmt.Errorf("--local only supports --surface git")
	}
	return surface, nil
}

// installGitHookSurface installs dispatchers and managed hook scripts under either global core.hooksPath or the current repo hooks path when --local is used.
func installGitHookSurface(c *cli.Context) error {
	localInstall := c.Bool("local")
	requestedPath := strings.TrimSpace(c.String("path"))
	var hooksPath string
	var prevGlobalPath string
	var gitDir string
	setConfig := false

	if localInstall {
		if !isGitRepository() {
			return fmt.Errorf("not in a git repository (no .git directory found)")
		}

		repoRoot, resolvedGitDir, gitCommonDir, err := resolveRepoContext()
		if err != nil {
			return err
		}
		gitDir = resolvedGitDir
		hooksPath, err = resolveRepoHooksPath(repoRoot, gitCommonDir)
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

	absHooksPath, err := hooksvc.NormalizeHooksPath(hooksPath)
	if err != nil {
		return fmt.Errorf("failed to resolve hooks path: %w", err)
	}

	if !localInstall && setConfig {
		if err := setGlobalHooksPath(absHooksPath); err != nil {
			return fmt.Errorf("failed to set core.hooksPath: %w", err)
		}
	}

	if err := storage.EnsureHooksPathDir(absHooksPath); err != nil {
		return fmt.Errorf("failed to create hooks path %s: %w", absHooksPath, err)
	}

	managedDir := filepath.Join(absHooksPath, "lrc")
	backupDir := filepath.Join(absHooksPath, ".lrc_backups")
	if err := storage.EnsureHooksBackupDir(backupDir); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	if err := writeManagedHookScripts(managedDir); err != nil {
		return err
	}

	if localInstall {
		if err := installEditorWrapper(gitDir); err != nil {
			return fmt.Errorf("failed to install local editor wrapper: %w", err)
		}
	} else {
		if err := installGlobalEditorWrapper(managedDir); err != nil {
			return fmt.Errorf("failed to install global editor wrapper: %w", err)
		}
	}

	for _, hookName := range managedHooks {
		hookPath := filepath.Join(absHooksPath, hookName)
		dispatcher := generateDispatcherHook(hookName)
		if err := installHook(hookPath, dispatcher, hookName, backupDir, true); err != nil {
			return fmt.Errorf("failed to install dispatcher for %s: %w", hookName, err)
		}
	}

	if !localInstall {
		if err := writeHooksMeta(absHooksPath, hooksMeta{Path: absHooksPath, PrevPath: prevGlobalPath, SetByLRC: setConfig}); err != nil {
			return fmt.Errorf("failed to write hooks metadata: %w", err)
		}
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

// runHooksInstall installs Git and/or Claude hook integrations depending on the selected surface.
func runHooksInstall(c *cli.Context) error {
	localInstall := c.Bool("local")
	surface, err := parseHookInstallSurface(c.String("surface"), localInstall)
	if err != nil {
		return err
	}

	if surface == hookSurfaceAll || surface == hookSurfaceGit {
		if err := installGitHookSurface(c); err != nil {
			return err
		}
	}

	if !localInstall && (surface == hookSurfaceAll || surface == hookSurfaceClaude) {
		claudeState, err := installClaudeGlobalHooks()
		if err != nil {
			return err
		}
		fmt.Printf("✅ LiveReview Claude hooks installed at %s\n", claudeState.HooksDir)
		fmt.Printf("✅ Claude user settings updated at %s\n", claudeState.SettingsPath)
	}

	if !localInstall {
		if repoRoot, _, _, err := resolveRepoContext(); err == nil {
			if _, err := removeLegacyRepoClaudeIntegration(repoRoot); err != nil {
				return fmt.Errorf("failed to remove legacy repo-local Claude hook files: %w", err)
			}
		}
	}

	return nil
}

// uninstallGitHookSurface removes lrc-managed sections from dispatchers and managed scripts (global or local).
func uninstallGitHookSurface(c *cli.Context) error {
	localUninstall := c.Bool("local")
	requestedPath := strings.TrimSpace(c.String("path"))
	var hooksPath string
	var gitDir string

	if localUninstall {
		if !isGitRepository() {
			return fmt.Errorf("not in a git repository (no .git directory found)")
		}
		repoRoot, resolvedGitDir, gitCommonDir, err := resolveRepoContext()
		if err != nil {
			return err
		}
		gitDir = resolvedGitDir
		hooksPath, err = resolveRepoHooksPath(repoRoot, gitCommonDir)
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

	absHooksPath, err := hooksvc.NormalizeHooksPath(hooksPath)
	if err != nil {
		return fmt.Errorf("failed to resolve hooks path: %w", err)
	}

	currentGlobalPath, _ := currentHooksPath()

	var meta *hooksMeta
	if !localUninstall {
		meta, _ = readHooksMeta(absHooksPath)
	}

	if localUninstall {
		if err := uninstallEditorWrapper(gitDir); err != nil {
			fmt.Printf("⚠️  Warning: failed to uninstall local editor wrapper: %v\n", err)
		}
	} else {
		if err := uninstallGlobalEditorWrapper(filepath.Join(absHooksPath, "lrc")); err != nil {
			fmt.Printf("⚠️  Warning: failed to uninstall global editor wrapper: %v\n", err)
		}
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

	_ = storage.RemoveManagedHooksDir(absHooksPath)
	_ = storage.RemoveHooksBackupDir(absHooksPath)
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

// runHooksUninstall removes Git and/or Claude hook integrations depending on the selected surface.
func runHooksUninstall(c *cli.Context) error {
	localUninstall := c.Bool("local")
	surface, err := parseHookInstallSurface(c.String("surface"), localUninstall)
	if err != nil {
		return err
	}

	if !localUninstall && (surface == hookSurfaceAll || surface == hookSurfaceClaude) {
		claudeState, err := uninstallClaudeGlobalHooks()
		if err != nil {
			return err
		}
		fmt.Printf("✅ Removed LiveReview Claude hook integration from %s\n", claudeState.SettingsPath)
	}

	if surface == hookSurfaceAll || surface == hookSurfaceGit {
		if err := uninstallGitHookSurface(c); err != nil {
			return err
		}
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

type hookSurface string

const (
	hookSurfaceAll    hookSurface = "all"
	hookSurfaceGit    hookSurface = "git"
	hookSurfaceClaude hookSurface = "claude"
)

type repoHookSurfaceState struct {
	gitDisabled    bool
	claudeDisabled bool
}

func parseHookSurface(raw string) (hookSurface, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(hookSurfaceAll):
		return hookSurfaceAll, nil
	case string(hookSurfaceGit):
		return hookSurfaceGit, nil
	case string(hookSurfaceClaude):
		return hookSurfaceClaude, nil
	default:
		return "", fmt.Errorf("invalid --surface %q (want all, git, or claude)", raw)
	}
}

func repoLRCStateDir(gitDir string) string {
	return filepath.Join(gitDir, "lrc")
}

func repoHooksDisabledMarker(lrcDir string) string {
	return filepath.Join(lrcDir, "disabled")
}

func repoGitHooksDisabledMarker(lrcDir string) string {
	return filepath.Join(lrcDir, "disabled-git")
}

func repoClaudeHooksDisabledMarker(lrcDir string) string {
	return filepath.Join(lrcDir, "disabled-claude")
}

func readRepoHookSurfaceState(gitDir string) repoHookSurfaceState {
	lrcDir := repoLRCStateDir(gitDir)
	if fileExists(repoHooksDisabledMarker(lrcDir)) {
		return repoHookSurfaceState{gitDisabled: true, claudeDisabled: true}
	}
	return repoHookSurfaceState{
		gitDisabled:    fileExists(repoGitHooksDisabledMarker(lrcDir)),
		claudeDisabled: fileExists(repoClaudeHooksDisabledMarker(lrcDir)),
	}
}

func updateRepoHookSurfaceState(current repoHookSurfaceState, surface hookSurface, disabled bool) repoHookSurfaceState {
	next := current
	switch surface {
	case hookSurfaceAll:
		next.gitDisabled = disabled
		next.claudeDisabled = disabled
	case hookSurfaceGit:
		next.gitDisabled = disabled
	case hookSurfaceClaude:
		next.claudeDisabled = disabled
	}
	return next
}

func writeRepoHookSurfaceState(gitDir string, state repoHookSurfaceState) error {
	lrcDir := repoLRCStateDir(gitDir)
	if state.gitDisabled || state.claudeDisabled {
		if err := storage.EnsureRepoLRCStateDir(lrcDir); err != nil {
			return fmt.Errorf("failed to create lrc directory: %w", err)
		}
	}

	markers := []string{
		repoHooksDisabledMarker(lrcDir),
		repoGitHooksDisabledMarker(lrcDir),
		repoClaudeHooksDisabledMarker(lrcDir),
	}
	for _, marker := range markers {
		if err := storage.RemoveRepoHooksStateMarker(marker); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	var marker string
	switch {
	case state.gitDisabled && state.claudeDisabled:
		marker = repoHooksDisabledMarker(lrcDir)
	case state.gitDisabled:
		marker = repoGitHooksDisabledMarker(lrcDir)
	case state.claudeDisabled:
		marker = repoClaudeHooksDisabledMarker(lrcDir)
	default:
		return nil
	}

	if err := storage.WriteFile(marker, []byte("disabled\n"), 0644); err != nil {
		return fmt.Errorf("failed to write disable marker: %w", err)
	}
	return nil
}

func hookSurfaceLabel(surface hookSurface) string {
	switch surface {
	case hookSurfaceGit:
		return "Git hooks"
	case hookSurfaceClaude:
		return "Claude hooks"
	default:
		return "LiveReview hooks"
	}
}

func effectiveHookSurfaceDisabled(state repoHookSurfaceState, surface hookSurface) bool {
	switch surface {
	case hookSurfaceGit:
		return state.gitDisabled
	case hookSurfaceClaude:
		return state.claudeDisabled
	default:
		return state.gitDisabled || state.claudeDisabled
	}
}

func hookSurfaceStatusMarker(gitDir string, surface hookSurface) string {
	lrcDir := repoLRCStateDir(gitDir)
	if fileExists(repoHooksDisabledMarker(lrcDir)) {
		return repoHooksDisabledMarker(lrcDir)
	}
	switch surface {
	case hookSurfaceGit:
		if fileExists(repoGitHooksDisabledMarker(lrcDir)) {
			return repoGitHooksDisabledMarker(lrcDir)
		}
	case hookSurfaceClaude:
		if fileExists(repoClaudeHooksDisabledMarker(lrcDir)) {
			return repoClaudeHooksDisabledMarker(lrcDir)
		}
	}
	return ""
}

func runHooksDisable(c *cli.Context) error {
	gitDir, err := reviewapi.ResolveGitDir()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	surface, err := parseHookSurface(c.String("surface"))
	if err != nil {
		return err
	}
	current := readRepoHookSurfaceState(gitDir)
	next := updateRepoHookSurfaceState(current, surface, true)
	if err := writeRepoHookSurfaceState(gitDir, next); err != nil {
		return err
	}

	fmt.Printf("🔕 %s disabled for this repository\n", hookSurfaceLabel(surface))
	return nil
}

func runHooksEnable(c *cli.Context) error {
	gitDir, err := reviewapi.ResolveGitDir()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	surface, err := parseHookSurface(c.String("surface"))
	if err != nil {
		return err
	}
	current := readRepoHookSurfaceState(gitDir)
	next := updateRepoHookSurfaceState(current, surface, false)
	if err := writeRepoHookSurfaceState(gitDir, next); err != nil {
		return err
	}

	fmt.Printf("🔔 %s enabled for this repository\n", hookSurfaceLabel(surface))
	return nil
}

func hookHasManagedSection(path string) bool {
	return hooksvc.HookHasManagedSection(path, lrcMarkerBegin)
}

func runHooksStatus(c *cli.Context) error {
	globalHooksPath, _ := currentHooksPath()
	defaultPath, _ := defaultGlobalHooksPath()
	surface, err := parseHookSurface(c.String("surface"))
	if err != nil {
		return err
	}

	claudeInstallState, err := claudeGlobalInstallStatus()
	if err != nil {
		return err
	}

	repoRoot, gitDir, gitCommonDir, gitErr := resolveRepoContext()
	repoState := repoHookSurfaceState{}
	if gitErr == nil {
		repoState = readRepoHookSurfaceState(gitDir)
	}

	hooksPath := globalHooksPath
	if gitErr == nil {
		var err error
		hooksPath, err = resolveEffectiveHooksPath(repoRoot, gitCommonDir)
		if err != nil {
			return err
		}
	} else if hooksPath == "" {
		hooksPath = defaultPath
	}

	absHooksPath, err := hooksvc.NormalizeHooksPath(hooksPath)
	if err != nil {
		return fmt.Errorf("failed to resolve hooks path: %w", err)
	}

	fmt.Printf("hooksPath: %s\n", absHooksPath)
	if localCfg, _ := currentLocalHooksPath(repoRoot); gitErr == nil && localCfg != "" {
		fmt.Printf("repo core.hooksPath: %s\n", localCfg)
	}
	if globalHooksPath != "" {
		fmt.Printf("global core.hooksPath: %s\n", globalHooksPath)
	} else {
		fmt.Println("global core.hooksPath: not set")
	}
	if surface == hookSurfaceAll || surface == hookSurfaceClaude {
		fmt.Printf("claude settings: %s\n", claudeInstallState.SettingsPath)
		fmt.Printf("claude skill: %s\n", claudeInstallState.SkillPath)
		switch {
		case claudeInstallState.SettingsManaged && claudeInstallState.ValidatorExists && claudeInstallState.WrapperExists:
			fmt.Println("global claude install: installed")
		case claudeInstallState.SettingsManaged || claudeInstallState.ValidatorExists || claudeInstallState.WrapperExists:
			fmt.Println("global claude install: partial")
		default:
			fmt.Println("global claude install: not installed")
		}
		if claudeInstallState.SkillExists {
			fmt.Println("global claude skill: installed")
		} else {
			fmt.Println("global claude skill: missing")
		}
	}

	if gitErr == nil {
		fmt.Printf("repo: %s\n", repoRoot)
		switch surface {
		case hookSurfaceAll:
			switch {
			case repoState.gitDisabled && repoState.claudeDisabled:
				fmt.Printf("status: disabled for git and claude via %s\n", hookSurfaceStatusMarker(gitDir, hookSurfaceAll))
			case repoState.gitDisabled || repoState.claudeDisabled:
				fmt.Println("status: mixed")
			default:
				fmt.Println("status: enabled")
			}
			if repoState.gitDisabled {
				fmt.Printf("git status: disabled via %s\n", hookSurfaceStatusMarker(gitDir, hookSurfaceGit))
			} else {
				fmt.Println("git status: enabled")
			}
			if repoState.claudeDisabled {
				fmt.Printf("claude status: disabled via %s\n", hookSurfaceStatusMarker(gitDir, hookSurfaceClaude))
			} else {
				fmt.Println("claude status: enabled")
			}
		default:
			if effectiveHookSurfaceDisabled(repoState, surface) {
				fmt.Printf("%s status: disabled via %s\n", strings.ToLower(hookSurfaceLabel(surface)), hookSurfaceStatusMarker(gitDir, surface))
			} else {
				fmt.Printf("%s status: enabled\n", strings.ToLower(hookSurfaceLabel(surface)))
			}
		}
	} else {
		fmt.Println("repo: not detected")
	}

	if gitErr == nil && (surface == hookSurfaceAll || surface == hookSurfaceClaude) {
		legacy := detectLegacyRepoClaudeIntegration(repoRoot)
		if len(legacy) == 0 {
			fmt.Println("legacy claude integration: none detected")
		} else {
			fmt.Println("legacy claude integration: detected")
			for _, path := range legacy {
				fmt.Printf("legacy claude path: %s\n", path)
			}
		}
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
	return gitops.IsGitRepository()
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
	repoRoot, err := reviewapi.ResolveRepoRoot()
	if err != nil {
		return fmt.Errorf("failed to resolve repository root: %w", err)
	}
	scriptPath := filepath.Join(gitDir, editorWrapperScript)
	backupPath := filepath.Join(gitDir, editorBackupFile)

	currentEditor, _ := readGitConfig(repoRoot, "core.editor")
	if shouldPersistEditorBackup(currentEditor, scriptPath) {
		_ = storage.WriteFile(backupPath, []byte(currentEditor), 0600)
	}

	script := generateEditorWrapperScript(backupPath)

	if err := storage.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("failed to write editor wrapper: %w", err)
	}

	if err := setGitConfig(repoRoot, "core.editor", scriptPath); err != nil {
		return fmt.Errorf("failed to set core.editor: %w", err)
	}

	return nil
}

func installGlobalEditorWrapper(managedDir string) error {
	if err := storage.EnsureHooksPathDir(managedDir); err != nil {
		return fmt.Errorf("failed to create managed hooks directory for editor wrapper: %w", err)
	}

	scriptPath := filepath.Join(managedDir, editorWrapperScript)
	backupPath := filepath.Join(managedDir, editorBackupFile)

	currentEditor, _ := readGlobalGitConfig("core.editor")
	if shouldPersistEditorBackup(currentEditor, scriptPath) {
		_ = storage.WriteFile(backupPath, []byte(currentEditor), 0600)
	}

	script := generateEditorWrapperScript(backupPath)
	if err := storage.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("failed to write global editor wrapper: %w", err)
	}

	if err := setGlobalGitConfig("core.editor", scriptPath); err != nil {
		return fmt.Errorf("failed to set global core.editor: %w", err)
	}

	return nil
}

// uninstallEditorWrapper restores the previous editor (if backed up) and removes wrapper files.
func uninstallEditorWrapper(gitDir string) error {
	repoRoot, err := reviewapi.ResolveRepoRoot()
	if err != nil {
		return fmt.Errorf("failed to resolve repository root: %w", err)
	}
	scriptPath := filepath.Join(gitDir, editorWrapperScript)
	backupPath := filepath.Join(gitDir, editorBackupFile)

	if data, err := storage.ReadEditorBackupFile(backupPath); err == nil {
		value := strings.TrimSpace(string(data))
		if value != "" {
			_ = setGitConfig(repoRoot, "core.editor", value)
		}
	} else if currentEditor, err := readGitConfig(repoRoot, "core.editor"); err == nil && currentEditor == scriptPath {
		_ = unsetGitConfig(repoRoot, "core.editor")
	}

	_ = storage.RemoveEditorWrapperScript(scriptPath)
	_ = storage.RemoveEditorBackupStateFile(backupPath)

	return nil
}

func uninstallGlobalEditorWrapper(managedDir string) error {
	scriptPath := filepath.Join(managedDir, editorWrapperScript)
	backupPath := filepath.Join(managedDir, editorBackupFile)

	if data, err := storage.ReadEditorBackupFile(backupPath); err == nil {
		value := strings.TrimSpace(string(data))
		if value != "" {
			_ = setGlobalGitConfig("core.editor", value)
		}
	} else if currentEditor, err := readGlobalGitConfig("core.editor"); err == nil && currentEditor == scriptPath {
		_ = unsetGlobalGitConfig("core.editor")
	}

	_ = storage.RemoveEditorWrapperScript(scriptPath)
	_ = storage.RemoveEditorBackupStateFile(backupPath)

	return nil
}

func generateEditorWrapperScript(backupPath string) string {
	return fmt.Sprintf(`#!/bin/sh
set -e

BACKUP_FILE="%s"

run_editor_command() {
	editor_cmd="$1"
	shift
	exec sh -c 'editor_cmd="$1"; shift; exec $editor_cmd "$@"' sh "$editor_cmd" "$@"
}

editor_command_exists() {
	editor_cmd="$1"
	editor_bin="${editor_cmd%% *}"
	if [ -z "$editor_bin" ]; then
		return 1
	fi
	command -v "$editor_bin" >/dev/null 2>&1
}

if [ $# -gt 0 ] && [ -n "$1" ]; then
	TARGET_FILE="$1"
	TARGET_DIR="$(dirname "$TARGET_FILE")"
	OVERRIDE_FILE="$TARGET_DIR/%s"
	OVERRIDE_STATE="$TARGET_DIR/livereview_editor_override"

	if [ -f "$OVERRIDE_STATE" ]; then
		if [ -f "$OVERRIDE_FILE" ] && [ -s "$OVERRIDE_FILE" ]; then
			cat "$OVERRIDE_FILE" > "$TARGET_FILE"
		fi
		exit 0
	fi
fi

if [ -f "$BACKUP_FILE" ] && [ -s "$BACKUP_FILE" ]; then
	BACKUP_EDITOR="$(cat "$BACKUP_FILE" 2>/dev/null || true)"
	if [ -n "$BACKUP_EDITOR" ]; then
		if editor_command_exists "$BACKUP_EDITOR"; then
			run_editor_command "$BACKUP_EDITOR" "$@"
		fi
	fi
fi

if [ -n "$LRC_FALLBACK_EDITOR" ]; then
	run_editor_command "$LRC_FALLBACK_EDITOR" "$@"
fi

if [ -n "$VISUAL" ]; then
	run_editor_command "$VISUAL" "$@"
fi

if [ -n "$EDITOR" ]; then
	run_editor_command "$EDITOR" "$@"
fi

exec vi "$@"
`, backupPath, commitMessageFile)
}

func shouldPersistEditorBackup(currentEditor, wrapperPath string) bool {
	trimmed := strings.TrimSpace(currentEditor)
	if trimmed == "" {
		return false
	}

	if trimmed == wrapperPath {
		return false
	}

	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return false
	}

	editorCmd := parts[0]
	if filepath.Base(editorCmd) == editorWrapperScript {
		return false
	}

	if strings.Contains(editorCmd, string(filepath.Separator)+"tmp"+string(filepath.Separator)+"lrc-test-hooks.") {
		return false
	}

	return true
}

// readGitConfig reads a single git config key from the repository root.
func readGitConfig(repoRoot, key string) (string, error) {
	return gitops.ReadGitConfig(repoRoot, key)
}

func readGlobalGitConfig(key string) (string, error) {
	return gitops.ReadGlobalGitConfig(key)
}

// setGitConfig sets a git config key in the given repository.
func setGitConfig(repoRoot, key, value string) error {
	return gitops.SetGitConfig(repoRoot, key, value)
}

func setGlobalGitConfig(key, value string) error {
	return gitops.SetGlobalGitConfig(key, value)
}

// unsetGitConfig removes a git config key in the given repository.
func unsetGitConfig(repoRoot, key string) error {
	return gitops.UnsetGitConfig(repoRoot, key)
}

func unsetGlobalGitConfig(key string) error {
	return gitops.UnsetGlobalGitConfig(key)
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
