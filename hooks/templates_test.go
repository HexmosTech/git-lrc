package hooks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testTemplateConfig() TemplateConfig {
	return TemplateConfig{
		MarkerBegin:       "# BEGIN lrc managed section - DO NOT EDIT",
		MarkerEnd:         "# END lrc managed section",
		Version:           "test-version",
		CommitMessageFile: "livereview_commit_message",
		PushRequestFile:   "livereview_push_request",
	}
}

func TestGeneratedHooksUseResolvedGitDirPaths(t *testing.T) {
	cfg := testTemplateConfig()

	tests := []struct {
		name      string
		hook      string
		contains  []string
		forbidden []string
	}{
		{
			name: "pre-commit",
			hook: GeneratePreCommitHook(cfg),
			contains: []string{
				"GIT_DIR=\"$(git rev-parse --git-dir 2>/dev/null || echo .git)\"",
				"LRC_DIR=\"$GIT_DIR/lrc\"",
				"DISABLED_GIT_FILE=\"$LRC_DIR/disabled-git\"",
				"ATTEST_FILE=\"$LRC_DIR/attestations/$TREE_HASH.json\"",
			},
			forbidden: []string{
				"DISABLED_FILE=\".git/lrc/disabled\"",
				"ATTEST_FILE=\".git/lrc/attestations/$TREE_HASH.json\"",
			},
		},
		{
			name: "prepare-commit-msg",
			hook: GeneratePrepareCommitMsgHook(cfg),
			contains: []string{
				"GIT_DIR=\"$(git rev-parse --git-dir 2>/dev/null || echo .git)\"",
				"COMMIT_MSG_OVERRIDE=\"$GIT_DIR/livereview_commit_message\"",
				"EDITOR_OVERRIDE_STATE=\"$GIT_DIR/livereview_editor_override\"",
				"DISABLED_GIT_FILE=\"$LRC_DIR/disabled-git\"",
				"STATE_FILE=\"$GIT_DIR/livereview_state\"",
				"LOCK_DIR=\"$GIT_DIR/livereview_state.lock\"",
				"INITIAL_MSG_FILE=\"$GIT_DIR/livereview_initial_message.$$\"",
				"rm -f \"$EDITOR_OVERRIDE_STATE\" 2>/dev/null || true",
				"LRC_ACTIVE_COMMIT_MSG_FILE=\"$COMMIT_MSG_FILE\"",
				"cat \"$COMMIT_MSG_OVERRIDE\" > \"$COMMIT_MSG_FILE\" 2>/dev/null || true",
				"printf 'prefilled\\n' > \"$EDITOR_OVERRIDE_STATE\"",
			},
			forbidden: []string{
				"LRC_DIR=\".git/lrc\"",
				"STATE_FILE=\".git/livereview_state\"",
				"LOCK_DIR=\".git/livereview_state.lock\"",
				"INITIAL_MSG_FILE=\".git/livereview_initial_message.$$\"",
				"git config --local core.editor true >/dev/null 2>&1 || true",
			},
		},
		{
			name: "commit-msg",
			hook: GenerateCommitMsgHook(cfg),
			contains: []string{
				"GIT_DIR=\"$(git rev-parse --git-dir 2>/dev/null || echo .git)\"",
				"EDITOR_OVERRIDE_STATE=\"$GIT_DIR/livereview_editor_override\"",
				"DISABLED_GIT_FILE=\"$LRC_DIR/disabled-git\"",
				"COMMIT_MSG_OVERRIDE=\"$GIT_DIR/livereview_commit_message\"",
				"STATE_FILE=\"$GIT_DIR/livereview_state\"",
				"rm -f \"$EDITOR_OVERRIDE_STATE\" 2>/dev/null || true",
			},
			forbidden: []string{
				"COMMIT_MSG_OVERRIDE=\".git/livereview_commit_message\"",
				"LRC_DIR=\".git/lrc\"",
				"STATE_FILE=\".git/livereview_state\"",
				"git config --local --unset core.editor >/dev/null 2>&1 || true",
			},
		},
		{
			name: "post-commit",
			hook: GeneratePostCommitHook(cfg),
			contains: []string{
				"GIT_DIR=\"$(git rev-parse --git-dir 2>/dev/null || echo .git)\"",
				"DISABLED_GIT_FILE=\"$LRC_DIR/disabled-git\"",
				"PUSH_FLAG=\"$GIT_DIR/livereview_push_request\"",
				"LRC_DIR=\"$GIT_DIR/lrc\"",
			},
			forbidden: []string{
				"PUSH_FLAG=\".git/livereview_push_request\"",
				"LRC_DIR=\".git/lrc\"",
			},
		},
		{
			name: "dispatcher",
			hook: GenerateDispatcherHook("pre-commit", cfg),
			contains: []string{
				"GIT_DIR=\"$(git rev-parse --git-dir 2>/dev/null || echo .git)\"",
				"GIT_COMMON_DIR=\"$(git rev-parse --git-common-dir 2>/dev/null || echo \"$GIT_DIR\")\"",
				"LRC_DISABLED_FILE=\"$GIT_DIR/lrc/disabled\"",
				"LRC_DISABLED_GIT_FILE=\"$GIT_DIR/lrc/disabled-git\"",
				"LOCAL_HOOK=\"$GIT_COMMON_DIR/hooks/pre-commit\"",
			},
			forbidden: []string{
				"LRC_DISABLED_FILE=\".git/lrc/disabled\"",
				"LRC_DISABLED_GIT_FILE=\".git/lrc/disabled-git\"",
				"LOCAL_HOOK=\"$(git rev-parse --git-path hooks/pre-commit 2>/dev/null || echo .git/hooks/pre-commit)\"",
				"LOCAL_HOOK=\".git/hooks/pre-commit\"",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, want := range tt.contains {
				if !strings.Contains(tt.hook, want) {
					t.Fatalf("expected generated %s hook to contain %q", tt.name, want)
				}
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(tt.hook, forbidden) {
					t.Fatalf("did not expect generated %s hook to contain %q", tt.name, forbidden)
				}
			}
		})
	}
}

