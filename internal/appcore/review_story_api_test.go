package appcore

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	"github.com/HexmosTech/git-lrc/story"
)

func TestBuildReviewStoryCommitContextUsesLatestCommitAndChangedFiles(t *testing.T) {
	original := reviewStoryRunGitCommand
	defer func() { reviewStoryRunGitCommand = original }()

	reviewStoryRunGitCommand = func(args ...string) ([]byte, error) {
		return []byte("abc123def456\n2026-05-07T09:15:00Z\n"), nil
	}

	now := time.Date(2026, time.May, 7, 11, 0, 0, 0, time.UTC)
	state := &ReviewState{
		Files: []reviewmodel.DiffReviewFileResult{
			{FilePath: "internal/staticserve/static/app.js"},
			{FilePath: "internal/appcore/review_story_api.go"},
			{FilePath: "internal/staticserve/static/app.js"},
		},
	}

	context := buildReviewStoryCommitContext(state, now)

	if context.HeadCommit != "abc123def456" {
		t.Fatalf("unexpected head commit: %q", context.HeadCommit)
	}
	if context.WindowStart == nil || !context.WindowStart.Equal(time.Date(2026, time.May, 7, 9, 15, 0, 0, time.UTC)) {
		t.Fatalf("unexpected window start: %#v", context.WindowStart)
	}
	if !context.WindowEnd.Equal(now) {
		t.Fatalf("unexpected window end: %s", context.WindowEnd)
	}
	if len(context.ChangedFiles) != 2 {
		t.Fatalf("expected deduped changed files, got %v", context.ChangedFiles)
	}
}

func TestScoreStorySessionAgainstCommitWindowPrefersOverlap(t *testing.T) {
	windowStart := time.Date(2026, time.May, 7, 9, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, time.May, 7, 11, 0, 0, 0, time.UTC)
	startedAt := time.Date(2026, time.May, 7, 9, 30, 0, 0, time.UTC)
	updatedAt := time.Date(2026, time.May, 7, 10, 45, 0, 0, time.UTC)

	score, reasons, overlaps := scoreStorySessionAgainstCommitWindow(story.SessionSummary{
		StartedAt: &startedAt,
		UpdatedAt: &updatedAt,
	}, reviewStoryCommitContext{
		WindowStart: &windowStart,
		WindowEnd:   windowEnd,
	}, windowEnd)

	if !overlaps {
		t.Fatal("expected session to overlap commit window")
	}
	if score <= 0 {
		t.Fatalf("expected positive score, got %d", score)
	}
	if len(reasons) == 0 {
		t.Fatal("expected overlap reasons")
	}
}

func TestScoreStorySessionSummaryAgainstChangedFilesFindsMatches(t *testing.T) {
	score, reasons, matchedFiles := scoreStorySessionSummaryAgainstChangedFiles(story.SessionSummary{
		DisplayTitle:    "Update review story ranking",
		Preview:         "Need to touch internal/appcore/review_story_api.go before commit",
		DraftInput:      "Also inspect app.js for the Story page loading path",
		ChatSessionPath: "/tmp/workspaceStorage/abc/chatSessions/session-1.jsonl",
		TranscriptPath:  "/tmp/workspaceStorage/abc/GitHub.copilot-chat/transcripts/session-1.jsonl",
	}, []string{
		"internal/appcore/review_story_api.go",
		"internal/staticserve/static/app.js",
	})

	if score <= 0 {
		t.Fatalf("expected positive score, got %d", score)
	}
	if len(matchedFiles) != 2 {
		t.Fatalf("expected both files to match, got %v", matchedFiles)
	}
	if len(reasons) == 0 {
		t.Fatal("expected summary-based match reasons")
	}
}

func TestAllowLocalStoryRequestAllowsLoopbackHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://localhost/api/story/sessions", nil)
	req.Host = "127.0.0.1:9999"
	req.Header.Set("Origin", "http://localhost:9999")
	w := httptest.NewRecorder()

	if !allowLocalStoryRequest(w, req) {
		t.Fatal("expected localhost story request to be allowed")
	}
}

func TestAllowLocalStoryRequestRejectsRemoteHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/story/sessions", nil)
	req.Host = "10.0.0.15:9999"
	w := httptest.NewRecorder()

	if allowLocalStoryRequest(w, req) {
		t.Fatal("expected remote story request to be rejected")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden status, got %d", w.Code)
	}
}
