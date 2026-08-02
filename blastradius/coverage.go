package blastradius

import (
	"context"
	"fmt"
	"strconv"

	"github.com/HexmosTech/blastradius/client"
)

// testCoverageCounts batches a single TESTS-edge query across every touched
// symbol (same batching style as score.FanIn): direction is
// (testFunction)-[:TESTS]->(testedFunction), confirmed live against a real
// codebase-memory-mcp graph. Symbols with no direct test are simply absent
// from the result map (callers should treat a missing entry as 0), and a
// query failure degrades to an empty map rather than failing the report -
// coverage-awareness is enrichment, not a hard requirement.
func testCoverageCounts(ctx context.Context, c GraphQuerier, qualifiedNames []string) map[string]int {
	counts := make(map[string]int, len(qualifiedNames))
	if len(qualifiedNames) == 0 {
		return counts
	}
	cypher := fmt.Sprintf(
		"MATCH (t)-[:TESTS]->(f) WHERE f.qualified_name IN %s RETURN f.qualified_name AS symbol, count(DISTINCT t) AS test_count",
		client.CypherStringList(qualifiedNames),
	)
	result, err := c.QueryGraph(ctx, cypher, 0)
	if err != nil {
		return counts
	}
	symbolIdx, countIdx := -1, -1
	for i, col := range result.Columns {
		switch col {
		case "symbol":
			symbolIdx = i
		case "test_count":
			countIdx = i
		}
	}
	if symbolIdx == -1 || countIdx == -1 {
		return counts
	}
	for _, row := range result.Rows {
		if symbolIdx >= len(row) || countIdx >= len(row) {
			continue
		}
		symbol := row[symbolIdx]
		if symbol == "" {
			continue
		}
		n, _ := strconv.Atoi(row[countIdx])
		counts[symbol] = n
	}
	return counts
}
