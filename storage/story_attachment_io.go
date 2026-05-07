package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

type StoryAttachmentState struct {
	ProviderID           string    `json:"provider_id"`
	SessionID            string    `json:"session_id"`
	ReviewTreeHash       string    `json:"review_tree_hash"`
	MarkdownRelativePath string    `json:"markdown_relative_path"`
	DisplayTitle         string    `json:"display_title,omitempty"`
	AttachedAt           time.Time `json:"attached_at"`
}

func StoryAttachmentMetadataPath(gitDir string) string {
	return filepath.Join(gitDir, "lrc", "story_attachment.json")
}

func StoryAttachmentMarkdownPath(repoRoot, reviewTreeHash string) string {
	return filepath.Join(repoRoot, ".lrc", fmt.Sprintf("%s.md", strings.TrimSpace(reviewTreeHash)))
}

func ReadStoryAttachmentState(path string) (*StoryAttachmentState, error) {
	data, err := ReadPendingUpdateStateBytes(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read story attachment state %s: %w", path, err)
	}

	var state StoryAttachmentState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse story attachment state %s: %w", path, err)
	}

	return &state, nil
}

func WriteStoryAttachmentState(path string, state StoryAttachmentState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal story attachment state: %w", err)
	}

	if err := WriteFileAtomically(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write story attachment state %s: %w", path, err)
	}

	return nil
}

func RemoveStoryAttachmentState(path string) error {
	if err := Remove(path); err != nil {
		return fmt.Errorf("failed to remove story attachment state %s: %w", path, err)
	}
	return nil
}

func WriteStoryAttachmentMarkdown(path string, data []byte) error {
	if err := WriteFileAtomically(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write story attachment markdown %s: %w", path, err)
	}
	return nil
}

func RemoveStoryAttachmentMarkdown(path string) error {
	if err := Remove(path); err != nil {
		return fmt.Errorf("failed to remove story attachment markdown %s: %w", path, err)
	}
	return nil
}
