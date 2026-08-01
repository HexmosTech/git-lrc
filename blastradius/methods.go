package blastradius

import (
	"context"
	"fmt"

	"github.com/HexmosTech/blastradius/client"
)

// methodsByParentClass batches a single query across every touched
// Struct/Class/Interface qualified name (same batching style as
// score.FanIn), returning each one's own methods' qualified names, via the
// Method node's parent_class property (confirmed live to exactly match the
// owning type's qualified_name). Types with no methods (or that failed to
// query) are simply absent from the result map.
//
// This lets a struct's blast-radius score be grounded partly in its own
// methods' CALLS-based fan-in - not just how often its name is grepped -
// since a struct's real behavioral impact lives in what its methods do, not
// in field/constructor mentions alone.
func methodsByParentClass(ctx context.Context, c GraphQuerier, typeQNs []string) map[string][]string {
	byParent := make(map[string][]string, len(typeQNs))
	if len(typeQNs) == 0 {
		return byParent
	}
	cypher := fmt.Sprintf(
		"MATCH (m:Method) WHERE m.parent_class IN %s RETURN m.parent_class AS parent, m.qualified_name AS qn",
		client.CypherStringList(typeQNs),
	)
	result, err := c.QueryGraph(ctx, cypher, 0)
	if err != nil {
		return byParent
	}
	parentIdx, qnIdx := -1, -1
	for i, col := range result.Columns {
		switch col {
		case "parent":
			parentIdx = i
		case "qn":
			qnIdx = i
		}
	}
	if parentIdx == -1 || qnIdx == -1 {
		return byParent
	}
	for _, row := range result.Rows {
		if parentIdx >= len(row) || qnIdx >= len(row) {
			continue
		}
		parent, qn := row[parentIdx], row[qnIdx]
		if parent == "" || qn == "" {
			continue
		}
		byParent[parent] = append(byParent[parent], qn)
	}
	return byParent
}
