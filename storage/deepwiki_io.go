package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/configpath"
)

// DeepwikiEntry is one cached wiki ref for a repository, used by the branch/
// tag/commit browser.
type DeepwikiEntry struct {
	Ref        string
	ModifiedAt time.Time
}

func deepwikiBaseDir() (string, error) {
	dataDir, err := configpath.ResolveLRCDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "deepwiki"), nil
}

// deepwikiRepoDir returns the per-repo cache directory, keyed by a short hash
// of the absolute repo path plus the basename for human readability.
func deepwikiRepoDir(repoPath string) (string, error) {
	base, err := deepwikiBaseDir()
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(abs))
	hash := hex.EncodeToString(sum[:])[:12]
	return filepath.Join(base, filepath.Base(abs)+"-"+hash), nil
}

// deepwikiCachePath returns the cache file path for a repo at a ref. The ref
// is path-escaped so branch names like "feature/foo" round-trip losslessly.
func deepwikiCachePath(repoPath, ref string) (string, error) {
	dir, err := deepwikiRepoDir(repoPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, url.PathEscape(ref)+".json"), nil
}

// DeepwikiSave persists a wiki artifact (JSON bytes) for a repo at a ref.
func DeepwikiSave(repoPath, ref string, data []byte) error {
	path, err := deepwikiCachePath(repoPath, ref)
	if err != nil {
		return err
	}
	if err := MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("deepwiki: creating cache dir: %w", err)
	}
	return WriteFileAtomically(path, data, 0o644)
}

// DeepwikiLoad reads a cached wiki artifact. Returns (nil, os.ErrNotExist) when
// absent.
func DeepwikiLoad(repoPath, ref string) ([]byte, error) {
	path, err := deepwikiCachePath(repoPath, ref)
	if err != nil {
		return nil, err
	}
	data, err := ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	return data, nil
}

// DeepwikiExists reports whether a cached wiki exists for a repo at a ref.
func DeepwikiExists(repoPath, ref string) (bool, error) {
	path, err := deepwikiCachePath(repoPath, ref)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// DeepwikiDelete removes a cached wiki for a repo at a ref.
func DeepwikiDelete(repoPath, ref string) error {
	path, err := deepwikiCachePath(repoPath, ref)
	if err != nil {
		return err
	}
	if err := Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// DeepwikiList returns cached refs for a repo, newest first.
func DeepwikiList(repoPath string) ([]DeepwikiEntry, error) {
	dir, err := deepwikiRepoDir(repoPath)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []DeepwikiEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw := strings.TrimSuffix(e.Name(), ".json")
		ref, err := url.PathUnescape(raw)
		if err != nil {
			ref = raw
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, DeepwikiEntry{Ref: ref, ModifiedAt: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModifiedAt.After(out[j].ModifiedAt) })
	return out, nil
}
