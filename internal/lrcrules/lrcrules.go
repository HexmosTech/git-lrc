// Package lrcrules implements the client-side half of the .lrc/ Repository
// Rules feature: loading the .lrc/ directory, building a preview of the
// instruction bundle that LiveReview will assemble server-side, validating
// structure, and scaffolding a new .lrc/ directory.
//
// The actual enforcement (concatenation used by reviews, ignore-pattern
// filtering) happens server-side in LiveReview's internal/lrcconfig
// package. This package's BuildRulesBundle implements the same
// concatenation rule purely for local, offline `lrc config check`/`preview`.
package lrcrules

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/HexmosTech/git-lrc/storage"
)

// CharLimit is the maximum size, in bytes (UTF-8), of the concatenated rules
// bundle that LiveReview will accept without truncating. It is measured via
// len() on the bundle text, matching the server-side truncation in
// LiveReview's internal/lrcconfig, so multi-byte characters count for more
// than one toward the limit.
const CharLimit = 3000

// Issue describes a problem found while validating .lrc/.
type Issue struct {
	Level   string // "error" | "warning"
	Path    string
	Message string
}

const rulesReadmeName = "README.md"
const rulesInstructionsName = "INSTRUCTIONS.md"

// Load returns the .lrc/ directory path under repoRoot. ok=false (with no
// error) when .lrc/ does not exist.
func Load(repoRoot string) (lrcDir string, ok bool, err error) {
	if abs, absErr := filepath.Abs(repoRoot); absErr == nil {
		repoRoot = abs
	}
	dir := filepath.Join(repoRoot, ".lrc")
	info, statErr := os.Stat(dir)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to stat %s: %w", dir, statErr)
	}
	if !info.IsDir() {
		return "", false, fmt.Errorf("%s exists but is not a directory", dir)
	}
	return dir, true, nil
}

// BuildRulesBundle concatenates .lrc/rules/*.md, excluding rules/README.md
// and skipping empty/whitespace-only files. rules/INSTRUCTIONS.md, if
// present and non-empty, is placed first as the entry point; every other
// file follows in lexicographic order. Each included file is preceded by a
// "## rules/<name>.md" header. Returns the concatenated text, its character
// count, and an error-level Issue if the result exceeds CharLimit.
func BuildRulesBundle(lrcDir string) (string, int, []Issue) {
	rulesDir := filepath.Join(lrcDir, "rules")
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", 0, nil
		}
		return "", 0, []Issue{{Level: "error", Path: "rules", Message: fmt.Sprintf("failed to read rules directory: %v", err)}}
	}

	var names []string
	hasInstructions := false
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		if name == rulesReadmeName {
			continue
		}
		if name == rulesInstructionsName {
			hasInstructions = true
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if hasInstructions {
		names = append([]string{rulesInstructionsName}, names...)
	}

	var b strings.Builder
	var issues []Issue
	for _, name := range names {
		relPath := filepath.ToSlash(filepath.Join("rules", name))
		content, err := storage.ReadFile(filepath.Join(rulesDir, name))
		if err != nil {
			issues = append(issues, Issue{Level: "error", Path: relPath, Message: fmt.Sprintf("failed to read file: %v", err)})
			continue
		}
		trimmed := strings.TrimSpace(string(content))
		if trimmed == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("## ")
		b.WriteString(relPath)
		b.WriteString("\n\n")
		b.WriteString(trimmed)
	}

	text := b.String()
	charCount := len(text)
	if charCount > CharLimit {
		issues = append(issues, Issue{
			Level:   "error",
			Path:    "rules",
			Message: fmt.Sprintf("concatenated rules bundle is %d characters, exceeding the %d character limit", charCount, CharLimit),
		})
	}

	return text, charCount, issues
}

// ValidateStructure flags structural problems with .lrc/: a missing
// rules/ directory or a missing ignore file. .lrc/ itself is optional, but
// once present it should be well-formed.
func ValidateStructure(lrcDir string) []Issue {
	var issues []Issue

	rulesDir := filepath.Join(lrcDir, "rules")
	if info, err := os.Stat(rulesDir); err != nil || !info.IsDir() {
		issues = append(issues, Issue{Level: "warning", Path: "rules", Message: "rules/ directory is missing"})
	}

	ignorePath := filepath.Join(lrcDir, "ignore")
	if _, err := os.Stat(ignorePath); err != nil {
		issues = append(issues, Issue{Level: "warning", Path: "ignore", Message: "ignore file is missing"})
	}

	return issues
}

// CheckIgnoreSyntax does a light syntax pass over .lrc/ignore. Real
// enforcement (gitignore-style matching against changed files) happens
// server-side; this only catches obviously malformed lines.
func CheckIgnoreSyntax(lrcDir string) []Issue {
	data, err := storage.ReadFile(filepath.Join(lrcDir, "ignore"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return []Issue{{Level: "error", Path: "ignore", Message: fmt.Sprintf("failed to read file: %v", err)}}
	}

	var issues []Issue
	for i, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, "**/**") {
			issues = append(issues, Issue{
				Level:   "warning",
				Path:    "ignore",
				Message: fmt.Sprintf("line %d: redundant '**/**' pattern: %q", i+1, trimmed),
			})
		}
	}
	return issues
}
