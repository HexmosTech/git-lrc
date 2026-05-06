package appcore

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/HexmosTech/git-lrc/internal/decisionflow"
	"github.com/HexmosTech/git-lrc/internal/reviewapi"
	"github.com/urfave/cli/v2"
)

type decisionExecutionContext struct {
	precommit          bool
	verbose            bool
	initialMsg         string
	commitMsgPath      string
	diffContent        []byte
	reviewID           string
	attestationWritten *bool
}

func normalizeDecisionCode(code int) int {
	return code
}

func precommitExitCodeForDecision(code int) int {
	code = normalizeDecisionCode(code)
	if code == decisionflow.DecisionVouch {
		return decisionflow.DecisionSkip
	}
	return code
}

func executeDecision(code int, message string, push bool, ctx decisionExecutionContext) error {
	code = normalizeDecisionCode(code)
	switch code {
	case decisionflow.DecisionAbort:
		syncedPrintln("\n❌ Commit aborted by user")
		return cli.Exit("", decisionflow.DecisionAbort)
	case decisionflow.DecisionCommit:
		if ctx.precommit {
			syncedPrintln("\n✅ Proceeding with commit")
		}
		finalMsg := strings.TrimSpace(message)
		if finalMsg == "" {
			finalMsg = strings.TrimSpace(ctx.initialMsg)
		}
		if ctx.precommit {
			if ctx.commitMsgPath != "" {
				if strings.TrimSpace(finalMsg) != "" {
					if err := persistCommitMessage(ctx.commitMsgPath, finalMsg); err != nil {
						syncedFprintf(os.Stderr, "Warning: failed to store commit message: %v\n", err)
					}
				} else {
					_ = clearCommitMessageFile(ctx.commitMsgPath)
				}
			}

			if push {
				if err := persistPushRequest(ctx.commitMsgPath); err != nil {
					syncedFprintf(os.Stderr, "Warning: failed to store push request: %v\n", err)
				}
			} else {
				_ = clearPushRequest(ctx.commitMsgPath)
			}

			return cli.Exit("", decisionflow.DecisionCommit)
		}
		if err := runCommitAndMaybePush(finalMsg, push, ctx.verbose); err != nil {
			return err
		}
		return nil
	case decisionflow.DecisionSkip:
		syncedPrintln("\n⏭️  Review skipped, proceeding with commit")
		if err := ensureAttestation("skipped", ctx.verbose, ctx.attestationWritten); err != nil {
			return err
		}
		if ctx.precommit {
			_ = clearCommitMessageFile(ctx.commitMsgPath)
			_ = clearPushRequest(ctx.commitMsgPath)
			return cli.Exit("", decisionflow.DecisionSkip)
		}
		if err := runCommitAndMaybePush(strings.TrimSpace(message), push, ctx.verbose); err != nil {
			return err
		}
		return nil
	case decisionflow.DecisionVouch:
		syncedPrintln("\n✅ Vouched, proceeding with commit")
		if err := recordCoverageAndAttest("vouched", ctx.diffContent, ctx.reviewID, ctx.verbose, ctx.attestationWritten); err != nil {
			return fmt.Errorf("vouch failed: %w", err)
		}
		if ctx.precommit {
			_ = clearCommitMessageFile(ctx.commitMsgPath)
			_ = clearPushRequest(ctx.commitMsgPath)
			return cli.Exit("", decisionflow.DecisionSkip)
		}
		if err := runCommitAndMaybePush(strings.TrimSpace(message), push, ctx.verbose); err != nil {
			return err
		}
		return nil
	case decisionflow.DecisionHandoff:
		syncedPrintln("\n🤖 Handing off to Claude Code...")

		gitDir, err := reviewapi.ResolveGitDir()
		if err != nil {
			return fmt.Errorf("failed to resolve git directory: %w", err)
		}

		reviewDir := filepath.Join(gitDir, "lrc", "reviews", ctx.reviewID)
		if err := os.MkdirAll(reviewDir, 0755); err != nil {
			return fmt.Errorf("failed to create review directory: %w", err)
		}

		jsonPath := filepath.Join(reviewDir, "review_findings.json")
		if err := os.WriteFile(jsonPath, []byte(message), 0644); err != nil {
			return fmt.Errorf("failed to write review findings: %w", err)
		}

		promptMsg := fmt.Sprintf(ClaudeHandoffPromptTemplate, jsonPath)
		cmdArgs := []string{promptMsg}
		syncedPrintln("🚀 Running: claude code")

		cmd := exec.Command("claude", cmdArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("claude agent failed: %w", err)
		}

		return cli.Exit("", 0) // exit cleanly after fix
	default:
		return fmt.Errorf("invalid decision code: %d", code)
	}
}
