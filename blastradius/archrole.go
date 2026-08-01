package blastradius

import (
	"context"
	"fmt"
	"regexp"

	"github.com/HexmosTech/blastradius/client"
)

// The following are lightweight, cross-language keyword-on-path heuristics
// for architectural roles the graph has no dedicated signal for. They are
// explicitly heuristic (false positive/negative risk accepted) - the
// tradeoff the user asked for over a heavier per-language semantic
// analysis. Each is a hunk-level (file-scoped) signal, not per-symbol,
// since "is this an auth file" is a property of the file being touched.
var (
	authPathRe        = regexp.MustCompile(`(?i)(^|[/_-])(auth|session|token|login|credential|jwt|oauth|permission|acl)([/_.-]|$)`)
	persistencePathRe = regexp.MustCompile(`(?i)(^|[/_-])(storage|repository|repo|dao|migrations?|db|models?)([/_.-]|$)`)
	configPathRe      = regexp.MustCompile(`(?i)(config|settings)|\.env(\.|$)|\.ya?ml$|\.toml$`)
	buildPathRe       = regexp.MustCompile(`(?i)(^|/)(Makefile|Dockerfile|go\.mod|go\.sum|package\.json|package-lock\.json)$|\.github/workflows/`)
	schemaPathRe      = regexp.MustCompile(`(?i)migrations?/|schema\.sql$|\.proto$|openapi`)
	cliSchedulerRe    = regexp.MustCompile(`(?i)(^|/)cmd/|cron|scheduler|worker|consumer|/jobqueue/`)
)

// archRoleSignals scores a hunk's own file path against the keyword
// heuristics above. Multiple can fire for one file (e.g. a persistence file
// under CI config would be unusual but not impossible).
func archRoleSignals(filePath string) []Signal {
	var out []Signal
	add := func(name, detail string, points float64) {
		out = append(out, Signal{Name: name, Detail: detail, Points: points, Category: "architecture"})
	}
	if authPathRe.MatchString(filePath) {
		add("Authentication-related", "file path matches an auth/session/credential naming pattern", 2.0)
	}
	if persistencePathRe.MatchString(filePath) {
		add("Persistence layer", "file path matches a storage/repository/migration naming pattern", 1.5)
	}
	if configPathRe.MatchString(filePath) {
		add("Configuration", "file path matches a config/settings naming pattern", 1.0)
	}
	if buildPathRe.MatchString(filePath) {
		add("Build system", "file path matches a build/CI naming pattern (Makefile, Dockerfile, go.mod, CI workflow, etc.)", 1.5)
	}
	if schemaPathRe.MatchString(filePath) {
		add("Schema change", "file path matches a migration/schema/API-contract naming pattern", 2.0)
	}
	if cliSchedulerRe.MatchString(filePath) {
		add("CLI / scheduler", "file path matches a cmd/cron/scheduler/worker naming pattern", 1.0)
	}
	return out
}

// symbolsThatWriteData batches a single WRITES-edge existence check across
// qualifiedNames - a graph-backed (not just path-heuristic) persistence
// signal: this symbol actually mutates persistent state, not just "lives in
// a storage-sounding directory".
func symbolsThatWriteData(ctx context.Context, c GraphQuerier, qns []string) map[string]bool {
	writers := make(map[string]bool, len(qns))
	if len(qns) == 0 {
		return writers
	}
	cypher := fmt.Sprintf(
		"MATCH (f)-[:WRITES]->(x) WHERE f.qualified_name IN %s RETURN DISTINCT f.qualified_name AS qn",
		client.CypherStringList(qns),
	)
	result, err := c.QueryGraph(ctx, cypher, 0)
	if err != nil {
		return writers
	}
	qnIdx := -1
	for i, col := range result.Columns {
		if col == "qn" {
			qnIdx = i
		}
	}
	if qnIdx == -1 {
		return writers
	}
	for _, row := range result.Rows {
		if qnIdx < len(row) && row[qnIdx] != "" {
			writers[row[qnIdx]] = true
		}
	}
	return writers
}
