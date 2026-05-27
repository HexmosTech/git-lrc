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
