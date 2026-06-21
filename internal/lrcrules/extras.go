package lrcrules

import (
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/HexmosTech/git-lrc/storage"
)

// maxExtraFileSize caps the size of any single file under .lrc/ that gets
// read into memory and bundled into the review zip. .lrc/ is meant to hold
// small text configs (rules, ignore patterns, policy); anything larger is
// almost certainly accidental and is skipped with a warning rather than
// silently inflating every review's payload.
const maxExtraFileSize = 1 << 20 // 1 MiB

// CollectZipExtras walks .lrc/ under repoRoot (if present) and returns its
// files as a map suitable for reviewapi.CreateZipArchiveWithExtras, keyed
// by repo-relative path (e.g. ".lrc/rules/security.md") with "/"
// separators. Returns a nil map (no error) when .lrc/ does not exist.
//
// A file or subdirectory that can't be read (e.g. a permission error) is
// skipped and reported via the returned warnings slice rather than
// aborting the whole walk, so a single unreadable entry doesn't drop all
// Repository Rules from the review bundle.
func CollectZipExtras(repoRoot string) (map[string][]byte, []string, error) {
	if abs, absErr := filepath.Abs(repoRoot); absErr == nil {
		repoRoot = abs
	}

	lrcDir, ok, err := Load(repoRoot)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, nil
	}

	extras := map[string][]byte{}
	var warnings []string
	walkErr := filepath.WalkDir(lrcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipping %s: %v", path, err))
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipping %s: failed to compute relative path: %v", path, err))
			return nil
		}
		if info, infoErr := d.Info(); infoErr == nil && info.Size() > maxExtraFileSize {
			warnings = append(warnings, fmt.Sprintf("skipping %s: file is %d bytes, exceeding the %d byte limit for .lrc/ files", relPath, info.Size(), maxExtraFileSize))
			return nil
		}
		content, err := storage.ReadFile(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipping %s: failed to read file: %v", relPath, err))
			return nil
		}
		extras[filepath.ToSlash(relPath)] = content
		return nil
	})
	if walkErr != nil {
		return extras, warnings, walkErr
	}

	return extras, warnings, nil
}
