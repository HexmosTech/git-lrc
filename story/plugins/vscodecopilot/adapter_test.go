package vscodecopilot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/HexmosTech/git-lrc/story"
)

func TestAdapterDiscoverListsTranscriptSources(t *testing.T) {
	userDataDir := createUserDataDirWithFixtures(t, map[string]string{
		"workspace-a/session-basic": readFixture(t, "basic.jsonl"),
		"workspace-a/session-tool":  readFixture(t, "tool.jsonl"),
	})

	adapter := NewAdapter()
	sources, err := adapter.Discover(story.DiscoverOptions{UserDataDir: userDataDir})
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}
	if sources[0].WorkspaceID != "workspace-a" {
		t.Fatalf("unexpected workspace id: %q", sources[0].WorkspaceID)
	}
	if sources[0].TranscriptCount != 2 {
		t.Fatalf("expected transcript count 2, got %d", sources[0].TranscriptCount)
	}
}

func TestAdapterListSessionsSummarizesTranscript(t *testing.T) {
	userDataDir := createUserDataDirWithFixtures(t, map[string]string{
		"workspace-a/session-basic": readFixture(t, "basic.jsonl"),
	})

	adapter := NewAdapter()
	sessions, err := adapter.ListSessions(story.ListSessionsOptions{UserDataDir: userDataDir})
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].SessionID != "session-basic" {
		t.Fatalf("unexpected session id: %q", sessions[0].SessionID)
	}
	if sessions[0].Preview != "Explain the git story architecture" {
		t.Fatalf("unexpected preview: %q", sessions[0].Preview)
	}
	if sessions[0].EventCount != 3 {
		t.Fatalf("expected event count 3, got %d", sessions[0].EventCount)
	}
}

func TestAdapterExportSessionPreservesRawEventsAndToolRequests(t *testing.T) {
	userDataDir := createUserDataDirWithFixtures(t, map[string]string{
		"workspace-a/session-tool": readFixture(t, "tool.jsonl"),
	})

	adapter := NewAdapter()
	now := time.Date(2026, time.May, 7, 10, 0, 0, 0, time.UTC)
	chat, err := adapter.ExportSession(story.ExportSessionOptions{
		UserDataDir: userDataDir,
		SessionID:   "session-tool",
		Now:         now,
	})
	if err != nil {
		t.Fatalf("ExportSession returned error: %v", err)
	}
	if chat.SchemaVersion != story.CommonChatSchemaVersion {
		t.Fatalf("unexpected schema version: %q", chat.SchemaVersion)
	}
	if len(chat.Events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(chat.Events))
	}
	if chat.ExtractedAt != now {
		t.Fatalf("unexpected extracted_at: %s", chat.ExtractedAt)
	}
	assistantEvent := chat.Events[2]
	if assistantEvent.Message == nil || assistantEvent.Message.Role != "assistant" {
		t.Fatalf("expected assistant message at index 2")
	}
	if len(assistantEvent.Tools) != 1 {
		t.Fatalf("expected one assistant tool request, got %d", len(assistantEvent.Tools))
	}
	if assistantEvent.Tools[0].Name != "read_file" {
		t.Fatalf("unexpected tool name: %q", assistantEvent.Tools[0].Name)
	}
	if chat.Raw.EventCount != 5 {
		t.Fatalf("unexpected raw event count: %d", chat.Raw.EventCount)
	}
	if !strings.Contains(string(assistantEvent.Raw), "reasoningText") {
		t.Fatalf("expected raw assistant event to preserve reasoningText")
	}
	if len(chat.Warnings) == 0 {
		t.Fatalf("expected warning about reasoningText provenance")
	}
}

func TestAdapterExportSessionMalformedFails(t *testing.T) {
	userDataDir := createUserDataDirWithFixtures(t, map[string]string{
		"workspace-a/session-bad": readFixture(t, "malformed.jsonl"),
	})

	adapter := NewAdapter()
	_, err := adapter.ExportSession(story.ExportSessionOptions{
		UserDataDir: userDataDir,
		SessionID:   "session-bad",
		Now:         time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected malformed transcript export to fail")
	}
	if !strings.Contains(err.Error(), "failed to parse transcript line") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterExportSessionRejectsTraversalSessionID(t *testing.T) {
	userDataDir := createUserDataDirWithFixtures(t, map[string]string{
		"workspace-a/session-tool": readFixture(t, "tool.jsonl"),
	})

	adapter := NewAdapter()
	_, err := adapter.ExportSession(story.ExportSessionOptions{
		UserDataDir: userDataDir,
		SessionID:   "../session-tool",
		Now:         time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected traversal session id to fail")
	}
	if !strings.Contains(err.Error(), "invalid story session id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMakePreviewPreservesUTF8(t *testing.T) {
	input := strings.Repeat("é", 100)
	preview := makePreview(input)
	if !utf8.ValidString(preview) {
		t.Fatal("expected preview to remain valid UTF-8")
	}
	if !strings.HasSuffix(preview, "...") {
		t.Fatalf("expected preview ellipsis, got %q", preview)
	}
	if utf8.RuneCountInString(preview) != 96 {
		t.Fatalf("expected preview length 96 runes including ellipsis, got %d", utf8.RuneCountInString(preview))
	}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return string(data)
}

func createUserDataDirWithFixtures(t *testing.T, fixtures map[string]string) string {
	t.Helper()
	root := t.TempDir()
	globalStorageDir := filepath.Join(root, "globalStorage", "github.copilot-chat")
	if err := os.MkdirAll(globalStorageDir, 0o755); err != nil {
		t.Fatalf("failed to create global storage dir: %v", err)
	}
	for key, content := range fixtures {
		parts := strings.Split(key, "/")
		if len(parts) != 2 {
			t.Fatalf("invalid fixture key: %s", key)
		}
		workspaceID := parts[0]
		sessionID := parts[1]
		transcriptDir := filepath.Join(root, "workspaceStorage", workspaceID, "GitHub.copilot-chat", "transcripts")
		if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
			t.Fatalf("failed to create transcript dir: %v", err)
		}
		transcriptPath := filepath.Join(transcriptDir, sessionID+".jsonl")
		if err := os.WriteFile(transcriptPath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write transcript fixture: %v", err)
		}
	}
	return root
}
