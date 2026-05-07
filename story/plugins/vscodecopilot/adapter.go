package vscodecopilot

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HexmosTech/git-lrc/storage"
	"github.com/HexmosTech/git-lrc/story"
)

const ProviderID = "vscode-copilot"

const (
	chatSessionMetadataReadLimit     = 256 * 1024
	chatSessionMetadataFallbackLimit = 1024 * 1024
	maxTranscriptLineBytes           = 32 * 1024 * 1024
)

type Adapter struct{}

type transcriptEnvelope struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	ID        string          `json:"id"`
	Timestamp string          `json:"timestamp"`
	ParentID  *string         `json:"parentId"`
}

type transcriptSessionStart struct {
	SessionID string `json:"sessionId"`
	StartTime string `json:"startTime"`
}

type transcriptMessage struct {
	MessageID     string               `json:"messageId"`
	Content       string               `json:"content"`
	ToolRequests  []transcriptToolCall `json:"toolRequests"`
	ReasoningText string               `json:"reasoningText"`
}

type transcriptToolCall struct {
	ToolCallID string `json:"toolCallId"`
	Name       string `json:"name"`
	Arguments  string `json:"arguments"`
	Type       string `json:"type"`
}

type chatSessionInitialState struct {
	CreationDate int64                 `json:"creationDate"`
	CustomTitle  string                `json:"customTitle"`
	SessionID    string                `json:"sessionId"`
	InputState   chatSessionInputState `json:"inputState"`
}

type chatSessionInputState struct {
	InputText string `json:"inputText"`
}

type chatSessionMetadata struct {
	SessionID       string
	WorkspaceID     string
	UserDataRoot    string
	SessionScope    string
	ChatSessionPath string
	DisplayTitle    string
	DraftInput      string
	CreatedAt       *time.Time
	UpdatedAt       *time.Time
	Warnings        []string
}

type sessionArtifacts struct {
	Summary story.SessionSummary
}

type transcriptAnalysis struct {
	Summary          story.SessionSummary
	EventTypes       []string
	VisibleMessages  int
	ToolRequestCount int
	Warnings         []string
	Events           []story.CommonChatEvent
	RawEventCount    int
}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func (a *Adapter) ID() string {
	return ProviderID
}

func (a *Adapter) Discover(opts story.DiscoverOptions) ([]story.Source, error) {
	roots, err := resolveUserDataRoots(opts.UserDataDir)
	if err != nil {
		return nil, err
	}

	var sources []story.Source
	for _, root := range roots {
		workspaceDirs, err := storage.ListVSCodeWorkspaceStorageDirs(root)
		if err != nil {
			return nil, err
		}
		globalStorageDir := filepath.Join(root, "globalStorage", "github.copilot-chat")
		for _, workspaceDir := range workspaceDirs {
			transcriptPaths, err := storage.ListCopilotTranscriptFiles(workspaceDir)
			if err != nil {
				return nil, err
			}
			chatSessionPaths, err := storage.ListVSCodeChatSessionFiles(workspaceDir)
			if err != nil {
				return nil, err
			}
			if len(transcriptPaths) == 0 && len(chatSessionPaths) == 0 {
				continue
			}
			sources = append(sources, story.Source{
				ProviderID:          ProviderID,
				UserDataRoot:        root,
				WorkspaceID:         filepath.Base(workspaceDir),
				WorkspaceStorageDir: workspaceDir,
				TranscriptDir:       filepath.Join(workspaceDir, "GitHub.copilot-chat", "transcripts"),
				ChatSessionDir:      filepath.Join(workspaceDir, "chatSessions"),
				GlobalStorageDir:    globalStorageDir,
				TranscriptCount:     len(transcriptPaths),
				ChatSessionCount:    len(chatSessionPaths),
			})
		}

		emptyWindowSessions, err := storage.ListVSCodeEmptyWindowChatSessionFiles(root)
		if err != nil {
			return nil, err
		}
		if len(emptyWindowSessions) > 0 {
			sources = append(sources, story.Source{
				ProviderID:       ProviderID,
				UserDataRoot:     root,
				ChatSessionDir:   filepath.Join(root, "globalStorage", "emptyWindowChatSessions"),
				GlobalStorageDir: globalStorageDir,
				ChatSessionCount: len(emptyWindowSessions),
			})
		}
	}

	sort.Slice(sources, func(i, j int) bool {
		if sources[i].UserDataRoot == sources[j].UserDataRoot {
			return sources[i].WorkspaceID < sources[j].WorkspaceID
		}
		return sources[i].UserDataRoot < sources[j].UserDataRoot
	})
	return sources, nil
}

