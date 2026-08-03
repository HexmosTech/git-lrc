// Package score computes symbol "importance" (a bounded, depth-decayed
// count of distinct transitive callers) and aggregates it into per-hunk
// blast-radius scores.
package score

import (
	"context"
	"fmt"
	"math"

	"github.com/HexmosTech/blastradius/client"
)

// Config tunes the fan-in computation. Zero values fall back to Defaults().
type Config struct {
	// MaxDepth is how many CALLS hops to traverse looking for callers.
	// Depth 1 = direct callers, depth 2 = callers-of-callers, etc.
	MaxDepth int
	// Decay is the per-extra-hop weight multiplier applied to callers found
	// at depth > 1 (a depth-1 caller always has weight 1.0). Must be in
	// (0, 1]; e.g. 0.5 means a depth-2 caller counts half as much as a
	// direct caller, depth-3 counts a quarter.
	Decay float64
	// MaxRows caps each individual query_graph call. 0 uses the tool's own
	// default/ceiling.
	MaxRows int
	// Transform compresses the linear sum of decayed caller weights
	// (SymbolScore.LinearSum) into SymbolScore.Raw. Without it, one hub
	// symbol with dozens of callers can swallow a diff's entire normalized
	// 0-100 scale while everything else clusters near 0 - a concave
	// (diminishing-returns) transform preserves ordering (more callers is
	// still always a higher score) while taming that dominance. Defaults to
	// math.Sqrt. Pass an identity function to disable.
	Transform func(float64) float64
}

// Defaults returns the recommended starting Config: depth 3, decay 0.5,
// sqrt-saturated aggregation.
func Defaults() Config {
	return Config{MaxDepth: 3, Decay: 0.5, MaxRows: 20000, Transform: math.Sqrt}
}

func (c Config) normalized() Config {
	if c.MaxDepth <= 0 {
		c.MaxDepth = Defaults().MaxDepth
	}
	if c.Decay <= 0 || c.Decay > 1 {
		c.Decay = Defaults().Decay
	}
	if c.Transform == nil {
		c.Transform = Defaults().Transform
	}
	return c
}

// GraphQuerier is the subset of client.Client this package depends on.
type GraphQuerier interface {
	QueryGraph(ctx context.Context, cypher string, maxRows int) (*client.QueryResult, error)
}

// CallerContribution is one caller's contribution to a symbol's score, kept
// around for transparency/debugging in the explorer UI.
type CallerContribution struct {
	QualifiedName string
	Depth         int
	Weight        float64
	// Path holds the intermediate qualified names between the symbol and
	// this caller, excluding both endpoints. Empty for depth 1. For depth 2,
	// Path[0] is the direct child of the symbol that this caller is reached
	// through. For depth 3, Path[0] is the depth-1 node and Path[1] is the
	// depth-2 node.
	Path []string
}

// SymbolScore is the computed importance of a single symbol.
type SymbolScore struct {
	QualifiedName string
	// LinearSum is the un-transformed sum of all callers' decay-weighted
	// contributions - kept for transparency (e.g. explaining "compressed via
	// sqrt from an underlying weighted-caller-sum of X" in a UI).
	LinearSum float64
	// Raw is Config.Transform(LinearSum) - this is the value that should be
	// used for scoring/aggregation.
	Raw float64
	// Callers lists every distinct caller found within Config.MaxDepth,
	// each counted once at its shallowest discovered depth.
	Callers []CallerContribution
}

