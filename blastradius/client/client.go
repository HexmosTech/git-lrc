// Package client talks to a locally installed codebase-memory-mcp binary via
// its one-shot "cli" subcommand mode (not the MCP stdio server), following the
// same exec.Command-based external-tool pattern already used elsewhere for
// shelling out to CLIs. See https://github.com/DeusData/codebase-memory-mcp.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultBinary = "codebase-memory-mcp"

// DefaultTimeout bounds a single CLI invocation so a slow/unresponsive graph
// query can never hang a caller indefinitely.
const DefaultTimeout = 30 * time.Second

// Client runs codebase-memory-mcp's "cli <tool>" one-shot mode against a
// single already-indexed project.
type Client struct {
	// Binary is the executable name or path. Defaults to "codebase-memory-mcp"
	// (resolved via PATH) when empty.
	Binary string
	// Project is the codebase-memory-mcp project name to query (see
	// list_projects). Required for every call.
	Project string
	// Timeout bounds each individual CLI invocation. Defaults to
	// DefaultTimeout when zero.
	Timeout time.Duration
}

// New returns a Client for the given already-indexed project name, using the
// default binary name and timeout.
func New(project string) *Client {
	return &Client{Project: project}
}

func (c *Client) binary() string {
	if c.Binary != "" {
		return c.Binary
	}
	return defaultBinary
}

func (c *Client) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultTimeout
}

// Available reports whether the codebase-memory-mcp binary can be found on
// PATH. Callers should check this before relying on the client and degrade
// gracefully (skip scoring) rather than failing outright when it's absent.
func (c *Client) Available() error {
	_, err := exec.LookPath(c.binary())
	if err != nil {
		return fmt.Errorf("codebase-memory-mcp not found on PATH: %w", err)
	}
	return nil
}

