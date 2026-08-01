package blastradius

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/HexmosTech/blastradius/client"
	"github.com/HexmosTech/blastradius/score"
)

// RouteInfo is one HTTP route a symbol directly handles.
type RouteInfo struct {
	Method string
	Path   string
}

// routeHandlers batches a single HANDLES query across qualifiedNames,
// returning which of them directly handle an HTTP route - confirmed live:
// `(handlerMethod)-[:HANDLES]->(Route)`, with the Route's `name` property
// holding the path (e.g. "/:id/assign") and `method` the HTTP verb.
func routeHandlers(ctx context.Context, c GraphQuerier, qualifiedNames []string) map[string]RouteInfo {
	handlers := make(map[string]RouteInfo, len(qualifiedNames))
	if len(qualifiedNames) == 0 {
		return handlers
	}
	cypher := fmt.Sprintf(
		"MATCH (f)-[:HANDLES]->(r:Route) WHERE f.qualified_name IN %s RETURN f.qualified_name AS handler, r.method AS method, r.name AS path",
		client.CypherStringList(qualifiedNames),
	)
	result, err := c.QueryGraph(ctx, cypher, 0)
	if err != nil {
		return handlers
	}
	handlerIdx, methodIdx, pathIdx := -1, -1, -1
	for i, col := range result.Columns {
		switch col {
		case "handler":
			handlerIdx = i
		case "method":
			methodIdx = i
		case "path":
			pathIdx = i
		}
	}
	if handlerIdx == -1 {
		return handlers
	}
	for _, row := range result.Rows {
		if handlerIdx >= len(row) {
			continue
		}
		handler := row[handlerIdx]
		if handler == "" {
			continue
		}
		info := RouteInfo{}
		if methodIdx != -1 && methodIdx < len(row) {
			info.Method = row[methodIdx]
		}
		if pathIdx != -1 && pathIdx < len(row) {
			info.Path = row[pathIdx]
		}
		handlers[handler] = info
	}
	return handlers
}

// entryPointFlags batches a single is_entry_point lookup across arbitrary
// qualified names - used for callers discovered via the fan-in walk, which
// live outside the touched files symbols.InFile already covers (that
// already fetches is_entry_point for touched symbols themselves).
func entryPointFlags(ctx context.Context, c GraphQuerier, qualifiedNames []string) map[string]bool {
	flags := make(map[string]bool, len(qualifiedNames))
	if len(qualifiedNames) == 0 {
		return flags
	}
	cypher := fmt.Sprintf(
		"MATCH (f) WHERE f.qualified_name IN %s AND f.is_entry_point = true RETURN f.qualified_name AS qn",
		client.CypherStringList(qualifiedNames),
	)
	result, err := c.QueryGraph(ctx, cypher, 0)
	if err != nil {
		return flags
	}
	qnIdx := -1
	for i, col := range result.Columns {
		if col == "qn" {
			qnIdx = i
		}
	}
	if qnIdx == -1 {
		return flags
	}
	for _, row := range result.Rows {
		if qnIdx < len(row) && row[qnIdx] != "" {
			flags[row[qnIdx]] = true
		}
	}
	return flags
}

func (r RouteInfo) String() string {
	if r.Method == "" && r.Path == "" {
		return ""
	}
	return fmt.Sprintf("%s %s", r.Method, r.Path)
}

// entryReachabilityUnit is the per-caller weight (before depth decay,
// already baked into CallerContribution.Weight) for a caller that is itself
// a route handler or other entry point. Set below entryPointBonus (2.0):
// being reached-from is one hop removed from being an entry point yourself,
// but still a real external-contract risk invisible to plain fan-in.
const entryReachabilityUnit = 1.5

// entryReachabilitySignal checks whether any caller in a symbol's fan-in
// walk is itself a route handler or entry point, returning a single Signal
// summarizing every distinct match found (deduped by label, keeping the
// strongest/shallowest weight per label) - or nil if none are found.
func entryReachabilitySignal(callers []score.CallerContribution, routesByQN map[string]RouteInfo, entryFlagsByQN map[string]bool) *Signal {
	bestWeight := make(map[string]float64)
	for _, caller := range callers {
		var label string
		if r, ok := routesByQN[caller.QualifiedName]; ok && r.String() != "" {
			label = r.String()
		} else if entryFlagsByQN[caller.QualifiedName] {
			label = lastSegment(caller.QualifiedName)
		} else {
			continue
		}
		if caller.Weight > bestWeight[label] {
			bestWeight[label] = caller.Weight
		}
	}
	if len(bestWeight) == 0 {
		return nil
	}

	labels := make([]string, 0, len(bestWeight))
	total := 0.0
	for label, weight := range bestWeight {
		labels = append(labels, label)
		total += entryReachabilityUnit * weight
	}
	sort.Strings(labels)
	detail := strings.Join(labels, ", ")
	const maxListed = 5
	if len(labels) > maxListed {
		detail = strings.Join(labels[:maxListed], ", ") + fmt.Sprintf(", +%d more", len(labels)-maxListed)
	}

	return &Signal{
		Name:     fmt.Sprintf("Reached from %d service entry point(s)", len(bestWeight)),
		Detail:   detail,
		Points:   total,
		Category: "architecture",
	}
}

// lastSegment returns the final dot-separated component of a qualified
// name, e.g. "home-shrsv-bin-LiveReview.cmd.main" -> "main".
func lastSegment(qualifiedName string) string {
	parts := strings.Split(qualifiedName, ".")
	return parts[len(parts)-1]
}
