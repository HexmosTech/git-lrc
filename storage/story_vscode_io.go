package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	entries, err := os.ReadDir(transcriptDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read Copilot transcript dir %s: %w", transcriptDir, err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		paths = append(paths, filepath.Join(transcriptDir, entry.Name()))
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
