package hooks

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Meta tracks hook path ownership so uninstall can restore prior state.
type Meta struct {
	Path     string `json:"path"`
	PrevPath string `json:"prev_path,omitempty"`
	SetByLRC bool   `json:"set_by_lrc"`
}

func DefaultGlobalHooksPath(defaultDir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, defaultDir), nil
}

func CurrentHooksPath() (string, error) {
	cmd := exec.Command("git", "config", "--global", "--get", "core.hooksPath")
	out, err := cmd.Output()
	if err != nil {
		return "", nil
	}

	return strings.TrimSpace(string(out)), nil
}

func CurrentLocalHooksPath(repoRoot string) (string, error) {
	cmd := exec.Command("git", "config", "--local", "--get", "core.hooksPath")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", nil
	}

	return strings.TrimSpace(string(out)), nil
}

func ResolveRepoHooksPath(repoRoot string) (string, error) {
	localPath, _ := CurrentLocalHooksPath(repoRoot)
	if localPath == "" {
		return filepath.Join(repoRoot, ".git", "hooks"), nil
	}
	if filepath.IsAbs(localPath) {
		return localPath, nil
	}
	return filepath.Join(repoRoot, localPath), nil
}

func SetGlobalHooksPath(path string) error {
	cmd := exec.Command("git", "config", "--global", "core.hooksPath", path)
	return cmd.Run()
}

func UnsetGlobalHooksPath() error {
	cmd := exec.Command("git", "config", "--global", "--unset", "core.hooksPath")
	return cmd.Run()
}

func MetaPath(hooksPath, metaFilename string) string {
	return filepath.Join(hooksPath, metaFilename)
}

func WriteMeta(hooksPath, metaFilename string, meta Meta) {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return
	}

	_ = os.MkdirAll(hooksPath, 0755)
	_ = os.WriteFile(MetaPath(hooksPath, metaFilename), data, 0644)
}

func ReadMeta(hooksPath, metaFilename string) (*Meta, error) {
	data, err := os.ReadFile(MetaPath(hooksPath, metaFilename))
	if err != nil {
		return nil, err
	}

	var meta Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

func RemoveMeta(hooksPath, metaFilename string) error {
	return os.Remove(MetaPath(hooksPath, metaFilename))
}

func PathsEqual(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return a == b
	}
	if absA == absB {
		return true
	}
	realA, errA := filepath.EvalSymlinks(absA)
	realB, errB := filepath.EvalSymlinks(absB)
	if errA != nil || errB != nil {
		return absA == absB
	}
	return realA == realB
}

func CleanEmptyHooksDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	if len(entries) == 0 {
		_ = os.Remove(dir)
	}
}

func HookHasManagedSection(path, markerBegin string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	return strings.Contains(string(content), markerBegin)
}