func TestPrepareCommitMsgHookPrefillsReviewedMessageBeforeEditor(t *testing.T) {
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script command not available")
	}

	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	hooksDir := filepath.Join(repoDir, ".git", "myhooks")
	binDir := filepath.Join(tmpDir, "bin")
	editorMarker := filepath.Join(tmpDir, "editor-ran")
	editorScript := filepath.Join(tmpDir, "editor.sh")
	wrapperScript := filepath.Join(tmpDir, "lrc_editor.sh")

	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", hooksDir, err)
	}
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", binDir, err)
	}

	run := func(dir string, env []string, name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v failed: %v\n%s", name, args, err, string(out))
		}
	}

	run(tmpDir, nil, "git", "init", "--initial-branch=main", repoDir)
	run(repoDir, nil, "git", "config", "user.email", "test@example.com")
	run(repoDir, nil, "git", "config", "user.name", "Test User")

	prepareHook := GeneratePrepareCommitMsgHook(testTemplateConfig())
	prepareHookPath := filepath.Join(hooksDir, "prepare-commit-msg")
	if err := os.WriteFile(prepareHookPath, []byte(prepareHook), 0755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", prepareHookPath, err)
	}

	commitHook := GenerateCommitMsgHook(testTemplateConfig())
	commitHookPath := filepath.Join(hooksDir, "commit-msg")
	if err := os.WriteFile(commitHookPath, []byte(commitHook), 0755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", commitHookPath, err)
	}

	fakeLRCPath := filepath.Join(binDir, "lrc")
	fakeLRC := `#!/bin/sh
set -eu
if [ "$1" = "review" ]; then
	if [ -n "${LRC_ACTIVE_COMMIT_MSG_FILE:-}" ]; then
	  printf 'chosen message\n' > "$LRC_ACTIVE_COMMIT_MSG_FILE"
	  exit 0
	fi
  git_dir="$(git rev-parse --git-dir 2>/dev/null || echo .git)"
  printf 'chosen message\n' > "$git_dir/livereview_commit_message"
  exit 0
fi
exit 0
`
	if err := os.WriteFile(fakeLRCPath, []byte(fakeLRC), 0755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", fakeLRCPath, err)
	}

	editorBody := "#!/bin/sh\nprintf 'editor-ran\\n' > \"MARKER\"\nexit 0\n"
	editorBody = strings.ReplaceAll(editorBody, "MARKER", editorMarker)
	if err := os.WriteFile(editorScript, []byte(editorBody), 0755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", editorScript, err)
	}
	wrapperBody := fmt.Sprintf("#!/bin/sh\nset -eu\nTARGET_FILE=\"${1:-}\"\nTARGET_DIR=\"$(dirname \"$TARGET_FILE\")\"\nif [ -f \"$TARGET_DIR/livereview_editor_override\" ]; then\n  exit 0\nfi\nexec %s \"$@\"\n", editorScript)
	if err := os.WriteFile(wrapperScript, []byte(wrapperBody), 0755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", wrapperScript, err)
	}
	run(repoDir, nil, "git", "config", "core.editor", wrapperScript)

	trackedFile := filepath.Join(repoDir, "note.txt")
	if err := os.WriteFile(trackedFile, []byte("hello\n"), 0644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", trackedFile, err)
	}
	run(repoDir, nil, "git", "add", "note.txt")

	env := append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmdText := "git -c core.hooksPath=.git/myhooks -c commit.cleanup=strip commit"
	cmd := exec.Command("script", "-qec", cmdText, "/dev/null")
	cmd.Dir = repoDir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plain git commit with generated prepare-commit-msg hook failed: %v\n%s", err, string(output))
	}

	if _, err := os.Stat(editorMarker); !os.IsNotExist(err) {
		t.Fatalf("expected editor to be skipped, editor marker stat err = %v", err)
	}

	logCmd := exec.Command("git", "log", "--format=%s", "-1")
	logCmd.Dir = repoDir
	logOutput, err := logCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log failed: %v\n%s", err, string(logOutput))
	}
	if got := strings.TrimSpace(string(logOutput)); got != "chosen message" {
		t.Fatalf("commit subject = %q, want %q", got, "chosen message")
	}

	markerPath := filepath.Join(repoDir, ".git", "livereview_editor_override")
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("expected editor override marker to be cleaned up, stat err = %v", err)
	}

	configCmd := exec.Command("git", "config", "--local", "--get", "core.editor")
	configCmd.Dir = repoDir
	configOutput, err := configCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git config --local --get core.editor failed: %v\n%s", err, string(configOutput))
	}
	if got := strings.TrimSpace(string(configOutput)); got != wrapperScript {
		t.Fatalf("core.editor should remain pointed at the wrapper, got %q want %q", got, wrapperScript)
	}
}

