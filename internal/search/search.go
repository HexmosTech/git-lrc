package search

import (
	"sort"
	"strings"
	"time"

	"github.com/sahilm/fuzzy"
	"github.com/shrsv/dbctx/internal/db"
)

type MatchResult struct {
	TableName string  `json:"table_name"`
	Score     float64 `json:"score"`
	Source    string  `json:"source"`
}

type TableContext struct {
	TableName   string       `json:"table_name"`
	Schema      string       `json:"schema"`
	Columns     []ColumnInfo `json:"columns"`
	PrimaryKey  []string     `json:"primary_key"`
	ForeignKeys []FKInfo     `json:"foreign_keys"`
	IsMatch     bool         `json:"is_match"`
	MatchScore  float64      `json:"match_score"`
	// Score documents exactly how MatchScore was computed, signal by
	// signal. Nil for tables that were only pulled in via foreign-key
	// expansion (MatchScore == 0) — nothing was scored for those, so
	// there's nothing to explain.
	Score *ScoreBreakdown `json:"score,omitempty"`
}

// ScoreBreakdown is the "show your work" behind TableContext.MatchScore:
// every lexical signal's raw score, its fixed weight, and the resulting
// weighted contribution, plus the semantic signal's contribution if one
// ran — so a result's ranking is inspectable rather than a single opaque
// number. Callers (the web UI, in particular) can render this as a
// step-by-step computation instead of asking users to trust a score.
type ScoreBreakdown struct {
	FTS          SignalContribution `json:"fts"`
	Fuzzy        SignalContribution `json:"fuzzy"`
	Value        SignalContribution `json:"value"`
	Terminology  SignalContribution `json:"terminology"`
	LexicalTotal float64            `json:"lexical_total"`

	// Semantic is nil if semantic search was disabled/unavailable for
	// this query, or ran but this table wasn't among its scored
	// candidates at all.
	Semantic *SemanticContribution `json:"semantic,omitempty"`

	FinalScore float64 `json:"final_score"`
}

// SignalContribution is one lexical signal's raw score, its fixed weight
// (see FTSWeight etc.), and the resulting weighted contribution
// (Raw * Weight) to ScoreBreakdown.LexicalTotal.
type SignalContribution struct {
	Raw          float64 `json:"raw"`
	Weight       float64 `json:"weight"`
	Contribution float64 `json:"contribution"`
}

// SemanticContribution documents how the semantic signal contributed to a
// table's final score — the exact inputs to FuseScores's formula:
//
//	contribution = Weight * Normalized * Scale
//
// Cosine is the best-matching embedded object's raw similarity to the
// query (see SemanticHit); Normalized is that same table's score after
// Scorer.Score's query-relative min-max normalization; Scale is the
// strongest lexical score found anywhere in this query (or 1.0 if lexical
// found nothing at all — see FuseScores).
type SemanticContribution struct {
	Cosine       float64 `json:"cosine"`
	Normalized   float64 `json:"normalized"`
	Weight       float64 `json:"weight"`
	Scale        float64 `json:"scale"`
	Contribution float64 `json:"contribution"`
	EvidenceKind string  `json:"evidence_kind"`
	EvidenceText string  `json:"evidence_text"`
}

type ColumnInfo struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Nullable    bool            `json:"nullable"`
	IsPK        bool            `json:"is_pk"`
	FKTarget    string          `json:"fk_target,omitempty"`
	IsState     bool            `json:"is_state"`
	IsCategoric bool            `json:"is_categoric"`
	Values      []ValueInfo     `json:"values,omitempty"`
	JSONBPaths  []JSONBPathInfo `json:"jsonb_paths,omitempty"`
}

type ValueInfo struct {
	Value     string `json:"value"`
	Frequency int    `json:"frequency"`
}

type JSONBPathInfo struct {
	Path         string `json:"path"`
	InferredType string `json:"inferred_type"`
	SampleValues string `json:"sample_values,omitempty"`
}