func (a *Adapter) ListSessions(opts story.ListSessionsOptions) ([]story.SessionSummary, error) {
	artifacts, err := a.collectSessionArtifacts(opts.UserDataDir, opts.IncludeTranscriptSummary)
	if err != nil {
		return nil, err
	}

	summaries := make([]story.SessionSummary, 0, len(artifacts))
	for _, artifact := range artifacts {
		summaries = append(summaries, artifact.Summary)
	}

	sort.Slice(summaries, func(i, j int) bool {
		left := summaries[i]
		right := summaries[j]
		if left.UpdatedAt != nil && right.UpdatedAt != nil && !left.UpdatedAt.Equal(*right.UpdatedAt) {
			return left.UpdatedAt.After(*right.UpdatedAt)
		}
		if left.UpdatedAt != nil && right.UpdatedAt == nil {
			return true
		}
		if left.UpdatedAt == nil && right.UpdatedAt != nil {
			return false
		}
		if left.DisplayTitle != right.DisplayTitle {
			return left.DisplayTitle < right.DisplayTitle
		}
		return left.SessionID < right.SessionID
	})
	return summaries, nil
}

func (a *Adapter) InspectSession(opts story.InspectSessionOptions) (*story.SessionInspect, error) {
	selected, transcriptBytes, err := a.loadSession(opts.UserDataDir, opts.SessionID)
	if err != nil {
		return nil, err
	}
	if len(transcriptBytes) == 0 {
		return &story.SessionInspect{
			Summary:          selected,
			EventTypes:       nil,
			VisibleMessages:  0,
			ToolRequestCount: 0,
			Warnings:         append([]string(nil), selected.Warnings...),
		}, nil
	}

	analysis, err := analyzeTranscript(selected, transcriptBytes, false)
	if err != nil {
		return nil, err
	}
	return &story.SessionInspect{
		Summary:          analysis.Summary,
		EventTypes:       analysis.EventTypes,
		VisibleMessages:  analysis.VisibleMessages,
		ToolRequestCount: analysis.ToolRequestCount,
		Warnings:         analysis.Warnings,
	}, nil
}

func (a *Adapter) ExportSession(opts story.ExportSessionOptions) (*story.CommonChat, error) {
	selected, transcriptBytes, err := a.loadSession(opts.UserDataDir, opts.SessionID)
	if err != nil {
		return nil, err
	}
	if len(transcriptBytes) == 0 {
		return metadataOnlyCommonChat(selected, opts.Now), nil
	}
	analysis, err := analyzeTranscript(selected, transcriptBytes, true)
	if err != nil {
		return nil, err
	}
	return analysis.toCommonChat(opts.Now), nil
}

func (a *Adapter) loadSession(userDataDir, sessionID string) (story.SessionSummary, []byte, error) {
	safeSessionID, err := validateSessionID(sessionID)
	if err != nil {
		return story.SessionSummary{}, nil, err
	}
	artifacts, err := a.collectSessionArtifacts(userDataDir, false)
	if err != nil {
		return story.SessionSummary{}, nil, err
	}
	artifact, ok := artifacts[safeSessionID]
	if !ok {
		return story.SessionSummary{}, nil, fmt.Errorf("story session not found for provider %s: %s", ProviderID, sessionID)
	}
	if strings.TrimSpace(artifact.Summary.TranscriptPath) == "" {
		return artifact.Summary, nil, nil
	}
	transcriptBytes, err := storage.ReadCopilotTranscriptFile(artifact.Summary.TranscriptPath)
	if err != nil {
		return story.SessionSummary{}, nil, err
	}
	return artifact.Summary, transcriptBytes, nil
}

