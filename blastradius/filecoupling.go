package blastradius

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/HexmosTech/blastradius/client"
)

// fileCouplingBonus batches a single FILE_CHANGES_WITH query across every
// touched file, returning a per-file blast-radius bonus derived from how
// strongly (and how often) that file has historically changed alongside
// other files. This is blast radius invisible to any static call graph -
// shared config, generated code, cross-cutting concerns that only show up
// as "these files keep changing together" in git history, which
// codebase-memory-mcp already precomputes.
//
// Uses an undirected pattern (confirmed live: FILE_CHANGES_WITH edges are
// queryable in either direction) so a touched file picks up coupling data
// regardless of which side of the edge it was recorded on.
func fileCouplingBonus(ctx context.Context, c GraphQuerier, filePaths []string) map[string]float64 {
	bonus := make(map[string]float64, len(filePaths))
	if len(filePaths) == 0 {
		return bonus
	}
	cypher := fmt.Sprintf(
		"MATCH (a:File)-[r:FILE_CHANGES_WITH]-(b:File) WHERE a.file_path IN %s "+
			"RETURN a.file_path AS file, r.coupling_score AS coupling_score",
		client.CypherStringList(filePaths),
	)
	result, err := c.QueryGraph(ctx, cypher, 0)
	if err != nil {
		return bonus
	}
	fileIdx, scoreIdx := -1, -1
	for i, col := range result.Columns {
		switch col {
		case "file":
			fileIdx = i
		case "coupling_score":
			scoreIdx = i
		}
	}
	if fileIdx == -1 || scoreIdx == -1 {
		return bonus
	}
	sums := make(map[string]float64, len(filePaths))
	for _, row := range result.Rows {
		if fileIdx >= len(row) || scoreIdx >= len(row) {
			continue
		}
		file := row[fileIdx]
		if file == "" {
			continue
		}
		score, _ := strconv.ParseFloat(row[scoreIdx], 64)
		sums[file] += score
	}
	// Saturate the same way as every other aggregate in this package, so one
	// heavily-coupled file doesn't dominate the bonus term.
	for file, sum := range sums {
		bonus[file] = math.Sqrt(sum)
	}
	return bonus
}
