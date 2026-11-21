// Package crossrepo is the CLI half of cross-repo blast radius.
//
// The CLI never clones or indexes a related repository. It extracts the
// outbound HTTP calls of the symbols its own diff touches (from the local
// codebase-memory-mcp project it already indexed for scoring), asks LiveReview
// to match those calls against its related-repo graphs, and folds the returned
// impacts/chains into the local report with the same helpers the server-side
// worker uses.
package crossrepo

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/HexmosTech/git-lrc/blastradius"
	"github.com/HexmosTech/git-lrc/blastradius/client"
	"github.com/HexmosTech/git-lrc/network"
)

const (
	// MaxSymbols bounds the request so one huge diff cannot turn into an
	// unbounded server-side graph walk.
	MaxSymbols = 500
	// MaxCallsPerSymbol bounds one symbol's outbound call list.
	MaxCallsPerSymbol = 50
	// DefaultDepth matches the server worker's chain budget.
	DefaultDepth = 3
)

// Call is one outbound HTTP call site.
type Call struct {
	Method string `json:"method,omitempty"`
	Path   string `json:"path"`
}

// SymbolCalls is one touched symbol and its outbound calls.
type SymbolCalls struct {
	Key   string `json:"key"`
	Calls []Call `json:"calls"`
}

// RepoRef is the server's own repositories row for the reviewed repo.
type RepoRef struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	WebURL   string `json:"web_url"`
}

// RelatedRepo is one related repo the server used.
type RelatedRepo struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	WebURL   string `json:"web_url"`
	Depth    int    `json:"depth"`
	Indexed  bool   `json:"indexed"`
}

// MatchesResponse mirrors LiveReview's
// POST /api/v1/diff-review/:id/cross-repo/matches response.
type MatchesResponse struct {
	ReviewID     int64                                    `json:"review_id"`
	Self         *RepoRef                                 `json:"self"`
	RelatedRepos []RelatedRepo                            `json:"related_repos"`
	Impacts      map[string][]blastradius.CrossRepoImpact `json:"impacts"`
	Chains       map[string][]blastradius.CrossRepoChain  `json:"chains"`
	Diagnostics  []string                                 `json:"diagnostics,omitempty"`
}