func (a *Adapter) collectSessionArtifacts(userDataDir string, includeTranscriptSummary bool) (map[string]*sessionArtifacts, error) {
	sources, err := a.Discover(story.DiscoverOptions{UserDataDir: userDataDir})
	if err != nil {
		return nil, err
	}

	transcriptsBySession := make(map[string][]transcriptArtifact)
	chatSessionsByID := make(map[string]chatSessionMetadata)
	for _, source := range sources {
		if source.WorkspaceStorageDir != "" {
			transcriptPaths, err := storage.ListCopilotTranscriptFiles(source.WorkspaceStorageDir)
			if err != nil {
				return nil, err
			}
			for _, transcriptPath := range transcriptPaths {
				sessionID := strings.TrimSuffix(filepath.Base(transcriptPath), filepath.Ext(transcriptPath))
				transcriptsBySession[sessionID] = append(transcriptsBySession[sessionID], transcriptArtifact{
					Source: source,
					Path:   transcriptPath,
				})
			}

			chatSessionPaths, err := storage.ListVSCodeChatSessionFiles(source.WorkspaceStorageDir)
			if err != nil {
				return nil, err
			}
			for _, chatSessionPath := range chatSessionPaths {
				metadata, err := readChatSessionMetadata(source, chatSessionPath, "workspace")
				if err != nil {
					return nil, err
				}
				chatSessionsByID[metadata.SessionID] = metadata
			}
			continue
		}

		emptyWindowPaths, err := storage.ListVSCodeEmptyWindowChatSessionFiles(source.UserDataRoot)
		if err != nil {
			return nil, err
		}
		for _, chatSessionPath := range emptyWindowPaths {
			metadata, err := readChatSessionMetadata(source, chatSessionPath, "empty-window")
			if err != nil {
				return nil, err
			}
			chatSessionsByID[metadata.SessionID] = metadata
		}
	}

	artifacts := make(map[string]*sessionArtifacts, len(chatSessionsByID))
	for sessionID, metadata := range chatSessionsByID {
		summary := buildSessionSummaryFromChatMetadata(metadata)
		transcripts := transcriptsBySession[sessionID]
		if len(transcripts) == 0 {
			summary.Warnings = uniqueStrings(append(summary.Warnings, "no Copilot transcript found for chat session metadata"))
			artifacts[sessionID] = &sessionArtifacts{Summary: summary}
			continue
		}

		selectedTranscript, err := selectLatestTranscriptArtifact(transcripts)
		if err != nil {
			return nil, err
		}
		summary.TranscriptPath = selectedTranscript.Path
		if !includeTranscriptSummary {
			artifacts[sessionID] = &sessionArtifacts{Summary: summary}
			continue
		}
		transcriptBytes, err := storage.ReadCopilotTranscriptFile(selectedTranscript.Path)
		if err != nil {
			return nil, err
		}
		analysis, err := analyzeTranscript(summaryWithTranscript(summary, selectedTranscript.Path), transcriptBytes, false)
		if err != nil {
			summary.TranscriptPath = selectedTranscript.Path
			summary.Warnings = uniqueStrings(append(summary.Warnings, err.Error()))
			artifacts[sessionID] = &sessionArtifacts{Summary: summary}
			continue
		}
		mergedSummary := mergeSessionSummary(summary, analysis.Summary)
		artifacts[sessionID] = &sessionArtifacts{Summary: mergedSummary}
	}

	for sessionID, transcripts := range transcriptsBySession {
		if _, ok := artifacts[sessionID]; ok {
			continue
		}
		selectedTranscript, err := selectLatestTranscriptArtifact(transcripts)
		if err != nil {
			return nil, err
		}
		summary := story.SessionSummary{
			ProviderID:     ProviderID,
			SessionID:      sessionID,
			WorkspaceID:    selectedTranscript.Source.WorkspaceID,
			UserDataRoot:   selectedTranscript.Source.UserDataRoot,
			TranscriptPath: selectedTranscript.Path,
			Warnings: []string{
				"no VS Code chat session metadata found for Copilot transcript",
			},
		}
		if !includeTranscriptSummary {
			artifacts[sessionID] = &sessionArtifacts{Summary: summary}
			continue
		}
		transcriptBytes, err := storage.ReadCopilotTranscriptFile(selectedTranscript.Path)
		if err != nil {
			return nil, err
		}
		analysis, err := analyzeTranscript(summary, transcriptBytes, false)
		if err != nil {
			summary.Warnings = uniqueStrings(append(summary.Warnings, err.Error()))
			artifacts[sessionID] = &sessionArtifacts{Summary: summary}
			continue
		}
		analysis.Summary.Warnings = uniqueStrings(append(summary.Warnings, analysis.Summary.Warnings...))
		artifacts[sessionID] = &sessionArtifacts{Summary: analysis.Summary}
	}

	return artifacts, nil
}

