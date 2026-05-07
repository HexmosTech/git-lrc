package vscodecopilot

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HexmosTech/git-lrc/storage"
	"github.com/HexmosTech/git-lrc/story"
)

const ProviderID = "vscode-copilot"

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
			if len(transcriptPaths) == 0 {
				continue
			}
			sources = append(sources, story.Source{
				ProviderID:          ProviderID,
				UserDataRoot:        root,
				WorkspaceID:         filepath.Base(workspaceDir),
				WorkspaceStorageDir: workspaceDir,
				TranscriptDir:       filepath.Join(workspaceDir, "GitHub.copilot-chat", "transcripts"),
				GlobalStorageDir:    globalStorageDir,
				TranscriptCount:     len(transcriptPaths),
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
	sources, err := a.Discover(story.DiscoverOptions{UserDataDir: opts.UserDataDir})
	if err != nil {
		return nil, err
	}

	summaries := make([]story.SessionSummary, 0)
	for _, source := range sources {
		transcriptPaths, err := storage.ListCopilotTranscriptFiles(source.WorkspaceStorageDir)
		if err != nil {
			return nil, err
		}
		for _, transcriptPath := range transcriptPaths {
			transcriptBytes, err := storage.ReadCopilotTranscriptFile(transcriptPath)
			if err != nil {
				return nil, err
			}
			summary, err := summarizeTranscript(source, transcriptPath, transcriptBytes)
			if err != nil {
				summary = story.SessionSummary{
					ProviderID:     ProviderID,
					SessionID:      strings.TrimSuffix(filepath.Base(transcriptPath), filepath.Ext(transcriptPath)),
					WorkspaceID:    source.WorkspaceID,
					UserDataRoot:   source.UserDataRoot,
					TranscriptPath: transcriptPath,
					Warnings:       []string{err.Error()},
				}
			}
			summaries = append(summaries, summary)
		}
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].UpdatedAt == nil {
			return false
		}
		if summaries[j].UpdatedAt == nil {
			return true
		}
		return summaries[i].UpdatedAt.After(*summaries[j].UpdatedAt)
	})
	return summaries, nil
}

func (a *Adapter) InspectSession(opts story.InspectSessionOptions) (*story.SessionInspect, error) {
	selected, transcriptBytes, err := a.loadSession(opts.UserDataDir, opts.SessionID)
	if err != nil {
		return nil, err
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

	sources, err := a.Discover(story.DiscoverOptions{UserDataDir: userDataDir})
	if err != nil {
		return story.SessionSummary{}, nil, err
	}
	for _, source := range sources {
		candidatePath := filepath.Join(source.TranscriptDir, safeSessionID+".jsonl")
		exists, err := storage.PathExists(candidatePath)
		if err != nil {
			return story.SessionSummary{}, nil, err
		}
		if !exists {
			continue
		}
		data, err := storage.ReadCopilotTranscriptFile(candidatePath)
		if err != nil {
			return story.SessionSummary{}, nil, err
		}
		return story.SessionSummary{
			ProviderID:     ProviderID,
			SessionID:      safeSessionID,
			WorkspaceID:    source.WorkspaceID,
			UserDataRoot:   source.UserDataRoot,
			TranscriptPath: candidatePath,
		}, data, nil
	}
	return story.SessionSummary{}, nil, fmt.Errorf("story session not found for provider %s: %s", ProviderID, sessionID)
}

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
			analysis.ToolRequestCount += len(message.ToolRequests)
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
		SchemaVersion: story.CommonChatSchemaVersion,
		ProviderID:    ProviderID,
		SessionID:     a.Summary.SessionID,
		WorkspaceID:   a.Summary.WorkspaceID,
		UserDataRoot:  a.Summary.UserDataRoot,
		SourcePath:    a.Summary.TranscriptPath,
		StartedAt:     a.Summary.StartedAt,
		UpdatedAt:     a.Summary.UpdatedAt,
		ExtractedAt:   now.UTC(),
		Warnings:      append([]string{}, a.Summary.Warnings...),
		Events:        append([]story.CommonChatEvent(nil), a.Events...),
		Raw: story.CommonChatRaw{
			ProviderFormat: "github.copilot-chat.transcript.jsonl",
			EventCount:     a.RawEventCount,
		},
	}
	chat.Warnings = uniqueStrings(chat.Warnings)
	return chat
}

func scanTranscript(transcriptBytes []byte, visit func(index int, envelope transcriptEnvelope, rawLine []byte) error) error {
	scanner := bufio.NewScanner(bytes.NewReader(transcriptBytes))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	index := 0
	for scanner.Scan() {
		rawLine := bytes.TrimSpace(scanner.Bytes())
		if len(rawLine) == 0 {
			continue
		}
		var envelope transcriptEnvelope
		if err := json.Unmarshal(rawLine, &envelope); err != nil {
			return fmt.Errorf("failed to parse transcript line %d: %w", index+1, err)
		}
		if err := visit(index, envelope, rawLine); err != nil {
			return err
		}
		index++
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan transcript: %w", err)
	}
	return nil
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
