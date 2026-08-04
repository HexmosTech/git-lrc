package blastradius

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/HexmosTech/blastradius/client"
	"github.com/HexmosTech/blastradius/score"
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
// pre-rename name after the declaration moved to rn.NewName. Weighted 1.5x
// relative to the plain sqrt(refs) used for an ordinary text-reference count
// elsewhere in this package, since an unmigrated reference is a live risk
// right now, not just historical usage. Unlike the ordinary text-reference
// count, no "-1 for the symbol's own definition" adjustment is applied: the
// definition itself was renamed away, so every remaining match under the old
// name is an external reference.
//
// The wording states only what was measured. search_code is grep-based, so a
// match can equally be a real call, a comment, or a string literal - calling
// these references "about to break" asserted a compile failure that nothing
// here verifies. Points are unchanged; this is phrasing only.
func renameOldNameSignal(ctx context.Context, c *client.Client, rn DeclRename) Signal {
	usage, err := c.SearchCodeUsage(ctx, rn.OldName)
	if err != nil {
		// Distinguish "checked, found nothing" from "couldn't check" - the
		// latter must not silently read as 0 points/0 references, which
		// would falsely assert the rename is fully migrated when it simply
		// wasn't verified.
		return Signal{
			Name:     "Still uses the old name",
			Detail:   fmt.Sprintf("could not check references to the old name %q: %v", rn.OldName, err),
			Points:   0,
			Category: "graph",
		}
	}
	refs := usage.TotalMatches
	return Signal{
		Name:     "Still uses the old name",
		Detail:   fmt.Sprintf("%d reference(s) to %q remain after the rename to %q", refs, rn.OldName, rn.NewName),
		Points:   1.5 * math.Sqrt(float64(refs)),
		Category: "graph",
	}
}

// oldNameCallerLimit caps how many enclosing symbols search_code is asked to
// resolve for a pre-rename name. Generous enough that a normal rename's call
// sites all fit, bounded so a rename of a very common word (e.g. "get") can't
// turn one hunk into a thousand-node graph.
const oldNameCallerLimit = 60

// preRenameCallerLabels are the node labels that can actually contain a
// reference that breaks. search_code resolves a match to whatever node encloses
// it, which includes documentation and structural nodes (Section, File, Folder,
// Package, ...) - a README mentioning the old name is not a caller, and listing
// it as one would misrepresent the breakage.
var preRenameCallerLabels = map[string]bool{
	"Function": true,
	"Method":   true,
	"Class":    true,
	"Struct":   true,
	"Variable": true,
	"Route":    true,
}

// oldNameCallers finds the symbols that still reference rn.OldName - the call
// sites that break until they're migrated to rn.NewName - and returns them as
// depth-1 CallerRefs flagged PreRename.
//
// These cannot come from CALLS fan-in: the graph is indexed against the
// post-rename tree, so the old name has no node and every edge that pointed at
// it was dropped as unresolvable. search_code's compact mode is the way in - it
// resolves each textual hit to the graph node enclosing it, which is exactly
// the caller we want.
//
// Best-effort by the same rule as renameOldNameSignal's error path: a failed
// lookup returns no callers rather than failing the report. It contributes no
// points either way - scoring stays entirely with renameOldNameSignal.
func oldNameCallers(ctx context.Context, c *client.Client, rn DeclRename, selfQN string) []CallerRef {
	matches, err := c.SearchCodeSymbols(ctx, rn.OldName, oldNameCallerLimit)
	if err != nil {
		return nil
	}
	return preRenameCallersFrom(matches.Matches, selfQN)
}

// preRenameCallersFrom is oldNameCallers' pure half: turn search_code's
// enclosing-symbol matches into a deduplicated, name-sorted caller list.
func preRenameCallersFrom(matches []client.CodeSymbolMatch, selfQN string) []CallerRef {
	var callers []CallerRef
	seen := make(map[string]bool)
	for _, m := range matches {
		// The renamed symbol's own declaration is not one of its callers.
		// It can still match: a method body referencing its own old name,
		// or a stale doc comment above the new declaration.
		if m.QualifiedName == "" || m.QualifiedName == selfQN || seen[m.QualifiedName] {
			continue
		}
		if !preRenameCallerLabels[m.Label] {
			continue
		}
		seen[m.QualifiedName] = true
		callers = append(callers, CallerRef{
			QualifiedName: m.QualifiedName,
			Depth:         1,
			Weight:        1,
			PreRename:     true,
		})
	}
	sort.Slice(callers, func(i, j int) bool { return callers[i].QualifiedName < callers[j].QualifiedName })
	return callers
}

// expandPreRenameCallers adds the transitive fan-in of direct pre-rename
// callers. Unlike the old name itself, those callers are ordinary present-day
// graph nodes, so score.FanIn can walk them - which is what gives a rename's
// sunburst/flamegraph the same depth structure as any other symbol's instead of
// a single flat ring.
//
// Each discovered caller comes back one hop further from the renamed symbol
// than it is from the direct caller, reached *through* that direct caller, so
// its Depth is shifted by 1 and its Path prefixed accordingly. Anything already
// present as a direct pre-rename caller is left alone - the shallower depth is
// the truthful one.
func expandPreRenameCallers(ctx context.Context, q score.GraphQuerier, direct []CallerRef, cfg score.Config, selfQN string) []CallerRef {
	if len(direct) == 0 {
		return nil
	}
	directQN := make([]string, 0, len(direct))
	seen := make(map[string]bool, len(direct))
	for _, c := range direct {
		directQN = append(directQN, c.QualifiedName)
		seen[c.QualifiedName] = true
	}
	// One fewer hop than a normal walk: the direct pre-rename callers are
	// already one hop out from the renamed symbol, so the budget left for
	// their own callers is MaxDepth-1.
	expandCfg := cfg
	expandCfg.MaxDepth = cfg.MaxDepth - 1
	if expandCfg.MaxDepth < 1 {
		return nil
	}
	scores, err := score.FanIn(ctx, q, directQN, expandCfg)
	if err != nil {
		return nil // best-effort: the direct callers alone are still worth showing
	}

	best := make(map[string]CallerRef)
	for viaQN, ss := range scores {
		for _, cc := range ss.Callers {
			qn := cc.QualifiedName
			if qn == "" || qn == selfQN || seen[qn] {
				continue
			}
			depth := cc.Depth + 1
			if existing, ok := best[qn]; ok && existing.Depth <= depth {
				continue
			}
			best[qn] = CallerRef{
				QualifiedName: qn,
				Depth:         depth,
				Weight:        cc.Weight * cfg.Decay,
				Path:          append([]string{viaQN}, cc.Path...),
				PreRename:     true,
			}
		}
	}
	expanded := make([]CallerRef, 0, len(best))
	for _, c := range best {
		expanded = append(expanded, c)
	}
	sort.Slice(expanded, func(i, j int) bool {
		if expanded[i].Depth != expanded[j].Depth {
			return expanded[i].Depth < expanded[j].Depth
		}
		return expanded[i].QualifiedName < expanded[j].QualifiedName
	})
	return expanded
}