var (
	chatSessionCreationDatePattern     = regexp.MustCompile(`"creationDate":(\d+)`)
	chatSessionSessionIDPattern        = regexp.MustCompile(`"sessionId":"((?:\\.|[^"\\])*)"`)
	chatSessionCustomTitlePattern      = regexp.MustCompile(`"customTitle":"((?:\\.|[^"\\])*)"`)
	chatSessionCustomTitlePatchPattern = regexp.MustCompile(`\["customTitle"\],"v":"((?:\\.|[^"\\])*)"`)
	chatSessionRequestTextPattern      = regexp.MustCompile(`"message":\{"text":"((?:\\.|[^"\\])*)"`)
	chatSessionInputTextPattern        = regexp.MustCompile(`"inputText":"((?:\\.|[^"\\])*)"`)
	chatSessionInputTextPatchPattern   = regexp.MustCompile(`\["inputState","inputText"\],"v":"((?:\\.|[^"\\])*)"`)
)

func resolveUserDataRoots(explicitRoot string) ([]string, error) {
	if strings.TrimSpace(explicitRoot) != "" {
		exists, err := storage.PathExists(explicitRoot)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("VS Code user-data root does not exist: %s", explicitRoot)
		}
		return []string{explicitRoot}, nil
	}

	homeDir, err := osUserHomeDir()
	if err != nil {
		return nil, err
	}
	candidates := []string{
		filepath.Join(homeDir, ".vscode-server", "data", "User"),
		filepath.Join(homeDir, ".config", "Code", "User"),
		filepath.Join(homeDir, ".config", "Code - Insiders", "User"),
	}
	if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
		candidates = append(candidates,
			filepath.Join(appData, "Code", "User"),
			filepath.Join(appData, "Code - Insiders", "User"),
		)
	}
	if runtime.GOOS != "windows" {
		windowsCandidates, err := filepath.Glob("/mnt/[a-zA-Z]/Users/*/AppData/Roaming/Code/User")
		if err == nil {
			candidates = append(candidates, windowsCandidates...)
		}
		windowsInsiders, err := filepath.Glob("/mnt/[a-zA-Z]/Users/*/AppData/Roaming/Code - Insiders/User")
		if err == nil {
			candidates = append(candidates, windowsInsiders...)
		}
	}

	roots := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		exists, err := storage.PathExists(candidate)
		if err != nil {
			return nil, err
		}
		if exists {
			roots = append(roots, candidate)
		}
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("no VS Code user-data roots found in default locations")
	}
	return roots, nil
}

func validateSessionID(sessionID string) (string, error) {
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" {
		return "", fmt.Errorf("story session id is required")
	}
	if trimmed == "." || trimmed == ".." || strings.ContainsAny(trimmed, `/\\`) {
		return "", fmt.Errorf("invalid story session id: %s", sessionID)
	}
	return trimmed, nil
}

var osUserHomeDir = func() (string, error) {
	return os.UserHomeDir()
}

func summarizeTranscript(source story.Source, transcriptPath string, transcriptBytes []byte) (story.SessionSummary, error) {
	analysis, err := analyzeTranscript(story.SessionSummary{
		ProviderID:     ProviderID,
		SessionID:      strings.TrimSuffix(filepath.Base(transcriptPath), filepath.Ext(transcriptPath)),
		WorkspaceID:    source.WorkspaceID,
		UserDataRoot:   source.UserDataRoot,
		TranscriptPath: transcriptPath,
	}, transcriptBytes, false)
	if err != nil {
		return story.SessionSummary{}, err
	}
	return analysis.Summary, nil
}

type transcriptArtifact struct {
	Source story.Source
	Path   string
}

