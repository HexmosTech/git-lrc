// Package blastradius scores diff hunks along two dimensions:
//
//   - BlastRadius: how broadly the change could affect the rest of the
//     repository (who/what breaks if this is wrong).
//   - ReviewPriority: how much attention this specific hunk deserves from a
//     reviewer, independent of who else is affected.
//
// Combined blends the two (see Weights) into one ranking number, but both
// dimensions - and every individual Signal that feeds them - are always
// exposed, since collapsing everything into one opaque number would hide
// *why* a hunk scored the way it did. Every Signal carries a human-readable
// Name/Detail alongside its point contribution, so BlastRadiusRaw and
// ReviewPriorityRaw are always literally the sum of their Signals' Points -
// the number and its explanation can never drift apart.
//
// BlastRadius uses one of two methods, depending on what the graph can tell
// us about a symbol:
//   - "calls": for Function/Method symbols, a bounded transitive fan-in over
//     CALLS edges (see package score) - direct callers count fully, callers
//     of callers count less, etc.
//   - "text-references": for Struct/Interface/Class/Type/Enum symbols, the
//     graph has no "references type X" edges at all (only call edges), so
//     these fall back to a grep-based occurrence count via codebase-memory-mcp's
//     search_code tool, blended with the aggregated call-graph fan-in of the
//     type's own methods (see methods.go).
//
// It exposes two entrypoints:
//   - ScoreDiff, for standalone use (e.g. from the blastradius CLI): feed it
//     raw unified-diff bytes.
//   - ScoreHunks, for embedding into a tool that has already parsed a diff
//     itself (e.g. git-lrc): feed it already-parsed hunks directly, no
//     re-parsing and no dependency on this module's diffparse package.
package blastradius

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/HexmosTech/blastradius/client"
	"github.com/HexmosTech/blastradius/diffparse"
	"github.com/HexmosTech/blastradius/score"
	"github.com/HexmosTech/blastradius/symbols"
)

// Hunk is the minimal per-hunk input needed for scoring. It deliberately
// mirrors diffparse.Hunk's relevant fields so a caller with its own diff
// parser (like git-lrc's parseDiffToFiles) can construct it directly.
type Hunk struct {
	FilePath string
	Header   string
	NewStart int
	NewLines int
	// Content is the hunk body (context/added/removed lines, each still
	// prefixed with ' '/'+'/'-'), newline-joined - optional, used only for
	// display (e.g. the blastradius CLI's explorer report). Callers that
	// already have a copy of the diff elsewhere (like git-lrc, which renders
	// its own diff view) can leave this empty.
	Content string
}

// Weights controls how BlastRadius and ReviewPriority (each already
// normalized to 0-100 within a Report) blend into Combined. Should sum to
// 1.0 so Combined stays within 0-100; DefaultWeights favors BlastRadius
// somewhat, since "what breaks" is usually the sharper triage signal, with
// ReviewPriority as a meaningful but secondary tiebreaker/booster.
type Weights struct {
	BlastRadius    float64
	ReviewPriority float64
}

// DefaultWeights returns the recommended starting blend: 60% BlastRadius,
// 40% ReviewPriority.
func DefaultWeights() Weights {
	return Weights{BlastRadius: 0.6, ReviewPriority: 0.4}
}

func (w Weights) normalized() Weights {
	if w.BlastRadius <= 0 && w.ReviewPriority <= 0 {
		return DefaultWeights()
	}
	total := w.BlastRadius + w.ReviewPriority
	if total <= 0 {
		return DefaultWeights()
	}
	return Weights{BlastRadius: w.BlastRadius / total, ReviewPriority: w.ReviewPriority / total}
}

// Options bundles every tunable for a scoring run.
type Options struct {
	Score   score.Config
	Weights Weights
	// Binary optionally points at a specific codebase-memory-mcp executable
	// (e.g. an lrc-managed install outside PATH). Empty means "resolve
	// codebase-memory-mcp via PATH".
	Binary string
}

// DefaultOptions returns score.Defaults() paired with DefaultWeights().
func DefaultOptions() Options {
	return Options{Score: score.Defaults(), Weights: DefaultWeights()}
}

// GraphQuerier is the subset of client.Client this package's own helper
// queries (coverage.go, methods.go) depend on, allowing tests to substitute
// a fake without shelling out to the real codebase-memory-mcp binary - same
// pattern as score.GraphQuerier / symbols.GraphQuerier.
type GraphQuerier interface {
	QueryGraph(ctx context.Context, cypher string, maxRows int) (*client.QueryResult, error)
}

// Signal is one independently-detected, human-explainable contribution to a
// score. BlastRadiusRaw/ReviewPriorityRaw are always exactly the sum of
// their respective Signals' Points (see blastRadiusCategories/
// reviewPriorityCategories) - there is no hidden math beyond this list.
type Signal struct {
	// Name is a short label, e.g. "Exported API", "Cross-package impact".
	Name string
	// Detail is the human-readable specifics, e.g. "reached from 3 service
	// entry points: POST /users, POST /orgs/{id}/users".
	Detail string
	// Points is this signal's signed contribution.
	Points float64
	// Category determines which dimension this signal feeds:
	// "architecture" | "graph" -> BlastRadius; "duplication" | "code-metrics"
	// -> ReviewPriority. "diff-shape" signals are informational/dampening
	// only (see hygiene.go) and are not summed into either raw score.
	Category string
}

