package appcore

import (
	"path/filepath"
	"strings"
	"testing"
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
		"run_editor_command \"$BACKUP_EDITOR\" \"$@\"",
	}
	for _, want := range checks {
		if !strings.Contains(script, want) {
			t.Fatalf("wrapper script missing %q", want)
		}
	}
}