func readChatSessionMetadata(source story.Source, path string, sessionScope string) (chatSessionMetadata, error) {
	data, err := storage.ReadVSCodeChatSessionFilePrefix(path, chatSessionMetadataReadLimit)
	if err != nil {
		return chatSessionMetadata{}, err
	}
	metadata, err := parseChatSessionMetadataPrefix(path, data)
	if err != nil {
		return chatSessionMetadata{}, fmt.Errorf("parse VS Code chat session %s: %w", path, err)
	}
	if shouldReadLargerChatSessionPrefix(metadata) {
		fallbackData, fallbackErr := storage.ReadVSCodeChatSessionFilePrefix(path, chatSessionMetadataFallbackLimit)
		if fallbackErr == nil {
			fallbackMetadata, parseErr := parseChatSessionMetadataPrefix(path, fallbackData)
			if parseErr == nil {
				metadata = mergeChatSessionMetadata(metadata, fallbackMetadata)
			}
		}
	}
	modTime, err := storage.StatFileModTime(path)
	if err != nil {
		return chatSessionMetadata{}, err
	}
	metadata.UserDataRoot = source.UserDataRoot
	metadata.WorkspaceID = source.WorkspaceID
	metadata.ChatSessionPath = path
	metadata.SessionScope = sessionScope
	metadata.UpdatedAt = maxTime(metadata.UpdatedAt, &modTime)
	if metadata.CreatedAt == nil {
		metadata.CreatedAt = metadata.UpdatedAt
	}
	return metadata, nil
}

func shouldReadLargerChatSessionPrefix(metadata chatSessionMetadata) bool {
	return metadata.CreatedAt == nil && strings.TrimSpace(metadata.DisplayTitle) == "" && strings.TrimSpace(metadata.DraftInput) == ""
}

func mergeChatSessionMetadata(base, incoming chatSessionMetadata) chatSessionMetadata {
	merged := base
	if strings.TrimSpace(merged.SessionID) == "" {
		merged.SessionID = incoming.SessionID
	}
	if strings.TrimSpace(merged.DisplayTitle) == "" {
		merged.DisplayTitle = incoming.DisplayTitle
	}
	if strings.TrimSpace(merged.DraftInput) == "" {
		merged.DraftInput = incoming.DraftInput
	}
	merged.CreatedAt = minTime(merged.CreatedAt, incoming.CreatedAt)
	merged.UpdatedAt = maxTime(merged.UpdatedAt, incoming.UpdatedAt)
	merged.Warnings = uniqueStrings(append(append([]string{}, merged.Warnings...), incoming.Warnings...))
	return merged
}

func parseChatSessionMetadataPrefix(path string, data []byte) (chatSessionMetadata, error) {
	metadata := chatSessionMetadata{
		SessionID: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
	}
	if sessionID := extractChatSessionString(data, chatSessionSessionIDPattern); sessionID != "" {
		metadata.SessionID = sessionID
	}
	if creationDate := extractChatSessionInt64(data, chatSessionCreationDatePattern); creationDate > 0 {
		createdAt := time.UnixMilli(creationDate).UTC()
		metadata.CreatedAt = &createdAt
		metadata.UpdatedAt = &createdAt
	}
	if title := extractLatestChatSessionString(data, chatSessionCustomTitlePatchPattern); title != "" {
		metadata.DisplayTitle = title
	} else if title := extractLatestChatSessionString(data, chatSessionCustomTitlePattern); title != "" {
		metadata.DisplayTitle = title
	}
	if inputText := extractChatSessionString(data, chatSessionRequestTextPattern); inputText != "" {
		metadata.DraftInput = inputText
	} else if inputText := extractLatestNonEmptyChatSessionString(data, chatSessionInputTextPatchPattern); inputText != "" {
		metadata.DraftInput = inputText
	} else if inputText := extractLatestNonEmptyChatSessionString(data, chatSessionInputTextPattern); inputText != "" {
		metadata.DraftInput = inputText
	}
	if metadata.DisplayTitle == "" && metadata.DraftInput != "" {
		metadata.DisplayTitle = makePreview(metadata.DraftInput)
	}
	if metadata.UpdatedAt == nil {
		metadata.UpdatedAt = metadata.CreatedAt
	}
	metadata.Warnings = nil
	return metadata, nil
}

func extractChatSessionString(data []byte, pattern *regexp.Regexp) string {
	matches := pattern.FindSubmatch(data)
	if len(matches) < 2 {
		return ""
	}
	return decodeChatSessionStringMatch(matches[1])
}

func extractLatestChatSessionString(data []byte, pattern *regexp.Regexp) string {
	matches := pattern.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return ""
	}
	latest := matches[len(matches)-1]
	if len(latest) < 2 {
		return ""
	}
	return decodeChatSessionStringMatch(latest[1])
}

func extractLatestNonEmptyChatSessionString(data []byte, pattern *regexp.Regexp) string {
	matches := pattern.FindAllSubmatch(data, -1)
	for idx := len(matches) - 1; idx >= 0; idx-- {
		match := matches[idx]
		if len(match) < 2 {
			continue
		}
		decoded := decodeChatSessionStringMatch(match[1])
		if strings.TrimSpace(decoded) != "" {
			return decoded
		}
	}
	return ""
}

