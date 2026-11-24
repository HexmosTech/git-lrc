package appcore

import (
	"bytes"
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

func TestParseHookSurface(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    hookSurface
		wantErr bool
	}{
		{name: "default all", input: "", want: hookSurfaceAll},
		{name: "explicit all", input: "all", want: hookSurfaceAll},
		{name: "git", input: "git", want: hookSurfaceGit},
		{name: "claude", input: "claude", want: hookSurfaceClaude},
		{name: "invalid", input: "both", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHookSurface(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseHookSurface(%q) error = nil, want non-nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseHookSurface(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("parseHookSurface(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseHookInstallSurface(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		local   bool
		want    hookSurface
		wantErr bool
	}{
		{name: "global default all", input: "", want: hookSurfaceAll},
		{name: "local default git", input: "", local: true, want: hookSurfaceGit},
		{name: "local git explicit", input: "git", local: true, want: hookSurfaceGit},
		{name: "local all rejected", input: "all", local: true, wantErr: true},
		{name: "local claude rejected", input: "claude", local: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHookInstallSurface(tt.input, tt.local)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseHookInstallSurface(%q, %v) error = nil, want non-nil", tt.input, tt.local)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseHookInstallSurface(%q, %v) error = %v", tt.input, tt.local, err)
			}
			if got != tt.want {
				t.Fatalf("parseHookInstallSurface(%q, %v) = %q, want %q", tt.input, tt.local, got, tt.want)
			}
		})
	}
}

func TestUpdateRepoHookSurfaceState(t *testing.T) {
	tests := []struct {
		name     string
		current  repoHookSurfaceState
		surface  hookSurface
		disabled bool
		want     repoHookSurfaceState
	}{
		{
			name:     "disable all",
			current:  repoHookSurfaceState{},
			surface:  hookSurfaceAll,
			disabled: true,
			want:     repoHookSurfaceState{gitDisabled: true, claudeDisabled: true},
		},
		{
			name:     "disable git only",
			current:  repoHookSurfaceState{},
			surface:  hookSurfaceGit,
			disabled: true,
			want:     repoHookSurfaceState{gitDisabled: true},
		},
		{
			name:     "enable git keeps claude disabled",
			current:  repoHookSurfaceState{gitDisabled: true, claudeDisabled: true},
			surface:  hookSurfaceGit,
			disabled: false,
			want:     repoHookSurfaceState{claudeDisabled: true},
		},
		{
			name:     "enable all clears both",
			current:  repoHookSurfaceState{gitDisabled: true, claudeDisabled: true},
			surface:  hookSurfaceAll,
			disabled: false,
			want:     repoHookSurfaceState{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updateRepoHookSurfaceState(tt.current, tt.surface, tt.disabled)
			if got != tt.want {
				t.Fatalf("updateRepoHookSurfaceState(%+v, %q, %v) = %+v, want %+v", tt.current, tt.surface, tt.disabled, got, tt.want)
			}
		})
	}
}

func TestWriteAndReadRepoHookSurfaceState(t *testing.T) {
	gitDir := t.TempDir()

	tests := []struct {
		name        string
		state       repoHookSurfaceState
		wantMarkers []string
	}{
		{name: "enabled", state: repoHookSurfaceState{}, wantMarkers: nil},
		{name: "all disabled", state: repoHookSurfaceState{gitDisabled: true, claudeDisabled: true}, wantMarkers: []string{"disabled"}},
		{name: "git disabled", state: repoHookSurfaceState{gitDisabled: true}, wantMarkers: []string{"disabled-git"}},
		{name: "claude disabled", state: repoHookSurfaceState{claudeDisabled: true}, wantMarkers: []string{"disabled-claude"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := writeRepoHookSurfaceState(gitDir, tt.state); err != nil {
				t.Fatalf("writeRepoHookSurfaceState() error = %v", err)
			}

			got := readRepoHookSurfaceState(gitDir)
			if got != tt.state {
				t.Fatalf("readRepoHookSurfaceState() = %+v, want %+v", got, tt.state)
			}

			lrcDir := repoLRCStateDir(gitDir)
			for _, name := range []string{"disabled", "disabled-git", "disabled-claude"} {
				path := filepath.Join(lrcDir, name)
				wantPresent := false
				for _, want := range tt.wantMarkers {
					if want == name {
						wantPresent = true
						break
					}
				}
				if fileExists(path) != wantPresent {
					t.Fatalf("marker %s present = %v, want %v", name, fileExists(path), wantPresent)
				}
			}
		})
	}
}

func TestGenerateEditorWrapperScriptUsesMarkerAndBackup(t *testing.T) {
	backupPath := filepath.Join("/tmp", ".lrc_editor_backup")
	script := generateEditorWrapperScript(backupPath)

	checks := []string{
		"BACKUP_FILE=\"" + backupPath + "\"",
		"OVERRIDE_FILE=\"$TARGET_DIR/" + commitMessageFile + "\"",
		"OVERRIDE_STATE=\"$TARGET_DIR/livereview_editor_override\"",
		"if [ -f \"$OVERRIDE_STATE\" ]; then",
		"if editor_command_exists \"$BACKUP_EDITOR\"; then",
		"run_editor_command \"$BACKUP_EDITOR\" \"$@\"",
	}
	for _, want := range checks {
		if !strings.Contains(script, want) {
			t.Fatalf("wrapper script missing %q", want)
		}
	}
}

func TestShouldPersistEditorBackup(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		wrapperPath string
		want        bool
	}{
		{
			name:        "empty editor",
			current:     "",
			wrapperPath: "/home/user/.git-hooks/lrc/lrc_editor.sh",
			want:        false,
		},
		{
			name:        "same wrapper path",
			current:     "/home/user/.git-hooks/lrc/lrc_editor.sh",
			wrapperPath: "/home/user/.git-hooks/lrc/lrc_editor.sh",
			want:        false,
		},
		{
			name:        "lrc wrapper command from another path",
			current:     "/tmp/lrc-test-hooks.ABCD/lrc/lrc_editor.sh",
			wrapperPath: "/home/user/.git-hooks/lrc/lrc_editor.sh",
			want:        false,
		},
		{
			name:        "tmp test hooks path",
			current:     "/tmp/lrc-test-hooks.QGAqgJ/lrc/lrc_editor.sh",
			wrapperPath: "/home/user/.git-hooks/lrc/lrc_editor.sh",
			want:        false,
		},
		{
			name:        "valid editor command",
			current:     "code --wait",
			wrapperPath: "/home/user/.git-hooks/lrc/lrc_editor.sh",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldPersistEditorBackup(tt.current, tt.wrapperPath)
			if got != tt.want {
				t.Fatalf("shouldPersistEditorBackup(%q, %q) = %v, want %v", tt.current, tt.wrapperPath, got, tt.want)
			}
		})
	}
}