// run executes `<binary> cli <tool> --flag value ...` and returns raw stdout.
// stderr (the tool logs informational lines there, e.g. "level=info msg=...")
// is captured separately so it never corrupts the JSON stdout payload, and is
// folded into the returned error only on failure.
func (c *Client) run(ctx context.Context, tool string, args ...string) ([]byte, error) {
	if c.Project == "" {
		return nil, fmt.Errorf("blastradius/client: Project is required")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()

	fullArgs := append([]string{"cli", tool, "--project", c.Project}, args...)
	cmd := exec.CommandContext(ctx, c.binary(), fullArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("codebase-memory-mcp cli %s: timed out after %s", tool, c.timeout())
		}
		return nil, fmt.Errorf("codebase-memory-mcp cli %s: %w: %s", tool, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// QueryResult is the raw response shape of `cli query_graph`: a set of named
// columns and rows whose values are, per observed behavior of the tool,
// always JSON strings (even for numeric/boolean Cypher results).
type QueryResult struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
	Total   int        `json:"total"`
}

// QueryGraph runs a read-only Cypher query against the project's knowledge
// graph via `cli query_graph --query <cypher>`. maxRows <= 0 leaves the
// tool's own default/ceiling in place.
func (c *Client) QueryGraph(ctx context.Context, cypher string, maxRows int) (*QueryResult, error) {
	args := []string{"--query", cypher}
	if maxRows > 0 {
		args = append(args, "--max-rows", strconv.Itoa(maxRows))
	}
	out, err := c.run(ctx, "query_graph", args...)
	if err != nil {
		return nil, err
	}
	var result QueryResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("blastradius/client: parsing query_graph output: %w", err)
	}
	return &result, nil
}

// CodeUsage is a text-based occurrence summary for a symbol name, used as a
// fallback importance signal for symbols with no CALLS edges of their own
// (structs, interfaces, types) - the knowledge graph only models call
// relationships, not "this function references type X" relationships.
type CodeUsage struct {
	// TotalMatches is the raw grep-style hit count across the project.
	TotalMatches int
	// Directories lists every directory containing at least one match,
	// sorted - a cheap proxy for "which parts of the codebase this symbol
	// reaches", since we already pay for the search either way.
	Directories []string
}

// SearchCodeUsage runs `cli search_code --mode files` for pattern and
// summarizes the result.
func (c *Client) SearchCodeUsage(ctx context.Context, pattern string) (*CodeUsage, error) {
	out, err := c.run(ctx, "search_code", "--pattern", pattern, "--mode", "files", "--limit", "1")
	if err != nil {
		return nil, err
	}
	var result struct {
		TotalGrepMatches int            `json:"total_grep_matches"`
		Directories      map[string]int `json:"directories"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("blastradius/client: parsing search_code output: %w", err)
	}
	dirs := make([]string, 0, len(result.Directories))
	for d := range result.Directories {
		dirs = append(dirs, strings.TrimSuffix(d, "/"))
	}
	sort.Strings(dirs)
	return &CodeUsage{TotalMatches: result.TotalGrepMatches, Directories: dirs}, nil
}

// CodeSymbolMatch is one textual hit resolved to the graph node that encloses
// it - search_code's "compact" mode maps each grep match back to the symbol
// whose source range contains it, which is what makes it usable as a caller
// list for names that have no node of their own (see SearchCodeSymbols).
type CodeSymbolMatch struct {
	QualifiedName string `json:"qualified_name"`
	Name          string `json:"node"`
	Label         string `json:"label"`
	File          string `json:"file"`
	StartLine     int    `json:"start_line"`
	// MatchLines are the 1-based line numbers inside this symbol where the
	// pattern actually occurred.
	MatchLines []int `json:"match_lines"`
}

// CodeSymbolMatches is the result of SearchCodeSymbols.
type CodeSymbolMatches struct {
	// Matches are the enclosing symbols, deduplicated by the tool itself
	// (one entry per symbol, however many lines inside it matched).
	Matches []CodeSymbolMatch
	// TotalMatches is the raw grep-style hit count, the same number
	// SearchCodeUsage reports - unaffected by the enrichment limit.
	TotalMatches int
	// Truncated reports that the tool found more enclosing symbols than the
	// limit allowed it to enrich, so Matches is a partial list.
	Truncated bool
}

// SearchCodeSymbols runs `cli search_code --mode compact` for pattern and
// returns the graph symbols enclosing each match. Unlike SearchCodeUsage
// (which only counts hits), this identifies *who* references the name - the
// only way to find references to a name the graph has no node for, e.g. a
// symbol's pre-rename name after the tree was reindexed. limit <= 0 leaves
// the tool's own default in place.
func (c *Client) SearchCodeSymbols(ctx context.Context, pattern string, limit int) (*CodeSymbolMatches, error) {
	args := []string{"--pattern", pattern, "--mode", "compact"}
	if limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}
	out, err := c.run(ctx, "search_code", args...)
	if err != nil {
		return nil, err
	}
	var result struct {
		Results          []CodeSymbolMatch `json:"results"`
		TotalGrepMatches int               `json:"total_grep_matches"`
		TotalResults     int               `json:"total_results"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("blastradius/client: parsing search_code output: %w", err)
	}
	return &CodeSymbolMatches{
		Matches:      result.Results,
		TotalMatches: result.TotalGrepMatches,
		Truncated:    result.TotalResults > len(result.Results),
	}, nil
}

// ArchitectureEntryPoint is one entry in get_architecture's "entry_points"
// aspect - a real, cross-language entry point (main functions, extension
// activate/deactivate hooks, script mains), not just the is_entry_point
// property on individual nodes.
type ArchitectureEntryPoint struct {
	Name          string `json:"name"`
	QualifiedName string `json:"qualified_name"`
	File          string `json:"file"`
}

// ArchitectureHotspot is one entry in the "hotspots" aspect: a repo-wide
// top-fan-in symbol, precomputed by the tool.
type ArchitectureHotspot struct {
	Name          string `json:"name"`
	QualifiedName string `json:"qualified_name"`
	FanIn         int    `json:"fan_in"`
}

// ArchitectureLayer is one entry in the "layers" aspect: a package
// classified as "api" (has HTTP routes), "entry" (only outbound calls),
// "core" (high fan-in), or "internal".
type ArchitectureLayer struct {
	Name   string `json:"name"` // package name
	Layer  string `json:"layer"`
	Reason string `json:"reason"`
}

// ArchitectureCluster is one Louvain community-detection cluster from the
// "clusters" aspect - a real, detected functional module, not just a
// directory grouping.
type ArchitectureCluster struct {
	ID       int      `json:"id"`
	Label    string   `json:"label"`
	Members  int      `json:"members"`
	Cohesion float64  `json:"cohesion"`
	TopNodes []string `json:"top_nodes"`
	Packages []string `json:"packages"`
}

// ArchitectureContext is the response shape of `cli get_architecture`,
// limited to the fields blastradius uses.
type ArchitectureContext struct {
	Project     string                   `json:"project"`
	TotalNodes  int                      `json:"total_nodes"`
	TotalEdges  int                      `json:"total_edges"`
	EntryPoints []ArchitectureEntryPoint `json:"entry_points"`
	Hotspots    []ArchitectureHotspot    `json:"hotspots"`
	Layers      []ArchitectureLayer      `json:"layers"`
	Clusters    []ArchitectureCluster    `json:"clusters"`
}

// GetArchitecture runs `cli get_architecture --aspects a --aspects b ...`
// (confirmed live: the --aspects array flag is passed by repeating it once
// per value, not comma-joined or JSON-encoded). Meant to be called once per
// report and cached - this is architecture-wide, not per-hunk/symbol.
func (c *Client) GetArchitecture(ctx context.Context, aspects []string) (*ArchitectureContext, error) {
	var args []string
	for _, a := range aspects {
		args = append(args, "--aspects", a)
	}
	out, err := c.run(ctx, "get_architecture", args...)
	if err != nil {
		return nil, err
	}
	var result ArchitectureContext
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("blastradius/client: parsing get_architecture output: %w", err)
	}
	return &result, nil
}

