package vscodecopilot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/HexmosTech/git-lrc/story"
)

func TestAdapterDiscoverListsTranscriptSources(t *testing.T) {
	userDataDir := createUserDataDirWithFixtures(
		t,
		map[string]string{
			"workspace-a/session-basic": readFixture(t, "basic.jsonl"),
			"workspace-a/session-tool":  readFixture(t, "tool.jsonl"),
		},
		map[string]string{
			"workspace-a/session-basic": buildChatSessionFixture("session-basic", "Git Story Architecture", "Explain the git story architecture"),
			"workspace-a/session-tool":  buildChatSessionFixture("session-tool", "Tooling Session", "Inspect the read_file tool usage"),
		},
		nil,
	)

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
	if sources[0].ChatSessionCount != 2 {
		t.Fatalf("expected chat session count 2, got %d", sources[0].ChatSessionCount)
	}
}

func TestAdapterListSessionsUsesMetadataFirstSummary(t *testing.T) {
	userDataDir := createUserDataDirWithFixtures(
		t,
		map[string]string{
			"workspace-a/session-basic": readFixture(t, "basic.jsonl"),
		},
		map[string]string{
			"workspace-a/session-basic": buildChatSessionFixture("session-basic", "Git Story Architecture", "Explain the git story architecture"),
		},
		nil,
	)

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
	if sessions[0].DisplayTitle != "Git Story Architecture" {
		t.Fatalf("unexpected display title: %q", sessions[0].DisplayTitle)
	}
	if sessions[0].ChatSessionPath == "" {
		t.Fatal("expected chat session path to be set")
	}
	if sessions[0].TranscriptPath == "" {
		t.Fatal("expected transcript path to be set")
	}
	if sessions[0].EventCount != 0 {
		t.Fatalf("expected metadata-first list path to skip transcript event counts, got %d", sessions[0].EventCount)
	}
	if sessions[0].StartedAt == nil || sessions[0].UpdatedAt == nil {
		t.Fatal("expected chat session timestamps to be available from metadata")
	}
	if len(sessions[0].Warnings) != 0 {
		t.Fatalf("expected no warnings for matched metadata/transcript pair, got %v", sessions[0].Warnings)
	}
}

func TestAdapterListSessionsUsesLatestChatSessionInputPatch(t *testing.T) {
	userDataDir := createUserDataDirWithFixtures(
		t,
		map[string]string{
			"workspace-a/session-progressive": readFixture(t, "basic.jsonl"),
		},
		map[string]string{
			"workspace-a/session-progressive": strings.Join([]string{
				`{"kind":0,"v":{"creationDate":1778141958563,"sessionId":"session-progressive","inputState":{"inputText":""}}}`,
				`{"kind":1,"k":["inputState","inputText"],"v":"I want you to build a \"plugin architecture\" for "}`,
				`{"kind":1,"k":["inputState","inputText"],"v":"I want you to build a \"plugin architecture\" for extracting chat history from claude, vscode copilot, opencode, and such systems."}`,
				`{"kind":1,"k":["customTitle"],"v":"Plugin architecture for chat history"}`,
				`{"kind":2,"k":["requests"],"v":[{"requestId":"request-1","message":{"text":"I want you to build a \"plugin architecture\" for extracting chat history from claude, vscode copilot, opencode, and such systems.\n\nThis will be the foundation for building \"git story\"."}}]}`,
				`{"kind":1,"k":["inputState","inputText"],"v":""}`,
			}, "\n"),
		},
		nil,
	)

	adapter := NewAdapter()
	sessions, err := adapter.ListSessions(story.ListSessionsOptions{UserDataDir: userDataDir})
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].DraftInput != "I want you to build a \"plugin architecture\" for extracting chat history from claude, vscode copilot, opencode, and such systems.\n\nThis will be the foundation for building \"git story\"." {
		t.Fatalf("unexpected draft input: %q", sessions[0].DraftInput)
	}
	if sessions[0].Preview != makePreview(sessions[0].DraftInput) {
		t.Fatalf("unexpected preview: %q", sessions[0].Preview)
	}
	if sessions[0].DisplayTitle != "Plugin architecture for chat history" {
		t.Fatalf("unexpected display title: %q", sessions[0].DisplayTitle)
	}
	chat, err := adapter.ExportSession(story.ExportSessionOptions{
		UserDataDir: userDataDir,
		SessionID:   "session-progressive",
		Now:         time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("ExportSession returned error: %v", err)
	}
	if chat.DraftInput != sessions[0].DraftInput {
		t.Fatalf("expected exported draft input to match session summary, got %q", chat.DraftInput)
	}
}

func TestAdapterListSessionsCanIncludeTranscriptSummary(t *testing.T) {
	userDataDir := createUserDataDirWithFixtures(
		t,
		map[string]string{
			"workspace-a/session-basic": readFixture(t, "basic.jsonl"),
		},
		map[string]string{
			"workspace-a/session-basic": buildChatSessionFixture("session-basic", "Git Story Architecture", "Explain the git story architecture"),
		},
		nil,
	)

	adapter := NewAdapter()
	sessions, err := adapter.ListSessions(story.ListSessionsOptions{UserDataDir: userDataDir, IncludeTranscriptSummary: true})
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].EventCount != 3 {
		t.Fatalf("expected event count 3, got %d", sessions[0].EventCount)
	}
}