var blastRadiusCategories = map[string]bool{"architecture": true, "graph": true}
var reviewPriorityCategories = map[string]bool{"duplication": true, "code-metrics": true}

func sumSignalPoints(signals []Signal, categories map[string]bool) float64 {
	total := 0.0
	for _, s := range signals {
		if categories[s.Category] {
			total += s.Points
		}
	}
	return total
}

// CallerRef is one caller contributing to a symbol's score. Only populated
// for the "calls" scoring method - text-references has no per-caller detail,
// just a count.
type CallerRef struct {
	QualifiedName string
	Depth         int // 1 = direct caller, 2+ = transitive
	Weight        float64
	// Path holds the intermediate qualified names between the symbol and
	// this caller (see CallerContribution.Path).
	Path []string
}

// SymbolContribution describes one touched symbol's part of a hunk's score.
type SymbolContribution struct {
	QualifiedName string
	Name          string
	Label         string
	// Method is "calls" (Function/Method, via CALLS-edge fan-in) or
	// "text-references" (everything else, via grep occurrence count) -
	// see the package doc comment.
	Method string

	// Signals is every independently-detected contribution to this symbol's
	// score - BlastRadiusRaw/ReviewPriorityRaw are always derived from this
	// list (see sumSignalPoints), never computed separately.
	Signals []Signal

	BlastRadiusRaw    float64
	ReviewPriorityRaw float64

	// DirectCount is depth-1 callers for "calls", or the raw reference count
	// for "text-references" (which has no depth concept).
	DirectCount int
	// TransitiveCount is callers found at depth > 1. Always 0 for
	// "text-references".
	TransitiveCount int
	// Callers is the full caller list (only for "calls"), sorted by depth
	// then name, for showing exactly which transitive calls contributed.
	Callers []CallerRef
	// ImpactedPackages are the distinct packages/directories this symbol's
	// influence reaches: for "calls", the packages its callers live in; for
	// "text-references", the directories search_code found matches in.
	ImpactedPackages []string
	// MethodBlastRadius is only set for "text-references" symbols that have
	// their own methods (Struct/Class): the aggregated CALLS-based blast
	// radius of those methods, blended into BlastRadiusRaw alongside the
	// grep-based reference count - 0 if the type has no methods.
	MethodBlastRadius float64

	// The following feed ReviewPriorityRaw's code-metrics Signals and are
	// exposed individually so a UI can explain the number, not just show it.
	IsEntryPoint bool
	Complexity   int
	Cognitive    int
	LoopDepth    int
	OutDegree    int
	// TestCount is how many distinct test functions directly TEST this
	// symbol (0 if none, or if this symbol's label doesn't have direct
	// tests, e.g. most structs). Low/no coverage raises ReviewPriorityRaw -
	// undetected breakage risk is a reviewer-attention signal, not a
	// blast-radius one.
	TestCount int
}

// HunkReport is the computed score for one hunk, along both dimensions.
type HunkReport struct {
	FilePath string
	Header   string
	NewStart int
	NewLines int
	Content  string // see Hunk.Content; empty unless the input Hunk set it

	// Signals collects hunk-level (not per-symbol) contributions, currently
	// just file-level co-change coupling. Per-symbol signals live on each
	// SymbolContribution instead - see Symbols.
	Signals []Signal

	BlastRadiusRaw  float64
	BlastRadiusNorm float64 // 0-100, relative to the highest BlastRadiusRaw in this Report
	// MaxBlastRadiusRaw is the denominator BlastRadiusNorm was divided by
	// (the highest BlastRadiusRaw across every hunk in this Report) - exposed
	// so a UI can show the normalization arithmetic explicitly
	// (BlastRadiusRaw / MaxBlastRadiusRaw * 100 == BlastRadiusNorm) instead of
	// presenting BlastRadiusNorm as an unexplained number. Same value on every
	// hunk in a Report.
	MaxBlastRadiusRaw float64
	// MaxBlastRadiusHunkFile/Header identify *which* hunk set
	// MaxBlastRadiusRaw (the first one encountered with that raw score, if
	// several tie) - a UI showing "divided by the highest score in this
	// diff" should be able to say which hunk that is, not just its number.
	MaxBlastRadiusHunkFile   string
	MaxBlastRadiusHunkHeader string
	ReviewPriorityRaw        float64
	ReviewPriorityNorm       float64 // 0-100, relative to the highest ReviewPriorityRaw in this Report
	// MaxReviewPriorityRaw mirrors MaxBlastRadiusRaw for ReviewPriorityNorm.
	MaxReviewPriorityRaw        float64
	MaxReviewPriorityHunkFile   string
	MaxReviewPriorityHunkHeader string
	// Combined is (Weights.BlastRadius*BlastRadiusNorm + Weights.ReviewPriority*ReviewPriorityNorm)
	// * HygieneMultiplier, already on a 0-100 scale - the single number to
	// sort by if you want one ranking, though BlastRadiusNorm/
	// ReviewPriorityNorm remain available for showing *why*.
	Combined float64
	// HygieneMultiplier is a [0,1] dampener applied to Combined for diff-shape
	// signals (formatting-only, comments-only, generated code, etc.) - see
	// classifyHunkHygiene. 1.0 (no effect) unless one of those fired.
	HygieneMultiplier float64
	// Weights is the actual (normalized) blend ratio Combined was computed
	// with - exposed so a UI can show the Combined formula with the real
	// numbers this report used rather than assuming DefaultWeights(). Same
	// value on every hunk in a Report.
	Weights Weights

	Symbols []SymbolContribution
	// ImpactedPackages is the union of every touched symbol's
	// ImpactedPackages, sorted.
	ImpactedPackages []string
	// FileCouplingBonus is the (already-saturated) contribution baked into
	// BlastRadiusRaw from this hunk's file historically changing alongside
	// other files (FILE_CHANGES_WITH) - exposed separately for transparency,
	// see fileCouplingBonus.
	FileCouplingBonus float64
}