func TestPrepareCommitMsgHookSetsNoopEditorForDirectLiveMessage(t *testing.T) {
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script command not available")
	}

	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	hooksDir := filepath.Join(repoDir, ".git", "myhooks")
	binDir := filepath.Join(tmpDir, "bin")
	editorScript := filepath.Join(tmpDir, "editor.sh")
	commitMsgFile := filepath.Join(repoDir, ".git", "COMMIT_EDITMSG")
	overrideMarker := filepath.Join(repoDir, ".git", "livereview_editor_override")

	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", hooksDir, err)
	}
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", binDir, err)
	}

	run := func(dir string, env []string, name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v failed: %v\n%s", name, args, err, string(out))
		}
	}

	run(tmpDir, nil, "git", "init", "--initial-branch=main", repoDir)
	run(repoDir, nil, "git", "config", "user.email", "test@example.com")
	run(repoDir, nil, "git", "config", "user.name", "Test User")

	prepareHook := GeneratePrepareCommitMsgHook(testTemplateConfig())
	prepareHookPath := filepath.Join(hooksDir, "prepare-commit-msg")
	if err := os.WriteFile(prepareHookPath, []byte(prepareHook), 0755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", prepareHookPath, err)
	}

	fakeLRCPath := filepath.Join(binDir, "lrc")
	fakeLRC := `#!/bin/sh
set -eu
if [ "$1" = "review" ]; then
  printf 'chosen message\n' > "$LRC_ACTIVE_COMMIT_MSG_FILE"
  exit 0
fi
exit 0
`
	if err := os.WriteFile(fakeLRCPath, []byte(fakeLRC), 0755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", fakeLRCPath, err)
	}

	editorBody := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(editorScript, []byte(editorBody), 0755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", editorScript, err)
	}
	run(repoDir, nil, "git", "config", "core.editor", editorScript)

	trackedFile := filepath.Join(repoDir, "note.txt")
	if err := os.WriteFile(trackedFile, []byte("hello\n"), 0644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", trackedFile, err)
	}
	run(repoDir, nil, "git", "add", "note.txt")

	env := append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmdText := "sh " + filepath.Join(hooksDir, "prepare-commit-msg") + " " + commitMsgFile
	cmd := exec.Command("script", "-qec", cmdText, "/dev/null")
	cmd.Dir = repoDir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare-commit-msg failed: %v\n%s", err, string(output))
	}

	msgData, readErr := os.ReadFile(commitMsgFile)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) error = %v", commitMsgFile, readErr)
	}
	if got := string(msgData); got != "chosen message\n" {
		t.Fatalf("commit message file = %q, want %q", got, "chosen message\n")
	}

	if _, err := os.Stat(overrideMarker); err != nil {
		t.Fatalf("expected editor override marker to exist, stat error = %v", err)
	}
}
