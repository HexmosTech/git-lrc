package blastradius

import (
	"context"
	"fmt"
	"strconv"

	"github.com/HexmosTech/blastradius/client"
)

// implementsInfo is one interface a struct/class implements.
type implementsInfo struct {
	interfaceQN   string
	interfaceName string
}

// implementedInterfaces batches a single IMPLEMENTS query, returning which
// interface(s) each of the given (presumably Struct/Class) qualified names
// implements. Symbols with no outbound IMPLEMENTS edge are simply absent.
func implementedInterfaces(ctx context.Context, c GraphQuerier, qns []string) map[string][]implementsInfo {
	byImpl := make(map[string][]implementsInfo, len(qns))
	if len(qns) == 0 {
		return byImpl
	}
	cypher := fmt.Sprintf(
		"MATCH (impl)-[:IMPLEMENTS]->(iface) WHERE impl.qualified_name IN %s RETURN impl.qualified_name AS impl, iface.qualified_name AS iface, iface.name AS iface_name",
		client.CypherStringList(qns),
	)
	result, err := c.QueryGraph(ctx, cypher, 0)
	if err != nil {
		return byImpl
	}
	implIdx, ifaceIdx, nameIdx := -1, -1, -1
	for i, col := range result.Columns {
		switch col {
		case "impl":
			implIdx = i
		case "iface":
			ifaceIdx = i
		case "iface_name":
			nameIdx = i
		}
	}
	if implIdx == -1 || ifaceIdx == -1 {
		return byImpl
	}
	for _, row := range result.Rows {
		if implIdx >= len(row) || ifaceIdx >= len(row) {
			continue
		}
		impl, iface := row[implIdx], row[ifaceIdx]
		if impl == "" || iface == "" {
			continue
		}
		name := ""
		if nameIdx != -1 && nameIdx < len(row) {
			name = row[nameIdx]
		}
		byImpl[impl] = append(byImpl[impl], implementsInfo{interfaceQN: iface, interfaceName: name})
	}
	return byImpl
}

// implementerCounts batches a single query returning, for each interface
// qualified name given, how many distinct structs/classes implement it -
// used both for "this struct implements a widely-used interface" (plugin
// point) and "this interface itself has N implementers that must stay
// compatible" (touching the interface directly).
func implementerCounts(ctx context.Context, c GraphQuerier, ifaceQNs []string) map[string]int {
	counts := make(map[string]int, len(ifaceQNs))
	if len(ifaceQNs) == 0 {
		return counts
	}
	cypher := fmt.Sprintf(
		"MATCH (impl)-[:IMPLEMENTS]->(iface) WHERE iface.qualified_name IN %s RETURN iface.qualified_name AS iface, count(DISTINCT impl) AS implementer_count",
		client.CypherStringList(ifaceQNs),
	)
	result, err := c.QueryGraph(ctx, cypher, 0)
	if err != nil {
		return counts
	}
	ifaceIdx, countIdx := -1, -1
	for i, col := range result.Columns {
		switch col {
		case "iface":
			ifaceIdx = i
		case "implementer_count":
			countIdx = i
		}
	}
	if ifaceIdx == -1 || countIdx == -1 {
		return counts
	}
	for _, row := range result.Rows {
		if ifaceIdx >= len(row) || countIdx >= len(row) {
			continue
		}
		iface := row[ifaceIdx]
		if iface == "" {
			continue
		}
		n, _ := strconv.Atoi(row[countIdx])
		counts[iface] = n
	}
	return counts
}