type FKInfo struct {
	SrcColumns string `json:"src_columns"`
	RefTable   string `json:"ref_table"`
	DstColumns string `json:"dst_columns"`
}

type SearchResult struct {
	Tables       []TableContext `json:"tables"`
	Query        string         `json:"query"`
	SemanticHits []SemanticHit  `json:"semantic_hits,omitempty"`
	// Timing breaks down how long each phase of the query took, so query
	// latency is inspectable the same way scoring is — "how long did this
	// take, and where did the time go" rather than a single opaque number.
	Timing QueryTiming `json:"timing"`
}

// QueryTiming records how long each phase of QueryHybrid took, in
// milliseconds. SemanticMs/SemanticRan are only meaningful when a
// SemanticScorer was actually passed to QueryHybrid — SemanticRan
// distinguishes "semantic search took 0ms" (impossible in practice, but
// meaningful as a concept) from "semantic search did not run at all".
type QueryTiming struct {
	LexicalMs   float64 `json:"lexical_ms"`
	SemanticMs  float64 `json:"semantic_ms"`
	SemanticRan bool    `json:"semantic_ran"`
	ExpandMs    float64 `json:"expand_ms"`
	TotalMs     float64 `json:"total_ms"`
}

// millisSince rounds an elapsed duration to microsecond precision (three
// decimal places of a millisecond) — enough resolution to be useful for a
// sub-millisecond phase without printing a dozen meaningless digits.
func millisSince(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000.0
}

// SemanticHit is one inspectable piece of evidence a [SemanticScorer]
// contributed to a query: the best-matching embedded object for a table,
// and its similarity to the query. Surfacing these (rather than just a
// fused number) is what makes a semantic-influenced result explainable —
// callers can show "why" a table not matched lexically still appeared.
type SemanticHit struct {
	TableName string  `json:"table_name"`
	Kind      string  `json:"kind"` // "table" | "column" | "jsonb_path"
	Text      string  `json:"text"`
	Score     float64 `json:"score"`
}

// SemanticScorer is the interface an optional embedding-based retrieval
// signal implements to participate in [QueryHybrid]. It is defined here
// (rather than in whatever package implements it) so that this package
// never has to import an embedding backend — lexical search stays usable,
// dependency-light, and CGO-free even when semantic search is compiled in
// elsewhere in the program.
//
// Score returns a per-table relevance score for query (implementations
// should normalize into a comparable 0..1 range — see internal/semantic)
// plus the evidence used to compute it. Returning an error is treated as
// "semantic signal unavailable for this query" by [QueryHybrid], which
// falls back to lexical-only scoring rather than failing the query.
type SemanticScorer interface {
	Score(query string) (scores map[string]float64, hits []SemanticHit, err error)
}

