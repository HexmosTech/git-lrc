package appcore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/HexmosTech/git-lrc/internal/decisionflow"
	"github.com/urfave/cli/v2"
)

func TestExecuteDecisionDeferredCommitPersistsArtifacts(t *testing.T) {
	gitDir := t.TempDir()
	commitMsgPath := filepath.Join(gitDir, commitMessageFile)
	attestationWritten := false

	err := executeDecision(decisionflow.DecisionCommit, "feat: blocking review", true, decisionExecutionContext{
		deferCommit:        true,
		commitMsgPath:      commitMsgPath,
		initialMsg:         "feat: initial",
		attestationWritten: &attestationWritten,
	})

	if err != nil {
		exitErr, ok := err.(cli.ExitCoder)
		if !ok {
			t.Fatalf("executeDecision() error = %T, want cli.ExitCoder or nil", err)
		}
		if exitErr.ExitCode() != decisionflow.DecisionCommit {
			t.Fatalf("exit code = %d, want %d", exitErr.ExitCode(), decisionflow.DecisionCommit)
		}
	}

	data, readErr := os.ReadFile(commitMsgPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) error = %v", commitMsgPath, readErr)
	}
	if got := string(data); got != "feat: blocking review\n" {
		t.Fatalf("commit message override = %q, want %q", got, "feat: blocking review\n")
	}

	pushMarkerPath := filepath.Join(gitDir, pushRequestFile)
	if _, statErr := os.Stat(pushMarkerPath); statErr != nil {
		t.Fatalf("expected push marker at %q: %v", pushMarkerPath, statErr)
	}
}

func TestExecuteDecisionDeferredCommitWritesLiveCommitMessageWhenProvided(t *testing.T) {
	gitDir := t.TempDir()
	overridePath := filepath.Join(gitDir, commitMessageFile)
	livePath := filepath.Join(gitDir, "COMMIT_EDITMSG")
	attestationWritten := false

	err := executeDecision(decisionflow.DecisionCommit, "feat: live path", false, decisionExecutionContext{
		deferCommit:        true,
		commitMsgPath:      overridePath,
		liveCommitMsgPath:  livePath,
		initialMsg:         "feat: initial",
		attestationWritten: &attestationWritten,
	})

	if err != nil {
		exitErr, ok := err.(cli.ExitCoder)
		if !ok {
			t.Fatalf("executeDecision() error = %T, want cli.ExitCoder or nil", err)
		}
		if exitErr.ExitCode() != decisionflow.DecisionCommit {
			t.Fatalf("exit code = %d, want %d", exitErr.ExitCode(), decisionflow.DecisionCommit)
		}
	}

	data, readErr := os.ReadFile(livePath)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) error = %v", livePath, readErr)
	}
	if got := string(data); got != "feat: live path\n" {
		t.Fatalf("live commit message = %q, want %q", got, "feat: live path\n")
	}

	if _, statErr := os.Stat(overridePath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no override file at %q, stat err = %v", overridePath, statErr)
	}
}

func TestParseTerminalDecision(t *testing.T) {
	initialMsg := "feat: test initial message"

	tests := []struct {
		name        string
		input       string
		wantCode    int
		wantMsg     string
		wantPush    bool
		wantMatched bool
	}{
		{
			name:        "empty input (Enter) commits with initial message",
			input:       "",
			wantCode:    0,
			wantMsg:     initialMsg,
			wantPush:    false,
			wantMatched: true,
		},
		{
			name:        "spaces input (Enter with whitespace) commits with initial message",
			input:       "   \n",
			wantCode:    0,
			wantMsg:     initialMsg,
			wantPush:    false,
			wantMatched: true,
		},
		{
			name:        "input 's' skips",
			input:       "s",
			wantCode:    2,
			wantMsg:     initialMsg,
			wantPush:    false,
			wantMatched: true,
		},
		{
			name:        "input 'S' with whitespace skips",
			input:       "  S  \n",
			wantCode:    2,
			wantMsg:     initialMsg,
			wantPush:    false,
			wantMatched: true,
		},
		{
			name:        "input 'v' vouches",
			input:       "v",
			wantCode:    4,
			wantMsg:     initialMsg,
			wantPush:    false,
			wantMatched: true,
		},
		{
			name:        "input 'V' vouches",
			input:       "V",
			wantCode:    4,
			wantMsg:     initialMsg,
			wantPush:    false,
			wantMatched: true,
		},
		{
			name:        "input 'a' aborts",
			input:       "a",
			wantCode:    1,
			wantMsg:     "",
			wantPush:    false,
			wantMatched: true,
		},
		{
			name:        "input 'A' aborts",
			input:       "A",
			wantCode:    1,
			wantMsg:     "",
			wantPush:    false,
			wantMatched: true,
		},
		{
			name:        "unknown input does not match",
			input:       "invalid",
			wantCode:    0,
			wantMsg:     "",
			wantPush:    false,
			wantMatched: false,
		},
		{
			name:        "unknown single character does not match",
			input:       "x",
			wantCode:    0,
			wantMsg:     "",
			wantPush:    false,
			wantMatched: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, msg, push, matched := parseTerminalDecision(tc.input, initialMsg)
			if code != tc.wantCode {
				t.Errorf("code = %d, want %d", code, tc.wantCode)
			}
			if msg != tc.wantMsg {
				t.Errorf("msg = %q, want %q", msg, tc.wantMsg)
			}
			if push != tc.wantPush {
				t.Errorf("push = %v, want %v", push, tc.wantPush)
			}
			if matched != tc.wantMatched {
				t.Errorf("matched = %v, want %v", matched, tc.wantMatched)
			}
		})
	}
}
