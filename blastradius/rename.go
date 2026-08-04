package blastradius

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/HexmosTech/blastradius/client"
	"github.com/HexmosTech/blastradius/symbols"
)

// DeclRename describes a single-identifier rename detected on a symbol's own
// declaration line within a hunk: OldName is what every existing caller/
// reference elsewhere in the repo is still using (until separately
// migrated), NewName is sym.Name (the post-rename name the graph now knows
// about).
type DeclRename struct {
	OldName string
	NewName string
}

var declRenameIdentRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// detectDeclRename inspects hunk.Content (raw ' '/'+'/'-' prefixed diff
// lines) for a unified-diff "replace block" - a contiguous run of removed
// lines immediately followed by a contiguous run of added lines of the same
// count - whose new-side line number lands exactly on sym's own declaration
// line (sym.StartLine), and where the paired old/new lines differ in
// exactly one identifier token, with the new-side token equal to sym.Name.
//
// This is deliberately strict: it only fires when a symbol's own
// declaration line changed one identifier and nothing else, so it does not
// mistake an unrelated rename elsewhere in the same hunk, a multi-line
// signature reshuffle, or a pure formatting change for a rename. Returns
// nil when no such rename is detected.
func detectDeclRename(hunk Hunk, sym symbols.Symbol) *DeclRename {
	if sym.StartLine <= 0 || hunk.Content == "" {
		return nil
	}

	type addedLine struct {
		body string
		line int
	}
	var removedRun []string
	var addedRun []addedLine

	tryMatch := func() *DeclRename {
		// Only a clean 1:1 replace block can be safely attributed line-by-
		// line; anything else (unequal added/removed counts) is ambiguous -
		// skip it rather than risk pairing the wrong lines.
		if len(removedRun) == 0 || len(removedRun) != len(addedRun) {
			return nil
		}
		for i, a := range addedRun {
			if a.line != sym.StartLine {
				continue
			}
			if rn := matchSingleIdentSwap(removedRun[i], a.body, sym.Name); rn != nil {
				return rn
			}
		}
		return nil
	}

	newLine := hunk.NewStart
	for line := range strings.SplitSeq(hunk.Content, "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case '-':
			removedRun = append(removedRun, line[1:])
		case '+':
			addedRun = append(addedRun, addedLine{body: line[1:], line: newLine})
			newLine++
		default: // context line (or a diff artifact like "\ No newline at end of file")
			if rn := tryMatch(); rn != nil {
				return rn
			}
			removedRun, addedRun = nil, nil
			newLine++
		}
	}
	return tryMatch()
}

// matchSingleIdentSwap tokenizes oldLine/newLine into identifiers and
// requires exactly one token to differ, with the differing new-side token
// equal to symName - confirming both that only one identifier changed and
// that it's specifically this symbol's own name, not some other identifier
// on the same line (e.g. a parameter type).
func matchSingleIdentSwap(oldLine, newLine, symName string) *DeclRename {
	oldTokens := declRenameIdentRe.FindAllString(oldLine, -1)
	newTokens := declRenameIdentRe.FindAllString(newLine, -1)
	if len(oldTokens) == 0 || len(oldTokens) != len(newTokens) {
		return nil
	}
	diffIdx := -1
	for i := range oldTokens {
		if oldTokens[i] != newTokens[i] {
			if diffIdx != -1 {
				return nil // more than one identifier differs - not a clean rename
			}
			diffIdx = i
		}
	}
	if diffIdx == -1 || newTokens[diffIdx] != symName {
		return nil
	}
	return &DeclRename{OldName: oldTokens[diffIdx], NewName: newTokens[diffIdx]}
}

// renameOldNameSignal runs a best-effort text search for rn.OldName and
// returns a Signal counting how many places in the repo still reference the
// pre-rename name - i.e. call sites/references that are about to break until
// they're migrated to rn.NewName. Weighted 1.5x relative to the plain
// sqrt(refs) used for an ordinary text-reference count elsewhere in this
// package, since this is live, unmigrated breakage risk right now, not just
// historical usage. Unlike the ordinary text-reference count, no "-1 for the
// symbol's own definition" adjustment is applied: the definition itself was
// renamed away, so every remaining match under the old name is an external
// reference.
func renameOldNameSignal(ctx context.Context, c *client.Client, rn DeclRename) Signal {
	usage, err := c.SearchCodeUsage(ctx, rn.OldName)
	if err != nil {
		// Distinguish "checked, found nothing" from "couldn't check" - the
		// latter must not silently read as 0 points/0 references, which
		// would falsely assert the rename is safe when it simply wasn't
		// verified.
		return Signal{
			Name:     "Callers of old name (about to break)",
			Detail:   fmt.Sprintf("could not check references to the pre-rename name %q: %v", rn.OldName, err),
			Points:   0,
			Category: "graph",
		}
	}
	refs := usage.TotalMatches
	return Signal{
		Name:     "Callers of old name (about to break)",
		Detail:   fmt.Sprintf("%d reference(s) to the pre-rename name %q still exist and will break until migrated to %q", refs, rn.OldName, rn.NewName),
		Points:   1.5 * math.Sqrt(float64(refs)),
		Category: "graph",
	}
}
