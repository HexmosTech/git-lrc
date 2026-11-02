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
	"github.com/pelletier/go-toml"
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

// CheckToolsSyntax checks .lrc/policy/tools.toml or .lrc/tools.toml for TOML syntax errors.
func CheckToolsSyntax(lrcDir string) []Issue {
	toolsPath := filepath.Join(lrcDir, "policy", "tools.toml")
	data, err := storage.ReadFile(toolsPath)
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		toolsPath = filepath.Join(lrcDir, "tools.toml")
		data, err = storage.ReadFile(toolsPath)
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return []Issue{{Level: "error", Path: "policy/tools.toml", Message: fmt.Sprintf("failed to read file: %v", err)}}
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}

	var cfg map[string]interface{}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		relPath, _ := filepath.Rel(lrcDir, toolsPath)
		return []Issue{{Level: "error", Path: relPath, Message: fmt.Sprintf("invalid TOML syntax: %v", err)}}
	}

	return nil
}

// ExtractToolsConfig extracts the [tools] section from TOML configuration bytes (e.g. from .lrc.toml / ~/.lrc.toml).
// It returns:
// 1. A slice of formatted tool summaries (e.g. ["gitleaks (enabled)", "ruff (enabled)"])
// 2. Sanitized TOML bytes containing only the [tools] section (safe for bundling into review payloads)
// 3. Any parsing error
func ExtractToolsConfig(data []byte) ([]string, []byte, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil, nil
	}

	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("invalid TOML syntax: %w", err)
	}

	var toolsMap map[string]interface{}
	if toolsRaw, exists := raw["tools"]; exists && toolsRaw != nil {
		if tm, ok := toolsRaw.(map[string]interface{}); ok {
			toolsMap = sanitizeMap(tm)
		}
	}

	// If no top-level "tools" key, check if top-level tables represent tools (e.g. in policy/tools.toml)
	if toolsMap == nil {
		candidateTools := make(map[string]interface{})
		for k, v := range raw {
			if isSensitiveKey(k) {
				continue
			}
			switch val := v.(type) {
			case bool:
				candidateTools[k] = val
			case map[string]interface{}:
				cleaned := sanitizeMap(val)
				if len(cleaned) > 0 {
					candidateTools[k] = cleaned
				}
			}
		}
		if len(candidateTools) > 0 {
			toolsMap = candidateTools
		}
	}

	if len(toolsMap) == 0 {
		return nil, nil, nil
	}

	var summaries []string
	for toolName, val := range toolsMap {
		status := "enabled"
		switch v := val.(type) {
		case bool:
			if !v {
				status = "disabled"
			}
		case map[string]interface{}:
			if en, exists := v["enabled"]; exists {
				if b, ok := en.(bool); ok && !b {
					status = "disabled"
				}
			}
		}
		summaries = append(summaries, fmt.Sprintf("%s (%s)", toolName, status))
	}
	sort.Strings(summaries)

	toolsOnlyMap := map[string]interface{}{
		"tools": toolsMap,
	}
	toolsBytes, err := toml.Marshal(toolsOnlyMap)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal tools config: %w", err)
	}

	return summaries, toolsBytes, nil
}

// isSensitiveKey reports whether a configuration key represents a credential, secret, or sensitive metadata.
func isSensitiveKey(k string) bool {
	lower := strings.ToLower(strings.TrimSpace(k))
	sensitive := map[string]bool{
		"api_key": true, "apikey": true, "api_url": true, "url": true,
		"org_id": true, "org_name": true, "jwt": true, "refresh_token": true,
		"user_email": true, "user_id": true, "user_first_name": true,
		"user_last_name": true, "avatar_url": true, "ai_connectors": true,
		"token": true, "access_token": true, "auth_token": true, "pat": true, "pat_token": true,
		"password": true, "passwd": true, "secret": true, "secret_key": true,
		"client_secret": true, "private_key": true, "credential": true, "credentials": true,
		"auth": true, "authorization": true,
		"client_id": true, "key_id": true, "access_key_id": true, "secret_key_id": true,
	}
	if sensitive[lower] {
		return true
	}
	if strings.HasSuffix(lower, "_secret") || strings.HasSuffix(lower, "_token") ||
		strings.HasSuffix(lower, "_password") || strings.HasSuffix(lower, "_api_key") ||
		strings.HasSuffix(lower, "_key_id") || strings.HasSuffix(lower, "_client_id") ||
		strings.HasPrefix(lower, "secret_") || strings.HasPrefix(lower, "token_") {
		return true
	}
	return false
}

// sanitizeMap recursively filters out sensitive keys at any nesting level in maps and arrays.
func sanitizeMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	cleaned := make(map[string]interface{})
	for k, v := range m {
		if isSensitiveKey(k) {
			continue
		}
		cleaned[k] = sanitizeValue(v)
	}
	return cleaned
}

// sanitizeValue recursively filters sensitive values contained in maps and slices.
// Note: In Go, []map[string]interface{} is not assignable to []interface{} (no slice covariance),
// so both slice types produced by the TOML parser are explicitly handled without reflection.
func sanitizeValue(val interface{}) interface{} {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case map[string]interface{}:
		return sanitizeMap(v)
	case []map[string]interface{}:
		cleaned := make([]map[string]interface{}, 0, len(v))
		for _, item := range v {
			cleaned = append(cleaned, sanitizeMap(item))
		}
		return cleaned
	case []interface{}:
		cleanedSlice := make([]interface{}, 0, len(v))
		for _, item := range v {
			cleanedSlice = append(cleanedSlice, sanitizeValue(item))
		}
		return cleanedSlice
	default:
		return val
	}
}


