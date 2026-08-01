package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// These allowlists are temporary carve-outs for legacy orchestration files that
// still own file or HTTP side effects directly. Keep them narrow, document each
// addition, and remove entries as the affected code is extracted into storage/
// or network/ helpers.
var (
	legacyReadFileAllowlist = []string{
		"internal/appcore/claude_setup.go",
		"internal/appcore/review_runtime.go",
		"internal/appcore/review_runtime_landing.go",
		"internal/staticserve/static_serve.go",
	}
	legacyWriteFileAllowlist = []string{
		"internal/appcore/claude_setup.go",
		"internal/appcore/interactive_decision.go",
		"internal/appcore/review_runtime_landing.go",
	}
	legacyMkdirAllAllowlist = []string{
		"internal/appcore/claude_setup.go",
		"internal/appcore/interactive_decision.go",
		"internal/appcore/review_runtime_landing.go",
	}
	legacyRemoveAllowlist = []string{
		"internal/appcore/claude_setup.go",
		"internal/appcore/review_runtime.go",
		"internal/appcore/review_runtime_landing.go",
	}
	legacyOpenFileAllowlist = []string{
		"internal/appcore/claude_setup.go",
	}
	legacyCreateTempAllowlist = []string{
		"internal/appcore/review_runtime.go",
	}
	legacyNewRequestAllowlist = []string{
		"internal/appcore/review_runtime_landing.go",
	}
)

func TestStorageBoundaryEnforcement(t *testing.T) {
	repoRoot := mustRepoRoot(t)
	violations := scanForPatternViolations(t, repoRoot, []patternRule{
		{re: regexp.MustCompile(`\bos\.ReadFile\(`), allowPrefixes: []string{"storage/"}, allowFiles: legacyReadFileAllowlist},
		{re: regexp.MustCompile(`\bos\.WriteFile\(`), allowPrefixes: []string{"storage/"}, allowFiles: legacyWriteFileAllowlist},
		{re: regexp.MustCompile(`\bos\.MkdirAll\(`), allowPrefixes: []string{"storage/"}, allowFiles: legacyMkdirAllAllowlist},
		{re: regexp.MustCompile(`\bos\.Rename\(`), allowPrefixes: []string{"storage/"}},
		{re: regexp.MustCompile(`\bos\.RemoveAll\(`), allowPrefixes: []string{"storage/"}},
		{re: regexp.MustCompile(`\bos\.Remove\(`), allowPrefixes: []string{"storage/"}, allowFiles: legacyRemoveAllowlist},
		{re: regexp.MustCompile(`\bos\.CreateTemp\(`), allowPrefixes: []string{"storage/"}, allowFiles: legacyCreateTempAllowlist},
		{re: regexp.MustCompile(`\bos\.Chmod\(`), allowPrefixes: []string{"storage/"}},
		{re: regexp.MustCompile(`\bos\.OpenFile\(`), allowPrefixes: []string{"storage/", "interactive/"}, allowFiles: legacyOpenFileAllowlist},
		{re: regexp.MustCompile(`\bos\.Open\(`), allowPrefixes: []string{"storage/", "interactive/"}},
		{re: regexp.MustCompile(`\bsql\.Open\(`), allowPrefixes: []string{"storage/"}},
		{re: regexp.MustCompile(`\bdb\.Exec\(`), allowPrefixes: []string{"storage/"}},
		{re: regexp.MustCompile(`\bdb\.QueryRow\(`), allowPrefixes: []string{"storage/"}},
		{re: regexp.MustCompile(`\bdb\.Query\(`), allowPrefixes: []string{"storage/"}},
	})

	if len(violations) > 0 {
		t.Fatalf("storage boundary violations found:\n%s", strings.Join(violations, "\n"))
	}
}

func TestNetworkBoundaryEnforcement(t *testing.T) {
	repoRoot := mustRepoRoot(t)
	violations := scanForPatternViolations(t, repoRoot, []patternRule{
		{re: regexp.MustCompile(`\bhttp\.NewRequest(?:WithContext)?\(`), allowPrefixes: []string{"network/"}, allowFiles: legacyNewRequestAllowlist},
		{re: regexp.MustCompile(`\bhttp\.DefaultClient\.Do\(`), allowPrefixes: []string{"network/"}},
	})

	if len(violations) > 0 {
		t.Fatalf("network boundary violations found:\n%s", strings.Join(violations, "\n"))
	}
}

type patternRule struct {
	re            *regexp.Regexp
	allowPrefixes []string
	allowFiles    []string
}

func scanForPatternViolations(t *testing.T, repoRoot string, rules []patternRule) []string {
	t.Helper()
	violations := make([]string, 0)
	walkErrors := make([]string, 0)

	if err := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			walkErrors = append(walkErrors, path+": "+err.Error())
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			if path == filepath.Join(repoRoot, "blastradius") {
				// blastradius is a separate, portable Go module (its own go.mod)
				// with no dependency on git-lrc's internal packages by design -
				// it doesn't participate in this module's storage/network
				// boundary conventions.
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		text := string(content)

		for _, rule := range rules {
			if hasAllowedPrefix(rel, rule.allowPrefixes) {
				continue
			}
			if hasAllowedFile(rel, rule.allowFiles) {
				continue
			}
			if rule.re.FindStringIndex(text) != nil {
				violations = append(violations, rel+": matches "+rule.re.String())
			}
		}

		return nil
	}); err != nil {
		t.Fatalf("failed to walk repository tree: %v", err)
	}

	if len(walkErrors) > 0 {
		t.Fatalf("encountered file walk errors:\n%s", strings.Join(walkErrors, "\n"))
	}

	return violations
}

func hasAllowedPrefix(rel string, allowPrefixes []string) bool {
	for _, prefix := range allowPrefixes {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}

func hasAllowedFile(rel string, allowFiles []string) bool {
	for _, file := range allowFiles {
		if rel == file {
			return true
		}
	}
	return false
}

func mustRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}
