package appcore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HexmosTech/git-lrc/internal/reviewapi"
	"github.com/HexmosTech/git-lrc/storage"
	"github.com/HexmosTech/git-lrc/story"
)

func TestBuildStoryAttachmentMarkdownUsesDraftInputAndAssistantMessages(t *testing.T) {
	now := time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC)
	chat := &story.CommonChat{
		ProviderID:   "vscode-copilot",
		SessionID:    "session-1",
		DisplayTitle: "Fix story attachment flow",
		DraftInput:   "Attach the correct Copilot conversation to this commit.",
		Events: []story.CommonChatEvent{
			{Message: &story.CommonChatMessage{Role: "user", Content: "internal tool prompt"}},
			{Message: &story.CommonChatMessage{Role: "assistant", Content: "First assistant answer."}},
			{Message: &story.CommonChatMessage{Role: "assistant", Content: "First assistant answer."}},
			{Tools: []story.CommonChatTool{{Name: "grep_search", Phase: "call"}}},
			{Message: &story.CommonChatMessage{Role: "assistant", Content: "Second assistant answer."}},
		},
	}

	markdown := buildStoryAttachmentMarkdown(chat, now, "abc123tree")

	if !strings.Contains(markdown, "Attach the correct Copilot conversation to this commit.") {
		t.Fatalf("expected markdown to include draft input, got:\n%s", markdown)
	}
	if strings.Contains(markdown, "internal tool prompt") {
		t.Fatalf("expected markdown to avoid transcript user prompt fallback when draft input exists, got:\n%s", markdown)
	}
	if strings.Count(markdown, "First assistant answer.") != 1 {
		t.Fatalf("expected duplicate assistant messages to be deduplicated, got:\n%s", markdown)
	}
	if !strings.Contains(markdown, "Second assistant answer.") {
		t.Fatalf("expected markdown to include assistant responses, got:\n%s", markdown)
	}
	if strings.Contains(markdown, "grep_search") {
		t.Fatalf("expected markdown to exclude tool call details, got:\n%s", markdown)
	}
}

func TestFinalizePendingStoryAttachmentMigratesAttestation(t *testing.T) {
	repoRoot := t.TempDir()
	hooksDir := filepath.Join(repoRoot, ".git-hooks-empty")
	runGit(t, repoRoot, "init")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("create empty hooks dir: %v", err)
	}
	runGit(t, repoRoot, "config", "user.email", "story-test@example.com")
	runGit(t, repoRoot, "config", "user.name", "Story Test")
	runGit(t, repoRoot, "config", "core.hooksPath", hooksDir)

	writeTestFile(t, filepath.Join(repoRoot, "code.txt"), "initial\n")
	runGit(t, repoRoot, "add", "code.txt")
	runGit(t, repoRoot, "commit", "-m", "initial")

	writeTestFile(t, filepath.Join(repoRoot, "code.txt"), "updated\n")
	runGit(t, repoRoot, "add", "code.txt")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWD)
	}()

	reviewTreeHash, err := reviewapi.CurrentTreeHash()
	if err != nil {
		t.Fatalf("current tree hash: %v", err)
	}
	payload := attestationPayload{
		Action:           "reviewed",
		Iterations:       4,
		PriorAICovPct:    83,
		PriorReviewCount: 9,
	}
	if _, err := writeAttestationFullForTree(reviewTreeHash, payload); err != nil {
		t.Fatalf("write attestation: %v", err)
	}

	gitDir, err := reviewapi.ResolveGitDir()
	if err != nil {
		t.Fatalf("resolve git dir: %v", err)
	}
	markdownRelativePath := filepath.ToSlash(filepath.Join(".lrc", reviewTreeHash+".md"))
	metadataPath := storage.StoryAttachmentMetadataPath(gitDir)
	if err := storage.WriteStoryAttachmentState(metadataPath, storage.StoryAttachmentState{
		ProviderID:           "vscode-copilot",
		SessionID:            "session-1",
		ReviewTreeHash:       reviewTreeHash,
		MarkdownRelativePath: markdownRelativePath,
		DisplayTitle:         "Attach Story",
		AttachedAt:           time.Date(2026, time.May, 7, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("write story attachment state: %v", err)
	}
	if err := storage.WriteStoryAttachmentMarkdown(filepath.Join(repoRoot, filepath.FromSlash(markdownRelativePath)), []byte("# Git Story\n")); err != nil {
		t.Fatalf("write story attachment markdown: %v", err)
	}

	if err := finalizePendingStoryAttachment(false); err != nil {
		t.Fatalf("finalize story attachment: %v", err)
	}

	state, err := storage.ReadStoryAttachmentState(metadataPath)
	if err != nil {
		t.Fatalf("read story attachment state: %v", err)
	}
	if state != nil {
		t.Fatalf("expected story attachment state to be cleared, got %#v", state)
	}

	finalTreeHash := strings.TrimSpace(runGit(t, repoRoot, "write-tree"))
	if finalTreeHash == reviewTreeHash {
		t.Fatalf("expected final tree hash to change after staging story attachment")
	}

	oldPayload, err := readAttestationForTree(reviewTreeHash)
	if err != nil {
		t.Fatalf("read old attestation: %v", err)
	}
	if oldPayload != nil {
		t.Fatalf("expected old attestation to be removed, got %#v", oldPayload)
	}

	newPayload, err := readAttestationForTree(finalTreeHash)
	if err != nil {
		t.Fatalf("read final attestation: %v", err)
	}
	if newPayload == nil {
		t.Fatal("expected final attestation to exist")
	}
	if *newPayload != payload {
		t.Fatalf("expected final attestation payload %#v, got %#v", payload, *newPayload)
	}

	stagedFiles := runGit(t, repoRoot, "diff", "--cached", "--name-only")
	if !strings.Contains(stagedFiles, markdownRelativePath) {
		t.Fatalf("expected staged files to include story attachment %q, got %q", markdownRelativePath, stagedFiles)
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