func TestDefaultUninstallRemovesLocalGitHooks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Chdir(t.TempDir())
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	flags := flag.NewFlagSet("hooks", flag.ContinueOnError)
	local := flags.Bool("local", true, "")
	ctx := cli.NewContext(cli.NewApp(), flags, nil)
	if err := installGitHookSurface(ctx); err != nil {
		t.Fatal(err)
	}
	*local = false
	if err := uninstallGitHookSurface(ctx); err != nil {
		t.Fatal(err)
	}
	for _, name := range append([]string{"lrc", ".lrc_backups"}, managedHooks...) {
		if _, err := os.Stat(filepath.Join(".git/hooks", name)); !os.IsNotExist(err) {
			t.Errorf("%s survived uninstall or stat failed: %v", name, err)
		}
	}
	if fileExists(filepath.Join(".git", editorWrapperScript)) {
		t.Error("local editor wrapper survived uninstall")
	}
}

func TestUninstallManagedGitHooksWarnsOnDirRemoveFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires non-root Unix permission semantics")
	}

	hooksDir := t.TempDir()
	managedDir := filepath.Join(hooksDir, "lrc")
	backupDir := filepath.Join(hooksDir, ".lrc_backups")
	for _, dir := range []string{managedDir, backupDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "keep"), []byte("x"), 0644); err != nil {
			t.Fatalf("write file in %s: %v", dir, err)
		}
		if err := os.Chmod(dir, 0555); err != nil {
			t.Fatalf("chmod %s: %v", dir, err)
		}
		t.Cleanup(func() {
			if err := os.Chmod(dir, 0755); err != nil {
				t.Errorf("restore permissions on %s: %v", dir, err)
			}
		})
	}

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
	}()

	uninstallManagedGitHooks(hooksDir)

	if err := w.Close(); err != nil {
		t.Fatalf("close stdout pipe writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close stdout pipe reader: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "failed to remove managed hooks directory "+managedDir) {
		t.Errorf("expected warning for managed hooks dir %s, got:\n%s", managedDir, output)
	}
	if !strings.Contains(output, "failed to remove hooks backup directory "+backupDir) {
		t.Errorf("expected warning for hooks backup dir %s, got:\n%s", backupDir, output)
	}
}