// ProjectInfo is the subset of `cli list_projects` output we care about.
type ProjectInfo struct {
	Name     string `json:"name"`
	RootPath string `json:"root_path"`
	Nodes    int    `json:"nodes"`
	Edges    int    `json:"edges"`
}

// ListProjects returns every project codebase-memory-mcp currently has
// indexed, regardless of c.Project.
func (c *Client) ListProjects(ctx context.Context) ([]ProjectInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()

	cmd := exec.CommandContext(ctx, c.binary(), "cli", "list_projects")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("codebase-memory-mcp cli list_projects: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	var payload struct {
		Projects []ProjectInfo `json:"projects"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		return nil, fmt.Errorf("blastradius/client: parsing list_projects output: %w", err)
	}
	return payload.Projects, nil
}

// IndexRepository creates or incrementally refreshes the knowledge-graph
// index for the repository at repoPath, returning the project name the tool
// derived (or reused) for it. Unlike the query methods it does NOT apply
// c.Timeout - a first-time index of a large repository legitimately takes
// minutes - so callers bound it via ctx instead. mode is the tool's indexing
// mode ("fast", "moderate", "full"); empty uses the tool default.
func (c *Client) IndexRepository(ctx context.Context, repoPath, mode string) (string, error) {
	args := []string{"cli", "index_repository", "--repo-path", repoPath}
	if mode != "" {
		args = append(args, "--mode", mode)
	}
	cmd := exec.CommandContext(ctx, c.binary(), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("codebase-memory-mcp cli index_repository: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	var payload struct {
		Project string `json:"project"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		return "", fmt.Errorf("blastradius/client: parsing index_repository output: %w", err)
	}
	if payload.Project == "" {
		return "", fmt.Errorf("blastradius/client: index_repository returned no project name (status %q)", payload.Status)
	}
	return payload.Project, nil
}

// ProjectIndexed reports whether c.Project appears in the current
// list_projects output.
func (c *Client) ProjectIndexed(ctx context.Context) (bool, error) {
	projects, err := c.ListProjects(ctx)
	if err != nil {
		return false, err
	}
	for _, p := range projects {
		if p.Name == c.Project {
			return true, nil
		}
	}
	return false, nil
}

// CypherString renders a Go string as a single-quoted Cypher string literal,
// escaping backslashes and single quotes.
func CypherString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return "'" + s + "'"
}

// CypherStringList renders a slice of Go strings as a Cypher list literal,
// e.g. []string{"a","b"} -> "['a','b']".
func CypherStringList(items []string) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = CypherString(it)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
