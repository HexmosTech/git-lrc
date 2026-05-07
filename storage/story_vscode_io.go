package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("failed to stat path %s: %w", path, err)
}

func ListVSCodeWorkspaceStorageDirs(userDataRoot string) ([]string, error) {
	workspaceStorageRoot := filepath.Join(userDataRoot, "workspaceStorage")
	entries, err := os.ReadDir(workspaceStorageRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read VS Code workspace storage root %s: %w", workspaceStorageRoot, err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		paths = append(paths, filepath.Join(workspaceStorageRoot, entry.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}

func ListCopilotTranscriptFiles(workspaceStorageDir string) ([]string, error) {
	transcriptDir := filepath.Join(workspaceStorageDir, "GitHub.copilot-chat", "transcripts")
	return listJSONLFiles(transcriptDir, "Copilot transcript dir")
}

func ListVSCodeChatSessionFiles(workspaceStorageDir string) ([]string, error) {
	chatSessionDir := filepath.Join(workspaceStorageDir, "chatSessions")
	return listJSONLFiles(chatSessionDir, "VS Code chat session dir")
}

func ListVSCodeEmptyWindowChatSessionFiles(userDataRoot string) ([]string, error) {
	emptyWindowDir := filepath.Join(userDataRoot, "globalStorage", "emptyWindowChatSessions")
	return listJSONLFiles(emptyWindowDir, "VS Code empty-window chat session dir")
}

func listJSONLFiles(dir string, label string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read %s %s: %w", label, dir, err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}

func ReadCopilotTranscriptFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read Copilot transcript file %s: %w", path, err)
	}
	return data, nil
}

func ReadVSCodeChatSessionFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read VS Code chat session file %s: %w", path, err)
	}
	return data, nil
}

func ReadVSCodeChatSessionFilePrefix(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open VS Code chat session file %s: %w", path, err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return nil, fmt.Errorf("failed to read VS Code chat session file prefix %s: %w", path, err)
	}
	return data, nil
}

func StatFileModTime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to stat file %s: %w", path, err)
	}
	return info.ModTime().UTC(), nil
}