func PopulateFTS(store *db.Store) error {
	if _, err := store.DB().Exec("DELETE FROM search_index"); err != nil {
		return err
	}

	rows, err := store.DB().Query(`
		SELECT t.id, t.name,
		       GROUP_CONCAT(c.name, ' ') as col_names
		FROM tables t
		JOIN columns c ON c.table_id = t.id
		GROUP BY t.id, t.name
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var tableID int
		var tableName, colNames string
		if err := rows.Scan(&tableID, &tableName, &colNames); err != nil {
			continue
		}

		var valueTokens []string
		valRows, _ := store.DB().Query(`
			SELECT DISTINCT value FROM field_values
			WHERE column_id IN (SELECT id FROM columns WHERE table_id = ?)
			LIMIT 100
		`, tableID)
		if valRows != nil {
			for valRows.Next() {
				var v string
				if valRows.Scan(&v) == nil {
					valueTokens = append(valueTokens, v)
				}
			}
			valRows.Close()
		}

		valStr := strings.Join(valueTokens, " ")
		store.DB().Exec(
			"INSERT INTO search_index (table_name, column_names, value_tokens) VALUES (?, ?, ?)",
			tableName, colNames, valStr,
		)
	}
	return rows.Err()
}

// Query searches the index using lexical signals only (FTS, fuzzy table
// name matching, and value matching), expands via foreign keys, and
// returns ranked table contexts. This is the original, unchanged retrieval
// path — it never consults a semantic index even if one is present on the
// store, and has no dependency on an embedding backend.
//
// Use [QueryHybrid] to additionally fold in a [SemanticScorer].
func Query(store *db.Store, query string) (*SearchResult, error) {
	return QueryHybrid(store, query, nil)
}

// QueryHybrid searches the index using lexical signals plus, if sem is
// non-nil, an additional semantic retrieval signal fused into the same
// per-table score used for ranking and foreign-key expansion. Passing a
// nil sem is equivalent to [Query].
//
// If sem.Score returns an error (e.g. the embedding backend is
// unavailable), the semantic signal is silently dropped and the query
// proceeds lexically — a missing/broken semantic index must never fail an
// otherwise-valid query.
func QueryHybrid(store *db.Store, query string, sem SemanticScorer) (*SearchResult, error) {
	queryStart := time.Now()
	result := &SearchResult{Query: query}

	lexStart := time.Now()
	lexBreakdown := computeLexicalBreakdown(store, query)
	lexical := make(map[string]float64, len(lexBreakdown))
	for t, b := range lexBreakdown {
		lexical[t] = b.weighted()
	}
	merged := lexical
	result.Timing.LexicalMs = millisSince(lexStart)

	var semScores map[string]float64
	hitByTable := make(map[string]SemanticHit)
	if sem != nil {
		semStart := time.Now()
		s, hits, err := sem.Score(query)
		result.Timing.SemanticMs = millisSince(semStart)
		result.Timing.SemanticRan = true
		if err == nil && len(s) > 0 {
			semScores = s
			merged = FuseScores(lexical, semScores)
			result.SemanticHits = hits
			for _, h := range hits {
				hitByTable[h.TableName] = h
			}
		}
	}
	scale := strongestLexicalScale(lexical)

	expandStart := time.Now()
	tables, err := expandAndBuild(store, merged, lexBreakdown, semScores, hitByTable, scale)
	if err != nil {
		return nil, err
	}
	result.Timing.ExpandMs = millisSince(expandStart)
	result.Tables = tables
	result.Timing.TotalMs = millisSince(queryStart)
	return result, nil
}

// ComputeLexicalScores runs the deterministic retrieval signals — FTS5
// full-text search, fuzzy table-name matching, value matching, and
// terminology — and combines them into a single per-table score map. This
// is the "table / field / value matching" step of dbctx's retrieval
// pipeline, kept separate from FK expansion and context building so a
// semantic score can be fused in before expansion happens.
//
// This collapses each table's per-signal breakdown (see
// computeLexicalBreakdown) down to one number; use QueryHybrid if you need
// the breakdown itself (e.g. to explain a result), not just the merged
// score.
func ComputeLexicalScores(store *db.Store, query string) map[string]float64 {
	breakdown := computeLexicalBreakdown(store, query)
	scores := make(map[string]float64, len(breakdown))
	for t, b := range breakdown {
		scores[t] = b.weighted()
	}
	return scores
}

// Signal weights used to combine dbctx's lexical retrieval signals into a
// single per-table score. Named constants (rather than inline literals)
// so ScoreBreakdown can report back the exact numbers actually used —
// there is one source of truth between the computation and anything that
// explains it, including the web UI's score breakdown panel.
const (
	FTSWeight         = 1.0
	FuzzyWeight       = 0.8
	ValueWeight       = 1.2
	TerminologyWeight = 1.5
)

// lexicalBreakdown holds one table's raw (unweighted) score from each
// lexical signal, before they're combined into a single number.
type lexicalBreakdown struct {
	FTS, Fuzzy, Value, Terminology float64
}

func (b lexicalBreakdown) weighted() float64 {
	return b.FTS*FTSWeight + b.Fuzzy*FuzzyWeight + b.Value*ValueWeight + b.Terminology*TerminologyWeight
}

// computeLexicalBreakdown runs every lexical signal and returns each
// table's raw per-signal scores, unweighted and unmerged. ComputeLexicalScores
// and QueryHybrid's score explanations both build on this rather than
// duplicating the underlying signal queries.
func computeLexicalBreakdown(store *db.Store, query string) map[string]lexicalBreakdown {
	tokens := strings.Fields(strings.ToLower(query))

	// 1. FTS5 search
	ftsScores := ftsSearch(store, tokens)

	// 2. Fuzzy match on table names
	fuzzyScores := fuzzyMatchTables(store, tokens)

	// 3. Value match
	valueScores := valueMatch(store, tokens)

	// 4. Terminology match (optional user-supplied alias -> object mappings;
	// see internal/terminology). Empty/absent terminology has no effect.
	termScores := terminologyMatch(store, query)

	out := make(map[string]lexicalBreakdown)
	for t, s := range ftsScores {
		b := out[t]
		b.FTS = s
		out[t] = b
	}
	for t, s := range fuzzyScores {
		b := out[t]
		b.Fuzzy = s
		out[t] = b
	}
	for t, s := range valueScores {
		b := out[t]
		b.Value = s
		out[t] = b
	}
	for t, s := range termScores {
		b := out[t]
		b.Terminology = s
		out[t] = b
	}
	return out
}

// minTerminologyAbbrevLen bounds the "alias contains query" direction of
// terminologyMatch (see below) — without a floor, a very short query
// fragment (e.g. "a", "or") would substring-match inside almost any
// alias and contribute pure noise.
const minTerminologyAbbrevLen = 3

// terminologyMatch checks query against every user-imported terminology
// alias in both directions, since neither direction alone is sufficient:
//
//   - query contains alias: the user typed a full phrase that mentions a
//     known alias, e.g. query "how many lines of code do we have" contains
//     the alias "lines of code".
//   - alias contains query: the user typed a short form that is itself a
//     prefix/substring of a longer known alias, e.g. query "org" is a
//     substring of the alias "organization". Without this direction,
//     importing "organization"/"organization account" as aliases for
//     "org" would only ever match the fully-spelled-out forms, silently
//     failing on the abbreviation a user is more likely to actually type.
//
// The second direction is weighted lower (weaker evidence, higher
// false-positive risk for short fragments) and floored at
// minTerminologyAbbrevLen characters to avoid matching on noise.
//
// Aliases are compared against the whole query string, not per-token —
// they're often multi-word phrases like "line of code" that tokenizing
// would break apart. A store that never had terminology imported simply
// has no `terminology` table yet; that's treated as "no matches" rather
// than an error, the same way the other signals silently no-op when their
// prerequisites are missing.
func terminologyMatch(store *db.Store, query string) map[string]float64 {
	scores := make(map[string]float64)
	rows, err := store.DB().Query("SELECT alias, target_table FROM terminology")
	if err != nil {
		return scores // no terminology table (never imported) — not an error
	}
	defer rows.Close()

	lowerQuery := strings.ToLower(strings.TrimSpace(query))
	for rows.Next() {
		var alias, table string
		if rows.Scan(&alias, &table) != nil {
			continue
		}
		if alias == "" {
			continue
		}
		lowerAlias := strings.ToLower(alias)
		switch {
		case strings.Contains(lowerQuery, lowerAlias):
			scores[table] += 1.0
		case len(lowerQuery) >= minTerminologyAbbrevLen && strings.Contains(lowerAlias, lowerQuery):
			scores[table] += 0.6
		}
	}
	return scores
}

// semanticFusionWeight controls how strongly the semantic signal can move
// a table's ranking relative to the strongest lexical match in the same
// query. It is deliberately well under 1.0: exact/near-exact lexical
// identifier matches (an FTS hit on the literal table name, a 100% fuzzy
// match) should stay dominant, and semantic similarity exists to recover
// recall — surface plausible tables lexical search missed entirely — not
// to outrank a direct name match.
const semanticFusionWeight = 0.6

// FuseScores combines lexical and semantic per-table scores using weighted
// normalized fusion: semantic contributes semanticFusionWeight * score,
// scaled by the strongest lexical score in this result set (or 1.0 if
// lexical found nothing at all, so a pure paraphrase query like "buyers"
// can still surface "customers" with a modest, present score rather than
// zero).
//
// This mirrors the additive, per-source-weighted style already used by
// ComputeLexicalScores (fts*1.0 + fuzzy*0.8 + value*1.2) rather than
// introducing a different fusion paradigm like reciprocal rank fusion —
// it's the natural extension of the scoring model dbctx already has.
// Semantic scores are expected to already be normalized into a 0..1
// relative-relevance range by the caller's SemanticScorer (see
// internal/semantic), since raw embedding similarity is not calibrated
// against the lexical score scale.
func FuseScores(lexical, semantic map[string]float64) map[string]float64 {
	merged := make(map[string]float64, len(lexical)+len(semantic))
	for t, s := range lexical {
		merged[t] = s
	}
	scale := strongestLexicalScale(lexical)
	for t, s := range semantic {
		merged[t] += semanticFusionWeight * s * scale
	}
	return merged
}

// strongestLexicalScale is the "scale" factor in FuseScores's formula: the
// strongest lexical score found anywhere in this query, or 1.0 if lexical
// matched nothing at all (so a pure paraphrase/terminology-only match
// still gets a modest, present contribution rather than being multiplied
// by zero). Shared between FuseScores and the score-breakdown explanation
// in expandAndBuild so both report the same number.
func strongestLexicalScale(lexical map[string]float64) float64 {
	var max float64
	for _, s := range lexical {
		if s > max {
			max = s
		}
	}
	if max <= 0 {
		return 1.0
	}
	return max
}

// expandAndBuild expands a scored table set via foreign keys and builds
// full [TableContext] results, sorted by score with FK-expanded (score 0)
// tables trailing. This is the "candidate tables -> foreign-key expansion
// -> relevant fields" portion of dbctx's retrieval pipeline.
//
// lexBreakdown, semScores, hitByTable, and semScale carry the intermediate
// scoring detail needed to attach a [ScoreBreakdown] to every actually-
// scored table (see buildScoreBreakdown) — purely-FK-expanded tables get
// no breakdown, since nothing was scored for them.
func expandAndBuild(
	store *db.Store,
	merged map[string]float64,
	lexBreakdown map[string]lexicalBreakdown,
	semScores map[string]float64,
	hitByTable map[string]SemanticHit,
	semScale float64,
) ([]TableContext, error) {
	type scored struct {
		name  string
		score float64
	}
	var sorted []scored
	for name, score := range merged {
		sorted = append(sorted, scored{name, score})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].score > sorted[j].score })

	// Expand via FKs
	expanded := make(map[string]bool)
	for _, s := range sorted {
		expanded[s.name] = true
		fkExpand(store, s.name, expanded, 2)
	}

	// Build table contexts
	var tables []TableContext
	seen := make(map[string]bool)
	for _, s := range sorted {
		if !expanded[s.name] {
			continue
		}
		if seen[s.name] {
			continue
		}
		seen[s.name] = true
		tc := buildTableContext(store, s.name, s.score)
		tc.Score = buildScoreBreakdown(s.name, s.score, lexBreakdown, semScores, hitByTable, semScale)
		tables = append(tables, tc)
	}

	// Add FK-expanded tables that weren't in the original match
	for tbl := range expanded {
		if seen[tbl] {
			continue
		}
		seen[tbl] = true
		tables = append(tables, buildTableContext(store, tbl, 0))
	}

	return tables, nil
}

// buildScoreBreakdown assembles the "show your work" explanation for one
// table's final score, using exactly the same weight constants and scale
// factor that produced it (see computeLexicalBreakdown, FuseScores).
func buildScoreBreakdown(
	table string,
	final float64,
	lexBreakdown map[string]lexicalBreakdown,
	semScores map[string]float64,
	hitByTable map[string]SemanticHit,
	semScale float64,
) *ScoreBreakdown {
	// Zero value if table only matched via semantic search and never had
	// any lexical signal at all.
	lb := lexBreakdown[table]

	bd := &ScoreBreakdown{
		FTS:         SignalContribution{Raw: lb.FTS, Weight: FTSWeight, Contribution: lb.FTS * FTSWeight},
		Fuzzy:       SignalContribution{Raw: lb.Fuzzy, Weight: FuzzyWeight, Contribution: lb.Fuzzy * FuzzyWeight},
		Value:       SignalContribution{Raw: lb.Value, Weight: ValueWeight, Contribution: lb.Value * ValueWeight},
		Terminology: SignalContribution{Raw: lb.Terminology, Weight: TerminologyWeight, Contribution: lb.Terminology * TerminologyWeight},
		FinalScore:  final,
	}
	bd.LexicalTotal = bd.FTS.Contribution + bd.Fuzzy.Contribution + bd.Value.Contribution + bd.Terminology.Contribution

	if normalized, ok := semScores[table]; ok {
		hit := hitByTable[table]
		bd.Semantic = &SemanticContribution{
			Cosine:       hit.Score,
			Normalized:   normalized,
			Weight:       semanticFusionWeight,
			Scale:        semScale,
			Contribution: semanticFusionWeight * normalized * semScale,
			EvidenceKind: hit.Kind,
			EvidenceText: hit.Text,
		}
	}

	return bd
}

func ftsSearch(store *db.Store, tokens []string) map[string]float64 {
	scores := make(map[string]float64)
	query := strings.Join(tokens, " OR ")
	if query == "" {
		return scores
	}
	rows, err := store.DB().Query(`
		SELECT table_name, rank FROM search_index
		WHERE search_index MATCH ?
		ORDER BY rank LIMIT 20
	`, query)
	if err != nil {
		return scores
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var rank float64
		if rows.Scan(&name, &rank) == nil {
			scores[name] = 1.0 / (1.0 + (-rank))
		}
	}
	return scores
}

func fuzzyMatchTables(store *db.Store, tokens []string) map[string]float64 {
	scores := make(map[string]float64)
	rows, err := store.DB().Query("SELECT name FROM tables")
	if err != nil {
		return scores
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			names = append(names, n)
		}
	}
	for _, token := range tokens {
		matches := fuzzy.Find(token, names)
		for _, m := range matches {
			scores[m.Str] += float64(m.Score) / 100.0
		}
	}
	return scores
}

func valueMatch(store *db.Store, tokens []string) map[string]float64 {
	scores := make(map[string]float64)
	for _, token := range tokens {
		rows, err := store.DB().Query(`
			SELECT DISTINCT t.name
			FROM field_values fv
			JOIN columns c ON c.id = fv.column_id
			JOIN tables t ON t.id = c.table_id
			WHERE LOWER(fv.value) LIKE '%' || LOWER(?) || '%'
			LIMIT 20
		`, token)
		if err != nil {
			continue
		}
		for rows.Next() {
			var name string
			if rows.Scan(&name) == nil {
				scores[name] += 0.5
			}
		}
		rows.Close()
	}
	return scores
}

func fkExpand(store *db.Store, tableName string, expanded map[string]bool, depth int) {
	if depth <= 0 {
		return
	}
	rows, err := store.DB().Query(`
		SELECT rt.name FROM foreign_keys fk
		JOIN tables st ON st.id = fk.table_id
		JOIN tables rt ON rt.id = fk.ref_table_id
		WHERE st.name = ?
		UNION
		SELECT st.name FROM foreign_keys fk
		JOIN tables st ON st.id = fk.table_id
		JOIN tables rt ON rt.id = fk.ref_table_id
		WHERE rt.name = ?
	`, tableName, tableName)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var related string
		if rows.Scan(&related) == nil && !expanded[related] {
			expanded[related] = true
			fkExpand(store, related, expanded, depth-1)
		}
	}
}

func buildTableContext(store *db.Store, tableName string, score float64) TableContext {
	tc := TableContext{TableName: tableName, MatchScore: score, IsMatch: score > 0}

	// Get schema
	store.DB().QueryRow("SELECT COALESCE(schema, 'public') FROM tables WHERE name = ?", tableName).Scan(&tc.Schema)

	// Get PK columns
	pkRows, _ := store.DB().Query(`
		SELECT column_name FROM primary_keys
		WHERE table_id = (SELECT id FROM tables WHERE name = ?)
	`, tableName)
	if pkRows != nil {
		for pkRows.Next() {
			var col string
			if pkRows.Scan(&col) == nil {
				tc.PrimaryKey = append(tc.PrimaryKey, col)
			}
		}
		pkRows.Close()
	}

	// Get FK info
	fkRows, _ := store.DB().Query(`
		SELECT fk.src_columns, rt.name, fk.dst_columns
		FROM foreign_keys fk
		JOIN tables st ON st.id = fk.table_id
		JOIN tables rt ON rt.id = fk.ref_table_id
		WHERE st.name = ?
	`, tableName)
	if fkRows != nil {
		for fkRows.Next() {
			var fk FKInfo
			if fkRows.Scan(&fk.SrcColumns, &fk.RefTable, &fk.DstColumns) == nil {
				tc.ForeignKeys = append(tc.ForeignKeys, fk)
			}
		}
		fkRows.Close()
	}

	// Get columns with metadata
	colRows, _ := store.DB().Query(`
		SELECT c.name, c.type, c.nullable,
		       CASE WHEN pk.column_name IS NOT NULL THEN 1 ELSE 0 END as is_pk,
		       fs.is_state_like, fs.is_categorical
		FROM columns c
		LEFT JOIN primary_keys pk ON pk.table_id = c.table_id AND pk.column_name = c.name
		LEFT JOIN field_stats fs ON fs.column_id = c.id
		WHERE c.table_id = (SELECT id FROM tables WHERE name = ?)
		ORDER BY c.position
	`, tableName)
	if colRows != nil {
		for colRows.Next() {
			var ci ColumnInfo
			var isPK int
			var isState, isCategoric *bool
			if colRows.Scan(&ci.Name, &ci.Type, &ci.Nullable, &isPK, &isState, &isCategoric) == nil {
				ci.IsPK = isPK == 1
				if isState != nil {
					ci.IsState = *isState
				}
				if isCategoric != nil {
					ci.IsCategoric = *isCategoric
				}
				// Find FK target for this column
				for _, fk := range tc.ForeignKeys {
					if strings.Contains(fk.SrcColumns, ci.Name) {
						dstSuffix := ""
						if fk.DstColumns != "id" {
							dstSuffix = "." + fk.DstColumns
						}
						ci.FKTarget = fk.RefTable + dstSuffix
						break
					}
				}
				tc.Columns = append(tc.Columns, ci)
			}
		}
		colRows.Close()
	}

	// Get values for state/categorical columns
	for i := range tc.Columns {
		if tc.Columns[i].IsState || tc.Columns[i].IsCategoric {
			valRows, _ := store.DB().Query(`
				SELECT fv.value, fv.frequency FROM field_values fv
				JOIN columns c ON c.id = fv.column_id
				JOIN tables t ON t.id = c.table_id
				WHERE t.name = ? AND c.name = ?
				ORDER BY fv.frequency DESC
			`, tableName, tc.Columns[i].Name)
			if valRows != nil {
				for valRows.Next() {
					var v ValueInfo
					if valRows.Scan(&v.Value, &v.Frequency) == nil {
						tc.Columns[i].Values = append(tc.Columns[i].Values, v)
					}
				}
				valRows.Close()
			}
		}
	}

	// Get JSONB paths
	for i := range tc.Columns {
		if tc.Columns[i].Type == "jsonb" || tc.Columns[i].Type == "json" {
			jRows, _ := store.DB().Query(`
				SELECT jp.path, jp.inferred_type, jp.sample_values
				FROM jsonb_paths jp
				JOIN columns c ON c.id = jp.column_id
				JOIN tables t ON t.id = c.table_id
				WHERE t.name = ? AND c.name = ?
				ORDER BY jp.path
			`, tableName, tc.Columns[i].Name)
			if jRows != nil {
				for jRows.Next() {
					var jp JSONBPathInfo
					if jRows.Scan(&jp.Path, &jp.InferredType, &jp.SampleValues) == nil {
						tc.Columns[i].JSONBPaths = append(tc.Columns[i].JSONBPaths, jp)
					}
				}
				jRows.Close()
			}
		}
	}

	return tc
}

// ShortenType converts a PostgreSQL type name returned by format_type()
// into a compact, human-friendly abbreviation. Types not in the map are
// returned unchanged.
func ShortenType(t string) string {
	if short, ok := typeAbbreviations[t]; ok {
		return short
	}
	return t
}

// typeAbbreviations maps PostgreSQL's verbose format_type() output to
// compact forms. Not all follow PostgreSQL's own abbreviations — some
// are shortened further (e.g. tstz instead of timestamptz) for token
// efficiency. LLMs understand them either way.
var typeAbbreviations = map[string]string{
	"character varying":              "varchar",
	"character":                      "char",
	"timestamp with time zone":       "tstz",
	"timestamp without time zone":    "ts",
	"time with time zone":            "ttz",
	"time without time zone":         "tt",
	"double precision":               "float8",
	"real":                           "float4",
	"boolean":                        "bool",
	"integer":                        "int",
	"bigint":                         "int8",
	"smallint":                       "int2",
	"numeric":                        "num",
	"decimal":                        "num",
	"text":                           "text",
	"jsonb":                          "jsonb",
	"json":                           "json",
	"uuid":                           "uuid",
	"bytea":                          "bytea",
	"name":                           "name",
	"oid":                            "oid",
	"xid":                            "xid",
	"cid":                            "cid",
	"pg_node_tree":                   "pg_node_tree",
	"pg_ndistinct":                   "pg_ndistinct",
	"pg_dependencies":                "pg_dependencies",
	"anyelement":                     "anyelement",
	"anyarray":                       "anyarray",
	"anyenum":                        "anyenum",
	"anyrange":                       "anyrange",
	"anynonarray":                    "anynonarray",
	"cstring":                        "cstring",
	"event_trigger":                  "event_trigger",
	"fdw_handler":                    "fdw_handler",
	"index_am_handler":               "index_am_handler",
	"internal":                       "internal",
	"language_handler":               "language_handler",
	"opaque":                         "opaque",
	"trigger":                        "trigger",
	"void":                           "void",
}

// ShortenTypeParam abbreviates a PostgreSQL type that may include a
// parameter suffix, e.g. "character varying(255)" → "varchar(255)".
// It splits on the first '(' and shortens just the base type name.
func ShortenTypeParam(t string) string {
	if idx := strings.Index(t, "("); idx > 0 {
		base := t[:idx]
		param := t[idx:]
		if short, ok := typeAbbreviations[base]; ok {
			return short + param
		}
		return t
	}
	return ShortenType(t)
}

// Legend returns the notation guide used in compact text output. It
// explains every symbol and annotation so that an LLM (or human reader)
// can interpret the schema without external documentation.
func Legend() string {
	return `--- notation ---
PK: primary key           col → table  foreign key
^  is primary key         ?  nullable   >target  FK target
[state] state-like categorical (< 100 distinct values)
[cat]   categorical field
{a, b, c}  representative values (from pg_stats)
$.path  type  {samples}  JSONB path with inferred type
(score: X.XX)  relevance score from query matching
`
}
