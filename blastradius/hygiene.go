package blastradius

import (
	"regexp"
	"strings"

	"github.com/HexmosTech/blastradius/symbols"
)

// Hygiene ("diff-shape") signals dampen a hunk's Combined score
// multiplicatively rather than adding a negative point to the raw sum - a
// formatting-only change to a critical function should score near-zero
// regardless of how important that function is, and an additive negative
// point risks being swamped by a large positive sum. Detected purely from
// the hunk's own diff content/file path/touched-symbol properties - no
// graph query needed.
var (
	commentLineRe     = regexp.MustCompile(`^\s*(//|#|/\*|\*|<!--)`)
	logCallRe         = regexp.MustCompile(`(?i)\b(log\.\w|logger\.\w|console\.log|fmt\.Print|logging\.\w)`)
	generatedPathRe   = regexp.MustCompile(`(?i)_generated\.|\.pb\.go$|_pb2\.py$|/generated/`)
	generatedHeaderRe = regexp.MustCompile(`(?i)code generated .* do not edit`)
	testFilePathRe    = regexp.MustCompile(`(?i)_test\.go$|\.test\.[jt]sx?$|\.spec\.[jt]sx?$|(^|/)test_[^/]+\.py$|(^|/)tests?/`)
)

// classifyHunkHygiene inspects a hunk's diff content, file path, and touched
// symbols, returning a [0,1] multiplier for Combined plus the Signal
// explaining it (nil Signal means multiplier 1.0, nothing detected).
// Categories are checked in priority order, most-confident first, and only
// one fires per hunk.
func classifyHunkHygiene(hunk Hunk, touched []symbols.Symbol) (float64, *Signal) {
	if generatedPathRe.MatchString(hunk.FilePath) || generatedHeaderRe.MatchString(hunk.Content) {
		return 0.1, &Signal{
			Name: "Generated code", Detail: "file path or header indicates auto-generated code",
			Points: -0.9, Category: "diff-shape",
		}
	}

	added, removed := diffLines(hunk.Content)
	changed := append(append([]string{}, added...), removed...)

	if len(changed) > 0 && isFormattingOnly(added, removed) {
		return 0.05, &Signal{
			Name: "Formatting only", Detail: "added/removed lines are identical once whitespace is stripped",
			Points: -0.95, Category: "diff-shape",
		}
	}

	// Dead code removal: a pure deletion where every touched symbol has no
	// callers repo-wide (not just within this diff) and isn't itself an
	// entry point or test.
	if hunk.NewLines == 0 && len(touched) > 0 && allDeadCode(touched) {
		return 0.1, &Signal{
			Name: "Dead code removal", Detail: "removed symbol(s) have no callers anywhere in the repository",
			Points: -0.9, Category: "diff-shape",
		}
	}

	if len(changed) > 0 && allLinesMatch(changed, commentLineRe) {
		return 0.15, &Signal{
			Name: "Comments only", Detail: "every changed line is a comment",
			Points: -0.85, Category: "diff-shape",
		}
	}

	if len(changed) > 0 && allLinesMatch(changed, logCallRe) {
		return 0.4, &Signal{
			Name: "Logging only", Detail: "every changed line is a log/print statement",
			Points: -0.6, Category: "diff-shape",
		}
	}

	if testFilePathRe.MatchString(hunk.FilePath) {
		return 0.6, &Signal{
			Name: "Test-only file", Detail: "change is confined to a test file",
			Points: -0.4, Category: "diff-shape",
		}
	}

	return 1.0, nil
}

// diffLines splits a hunk's raw Content into added and removed line bodies
// (the leading +/- stripped), skipping the +++/--- file-header lines that
// can appear in some diff representations.
func diffLines(content string) (added, removed []string) {
	for line := range strings.SplitSeq(content, "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "+"):
			added = append(added, line[1:])
		case strings.HasPrefix(line, "-"):
			removed = append(removed, line[1:])
		}
	}
	return added, removed
}

func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), "")
}

// isFormattingOnly reports whether added and removed are the same lines,
// in the same order, once all whitespace is stripped from each.
func isFormattingOnly(added, removed []string) bool {
	if len(added) == 0 || len(added) != len(removed) {
		return false
	}
	for i := range added {
		if normalizeWhitespace(added[i]) != normalizeWhitespace(removed[i]) {
			return false
		}
	}
	return true
}

// allLinesMatch reports whether every non-blank line matches re.
func allLinesMatch(lines []string, re *regexp.Regexp) bool {
	seenNonBlank := false
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		seenNonBlank = true
		if !re.MatchString(l) {
			return false
		}
	}
	return seenNonBlank
}

// allDeadCode reports whether every symbol has zero callers repo-wide and
// isn't itself an entry point or test - verified live as a real dead-code
// query pattern (in_degree=0 AND is_entry_point=false AND is_test=false).
func allDeadCode(touched []symbols.Symbol) bool {
	for _, s := range touched {
		if s.InDegree != 0 || s.IsEntryPoint || s.IsTest || s.RouteMethod != "" {
			return false
		}
	}
	return true
}
