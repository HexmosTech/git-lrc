package lrcrules

import (
	"errors"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	gitignore "github.com/sabhiram/go-gitignore"

	"github.com/HexmosTech/git-lrc/storage"
)

// diffHeaderRe matches the "diff --git a/X b/Y" line that begins each
// per-file section of a unified diff produced by `git diff`/`git show`. The
// capture groups are greedy so that paths containing spaces (which git
// does not quote by default) are still split correctly at the " b/"
// separator.
var diffHeaderRe = regexp.MustCompile(`(?m)^diff --git a/(.+) b/(.+)`)

// LoadIgnorePatterns parses .lrc/ignore (gitignore syntax) from lrcDir.
// Returns nil patterns and nil error if the file is absent or empty.
func LoadIgnorePatterns(lrcDir string) ([]string, error) {
	data, err := storage.ReadFile(filepath.Join(lrcDir, "ignore"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var patterns []string
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		patterns = append(patterns, line)
	}

	return patterns, nil
}

// FilterDiff splits diffContent on "diff --git a/X b/Y" section boundaries
// and drops whole per-file sections whose path matches an ignore pattern.
// Returns the filtered diff and the list of excluded file paths, in the
// order they appear in diffContent. If patterns is empty, diffContent is
// returned unchanged.
func FilterDiff(diffContent []byte, patterns []string) ([]byte, []string) {
	if len(patterns) == 0 {
		return diffContent, nil
	}

	matches := diffHeaderRe.FindAllSubmatchIndex(diffContent, -1)
	if len(matches) == 0 {
		return diffContent, nil
	}

	matcher := gitignore.CompileIgnoreLines(patterns...)

	var kept []byte
	var excluded []string

	// Preserve any content before the first section header.
	kept = append(kept, diffContent[:matches[0][0]]...)

	for i, m := range matches {
		start := m[0]
		end := len(diffContent)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		path := string(diffContent[m[4]:m[5]]) // "b/<path>" capture group

		if matcher.MatchesPath(path) {
			excluded = append(excluded, path)
			continue
		}
		kept = append(kept, diffContent[start:end]...)
	}

	return kept, excluded
}
