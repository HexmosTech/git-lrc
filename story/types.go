package story

import (
	"encoding/json"
	"time"
)

const CommonChatSchemaVersion = "commonchat.v1alpha1"

type DiscoverOptions struct {
	UserDataDir string
}

type ListSessionsOptions struct {
	UserDataDir string
}

type InspectSessionOptions struct {
	UserDataDir string
	SessionID   string
}

type ExportSessionOptions struct {
	UserDataDir string
	SessionID   string
	Now         time.Time
}

type Source struct {
	ProviderID          string `json:"provider_id"`
	UserDataRoot        string `json:"user_data_root"`
	WorkspaceID         string `json:"workspace_id,omitempty"`
	WorkspaceStorageDir string `json:"workspace_storage_dir,omitempty"`
	TranscriptDir       string `json:"transcript_dir,omitempty"`
	GlobalStorageDir    string `json:"global_storage_dir,omitempty"`
	TranscriptCount     int    `json:"transcript_count"`
}

type SessionSummary struct {
	ProviderID     string     `json:"provider_id"`
	SessionID      string     `json:"session_id"`
	WorkspaceID    string     `json:"workspace_id,omitempty"`
	UserDataRoot   string     `json:"user_data_root,omitempty"`
	TranscriptPath string     `json:"transcript_path"`
	Preview        string     `json:"preview,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
	EventCount     int        `json:"event_count"`
	Warnings       []string   `json:"warnings,omitempty"`
}

type SessionInspect struct {
	Summary          SessionSummary `json:"summary"`
	EventTypes       []string       `json:"event_types"`
	VisibleMessages  int            `json:"visible_messages"`
	ToolRequestCount int            `json:"tool_request_count"`
	Warnings         []string       `json:"warnings,omitempty"`
}

type CommonChat struct {
	SchemaVersion string            `json:"schema_version"`
	ProviderID    string            `json:"provider_id"`
	SessionID     string            `json:"session_id"`
	WorkspaceID   string            `json:"workspace_id,omitempty"`
	UserDataRoot  string            `json:"user_data_root,omitempty"`
	SourcePath    string            `json:"source_path"`
	StartedAt     *time.Time        `json:"started_at,omitempty"`
	UpdatedAt     *time.Time        `json:"updated_at,omitempty"`
	ExtractedAt   time.Time         `json:"extracted_at"`
	Warnings      []string          `json:"warnings,omitempty"`
	Events        []CommonChatEvent `json:"events"`
	Raw           CommonChatRaw     `json:"raw"`
}

type CommonChatRaw struct {
	ProviderFormat string `json:"provider_format"`
	EventCount     int    `json:"event_count"`
}

type CommonChatEvent struct {
	Index     int                `json:"index"`
	Type      string             `json:"type"`
	Category  string             `json:"category"`
	Timestamp *time.Time         `json:"timestamp,omitempty"`
	ParentID  string             `json:"parent_id,omitempty"`
	Message   *CommonChatMessage `json:"message,omitempty"`
	Tools     []CommonChatTool   `json:"tools,omitempty"`
	Raw       json.RawMessage    `json:"raw"`
}

type CommonChatMessage struct {
	Role      string `json:"role"`
	MessageID string `json:"message_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

type CommonChatTool struct {
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Phase     string `json:"phase,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type Provider interface {
	ID() string
	Discover(DiscoverOptions) ([]Source, error)
	ListSessions(ListSessionsOptions) ([]SessionSummary, error)
	InspectSession(InspectSessionOptions) (*SessionInspect, error)
	ExportSession(ExportSessionOptions) (*CommonChat, error)
}
