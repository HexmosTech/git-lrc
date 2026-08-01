// Package symbols maps diff hunks to the code symbols (functions, methods,
// structs, classes, interfaces) they touch, using a codebase-memory-mcp
// knowledge graph as the source of truth for symbol locations.
package symbols

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/HexmosTech/blastradius/client"
	"github.com/HexmosTech/blastradius/diffparse"
)

// Symbol is one function/method/struct/class/interface node from the
// knowledge graph, scoped to the file it's defined in.
type Symbol struct {
	QualifiedName string
	Name          string
	Label         string
	FilePath      string
	StartLine     int
	EndLine       int

	// The following are review-priority signals, not blast-radius signals -
	// see blastradius.go. Present on Function/Method (and, for the boolean/
	// complexity fields, Struct/Class/Interface too); zero-valued when the
	// underlying property doesn't apply to this symbol's label.
	Complexity   int // cyclomatic complexity
	Cognitive    int // cognitive complexity
	LoopDepth    int // max nested-loop depth
	OutDegree    int // fan-out: how many other symbols this one calls/uses
	InDegree     int // fan-in: how many other symbols call/use this one, repo-wide (not diff-scoped)
	IsEntryPoint bool
	IsExported   bool
	IsTest       bool
	// RouteMethod/RoutePath are set when this symbol is itself an HTTP route
	// handler (e.g. "GET", "/api/v1/users") - blast radius reaching outside
	// the codebase via an external API contract, invisible to CALLS edges.
	RouteMethod string
	RoutePath   string
}

// symbolLabels are the node labels considered "symbols" for blast-radius
// purposes; File/Folder/Module/Package/Variable nodes are excluded.
var symbolLabels = []string{"Function", "Method", "Struct", "Class", "Interface"}

// GraphQuerier is the subset of client.Client this package depends on,
// allowing tests to substitute a fake without shelling out to the real
// codebase-memory-mcp binary.
type GraphQuerier interface {
	QueryGraph(ctx context.Context, cypher string, maxRows int) (*client.QueryResult, error)
}

// InFile returns every symbol defined in filePath, in no particular order.
// One query_graph call per file — callers should invoke this once per
// distinct file touched by a diff, not once per hunk.
func InFile(ctx context.Context, q GraphQuerier, filePath string) ([]Symbol, error) {
	preds := make([]string, len(symbolLabels))
	for i, l := range symbolLabels {
		preds[i] = "f:" + l
	}
	labelPred := strings.Join(preds, " OR ")
	cypher := fmt.Sprintf(
		"MATCH (f) WHERE f.file_path = %s AND (%s) RETURN "+
			"f.name AS name, f.qualified_name AS qualified_name, f.label AS label, "+
			"f.start_line AS start_line, f.end_line AS end_line, "+
			"f.complexity AS complexity, f.cognitive AS cognitive, f.loop_depth AS loop_depth, "+
			"f.out_degree AS out_degree, f.in_degree AS in_degree, f.is_entry_point AS is_entry_point, "+
			"f.is_exported AS is_exported, f.is_test AS is_test, "+
			"f.route_method AS route_method, f.route_path AS route_path",
		client.CypherString(filePath), labelPred,
	)

	result, err := q.QueryGraph(ctx, cypher, 0)
	if err != nil {
		return nil, fmt.Errorf("blastradius/symbols: querying symbols for %s: %w", filePath, err)
	}

	col := columnIndex(result.Columns)
	symbols := make([]Symbol, 0, len(result.Rows))
	for _, row := range result.Rows {
		start, _ := strconv.Atoi(col.get(row, "start_line"))
		end, _ := strconv.Atoi(col.get(row, "end_line"))
		complexity, _ := strconv.Atoi(col.get(row, "complexity"))
		cognitive, _ := strconv.Atoi(col.get(row, "cognitive"))
		loopDepth, _ := strconv.Atoi(col.get(row, "loop_depth"))
		outDegree, _ := strconv.Atoi(col.get(row, "out_degree"))
		inDegree, _ := strconv.Atoi(col.get(row, "in_degree"))
		symbols = append(symbols, Symbol{
			Name:          col.get(row, "name"),
			QualifiedName: col.get(row, "qualified_name"),
			Label:         col.get(row, "label"),
			FilePath:      filePath,
			StartLine:     start,
			EndLine:       end,
			Complexity:    complexity,
			Cognitive:     cognitive,
			LoopDepth:     loopDepth,
			OutDegree:     outDegree,
			InDegree:      inDegree,
			IsEntryPoint:  col.get(row, "is_entry_point") == "true",
			IsExported:    col.get(row, "is_exported") == "true",
			IsTest:        col.get(row, "is_test") == "true",
			RouteMethod:   col.get(row, "route_method"),
			RoutePath:     col.get(row, "route_path"),
		})
	}
	return symbols, nil
}

// ForHunk returns the symbols from fileSymbols (all belonging to the same
// file) whose [StartLine, EndLine] range overlaps the hunk's changed range
// on the new side of the diff. For a pure-deletion hunk (NewLines == 0), the
// hunk's NewStart line (the anchor point in the post-change file) is used as
// a single-line probe, since there is no new-side content to compute a range
// from.
func ForHunk(fileSymbols []Symbol, hunk diffparse.Hunk) []Symbol {
	lo := hunk.NewStart
	hi := hunk.NewStart
	if hunk.NewLines > 0 {
		hi = hunk.NewStart + hunk.NewLines - 1
	}

	var touched []Symbol
	for _, s := range fileSymbols {
		if s.StartLine == 0 && s.EndLine == 0 {
			continue // e.g. the File node itself, or a symbol with no known range
		}
		if s.StartLine <= hi && lo <= s.EndLine {
			touched = append(touched, s)
		}
	}
	return touched
}

type columns []string

func columnIndex(cols []string) columns { return columns(cols) }

func (c columns) get(row []string, name string) string {
	for i, col := range c {
		if col == name && i < len(row) {
			return row[i]
		}
	}
	return ""
}
