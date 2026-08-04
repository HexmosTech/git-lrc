package blastradius

import (
	"context"
	"strings"
	"testing"

	"github.com/HexmosTech/blastradius/client"
	"github.com/HexmosTech/blastradius/score"
)

func TestPreRenameCallersFromFiltersAndDedups(t *testing.T) {
	matches := []client.CodeSymbolMatch{
		{QualifiedName: "pkg.main", Label: "Function"},
		// The renamed symbol's own declaration is not one of its callers -
		// it matches because a stale doc comment still names it.
		{QualifiedName: "pkg.RuHelp", Label: "Function"},
		// A README section mentioning the old name is not a caller.
		{QualifiedName: "docs.README.Help", Label: "Section"},
		{QualifiedName: "pkg.handler", Label: "Method"},
		// The tool can report the same enclosing symbol twice.
		{QualifiedName: "pkg.main", Label: "Function"},
		{QualifiedName: "pkg.registry", Label: "Variable"},
		// Structural nodes carry no call site of their own.
		{QualifiedName: "pkg", Label: "Package"},
		{QualifiedName: "", Label: "Function"},
	}

	got := preRenameCallersFrom(matches, "pkg.RuHelp")

	want := []string{"pkg.handler", "pkg.main", "pkg.registry"}
	if len(got) != len(want) {
		t.Fatalf("got %d callers %+v, want %d", len(got), got, len(want))
	}
	for i, qn := range want {
		if got[i].QualifiedName != qn {
			t.Fatalf("caller[%d] = %q, want %q (results must be name-sorted)", i, got[i].QualifiedName, qn)
		}
		if got[i].Depth != 1 {
			t.Errorf("caller %q Depth = %d, want 1", qn, got[i].Depth)
		}
		if !got[i].PreRename {
			t.Errorf("caller %q PreRename = false, want true", qn)
		}
	}
}

func TestPreRenameCallersFromNoMatches(t *testing.T) {
	if got := preRenameCallersFrom(nil, "pkg.RuHelp"); got != nil {
		t.Fatalf("got %+v, want nil for an empty match list", got)
	}
	// A fully-migrated rename: the only remaining hit is the symbol itself.
	only := []client.CodeSymbolMatch{{QualifiedName: "pkg.RuHelp", Label: "Function"}}
	if got := preRenameCallersFrom(only, "pkg.RuHelp"); got != nil {
		t.Fatalf("got %+v, want nil when the only match is the symbol itself", got)
	}
}

// chainQuerier answers depth-1 fan-in for a fixed caller->callee chain:
// pkg.top -> pkg.mid -> pkg.direct. Deeper queries return nothing, so a walk
// terminates naturally.
type chainQuerier struct{}

func (chainQuerier) QueryGraph(ctx context.Context, cypher string, maxRows int) (*client.QueryResult, error) {
	// score.FanIn's depth-1 query is the only single-hop MATCH it issues.
	if !strings.Contains(cypher, "(caller)-[:CALLS]->(f)") {
		return &client.QueryResult{Columns: []string{"symbol", "caller"}}, nil
	}
	rows := [][]string{{"pkg.direct", "pkg.mid"}}
	if strings.Contains(cypher, "pkg.mid") {
		rows = append(rows, []string{"pkg.mid", "pkg.top"})
	}
	return &client.QueryResult{Columns: []string{"symbol", "caller"}, Rows: rows}, nil
}

func TestExpandPreRenameCallersShiftsDepthAndPath(t *testing.T) {
	direct := []CallerRef{{QualifiedName: "pkg.direct", Depth: 1, Weight: 1, PreRename: true}}
	cfg := score.Config{MaxDepth: 3, Decay: 0.5}

	got := expandPreRenameCallers(context.Background(), chainQuerier{}, direct, cfg, "pkg.RuHelp")

	if len(got) != 1 {
		t.Fatalf("got %d expanded callers %+v, want 1", len(got), got)
	}
	c := got[0]
	if c.QualifiedName != "pkg.mid" {
		t.Fatalf("QualifiedName = %q, want pkg.mid", c.QualifiedName)
	}
	// pkg.mid is 1 hop from pkg.direct, which is itself 1 hop from the
	// renamed symbol - so 2 from the symbol, reached through pkg.direct.
	if c.Depth != 2 {
		t.Errorf("Depth = %d, want 2 (one hop further than the direct caller)", c.Depth)
	}
	if len(c.Path) != 1 || c.Path[0] != "pkg.direct" {
		t.Errorf("Path = %v, want [pkg.direct]", c.Path)
	}
	if !c.PreRename {
		t.Error("PreRename = false, want true - the whole branch reaches via the old name")
	}
}

func TestExpandPreRenameCallersSkipsAlreadyDirectCallers(t *testing.T) {
	// pkg.mid is already a direct pre-rename caller, so its rediscovery at
	// depth 2 must not add a duplicate at the deeper depth.
	direct := []CallerRef{
		{QualifiedName: "pkg.direct", Depth: 1, Weight: 1, PreRename: true},
		{QualifiedName: "pkg.mid", Depth: 1, Weight: 1, PreRename: true},
	}
	cfg := score.Config{MaxDepth: 3, Decay: 0.5}

	for _, c := range expandPreRenameCallers(context.Background(), chainQuerier{}, direct, cfg, "pkg.RuHelp") {
		if c.QualifiedName == "pkg.mid" {
			t.Fatalf("pkg.mid re-added at depth %d despite already being a direct caller", c.Depth)
		}
	}
}

func TestExpandPreRenameCallersEmptyInput(t *testing.T) {
	cfg := score.Config{MaxDepth: 3, Decay: 0.5}
	if got := expandPreRenameCallers(context.Background(), chainQuerier{}, nil, cfg, "pkg.X"); got != nil {
		t.Fatalf("got %+v, want nil when there are no direct pre-rename callers", got)
	}
	// MaxDepth 1 leaves no hop budget for expansion.
	direct := []CallerRef{{QualifiedName: "pkg.direct", Depth: 1, Weight: 1, PreRename: true}}
	if got := expandPreRenameCallers(context.Background(), chainQuerier{}, direct, score.Config{MaxDepth: 1, Decay: 0.5}, "pkg.X"); got != nil {
		t.Fatalf("got %+v, want nil when MaxDepth leaves no room to expand", got)
	}
}