func TestAdapterListSessionsIncludesEmptyWindowChatSessions(t *testing.T) {
	userDataDir := createUserDataDirWithFixtures(
		t,
		nil,
		nil,
		map[string]string{
			"session-empty": buildChatSessionFixture("session-empty", "Loose Planning", "Plan the plugin architecture outside a workspace"),
		},
	)

	adapter := NewAdapter()
	sessions, err := adapter.ListSessions(story.ListSessionsOptions{UserDataDir: userDataDir})
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].SessionScope != "empty-window" {
		t.Fatalf("unexpected session scope: %q", sessions[0].SessionScope)
	}
	if sessions[0].EventCount != 0 {
		t.Fatalf("expected no transcript-backed events, got %d", sessions[0].EventCount)
	}
	if len(sessions[0].Warnings) == 0 {
		t.Fatal("expected warning for transcript-less chat session")
	}
}

func TestAdapterListSessionsHandlesLargeChatSessionRecord(t *testing.T) {
	largeToolOutput := strings.Repeat("x", 11*1024*1024)
	userDataDir := createUserDataDirWithFixtures(
		t,
		map[string]string{
			"workspace-a/session-large": readFixture(t, "basic.jsonl"),
		},
		map[string]string{
			"workspace-a/session-large": buildLargeChatSessionFixture("session-large", "Large Session", "Show me the session summary", largeToolOutput),
		},
		nil,
	)

	adapter := NewAdapter()
	sessions, err := adapter.ListSessions(story.ListSessionsOptions{UserDataDir: userDataDir})
	if err != nil {
		t.Fatalf("ListSessions returned error for large chat session record: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].DisplayTitle != "Large Session" {
		t.Fatalf("unexpected display title: %q", sessions[0].DisplayTitle)
	}
	if sessions[0].Preview != "Show me the session summary" {
		t.Fatalf("unexpected preview: %q", sessions[0].Preview)
	}
	if sessions[0].EventCount != 0 {
		t.Fatalf("expected metadata-first list path to avoid transcript parsing, got %d", sessions[0].EventCount)
	}
}

func TestAdapterExportSessionHandlesLargeTranscriptRecord(t *testing.T) {
	largeContent := strings.Repeat("x", 11*1024*1024)
	encodedContent, err := json.Marshal(largeContent)
	if err != nil {
		t.Fatalf("failed to marshal large transcript content: %v", err)
	}
	transcript := strings.Join([]string{
		`{"type":"session.start","data":{"sessionId":"session-large-transcript","startTime":"2026-05-07T09:00:00Z"},"timestamp":"2026-05-07T09:00:00Z"}`,
		`{"type":"user.message","data":{"messageId":"msg-1","content":` + string(encodedContent) + `},"timestamp":"2026-05-07T09:00:01Z"}`,
	}, "\n")
	userDataDir := createUserDataDirWithFixtures(
		t,
		map[string]string{
			"workspace-a/session-large-transcript": transcript,
		},
		map[string]string{
			"workspace-a/session-large-transcript": buildChatSessionFixture("session-large-transcript", "Large Transcript", "Summarize the huge output"),
		},
		nil,
	)

	adapter := NewAdapter()
	chat, err := adapter.ExportSession(story.ExportSessionOptions{
		UserDataDir: userDataDir,
		SessionID:   "session-large-transcript",
		Now:         time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("ExportSession returned error for large transcript record: %v", err)
	}
	if len(chat.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(chat.Events))
	}
	if chat.Raw.EventCount != 2 {
		t.Fatalf("expected raw event count 2, got %d", chat.Raw.EventCount)
	}
}

func TestAdapterExportSessionPreservesRawEventsAndToolRequests(t *testing.T) {
	userDataDir := createUserDataDirWithFixtures(
		t,
		map[string]string{
			"workspace-a/session-tool": readFixture(t, "tool.jsonl"),
		},
		map[string]string{
			"workspace-a/session-tool": buildChatSessionFixture("session-tool", "Tooling Session", "Inspect the read_file tool usage"),
		},
		nil,
	)

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
	if chat.DisplayTitle != "Tooling Session" {
		t.Fatalf("unexpected display title: %q", chat.DisplayTitle)
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
	userDataDir := createUserDataDirWithFixtures(
		t,
		map[string]string{
			"workspace-a/session-bad": readFixture(t, "malformed.jsonl"),
		},
		map[string]string{
			"workspace-a/session-bad": buildChatSessionFixture("session-bad", "Broken Transcript", "This transcript should fail to parse"),
		},
		nil,
	)

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
	userDataDir := createUserDataDirWithFixtures(
		t,
		map[string]string{
			"workspace-a/session-tool": readFixture(t, "tool.jsonl"),
		},
		map[string]string{
			"workspace-a/session-tool": buildChatSessionFixture("session-tool", "Tooling Session", "Inspect the read_file tool usage"),
		},
		nil,
	)

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

func createUserDataDirWithFixtures(t *testing.T, transcripts map[string]string, chatSessions map[string]string, emptyWindow map[string]string) string {
	t.Helper()
	root := t.TempDir()
	globalStorageDir := filepath.Join(root, "globalStorage", "github.copilot-chat")
	if err := os.MkdirAll(globalStorageDir, 0o755); err != nil {
		t.Fatalf("failed to create global storage dir: %v", err)
	}
	for key, content := range transcripts {
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
	for key, content := range chatSessions {
		parts := strings.Split(key, "/")
		if len(parts) != 2 {
			t.Fatalf("invalid chat session fixture key: %s", key)
		}
		workspaceID := parts[0]
		sessionID := parts[1]
		chatSessionDir := filepath.Join(root, "workspaceStorage", workspaceID, "chatSessions")
		if err := os.MkdirAll(chatSessionDir, 0o755); err != nil {
			t.Fatalf("failed to create chat session dir: %v", err)
		}
		chatSessionPath := filepath.Join(chatSessionDir, sessionID+".jsonl")
		if err := os.WriteFile(chatSessionPath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write chat session fixture: %v", err)
		}
	}
	if len(emptyWindow) > 0 {
		emptyWindowDir := filepath.Join(root, "globalStorage", "emptyWindowChatSessions")
		if err := os.MkdirAll(emptyWindowDir, 0o755); err != nil {
			t.Fatalf("failed to create empty-window chat session dir: %v", err)
		}
		for sessionID, content := range emptyWindow {
			chatSessionPath := filepath.Join(emptyWindowDir, sessionID+".jsonl")
			if err := os.WriteFile(chatSessionPath, []byte(content), 0o644); err != nil {
				t.Fatalf("failed to write empty-window chat session fixture: %v", err)
			}
		}
	}
	return root
}

func buildChatSessionFixture(sessionID string, title string, inputText string) string {
	return mustBuildJSONLLines(
		struct {
			Kind int `json:"kind"`
			V    struct {
				CreationDate int64  `json:"creationDate"`
				SessionID    string `json:"sessionId"`
				InputState   struct {
					InputText string `json:"inputText"`
				} `json:"inputState"`
			} `json:"v"`
		}{
			Kind: 0,
			V: struct {
				CreationDate int64  `json:"creationDate"`
				SessionID    string `json:"sessionId"`
				InputState   struct {
					InputText string `json:"inputText"`
				} `json:"inputState"`
			}{
				CreationDate: 1778141958563,
				SessionID:    sessionID,
				InputState: struct {
					InputText string `json:"inputText"`
				}{
					InputText: inputText,
				},
			},
		},
		struct {
			Kind int      `json:"kind"`
			K    []string `json:"k"`
			V    string   `json:"v"`
		}{
			Kind: 1,
			K:    []string{"customTitle"},
			V:    title,
		},
		struct {
			Kind int   `json:"kind"`
			K    []any `json:"k"`
			V    []struct {
				Timestamp int64 `json:"timestamp"`
			} `json:"v"`
		}{
			Kind: 2,
			K:    []any{"requests"},
			V: []struct {
				Timestamp int64 `json:"timestamp"`
			}{
				{Timestamp: 1778143457308},
			},
		},
	)
}

func buildLargeChatSessionFixture(sessionID string, title string, inputText string, toolOutput string) string {
	return mustBuildJSONLLines(
		struct {
			Kind int `json:"kind"`
			V    struct {
				CreationDate int64  `json:"creationDate"`
				CustomTitle  string `json:"customTitle"`
				SessionID    string `json:"sessionId"`
				InputState   struct {
					InputText string `json:"inputText"`
				} `json:"inputState"`
			} `json:"v"`
		}{
			Kind: 0,
			V: struct {
				CreationDate int64  `json:"creationDate"`
				CustomTitle  string `json:"customTitle"`
				SessionID    string `json:"sessionId"`
				InputState   struct {
					InputText string `json:"inputText"`
				} `json:"inputState"`
			}{
				CreationDate: 1778141958563,
				CustomTitle:  title,
				SessionID:    sessionID,
				InputState: struct {
					InputText string `json:"inputText"`
				}{
					InputText: inputText,
				},
			},
		},
		struct {
			Kind int   `json:"kind"`
			K    []any `json:"k"`
			V    []struct {
				Kind  string `json:"kind"`
				Value string `json:"value"`
			} `json:"v"`
		}{
			Kind: 2,
			K:    []any{"requests", 1, "response"},
			V: []struct {
				Kind  string `json:"kind"`
				Value string `json:"value"`
			}{
				{Kind: "thinking", Value: toolOutput},
			},
		},
	)
}

func mustBuildJSONLLines(records ...any) string {
	lines := make([]string, 0, len(records))
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			panic(err)
		}
		lines = append(lines, string(encoded))
	}
	return strings.Join(lines, "\n")
}