// FileReport groups HunkReports for one file, in diff order.
type FileReport struct {
	Path  string
	Hunks []HunkReport
}

// PackageImpact summarizes how many hunks (and how severely) reach a given
// package/directory, across the whole report.
type PackageImpact struct {
	Package           string
	HunkCount         int
	MaxBlastRadiusRaw float64
}

// Report is the full result of scoring a diff (or a set of hunks).
type Report struct {
	Project     string
	GeneratedAt time.Time
	Files       []FileReport
	// ImpactedPackages ranks packages/directories by how many scored hunks
	// reach them, descending - a coarse "which parts of the codebase does
	// this change ripple into" view.
	ImpactedPackages []PackageImpact
}

// ScoreDiff parses raw unified-diff bytes and scores every hunk against the
// given codebase-memory-mcp project.
func ScoreDiff(ctx context.Context, diffBytes []byte, project string, opts ...Options) (*Report, error) {
	files, err := diffparse.Parse(diffBytes)
	if err != nil {
		return nil, fmt.Errorf("blastradius: parsing diff: %w", err)
	}

	var hunks []Hunk
	for _, f := range files {
		if f.IsBinary || f.Path == "" {
			continue
		}
		for _, h := range f.Hunks {
			hunks = append(hunks, Hunk{
				FilePath: f.Path,
				Header:   h.Header,
				NewStart: h.NewStart,
				NewLines: h.NewLines,
				Content:  h.Content,
			})
		}
	}
	return ScoreHunks(ctx, project, hunks, opts...)
}

// packageOf derives a coarse package/directory grouping from a
// codebase-memory-mcp qualified_name of the form
// "<project>.<pkg>.<pkg>....<symbol>", by dropping the project prefix and
// the symbol name suffix. Returns "" if there's nothing in between.
func packageOf(qualifiedName string) string {
	parts := strings.Split(qualifiedName, ".")
	if len(parts) <= 2 {
		return ""
	}
	return strings.Join(parts[1:len(parts)-1], "/")
}

