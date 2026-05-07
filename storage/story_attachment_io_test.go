package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoryAttachmentStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "story_attachment.json")
	want := StoryAttachmentState{
		ProviderID:           "vscode-copilot",
		SessionID:            "session-1",
		ReviewTreeHash:       "abc123",
		MarkdownRelativePath: ".lrc/abc123.md",
		DisplayTitle:         "Story session",
		AttachedAt:           time.Date(2026, time.May, 7, 12, 30, 0, 0, time.UTC),
	}

	if err := WriteStoryAttachmentState(path, want); err != nil {
		t.Fatalf("write story attachment state: %v", err)
	}

	got, err := ReadStoryAttachmentState(path)
	if err != nil {
		t.Fatalf("read story attachment state: %v", err)
	}
	if got == nil {
		t.Fatal("expected story attachment state to exist")
	}
	if *got != want {
		t.Fatalf("expected %#v, got %#v", want, *got)
	}
}