// FanIn computes SymbolScore for every symbol in qualifiedNames in as few
// query_graph round trips as possible: one query per depth level (1..MaxDepth),
// each covering the whole batch via a single `WHERE qualified_name IN [...]`
// clause, rather than one query per symbol or per hunk.
func FanIn(ctx context.Context, q GraphQuerier, qualifiedNames []string, cfg Config) (map[string]*SymbolScore, error) {
	cfg = cfg.normalized()
	results := make(map[string]*SymbolScore, len(qualifiedNames))
	if len(qualifiedNames) == 0 {
		return results, nil
	}
	for _, qn := range qualifiedNames {
		results[qn] = &SymbolScore{QualifiedName: qn}
	}
	// caller -> symbol -> best (smallest) depth seen so far + path nodes.
	bestDepth := make(map[string]map[string]int)
	callerPath := make(map[string]map[string][]string)

	symbolList := client.CypherStringList(qualifiedNames)
	for depth := 1; depth <= cfg.MaxDepth; depth++ {
		var cypher string
		if depth == 1 {
			cypher = fmt.Sprintf(
				"MATCH (caller)-[:CALLS]->(f) WHERE f.qualified_name IN %s RETURN f.qualified_name AS symbol, caller.qualified_name AS caller",
				symbolList,
			)
		} else if depth == 2 {
			cypher = fmt.Sprintf(
				"MATCH (caller)-[:CALLS]->(mid)-[:CALLS]->(f) WHERE f.qualified_name IN %s RETURN f.qualified_name AS symbol, caller.qualified_name AS caller, mid.qualified_name AS via",
				symbolList,
			)
		} else if depth == 3 {
			cypher = fmt.Sprintf(
				"MATCH (caller)-[:CALLS]->(mid1)-[:CALLS]->(mid2)-[:CALLS]->(f) WHERE f.qualified_name IN %s RETURN f.qualified_name AS symbol, caller.qualified_name AS caller, mid1.qualified_name AS via1, mid2.qualified_name AS via2",
				symbolList,
			)
		} else {
			cypher = fmt.Sprintf(
				"MATCH (caller)-[:CALLS*%d..%d]->(f) WHERE f.qualified_name IN %s RETURN f.qualified_name AS symbol, caller.qualified_name AS caller",
				depth, depth, symbolList,
			)
		}
		result, err := q.QueryGraph(ctx, cypher, cfg.MaxRows)
		if err != nil {
			return nil, fmt.Errorf("blastradius/score: fan-in query at depth %d: %w", depth, err)
		}
		col := columnIndex(result.Columns)
		for _, row := range result.Rows {
			symbol := col.get(row, "symbol")
			caller := col.get(row, "caller")
			if symbol == "" || caller == "" || symbol == caller {
				continue
			}
			byCaller, ok := bestDepth[symbol]
			if !ok {
				byCaller = make(map[string]int)
				bestDepth[symbol] = byCaller
			}
			if existing, seen := byCaller[caller]; !seen || depth < existing {
				byCaller[caller] = depth
				// Store the path through intermediate nodes.
				byPath, ok := callerPath[symbol]
				if !ok {
					byPath = make(map[string][]string)
					callerPath[symbol] = byPath
				}
				byPath[caller] = nil
				if depth == 2 {
					if via := col.get(row, "via"); via != "" {
						byPath[caller] = []string{via}
					}
				} else if depth >= 3 {
					// MATCH (caller)->(mid1)->(mid2)->(f): mid2 is 1 hop from f (direct),
					// mid1 is 2 hops from f. Build path from root outward.
					var path []string
					if via2 := col.get(row, "via2"); via2 != "" {
						path = append(path, via2)
					}
					if via1 := col.get(row, "via1"); via1 != "" {
						path = append(path, via1)
					}
					if len(path) > 0 {
						byPath[caller] = path
					}
				}
			}
		}
	}

	for symbol, byCaller := range bestDepth {
		s, ok := results[symbol]
		if !ok {
			continue
		}
		for caller, depth := range byCaller {
			weight := 1.0
			for i := 1; i < depth; i++ {
				weight *= cfg.Decay
			}
			s.LinearSum += weight
			s.Callers = append(s.Callers, CallerContribution{
				QualifiedName: caller,
				Depth:         depth,
				Weight:        weight,
				Path:          callerPath[symbol][caller],
			})
		}
		s.Raw = cfg.Transform(s.LinearSum)
	}
	return results, nil
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