func decodeChatSessionStringMatch(raw []byte) string {
	quoted := make([]byte, 0, len(raw)+2)
	quoted = append(quoted, '"')
	quoted = append(quoted, raw...)
	quoted = append(quoted, '"')
	var decoded string
	if err := json.Unmarshal(quoted, &decoded); err != nil {
		return ""
	}
	return strings.TrimSpace(decoded)
}

func extractChatSessionInt64(data []byte, pattern *regexp.Regexp) int64 {
	matches := pattern.FindSubmatch(data)
	if len(matches) < 2 {
		return 0
	}
	value, err := strconv.ParseInt(string(matches[1]), 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func buildSessionSummaryFromChatMetadata(metadata chatSessionMetadata) story.SessionSummary {
	return story.SessionSummary{
		ProviderID:      ProviderID,
		SessionID:       metadata.SessionID,
		WorkspaceID:     metadata.WorkspaceID,
		UserDataRoot:    metadata.UserDataRoot,
		SessionScope:    metadata.SessionScope,
		ChatSessionPath: metadata.ChatSessionPath,
		DisplayTitle:    metadata.DisplayTitle,
		Preview:         makePreview(metadata.DraftInput),
		DraftInput:      metadata.DraftInput,
		StartedAt:       metadata.CreatedAt,
		UpdatedAt:       metadata.UpdatedAt,
		Warnings:        append([]string(nil), metadata.Warnings...),
	}
}

func summaryWithTranscript(summary story.SessionSummary, transcriptPath string) story.SessionSummary {
	summary.TranscriptPath = transcriptPath
	return summary
}

func mergeSessionSummary(base story.SessionSummary, transcript story.SessionSummary) story.SessionSummary {
	merged := base
	merged.TranscriptPath = transcript.TranscriptPath
	if merged.Preview == "" {
		merged.Preview = transcript.Preview
	}
	merged.EventCount = transcript.EventCount
	merged.VisibleMessageCount = transcript.VisibleMessageCount
	merged.ToolRequestCount = transcript.ToolRequestCount
	merged.StartedAt = minTime(merged.StartedAt, transcript.StartedAt)
	merged.UpdatedAt = maxTime(merged.UpdatedAt, transcript.UpdatedAt)
	merged.Warnings = uniqueStrings(append(append([]string{}, merged.Warnings...), transcript.Warnings...))
	return merged
}

func metadataOnlyCommonChat(summary story.SessionSummary, now time.Time) *story.CommonChat {
	chat := &story.CommonChat{
		SchemaVersion:   story.CommonChatSchemaVersion,
		ProviderID:      ProviderID,
		SessionID:       summary.SessionID,
		WorkspaceID:     summary.WorkspaceID,
		UserDataRoot:    summary.UserDataRoot,
		SessionScope:    summary.SessionScope,
		DisplayTitle:    summary.DisplayTitle,
		DraftInput:      summary.DraftInput,
		ChatSessionPath: summary.ChatSessionPath,
		SourcePath:      summary.ChatSessionPath,
		StartedAt:       summary.StartedAt,
		UpdatedAt:       summary.UpdatedAt,
		ExtractedAt:     now.UTC(),
		Warnings:        uniqueStrings(append([]string{}, summary.Warnings...)),
		Events:          nil,
		Raw: story.CommonChatRaw{
			ProviderFormat: "vscode.chatSessions.jsonl",
			EventCount:     0,
		},
	}
	return chat
}

func analyzeTranscript(base story.SessionSummary, transcriptBytes []byte, collectEvents bool) (*transcriptAnalysis, error) {
	analysis := &transcriptAnalysis{
		Summary: base,
	}
	analysis.Summary.EventCount = 0
	analysis.Summary.Warnings = nil
	typeCounts := map[string]int{}
	warnings := make([]string, 0)
	fallbackPreview := ""

	err := scanTranscript(transcriptBytes, func(index int, envelope transcriptEnvelope, rawLine []byte) error {
		typeCounts[envelope.Type]++
		analysis.Summary.EventCount++
		analysis.RawEventCount++
		timestamp, parseErr := parseTimestamp(envelope.Timestamp)
		if parseErr == nil {
			if analysis.Summary.StartedAt == nil || timestamp.Before(*analysis.Summary.StartedAt) {
				analysis.Summary.StartedAt = &timestamp
			}
			if analysis.Summary.UpdatedAt == nil || timestamp.After(*analysis.Summary.UpdatedAt) {
				analysis.Summary.UpdatedAt = &timestamp
			}
		}

		if collectEvents {
			event := story.CommonChatEvent{
				Index:    index,
				Type:     envelope.Type,
				Category: categorizeEvent(envelope.Type),
				Raw:      append([]byte(nil), rawLine...),
			}
			if envelope.ParentID != nil {
				event.ParentID = *envelope.ParentID
			}
			if timestamp, err := parseTimestamp(envelope.Timestamp); err == nil {
				event.Timestamp = &timestamp
			}
			analysis.Events = append(analysis.Events, event)
		}

		switch envelope.Type {
		case "session.start":
			var start transcriptSessionStart
			if err := json.Unmarshal(envelope.Data, &start); err != nil {
				return fmt.Errorf("failed to parse session.start event: %w", err)
			}
			if strings.TrimSpace(start.SessionID) != "" {
				analysis.Summary.SessionID = strings.TrimSpace(start.SessionID)
			}
			if startedAt, err := parseTimestamp(start.StartTime); err == nil {
				analysis.Summary.StartedAt = &startedAt
			}
			if collectEvents {
				analysis.Events[len(analysis.Events)-1].Message = nil
			}
		case "user.message", "assistant.message":
			var message transcriptMessage
			if err := json.Unmarshal(envelope.Data, &message); err != nil {
				return fmt.Errorf("failed to parse %s event: %w", envelope.Type, err)
			}
			analysis.VisibleMessages++
			analysis.Summary.VisibleMessageCount++
			analysis.ToolRequestCount += len(message.ToolRequests)
			analysis.Summary.ToolRequestCount += len(message.ToolRequests)
			if envelope.Type == "user.message" && analysis.Summary.Preview == "" {
				analysis.Summary.Preview = makePreview(message.Content)
			} else if envelope.Type == "assistant.message" && fallbackPreview == "" {
				fallbackPreview = makePreview(message.Content)
			}
			if strings.TrimSpace(message.ReasoningText) != "" {
				warnings = append(warnings, "transcript contains assistant reasoningText in raw provenance")
			}
			if collectEvents {
				messageRole := "assistant"
				if envelope.Type == "user.message" {
					messageRole = "user"
				}
				analysis.Events[len(analysis.Events)-1].Message = &story.CommonChatMessage{
					Role:      messageRole,
					MessageID: strings.TrimSpace(message.MessageID),
					Content:   strings.TrimSpace(message.Content),
				}
				if envelope.Type == "assistant.message" {
					tools := make([]story.CommonChatTool, 0, len(message.ToolRequests))
					for _, request := range message.ToolRequests {
						tools = append(tools, story.CommonChatTool{
							CallID:    strings.TrimSpace(request.ToolCallID),
							Name:      strings.TrimSpace(request.Name),
							Phase:     "requested",
							Arguments: strings.TrimSpace(request.Arguments),
						})
					}
					analysis.Events[len(analysis.Events)-1].Tools = tools
				}
			}
		case "tool.execution_start", "tool.execution_complete":
			if collectEvents {
				analysis.Events[len(analysis.Events)-1].Tools = parseGenericToolEvent(envelope.Data, envelope.Type)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if analysis.Summary.Preview == "" {
		analysis.Summary.Preview = fallbackPreview
	}

	analysis.EventTypes = make([]string, 0, len(typeCounts))
	for eventType := range typeCounts {
		analysis.EventTypes = append(analysis.EventTypes, eventType)
	}
	sort.Strings(analysis.EventTypes)

	analysis.Summary.Warnings = uniqueStrings(warnings)
	analysis.Warnings = analysis.Summary.Warnings
	return analysis, nil
}

func (a *transcriptAnalysis) toCommonChat(now time.Time) *story.CommonChat {
	chat := &story.CommonChat{
		SchemaVersion:   story.CommonChatSchemaVersion,
		ProviderID:      ProviderID,
		SessionID:       a.Summary.SessionID,
		WorkspaceID:     a.Summary.WorkspaceID,
		UserDataRoot:    a.Summary.UserDataRoot,
		SessionScope:    a.Summary.SessionScope,
		DisplayTitle:    a.Summary.DisplayTitle,
		DraftInput:      a.Summary.DraftInput,
		ChatSessionPath: a.Summary.ChatSessionPath,
		SourcePath:      a.Summary.TranscriptPath,
		StartedAt:       a.Summary.StartedAt,
		UpdatedAt:       a.Summary.UpdatedAt,
		ExtractedAt:     now.UTC(),
		Warnings:        append([]string{}, a.Summary.Warnings...),
		Events:          append([]story.CommonChatEvent(nil), a.Events...),
		Raw: story.CommonChatRaw{
			ProviderFormat: "vscode.chatSessions.jsonl merged with github.copilot-chat.transcript.jsonl",
			EventCount:     a.RawEventCount,
		},
	}
	chat.Warnings = uniqueStrings(chat.Warnings)
	return chat
}

func minTime(left, right *time.Time) *time.Time {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if left.Before(*right) {
		return left
	}
	return right
}

func maxTime(left, right *time.Time) *time.Time {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if left.After(*right) {
		return left
	}
	return right
}

func scanTranscript(transcriptBytes []byte, visit func(index int, envelope transcriptEnvelope, rawLine []byte) error) error {
	reader := bufio.NewReader(bytes.NewReader(transcriptBytes))
	index := 0
	lineNumber := 0
	for {
		rawLine, err := reader.ReadBytes('\n')
		if len(rawLine) > maxTranscriptLineBytes {
			return fmt.Errorf("failed to scan transcript: line %d exceeds %d bytes", lineNumber+1, maxTranscriptLineBytes)
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("failed to scan transcript: %w", err)
		}
		if len(rawLine) == 0 && errors.Is(err, io.EOF) {
			break
		}
		lineNumber++
		trimmed := bytes.TrimSpace(rawLine)
		if len(trimmed) != 0 {
			var envelope transcriptEnvelope
			if unmarshalErr := json.Unmarshal(trimmed, &envelope); unmarshalErr != nil {
				return fmt.Errorf("failed to parse transcript line %d: %w", lineNumber, unmarshalErr)
			}
			if visitErr := visit(index, envelope, trimmed); visitErr != nil {
				return visitErr
			}
			index++
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
	return nil
}

func selectLatestTranscriptArtifact(artifacts []transcriptArtifact) (transcriptArtifact, error) {
	if len(artifacts) == 0 {
		return transcriptArtifact{}, fmt.Errorf("no transcript artifacts available")
	}
	selected := artifacts[0]
	selectedTime, err := storage.StatFileModTime(selected.Path)
	if err != nil {
		return transcriptArtifact{}, err
	}
	for _, candidate := range artifacts[1:] {
		candidateTime, statErr := storage.StatFileModTime(candidate.Path)
		if statErr != nil {
			return transcriptArtifact{}, statErr
		}
		if candidateTime.After(selectedTime) {
			selected = candidate
			selectedTime = candidateTime
		}
	}
	return selected, nil
}

func parseGenericToolEvent(raw json.RawMessage, eventType string) []story.CommonChatTool {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	phase := strings.TrimPrefix(eventType, "tool.")
	callID, _ := payload["toolCallId"].(string)
	name, _ := payload["toolName"].(string)
	if name == "" {
		name, _ = payload["name"].(string)
	}
	if callID == "" && name == "" {
		return nil
	}
	arguments := ""
	if rawArgs, ok := payload["arguments"]; ok {
		if encoded, err := json.Marshal(rawArgs); err == nil {
			arguments = string(encoded)
		}
	}
	return []story.CommonChatTool{{
		CallID:    strings.TrimSpace(callID),
		Name:      strings.TrimSpace(name),
		Phase:     phase,
		Arguments: arguments,
	}}
}

func categorizeEvent(eventType string) string {
	switch eventType {
	case "session.start":
		return "session"
	case "user.message", "assistant.message":
		return "message"
	case "tool.execution_start", "tool.execution_complete":
		return "tool"
	case "assistant.turn_start", "assistant.turn_end":
		return "turn"
	default:
		return "system"
	}
}

func parseTimestamp(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
}

func makePreview(content string) string {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if utf8.RuneCountInString(trimmed) <= 96 {
		return trimmed
	}
	runes := []rune(trimmed)
	return string(runes[:93]) + "..."
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
