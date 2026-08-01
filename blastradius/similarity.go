package blastradius

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/HexmosTech/blastradius/client"
)

// similarityThreshold is the minimum jaccard similarity (0-1) for a
// SIMILAR_TO match to be considered "duplicated logic" worth flagging.
// 0.85 is high enough to filter out coincidental structural similarity
// between unrelated small functions.
const similarityThreshold = 0.85

// duplicationUnit is the base weight per matched clone; cross-file matches
// (same_file=false) are weighted 1.5x - a duplicate living in a different
// file is the higher-risk "did you remember to fix it in both places" case
// the user described, versus a same-file near-duplicate (often intentional
// overloads/variants, lower coordination risk).
const duplicationUnit = 2.0

// SimilarSymbol is one SIMILAR_TO match for a touched symbol.
type SimilarSymbol struct {
	QualifiedName string
	Jaccard       float64
	SameFile      bool
}

// similarSymbols batches a single SIMILAR_TO query across qualifiedNames
// (undirected - confirmed live the edge is queryable from either side),
// returning matches at or above similarityThreshold.
func similarSymbols(ctx context.Context, c GraphQuerier, qns []string) map[string][]SimilarSymbol {
	byQN := make(map[string][]SimilarSymbol, len(qns))
	if len(qns) == 0 {
		return byQN
	}
	cypher := fmt.Sprintf(
		"MATCH (a)-[r:SIMILAR_TO]-(b) WHERE a.qualified_name IN %s AND r.jaccard >= %s "+
			"RETURN a.qualified_name AS a, b.qualified_name AS b, r.jaccard AS jaccard, r.same_file AS same_file",
		client.CypherStringList(qns), strconv.FormatFloat(similarityThreshold, 'f', -1, 64),
	)
	result, err := c.QueryGraph(ctx, cypher, 0)
	if err != nil {
		return byQN
	}
	aIdx, bIdx, jaccardIdx, sameFileIdx := -1, -1, -1, -1
	for i, col := range result.Columns {
		switch col {
		case "a":
			aIdx = i
		case "b":
			bIdx = i
		case "jaccard":
			jaccardIdx = i
		case "same_file":
			sameFileIdx = i
		}
	}
	if aIdx == -1 || bIdx == -1 {
		return byQN
	}
	for _, row := range result.Rows {
		if aIdx >= len(row) || bIdx >= len(row) {
			continue
		}
		a, b := row[aIdx], row[bIdx]
		if a == "" || b == "" || a == b {
			continue
		}
		match := SimilarSymbol{QualifiedName: b}
		if jaccardIdx != -1 && jaccardIdx < len(row) {
			match.Jaccard, _ = strconv.ParseFloat(row[jaccardIdx], 64)
		}
		if sameFileIdx != -1 && sameFileIdx < len(row) {
			match.SameFile = row[sameFileIdx] == "true"
		}
		byQN[a] = append(byQN[a], match)
	}
	return byQN
}

// duplicationSignal summarizes every SIMILAR_TO match for a symbol into one
// Signal - "changing one implementation when many similar implementations
// exist increases review complexity" (did you remember to fix it
// everywhere?), a reviewer-attention claim, not a blast-radius one.
func duplicationSignal(matches []SimilarSymbol) *Signal {
	if len(matches) == 0 {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Jaccard > matches[j].Jaccard })

	total := 0.0
	var parts []string
	const maxListed = 3
	for i, m := range matches {
		weight := duplicationUnit * m.Jaccard
		if !m.SameFile {
			weight *= 1.5
		}
		total += weight
		if i < maxListed {
			scope := "same file"
			if !m.SameFile {
				scope = "different file"
			}
			parts = append(parts, fmt.Sprintf("%.0f%% similar to %s (%s)", m.Jaccard*100, lastSegment(m.QualifiedName), scope))
		}
	}
	detail := strings.Join(parts, "; ")
	if len(matches) > maxListed {
		detail += fmt.Sprintf("; +%d more", len(matches)-maxListed)
	}

	return &Signal{
		Name:     "Similar implementation exists elsewhere",
		Detail:   detail,
		Points:   total,
		Category: "duplication",
	}
}