// Symbols returns every distinct touched symbol qualified name in report,
// deterministically ordered and capped.
func Symbols(report *blastradius.Report) []string {
	if report == nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, 16)
	for _, file := range report.Files {
		for _, hunk := range file.Hunks {
			for _, sym := range hunk.Symbols {
				if sym.QualifiedName == "" || seen[sym.QualifiedName] {
					continue
				}
				seen[sym.QualifiedName] = true
				out = append(out, sym.QualifiedName)
				if len(out) >= MaxSymbols {
					sort.Strings(out)
					return out
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// ExtractOutboundCalls queries the reviewed repo's own graph for the outbound
// HTTP calls of symbolQNs. One query, no indexing.
func ExtractOutboundCalls(ctx context.Context, binary, project string, symbolQNs []string) (map[string][]Call, error) {
	out := map[string][]Call{}
	if len(symbolQNs) == 0 {
		return out, nil
	}
	if len(symbolQNs) > MaxSymbols {
		symbolQNs = symbolQNs[:MaxSymbols]
	}
	c := &client.Client{Binary: binary, Project: project, Timeout: 30 * time.Second}
	cypher := fmt.Sprintf(
		"MATCH (f)-[:HTTP_CALLS]->(r) WHERE f.qualified_name IN %s RETURN f.qualified_name AS symbol, r.method AS method, r.name AS path",
		client.CypherStringList(symbolQNs),
	)
	result, err := c.QueryGraph(ctx, cypher, 0)
	if err != nil {
		return nil, fmt.Errorf("query outbound calls: %w", err)
	}
	symbolIdx, methodIdx, pathIdx := -1, -1, -1
	for i, col := range result.Columns {
		switch col {
		case "symbol":
			symbolIdx = i
		case "method":
			methodIdx = i
		case "path":
			pathIdx = i
		}
	}
	if symbolIdx == -1 || pathIdx == -1 {
		return out, nil
	}
	for _, row := range result.Rows {
		if symbolIdx >= len(row) || pathIdx >= len(row) {
			continue
		}
		key := row[symbolIdx]
		path := row[pathIdx]
		if key == "" || path == "" {
			continue
		}
		if len(out[key]) >= MaxCallsPerSymbol {
			continue
		}
		method := ""
		if methodIdx >= 0 && methodIdx < len(row) {
			method = row[methodIdx]
		}
		out[key] = append(out[key], Call{Method: method, Path: path})
	}
	return out, nil
}

// BuildSymbolCalls turns the extraction map into a deterministic request body.
func BuildSymbolCalls(calls map[string][]Call) []SymbolCalls {
	keys := make([]string, 0, len(calls))
	for k := range calls {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]SymbolCalls, 0, len(keys))
	for _, k := range keys {
		if len(calls[k]) == 0 {
			continue
		}
		out = append(out, SymbolCalls{Key: k, Calls: calls[k]})
	}
	return out
}

// FetchMatches asks the server to match symbols to routes in related repos.
// It is intentionally a single bounded request with no retries: cross-repo
// enrichment must never delay or fail a review.
func FetchMatches(ctx context.Context, apiURL, apiKey string, reviewID int64, symbols []SymbolCalls, depth int, timeout time.Duration) (*MatchesResponse, error) {
	if reviewID <= 0 {
		return nil, fmt.Errorf("review id is required")
	}
	if depth <= 0 {
		depth = DefaultDepth
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	payload := map[string]any{"symbols": symbols, "depth": depth}
	httpClient := network.NewReviewAPIClient(timeout)
	resp, err := network.ReviewCrossRepoMatches(httpClient, apiURL, fmt.Sprintf("%d", reviewID), payload, apiKey)
	if err != nil {
		return nil, fmt.Errorf("cross-repo match request: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cross-repo match request rejected (status %d): %s", resp.StatusCode, truncate(string(resp.Body), 300))
	}
	var out MatchesResponse
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("parse cross-repo match response: %w", err)
	}
	return &out, nil
}

// Apply folds a matches response into report. It is idempotent for a given
// response (RecordCrossRepoImpact replaces prior bonuses) and renormalizes
// once at the end. A symbol touched by several hunks keeps its first hunk,
// matching the server worker's rule.
func Apply(report *blastradius.Report, resp *MatchesResponse) (impactedSymbols, chainedSymbols int) {
	if report == nil || resp == nil {
		return 0, 0
	}
	locate := make(map[string]struct {
		filePath           string
		newStart, newLines int
	}, len(resp.Impacts))
	for _, file := range report.Files {
		for _, hunk := range file.Hunks {
			for _, sym := range hunk.Symbols {
				if sym.QualifiedName == "" {
					continue
				}
				if _, ok := locate[sym.QualifiedName]; ok {
					continue
				}
				locate[sym.QualifiedName] = struct {
					filePath           string
					newStart, newLines int
				}{file.Path, hunk.NewStart, hunk.NewLines}
			}
		}
	}

	changed := false
	for key, impacts := range resp.Impacts {
		loc, ok := locate[key]
		if !ok {
			continue
		}
		if blastradius.RecordCrossRepoImpact(report, loc.filePath, loc.newStart, loc.newLines, key, impacts) {
			impactedSymbols++
			changed = true
		}
	}
	for key, chains := range resp.Chains {
		loc, ok := locate[key]
		if !ok {
			continue
		}
		if blastradius.RecordCrossRepoChains(report, loc.filePath, loc.newStart, loc.newLines, key, chains) {
			chainedSymbols++
			changed = true
		}
	}
	for _, d := range resp.Diagnostics {
		report.CrossRepoDiagnostics = append(report.CrossRepoDiagnostics, "cross_repo: "+d)
	}
	if changed {
		blastradius.Renormalize(report)
	}
	return impactedSymbols, chainedSymbols
}

// RelatedRepoCount returns how many related repos the server actually used.
func RelatedRepoCount(resp *MatchesResponse) int {
	if resp == nil {
		return 0
	}
	return len(resp.RelatedRepos)
}

// Diagnostics returns a short, UI-safe description of why enrichment did not
// happen. The caller decides whether to log it.
func Diagnostics(resp *MatchesResponse) []string {
	if resp == nil {
		return nil
	}
	return resp.Diagnostics
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