func sortedUnique(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// entryPointBonus is a flat BlastRadiusRaw addition for symbols that are
// themselves HTTP/CLI entry points. Internal CALLS-edge fan-in is blind to
// this risk entirely - an entry point can have zero internal callers (it's
// invoked from outside the codebase) yet changing it breaks an external
// contract. Kept as a simple flat bonus rather than something scaled, since
// there's no graph signal available to scale it by.
const entryPointBonus = 2.0

// packageDiversityBonus rewards a symbol's callers being spread across many
// packages over the same count concentrated in one: N callers in one package
// are usually one call site pattern repeated, easier to reason about in one
// pass; N callers spread across N different packages means N different
// contexts to individually verify aren't broken. log1p keeps this a gentle,
// diminishing-returns nudge rather than letting package count dominate the
// call-graph signal it's supplementing.
func packageDiversityBonus(packages []string) float64 {
	if len(packages) <= 1 {
		return 0
	}
	return math.Log1p(float64(len(packages)))
}

// codeMetricSignals scores how much attention a symbol's *own* code deserves,
// independent of blast radius, as a list of individually-explainable Signals
// (only emitted when non-zero, to avoid noise). Weights are deliberately
// simple and tunable:
//   - Complexity (cyclomatic): each independent path through the code is
//     something a reviewer has to individually trace - weighted 1:1 as the
//     baseline unit.
//   - Cognitive complexity: tracks nesting/control-flow hardness more than
//     raw branch count, and empirically runs somewhat higher than cyclomatic
//     for the same function - down-weighted to 0.5 so it doesn't just double
//     count the same signal as Complexity.
//   - LoopDepth: nested loops are a well-known hotspot for subtle bugs
//     (off-by-one, quadratic blowups) - weighted heavily (x3) despite its
//     small numeric range (usually 0-3).
//   - OutDegree (fan-out): a symbol that calls/uses many other things has
//     more surface area where a change could have knock-on effects - a
//     light secondary signal.
//   - Missing test coverage: an untested symbol carries more undetected-
//     breakage risk than an identical, well-tested one. `3/(1+testCount)`
//     gives a full +3 boost at zero coverage, halves at one test, and fades
//     out past a handful - a diminishing bonus, not a cliff at "any
//     coverage at all". Always emitted (even at low points) since "N direct
//     tests" is useful context either way.
//
// NOTE: these are deliberately weighted as *supplementary* signals, not the
// primary "how much attention does this deserve" driver - that's now the
// architecture/graph/duplication signals built elsewhere (routes.go,
// interfaces.go, similarity.go, archrole.go), which better capture customer/
// production impact than intrinsic code complexity alone.
func codeMetricSignals(s symbols.Symbol, testCount int) []Signal {
	var out []Signal
	if s.Complexity > 0 {
		out = append(out, Signal{
			Name: "Cyclomatic complexity", Detail: fmt.Sprintf("complexity score %d", s.Complexity),
			Points: float64(s.Complexity) * 1.0, Category: "code-metrics",
		})
	}
	if s.Cognitive > 0 {
		out = append(out, Signal{
			Name: "Cognitive complexity", Detail: fmt.Sprintf("cognitive score %d", s.Cognitive),
			Points: float64(s.Cognitive) * 0.5, Category: "code-metrics",
		})
	}
	if s.LoopDepth > 0 {
		out = append(out, Signal{
			Name: "Nested loops", Detail: fmt.Sprintf("max loop nesting depth %d", s.LoopDepth),
			Points: float64(s.LoopDepth) * 3.0, Category: "code-metrics",
		})
	}
	if s.OutDegree > 0 {
		out = append(out, Signal{
			Name: "Fan-out", Detail: fmt.Sprintf("calls/uses %d other symbols", s.OutDegree),
			Points: float64(s.OutDegree) * 0.3, Category: "code-metrics",
		})
	}
	covDetail := fmt.Sprintf("%d direct test(s)", testCount)
	if testCount == 0 {
		covDetail = "no direct test coverage"
	}
	out = append(out, Signal{
		Name: "Test coverage", Detail: covDetail,
		Points: 3.0 / (1.0 + float64(testCount)), Category: "code-metrics",
	})
	return out
}

// ScoreHunks scores an already-parsed set of hunks against the given
// codebase-memory-mcp project. Hunks are processed file-by-file: each
// distinct FilePath incurs exactly one symbol lookup, regardless of how many
// hunks touch it, and the whole batch of touched symbols across every hunk
// is fanned-in with a fixed small number of graph queries (see score.FanIn).
func ScoreHunks(ctx context.Context, project string, hunks []Hunk, opts ...Options) (*Report, error) {
	o := DefaultOptions()
	if len(opts) > 0 {
		o = opts[0]
	}

	c := client.New(project)
	c.Binary = o.Binary
	if err := c.Available(); err != nil {
		return nil, err
	}
	weights := o.Weights.normalized()
	maxDepth := o.Score.MaxDepth
	if maxDepth <= 0 {
		maxDepth = score.Defaults().MaxDepth
	}

	report := &Report{Project: project, GeneratedAt: time.Now()}

	// Preserve file order of first appearance; group hunks per file.
	var fileOrder []string
	hunksByFile := make(map[string][]Hunk)
	for _, h := range hunks {
		if _, seen := hunksByFile[h.FilePath]; !seen {
			fileOrder = append(fileOrder, h.FilePath)
		}
		hunksByFile[h.FilePath] = append(hunksByFile[h.FilePath], h)
	}

	// One symbols.InFile call per distinct file.
	symbolsByFile := make(map[string][]symbols.Symbol, len(fileOrder))
	for _, path := range fileOrder {
		fileSymbols, err := symbols.InFile(ctx, c, path)
		if err != nil {
			return nil, err
		}
		symbolsByFile[path] = fileSymbols
	}

	// Map each hunk to its touched symbols, and collect the deduplicated
	// set of qualified names touched anywhere in the whole batch, split by
	// scoring method: Function/Method symbols have CALLS edges ("calls");
	// everything else falls back to a text-occurrence proxy
	// ("text-references") since the graph only models call relationships.
	type pendingHunk struct {
		hunk    Hunk
		touched []symbols.Symbol
	}
	var pending []pendingHunk
	symbolByQN := make(map[string]symbols.Symbol)
	var callableQN []string
	nameToTypeQNs := make(map[string][]string) // bare Name -> qualified names sharing it
	// renameByQN holds, for every touched symbol whose own declaration line
	// was detected as a single-identifier rename within its hunk, the old
	// name that callers/references elsewhere in the repo are still using
	// until they're migrated (see rename.go) - both scoring paths below use
	// this to score the symbol against BOTH names, since internal callers of
	// the pre-rename name are about to break, which the new name alone can't
	// see.
	renameByQN := make(map[string]DeclRename)
	for _, path := range fileOrder {
		for _, h := range hunksByFile[path] {
			touched := symbols.ForHunk(symbolsByFile[path], diffparse.Hunk{
				NewStart: h.NewStart,
				NewLines: h.NewLines,
			})
			pending = append(pending, pendingHunk{hunk: h, touched: touched})
			for _, s := range touched {
				if _, seen := symbolByQN[s.QualifiedName]; seen {
					continue
				}
				symbolByQN[s.QualifiedName] = s
				if rn := detectDeclRename(h, s); rn != nil {
					renameByQN[s.QualifiedName] = *rn
				}
				if s.Label == "Function" || s.Label == "Method" {
					callableQN = append(callableQN, s.QualifiedName)
				} else {
					nameToTypeQNs[s.Name] = append(nameToTypeQNs[s.Name], s.QualifiedName)
				}
			}
		}
	}

	// Extend callableQN with the touched types' own methods (via
	// parent_class), so a struct's blast radius can be grounded partly in
	// what its methods actually do (CALLS-based fan-in), not just how often
	// its name is grepped - one extra batched query, then folded into the
	// same score.FanIn call rather than a second round trip.
	var typeQNs []string
	for _, qns := range nameToTypeQNs {
		typeQNs = append(typeQNs, qns...)
	}
	methodsByType := methodsByParentClass(ctx, c, typeQNs)

	// Interface implementation/definition signals: which interfaces do the
	// touched structs/classes implement, and how many implementers does
	// each of those interfaces (plus any touched Interface symbol itself)
	// have - two batched queries regardless of how many types are touched.
	implementsByType := implementedInterfaces(ctx, c, typeQNs)
	var ifaceQNs []string
	seenIface := make(map[string]bool)
	for _, qn := range typeQNs {
		if symbolByQN[qn].Label == "Interface" && !seenIface[qn] {
			seenIface[qn] = true
			ifaceQNs = append(ifaceQNs, qn)
		}
	}
	for _, infos := range implementsByType {
		for _, info := range infos {
			if !seenIface[info.interfaceQN] {
				seenIface[info.interfaceQN] = true
				ifaceQNs = append(ifaceQNs, info.interfaceQN)
			}
		}
	}
	implementerCountByIface := implementerCounts(ctx, c, ifaceQNs)
	seenCallable := make(map[string]bool, len(callableQN))
	for _, qn := range callableQN {
		seenCallable[qn] = true
	}
	for _, methodQNs := range methodsByType {
		for _, mqn := range methodQNs {
			if seenCallable[mqn] {
				continue
			}
			seenCallable[mqn] = true
			callableQN = append(callableQN, mqn)
		}
	}

	funcScores, err := score.FanIn(ctx, c, callableQN, o.Score)
	if err != nil {
		return nil, err
	}

	// Batch-check every callable symbol AND every caller found in their
	// fan-in walk for route-handler / entry-point status, in two queries
	// total regardless of how many symbols or callers are involved - same
	// batching discipline as score.FanIn itself.
	var routeCheckQN []string
	seenRouteCheck := make(map[string]bool)
	addRouteCheck := func(qn string) {
		if !seenRouteCheck[qn] {
			seenRouteCheck[qn] = true
			routeCheckQN = append(routeCheckQN, qn)
		}
	}
	for qn, ss := range funcScores {
		addRouteCheck(qn)
		for _, caller := range ss.Callers {
			addRouteCheck(caller.QualifiedName)
		}
	}
	routesByQN := routeHandlers(ctx, c, routeCheckQN)
	entryFlagsByQN := entryPointFlags(ctx, c, routeCheckQN)

	// One get_architecture call, cached for the whole report: hotspot/layer
	// classification per touched symbol, plus a second, independently-
	// derived source of entry-point confirmation folded into the same
	// reachability check above (get_architecture's entry_points list can
	// catch cross-language entry points - e.g. a TypeScript extension's
	// activate() - that is_entry_point alone might miss).
	archCtx := fetchArchitectureContext(ctx, c)
	for qn := range archCtx.entryPointQN {
		entryFlagsByQN[qn] = true
	}

	// contribByQN holds the fully-built SymbolContribution (minus
	// QualifiedName/Name/Label, filled in per-occurrence below) for every
	// scored symbol, computed once regardless of how many hunks touch it.
	contribByQN := make(map[string]SymbolContribution, len(symbolByQN))
	for qn, ss := range funcScores {
		sort.Slice(ss.Callers, func(i, j int) bool {
			if ss.Callers[i].Depth != ss.Callers[j].Depth {
				return ss.Callers[i].Depth < ss.Callers[j].Depth
			}
			return ss.Callers[i].QualifiedName < ss.Callers[j].QualifiedName
		})
		var callers []CallerRef
		var packages []string
		direct, transitive := 0, 0
		for _, caller := range ss.Callers {
			callers = append(callers, CallerRef{QualifiedName: caller.QualifiedName, Depth: caller.Depth, Weight: caller.Weight, Path: caller.Path})
			packages = append(packages, packageOf(caller.QualifiedName))
			if caller.Depth == 1 {
				direct++
			} else {
				transitive++
			}
		}
		impactedPackages := sortedUnique(packages)

		rename, renamed := renameByQN[qn]
		callerReachDetail := fmt.Sprintf("%d direct + %d transitive caller(s), up to %d hops", direct, transitive, maxDepth)
		if renamed {
			callerReachDetail = fmt.Sprintf("%d direct + %d transitive caller(s) already migrated to the new name %q, up to %d hops", direct, transitive, rename.NewName, maxDepth)
		}
		var signals []Signal
		signals = append(signals, Signal{
			Name:     "Caller reach",
			Detail:   callerReachDetail,
			Points:   ss.Raw,
			Category: "graph",
		})
		if renamed {
			signals = append(signals, renameOldNameSignal(ctx, c, rename))
		}
		if div := packageDiversityBonus(impactedPackages); div > 0 {
			signals = append(signals, Signal{
				Name:     "Cross-package impact",
				Detail:   fmt.Sprintf("callers span %d packages: %s", len(impactedPackages), strings.Join(impactedPackages, ", ")),
				Points:   div,
				Category: "graph",
			})
		} else {
			signals = append(signals, Signal{
				Name:     "Cross-package impact",
				Detail:   fmt.Sprintf("callers concentrated in %d package(s), no cross-package spread", len(impactedPackages)),
				Points:   0,
				Category: "graph",
			})
		}
		if route, ok := routesByQN[qn]; ok && route.String() != "" {
			signals = append(signals, Signal{
				Name:     "HTTP handler",
				Detail:   route.String(),
				Points:   entryPointBonus,
				Category: "architecture",
			})
		} else if symbolByQN[qn].IsEntryPoint || symbolByQN[qn].RouteMethod != "" || entryFlagsByQN[qn] {
			signals = append(signals, Signal{
				Name:     "Entry point",
				Detail:   "directly reachable from outside the codebase (e.g. CLI/main)",
				Points:   entryPointBonus,
				Category: "architecture",
			})
		} else {
			signals = append(signals, Signal{
				Name:     "Entry point",
				Detail:   "not a route handler, not flagged as an entry point",
				Points:   0,
				Category: "architecture",
			})
		}

		// Transitive entry-point reachability: does any caller within the
		// fan-in walk itself handle a route or count as an entry point?
		// Internal CALLS-edge fan-in alone can't see this - a function with
		// only 2 direct callers is still high-risk if one of those callers
		// is a service entry point reached from outside the codebase.
		if reach := entryReachabilitySignal(ss.Callers, routesByQN, entryFlagsByQN); reach != nil {
			signals = append(signals, *reach)
		}
		if hotspot := archCtx.hotspotSignal(qn); hotspot != nil {
			signals = append(signals, *hotspot)
		}
		if layer := archCtx.layerSignal(qn); layer != nil {
			signals = append(signals, *layer)
		}

		contribByQN[qn] = SymbolContribution{
			Method:           "calls",
			Signals:          signals,
			BlastRadiusRaw:   sumSignalPoints(signals, blastRadiusCategories),
			DirectCount:      direct,
			TransitiveCount:  transitive,
			Callers:          callers,
			ImpactedPackages: impactedPackages,
		}
	}
	for name, qns := range nameToTypeQNs {
		usage, err := c.SearchCodeUsage(ctx, name)
		if err != nil {
			continue // best-effort: leave these symbols at raw=0 rather than fail the whole report
		}
		refs := max(usage.TotalMatches-1, 0) // subtract the symbol's own definition line
		textRefBlastRadius := math.Sqrt(float64(refs))

		for _, qn := range qns {
			// Aggregate this specific type's own methods' CALLS-based fan-in
			// (sqrt-of-sum, same saturating style as everywhere else, so one
			// type with many methods doesn't automatically dominate one with
			// few). Weighted at 0.5 relative to the text-reference signal -
			// supplementary, not a replacement, since each signal catches
			// mentions the other misses (field/constructor use vs. actual
			// method-call behavior).
			methodSum := 0.0
			for _, methodQN := range methodsByType[qn] {
				if ms, ok := funcScores[methodQN]; ok {
					methodSum += ms.Raw
				}
			}
			methodBlastRadius := math.Sqrt(methodSum)

			rename, renamed := renameByQN[qn]
			textRefDetail := fmt.Sprintf("%d reference(s) to this name across the codebase", refs)
			if renamed {
				textRefDetail = fmt.Sprintf("%d reference(s) already migrated to the new name %q", refs, rename.NewName)
			}
			var signals []Signal
			signals = append(signals, Signal{
				Name:     "Text references",
				Detail:   textRefDetail,
				Points:   textRefBlastRadius,
				Category: "graph",
			})
			if renamed {
				signals = append(signals, renameOldNameSignal(ctx, c, rename))
			}
			if methodBlastRadius > 0 {
				signals = append(signals, Signal{
					Name:     "Method call-graph activity",
					Detail:   fmt.Sprintf("this type's own methods have an aggregated caller reach of %.2f", methodBlastRadius),
					Points:   0.5 * methodBlastRadius,
					Category: "graph",
				})
			} else if len(methodsByType[qn]) > 0 {
				signals = append(signals, Signal{
					Name:     "Method call-graph activity",
					Detail:   fmt.Sprintf("%d method(s), none reached by any caller within scope", len(methodsByType[qn])),
					Points:   0,
					Category: "graph",
				})
			}
			if div := packageDiversityBonus(usage.Directories); div > 0 {
				signals = append(signals, Signal{
					Name:     "Cross-package impact",
					Detail:   fmt.Sprintf("references span %d directories: %s", len(usage.Directories), strings.Join(usage.Directories, ", ")),
					Points:   div,
					Category: "graph",
				})
			} else {
				signals = append(signals, Signal{
					Name:     "Cross-package impact",
					Detail:   fmt.Sprintf("references concentrated in %d directory(ies), no cross-package spread", len(usage.Directories)),
					Points:   0,
					Category: "graph",
				})
			}

			if symbolByQN[qn].Label == "Interface" {
				n := implementerCountByIface[qn]
				detail := fmt.Sprintf("%d implementer(s) must stay compatible with this interface", n)
				if n == 0 {
					detail = "no known implementers"
				}
				signals = append(signals, Signal{
					Name:     "Interface definition",
					Detail:   detail,
					Points:   math.Log1p(float64(n)) * 1.0,
					Category: "architecture",
				})
			} else if len(implementsByType[qn]) == 0 {
				signals = append(signals, Signal{
					Name:     "Interface implementation",
					Detail:   "implements no interfaces",
					Points:   0,
					Category: "architecture",
				})
			} else {
				for _, info := range implementsByType[qn] {
					// -1 to exclude this symbol itself from its interface's
					// implementer count.
					others := implementerCountByIface[info.interfaceQN] - 1
					detail := fmt.Sprintf("implements %s (%d other implementer(s) - a plugin-style extensibility point)", info.interfaceName, others)
					points := math.Log1p(float64(others)) * 1.0
					if others <= 0 {
						detail = fmt.Sprintf("implements %s (sole implementer)", info.interfaceName)
						points = 0
					}
					signals = append(signals, Signal{
						Name:     "Interface implementation",
						Detail:   detail,
						Points:   points,
						Category: "architecture",
					})
				}
			}

			contribByQN[qn] = SymbolContribution{
				Method:            "text-references",
				Signals:           signals,
				BlastRadiusRaw:    sumSignalPoints(signals, blastRadiusCategories),
				DirectCount:       refs,
				ImpactedPackages:  usage.Directories,
				MethodBlastRadius: methodBlastRadius,
			}
		}
	}
	// Fill in ReviewPriorityRaw and the raw metric fields for every touched
	// symbol. Complexity/cognitive/loop/fan-out are already-fetched symbol
	// properties (no extra query); test coverage needs one more batched
	// query across every touched symbol.
	allQN := make([]string, 0, len(symbolByQN))
	for qn := range symbolByQN {
		allQN = append(allQN, qn)
	}
	testCounts := testCoverageCounts(ctx, c, allQN)
	similarByQN := similarSymbols(ctx, c, allQN)
	writesDataByQN := symbolsThatWriteData(ctx, c, allQN)
	for qn, contrib := range contribByQN {
		s := symbolByQN[qn]
		testCount := testCounts[qn]
		metricSignals := codeMetricSignals(s, testCount)
		contrib.Signals = append(contrib.Signals, metricSignals...)
		if dup := duplicationSignal(similarByQN[qn]); dup != nil {
			contrib.Signals = append(contrib.Signals, *dup)
		}
		if writesDataByQN[qn] {
			contrib.Signals = append(contrib.Signals, Signal{
				Name:     "Writes persistent data",
				Detail:   "mutates a database/persistent store directly",
				Points:   1.5,
				Category: "architecture",
			})
		} else {
			contrib.Signals = append(contrib.Signals, Signal{
				Name:     "Writes persistent data",
				Detail:   "does not mutate a database/persistent store directly",
				Points:   0,
				Category: "architecture",
			})
		}
		// Recompute from the now-complete Signals list rather than trusting
		// the value set when this contribution was first built, since
		// duplication/writes signals were appended after that point.
		contrib.BlastRadiusRaw = sumSignalPoints(contrib.Signals, blastRadiusCategories)
		contrib.ReviewPriorityRaw = sumSignalPoints(contrib.Signals, reviewPriorityCategories)
		contrib.IsEntryPoint = s.IsEntryPoint || s.RouteMethod != ""
		contrib.Complexity = s.Complexity
		contrib.Cognitive = s.Cognitive
		contrib.LoopDepth = s.LoopDepth
		contrib.OutDegree = s.OutDegree
		contrib.TestCount = testCount
		contribByQN[qn] = contrib
	}

	// One more batched query: file-level co-change coupling, applied per
	// hunk based on its own file (not per symbol - this is file-scoped
	// history, not code-structure data). Weighted at 0.3 - supplementary to
	// the symbol-level signal, not a replacement.
	const fileCouplingWeight = 0.3
	couplingByFile := fileCouplingBonus(ctx, c, fileOrder)

	// Build HunkReports and track the maximum raw scores (per dimension) for
	// normalization.
	hunkReportsByFile := make(map[string][]HunkReport)
	packageHunkCount := make(map[string]int)
	packageMaxBlastRadius := make(map[string]float64)
	maxBlastRadius, maxReviewPriority := 0.0, 0.0
	var maxBlastRadiusFile, maxBlastRadiusHeader string
	var maxReviewPriorityFile, maxReviewPriorityHeader string
	for _, p := range pending {
		hr := HunkReport{
			FilePath: p.hunk.FilePath,
			Header:   p.hunk.Header,
			NewStart: p.hunk.NewStart,
			NewLines: p.hunk.NewLines,
			Content:  p.hunk.Content,
		}
		var hunkPackages []string
		for _, s := range p.touched {
			contrib := contribByQN[s.QualifiedName]
			contrib.QualifiedName = s.QualifiedName
			contrib.Name = s.Name
			contrib.Label = s.Label
			hr.Symbols = append(hr.Symbols, contrib)
			hr.BlastRadiusRaw += contrib.BlastRadiusRaw
			hr.ReviewPriorityRaw += contrib.ReviewPriorityRaw
			hunkPackages = append(hunkPackages, contrib.ImpactedPackages...)
		}
		hr.FileCouplingBonus = fileCouplingWeight * couplingByFile[p.hunk.FilePath]
		if hr.FileCouplingBonus > 0 {
			hr.Signals = append(hr.Signals, Signal{
				Name:     "File co-change coupling",
				Detail:   "this file has historically changed alongside other files not otherwise touched here",
				Points:   hr.FileCouplingBonus,
				Category: "graph",
			})
		}
		hr.BlastRadiusRaw += hr.FileCouplingBonus
		for _, sig := range archRoleSignals(p.hunk.FilePath) {
			hr.Signals = append(hr.Signals, sig)
			hr.BlastRadiusRaw += sig.Points
		}
		if multiplier, sig := classifyHunkHygiene(p.hunk, p.touched); sig != nil {
			hr.HygieneMultiplier = multiplier
			hr.Signals = append(hr.Signals, *sig)
		} else {
			hr.HygieneMultiplier = 1.0
		}
		hr.ImpactedPackages = sortedUnique(hunkPackages)
		for _, pkg := range hr.ImpactedPackages {
			packageHunkCount[pkg]++
			if hr.BlastRadiusRaw > packageMaxBlastRadius[pkg] {
				packageMaxBlastRadius[pkg] = hr.BlastRadiusRaw
			}
		}
		if hr.BlastRadiusRaw > maxBlastRadius {
			maxBlastRadiusFile, maxBlastRadiusHeader = hr.FilePath, hr.Header
		}
		maxBlastRadius = math.Max(maxBlastRadius, hr.BlastRadiusRaw)
		if hr.ReviewPriorityRaw > maxReviewPriority {
			maxReviewPriorityFile, maxReviewPriorityHeader = hr.FilePath, hr.Header
		}
		maxReviewPriority = math.Max(maxReviewPriority, hr.ReviewPriorityRaw)
		hunkReportsByFile[p.hunk.FilePath] = append(hunkReportsByFile[p.hunk.FilePath], hr)
	}

	for _, path := range fileOrder {
		hrs := hunkReportsByFile[path]
		for i := range hrs {
			hrs[i].MaxBlastRadiusRaw = maxBlastRadius
			hrs[i].MaxBlastRadiusHunkFile = maxBlastRadiusFile
			hrs[i].MaxBlastRadiusHunkHeader = maxBlastRadiusHeader
			hrs[i].MaxReviewPriorityRaw = maxReviewPriority
			hrs[i].MaxReviewPriorityHunkFile = maxReviewPriorityFile
			hrs[i].MaxReviewPriorityHunkHeader = maxReviewPriorityHeader
			hrs[i].Weights = weights
			if maxBlastRadius > 0 {
				hrs[i].BlastRadiusNorm = hrs[i].BlastRadiusRaw / maxBlastRadius * 100
			}
			if maxReviewPriority > 0 {
				hrs[i].ReviewPriorityNorm = hrs[i].ReviewPriorityRaw / maxReviewPriority * 100
			}
			hrs[i].Combined = (weights.BlastRadius*hrs[i].BlastRadiusNorm + weights.ReviewPriority*hrs[i].ReviewPriorityNorm) * hrs[i].HygieneMultiplier
		}
		report.Files = append(report.Files, FileReport{Path: path, Hunks: hrs})
	}

	for pkg, count := range packageHunkCount {
		report.ImpactedPackages = append(report.ImpactedPackages, PackageImpact{
			Package:           pkg,
			HunkCount:         count,
			MaxBlastRadiusRaw: packageMaxBlastRadius[pkg],
		})
	}
	sort.Slice(report.ImpactedPackages, func(i, j int) bool {
		if report.ImpactedPackages[i].HunkCount != report.ImpactedPackages[j].HunkCount {
			return report.ImpactedPackages[i].HunkCount > report.ImpactedPackages[j].HunkCount
		}
		return report.ImpactedPackages[i].MaxBlastRadiusRaw > report.ImpactedPackages[j].MaxBlastRadiusRaw
	})

	return report, nil
}
