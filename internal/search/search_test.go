package search

import (
	"fmt"
	"testing"

	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/testutil"
)

func seedTestStore(t *testing.T) *db.Store {
	t.Helper()
	store := testutil.NewSeedStore(t)
	if err := store.InitFTS(); err != nil {
		t.Fatalf("init fts: %v", err)
	}
	if err := PopulateFTS(store); err != nil {
		t.Fatalf("populate fts: %v", err)
	}
	return store
}

func TestPopulateFTS(t *testing.T) {
	store := seedTestStore(t)

	// Verify FTS rows were created
	var count int
	err := store.DB().QueryRow("SELECT COUNT(*) FROM search_index").Scan(&count)
	if err != nil {
		t.Fatalf("count search_index: %v", err)
	}
	if count == 0 {
		t.Error("search_index has 0 rows after PopulateFTS")
	}
	// We have 4 tables in the fixture
	if count != 4 {
		t.Errorf("search_index has %d rows, want 4", count)
	}
}

func TestQuery_ExactTableName(t *testing.T) {
	store := seedTestStore(t)

	result, err := Query(store, "reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result.Tables) == 0 {
		t.Fatal("Query('reviews') returned 0 tables")
	}

	found := false
	for _, tbl := range result.Tables {
		if tbl.TableName == "reviews" && tbl.IsMatch {
			found = true
			break
		}
	}
	if !found {
		t.Error("reviews not found as a match in results")
	}
}

func TestQuery_FuzzyMatch(t *testing.T) {
	store := seedTestStore(t)

	// "revew" is a typo for "reviews"
	result, err := Query(store, "revew")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	found := false
	for _, tbl := range result.Tables {
		if tbl.TableName == "reviews" {
			found = true
			break
		}
	}
	if !found {
		t.Error("fuzzy match did not find 'reviews' for query 'revew'")
	}
}

func TestQuery_ValueMatch(t *testing.T) {
	store := seedTestStore(t)

	// "completed" is a field_value for reviews.status
	result, err := Query(store, "completed")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	found := false
	for _, tbl := range result.Tables {
		if tbl.TableName == "reviews" {
			found = true
			break
		}
	}
	if !found {
		t.Error("value match did not find 'reviews' for query 'completed'")
	}
}

func TestQuery_FKExpansion(t *testing.T) {
	store := seedTestStore(t)

	// Querying "orgs" should also surface reviews and pull_requests via FK
	result, err := Query(store, "orgs")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	tableNames := make(map[string]bool)
	for _, tbl := range result.Tables {
		tableNames[tbl.TableName] = true
	}

	if !tableNames["orgs"] {
		t.Error("orgs not in results")
	}
	// FK expansion should bring in tables that reference orgs
	if !tableNames["reviews"] {
		t.Error("reviews not in results (FK expansion from orgs)")
	}
	if !tableNames["pull_requests"] {
		t.Error("pull_requests not in results (FK expansion from orgs)")
	}
}

func TestQuery_NoMatch(t *testing.T) {
	store := seedTestStore(t)

	result, err := Query(store, "xyznonexistent")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	// Should have 0 or only FK-expanded tables
	for _, tbl := range result.Tables {
		if tbl.IsMatch {
			t.Errorf("unexpected match: %s (score=%.2f)", tbl.TableName, tbl.MatchScore)
		}
	}
}

func TestQuery_MultipleTerms(t *testing.T) {
	store := seedTestStore(t)

	result, err := Query(store, "failed reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result.Tables) == 0 {
		t.Fatal("Query('failed reviews') returned 0 tables")
	}

	// "reviews" should match on both "reviews" (name) and "failed" (value)
	found := false
	for _, tbl := range result.Tables {
		if tbl.TableName == "reviews" && tbl.IsMatch {
			found = true
			break
		}
	}
	if !found {
		t.Error("reviews not found as match for 'failed reviews'")
	}
}

func TestQuery_TableContextStructure(t *testing.T) {
	store := seedTestStore(t)

	result, err := Query(store, "reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	var reviews TableContext
	for _, tbl := range result.Tables {
		if tbl.TableName == "reviews" {
			reviews = tbl
			break
		}
	}

	if reviews.PrimaryKey == nil {
		t.Error("reviews.PrimaryKey is nil")
	} else if len(reviews.PrimaryKey) != 1 || reviews.PrimaryKey[0] != "id" {
		t.Errorf("reviews.PrimaryKey = %v, want [id]", reviews.PrimaryKey)
	}

	if len(reviews.ForeignKeys) == 0 {
		t.Error("reviews.ForeignKeys is empty")
	}

	// Check that state field has values
	for _, col := range reviews.Columns {
		if col.Name == "status" {
			if !col.IsState {
				t.Error("status column should be marked as state")
			}
			if len(col.Values) == 0 {
				t.Error("status column has no values")
			}
		}
		if col.Name == "metadata" {
			if len(col.JSONBPaths) == 0 {
				t.Error("metadata column has no JSONB paths")
			}
		}
		if col.Name == "body" {
			if !col.Nullable {
				t.Error("body column should be nullable")
			}
		}
	}
}

// fakeScorer is a test-only SemanticScorer with canned responses.
type fakeScorer struct {
	scores map[string]float64
	hits   []SemanticHit
	err    error
}

func (f *fakeScorer) Score(query string) (map[string]float64, []SemanticHit, error) {
	return f.scores, f.hits, f.err
}

func TestFuseScores_LexicalDominatesExactMatch(t *testing.T) {
	lexical := map[string]float64{"orders": 10.0}
	semantic := map[string]float64{"orders": 0.9, "purchases": 0.95}

	merged := FuseScores(lexical, semantic)

	if merged["orders"] <= merged["purchases"] {
		t.Errorf("exact lexical match 'orders' (%.2f) should stay above a purely-semantic hit 'purchases' (%.2f)",
			merged["orders"], merged["purchases"])
	}
	if merged["orders"] <= lexical["orders"] {
		t.Error("semantic agreement should still boost the lexical score, not just leave it unchanged")
	}
}

func TestFuseScores_PureSemanticSurfacesWithNoLexicalSignal(t *testing.T) {
	lexical := map[string]float64{} // "buyers" matched nothing lexically
	semantic := map[string]float64{"customers": 0.8}

	merged := FuseScores(lexical, semantic)

	if merged["customers"] <= 0 {
		t.Errorf("customers score = %.2f, want > 0 even with zero lexical signal", merged["customers"])
	}
}

func TestQueryHybrid_NilScorerMatchesQuery(t *testing.T) {
	store := seedTestStore(t)

	viaHybrid, err := QueryHybrid(store, "reviews", nil)
	if err != nil {
		t.Fatalf("QueryHybrid: %v", err)
	}
	viaQuery, err := Query(store, "reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(viaHybrid.Tables) != len(viaQuery.Tables) {
		t.Errorf("QueryHybrid(nil) returned %d tables, Query returned %d", len(viaHybrid.Tables), len(viaQuery.Tables))
	}
}

func TestQueryHybrid_SurfacesTableLexicalMissed(t *testing.T) {
	store := seedTestStore(t)

	sem := &fakeScorer{
		scores: map[string]float64{"orgs": 0.9},
		hits:   []SemanticHit{{TableName: "orgs", Kind: "table", Text: "orgs organizations", Score: 0.9}},
	}

	// "xyznonexistent" matches nothing lexically in the fixture.
	result, err := QueryHybrid(store, "xyznonexistent", sem)
	if err != nil {
		t.Fatalf("QueryHybrid: %v", err)
	}

	found := false
	for _, tbl := range result.Tables {
		if tbl.TableName == "orgs" && tbl.IsMatch {
			found = true
		}
	}
	if !found {
		t.Error("expected semantic-only signal to surface 'orgs' as a match")
	}
	if len(result.SemanticHits) != 1 || result.SemanticHits[0].TableName != "orgs" {
		t.Errorf("SemanticHits = %v, want one hit for orgs", result.SemanticHits)
	}
}

func almostEqual(a, b float64) bool {
	const eps = 1e-9
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < eps
}

func TestQueryHybrid_ScoreBreakdown_LexicalOnly(t *testing.T) {
	store := seedTestStore(t)

	result, err := QueryHybrid(store, "reviews", nil)
	if err != nil {
		t.Fatalf("QueryHybrid: %v", err)
	}

	var reviews *TableContext
	for i := range result.Tables {
		if result.Tables[i].TableName == "reviews" {
			reviews = &result.Tables[i]
		}
	}
	if reviews == nil {
		t.Fatal("reviews not found in results")
	}
	if reviews.Score == nil {
		t.Fatal("expected a ScoreBreakdown on a directly-matched table")
	}

	bd := reviews.Score
	if bd.Semantic != nil {
		t.Error("Semantic should be nil when no SemanticScorer was passed")
	}

	// Every weight must match the documented constants exactly — this is
	// what a UI renders as "the formula", so it must be the literal
	// number used in computation, not an approximation.
	if bd.FTS.Weight != FTSWeight || bd.Fuzzy.Weight != FuzzyWeight ||
		bd.Value.Weight != ValueWeight || bd.Terminology.Weight != TerminologyWeight {
		t.Errorf("breakdown weights don't match the exported constants: %+v", bd)
	}

	// contribution = raw * weight, for every signal.
	if !almostEqual(bd.FTS.Contribution, bd.FTS.Raw*FTSWeight) {
		t.Errorf("FTS.Contribution = %v, want raw*weight = %v", bd.FTS.Contribution, bd.FTS.Raw*FTSWeight)
	}
	if !almostEqual(bd.Fuzzy.Contribution, bd.Fuzzy.Raw*FuzzyWeight) {
		t.Errorf("Fuzzy.Contribution = %v, want raw*weight = %v", bd.Fuzzy.Contribution, bd.Fuzzy.Raw*FuzzyWeight)
	}
	if !almostEqual(bd.Value.Contribution, bd.Value.Raw*ValueWeight) {
		t.Errorf("Value.Contribution = %v, want raw*weight = %v", bd.Value.Contribution, bd.Value.Raw*ValueWeight)
	}
	if !almostEqual(bd.Terminology.Contribution, bd.Terminology.Raw*TerminologyWeight) {
		t.Errorf("Terminology.Contribution = %v, want raw*weight = %v", bd.Terminology.Contribution, bd.Terminology.Raw*TerminologyWeight)
	}

	// lexical_total = sum of contributions
	wantTotal := bd.FTS.Contribution + bd.Fuzzy.Contribution + bd.Value.Contribution + bd.Terminology.Contribution
	if !almostEqual(bd.LexicalTotal, wantTotal) {
		t.Errorf("LexicalTotal = %v, want %v", bd.LexicalTotal, wantTotal)
	}

	// With no semantic signal, final score == lexical total == the
	// table's MatchScore.
	if !almostEqual(bd.FinalScore, bd.LexicalTotal) {
		t.Errorf("FinalScore = %v, want == LexicalTotal (%v) with no semantic signal", bd.FinalScore, bd.LexicalTotal)
	}
	if !almostEqual(bd.FinalScore, reviews.MatchScore) {
		t.Errorf("FinalScore = %v, want == MatchScore (%v)", bd.FinalScore, reviews.MatchScore)
	}
}

func TestQueryHybrid_ScoreBreakdown_WithSemantic(t *testing.T) {
	store := seedTestStore(t)

	sem := &fakeScorer{
		scores: map[string]float64{"orgs": 0.75},
		hits:   []SemanticHit{{TableName: "orgs", Kind: "table", Text: "orgs organizations account", Score: 0.512}},
	}

	// "xyznonexistent" matches nothing lexically, so the fusion scale
	// falls back to 1.0 (see strongestLexicalScale) — makes the expected
	// numbers easy to hand-verify.
	result, err := QueryHybrid(store, "xyznonexistent", sem)
	if err != nil {
		t.Fatalf("QueryHybrid: %v", err)
	}

	var orgs *TableContext
	for i := range result.Tables {
		if result.Tables[i].TableName == "orgs" {
			orgs = &result.Tables[i]
		}
	}
	if orgs == nil || orgs.Score == nil {
		t.Fatal("expected orgs with a ScoreBreakdown")
	}

	sc := orgs.Score.Semantic
	if sc == nil {
		t.Fatal("expected a non-nil Semantic breakdown")
	}
	if sc.Cosine != 0.512 {
		t.Errorf("Cosine = %v, want the raw hit score 0.512", sc.Cosine)
	}
	if sc.Normalized != 0.75 {
		t.Errorf("Normalized = %v, want the scorer's normalized score 0.75", sc.Normalized)
	}
	if sc.Weight != semanticFusionWeight {
		t.Errorf("Weight = %v, want semanticFusionWeight (%v)", sc.Weight, semanticFusionWeight)
	}
	if sc.Scale != 1.0 {
		t.Errorf("Scale = %v, want 1.0 (no lexical signal at all)", sc.Scale)
	}
	wantContribution := semanticFusionWeight * 0.75 * 1.0
	if !almostEqual(sc.Contribution, wantContribution) {
		t.Errorf("Contribution = %v, want weight*normalized*scale = %v", sc.Contribution, wantContribution)
	}
	if sc.EvidenceText != "orgs organizations account" {
		t.Errorf("EvidenceText = %q, want the hit's Text", sc.EvidenceText)
	}

	// final = lexical_total + semantic contribution
	wantFinal := orgs.Score.LexicalTotal + sc.Contribution
	if !almostEqual(orgs.Score.FinalScore, wantFinal) {
		t.Errorf("FinalScore = %v, want lexical_total + semantic contribution = %v", orgs.Score.FinalScore, wantFinal)
	}
}

func TestExpandAndBuild_FKOnlyTablesHaveNoScoreBreakdown(t *testing.T) {
	store := seedTestStore(t)

	// "orgs" pulls in reviews/pull_requests via FK expansion; comments is
	// two hops away and enters purely through expansion too.
	result, err := QueryHybrid(store, "orgs", nil)
	if err != nil {
		t.Fatalf("QueryHybrid: %v", err)
	}

	sawUnscored := false
	for _, tbl := range result.Tables {
		if !tbl.IsMatch {
			if tbl.Score != nil {
				t.Errorf("table %q was only FK-expanded (not matched) but has a non-nil Score breakdown", tbl.TableName)
			}
			sawUnscored = true
		}
	}
	if !sawUnscored {
		t.Skip("fixture didn't produce any FK-expanded-only tables for this query; nothing to assert")
	}
}

func TestQueryHybrid_Timing_LexicalOnly(t *testing.T) {
	store := seedTestStore(t)

	result, err := QueryHybrid(store, "reviews", nil)
	if err != nil {
		t.Fatalf("QueryHybrid: %v", err)
	}

	if result.Timing.SemanticRan {
		t.Error("SemanticRan should be false when no SemanticScorer was passed")
	}
	if result.Timing.SemanticMs != 0 {
		t.Errorf("SemanticMs = %v, want 0 when semantic didn't run", result.Timing.SemanticMs)
	}
	if result.Timing.TotalMs <= 0 {
		t.Errorf("TotalMs = %v, want > 0", result.Timing.TotalMs)
	}
	if result.Timing.TotalMs < result.Timing.LexicalMs+result.Timing.ExpandMs-0.01 {
		// allow tiny floating rounding slack; total should be >= the sum
		// of its measured phases.
		t.Errorf("TotalMs (%v) should be >= LexicalMs+ExpandMs (%v)", result.Timing.TotalMs, result.Timing.LexicalMs+result.Timing.ExpandMs)
	}
}

func TestQueryHybrid_Timing_WithSemantic(t *testing.T) {
	store := seedTestStore(t)

	sem := &fakeScorer{
		scores: map[string]float64{"orgs": 0.9},
		hits:   []SemanticHit{{TableName: "orgs", Kind: "table", Text: "orgs", Score: 0.9}},
	}
	result, err := QueryHybrid(store, "orgs", sem)
	if err != nil {
		t.Fatalf("QueryHybrid: %v", err)
	}

	if !result.Timing.SemanticRan {
		t.Error("SemanticRan should be true when a SemanticScorer was passed")
	}
	if result.Timing.TotalMs <= 0 {
		t.Errorf("TotalMs = %v, want > 0", result.Timing.TotalMs)
	}
}

func TestQueryHybrid_ScorerErrorFallsBackToLexical(t *testing.T) {
	store := seedTestStore(t)

	sem := &fakeScorer{err: fmt.Errorf("embedding backend unavailable")}

	result, err := QueryHybrid(store, "reviews", sem)
	if err != nil {
		t.Fatalf("QueryHybrid should not fail when the semantic scorer errors: %v", err)
	}
	found := false
	for _, tbl := range result.Tables {
		if tbl.TableName == "reviews" && tbl.IsMatch {
			found = true
		}
	}
	if !found {
		t.Error("lexical match should still succeed when semantic scorer errors")
	}
	if len(result.SemanticHits) != 0 {
		t.Error("SemanticHits should be empty when the scorer errored")
	}
}

func TestTerminologyMatch_NoTableIsANoOp(t *testing.T) {
	store := seedTestStore(t) // never had InitTerminologySchema called

	scores := ComputeLexicalScores(store, "monthly recurring revenue")
	if len(scores) != 0 {
		t.Errorf("expected no matches without a terminology table, got %v", scores)
	}
}

func TestTerminologyMatch_MultiWordAlias(t *testing.T) {
	store := seedTestStore(t)
	if err := store.InitTerminologySchema(); err != nil {
		t.Fatalf("InitTerminologySchema: %v", err)
	}
	store.DB().Exec(`INSERT INTO terminology (term, alias, target_table, source) VALUES ('pr', 'pull request', 'pull_requests', 'user')`)

	scores := ComputeLexicalScores(store, "how many open pull request items are there")
	if scores["pull_requests"] <= 0 {
		t.Errorf("expected terminology to match the multi-word alias 'pull request', scores = %v", scores)
	}
}

func TestTerminologyMatch_QueryContainsAlias(t *testing.T) {
	store := seedTestStore(t)
	if err := store.InitTerminologySchema(); err != nil {
		t.Fatalf("InitTerminologySchema: %v", err)
	}
	store.DB().Exec(`INSERT INTO terminology (term, alias, target_table, source) VALUES ('org', 'organization', 'orgs', 'user')`)

	scores := ComputeLexicalScores(store, "show me every organization")
	if scores["orgs"] <= 0 {
		t.Errorf("expected terminology match when query contains the full alias, scores = %v", scores)
	}
}

func TestTerminologyMatch_AliasContainsShortQuery(t *testing.T) {
	// Regression: importing a long alias like "organization" for a short
	// abbreviation like "org" must also match when the user types the
	// short form, not just the fully-spelled-out alias. Before this was
	// fixed, only "query contains alias" was checked, so a query shorter
	// than the stored alias never matched at all.
	store := seedTestStore(t)
	if err := store.InitTerminologySchema(); err != nil {
		t.Fatalf("InitTerminologySchema: %v", err)
	}
	store.DB().Exec(`INSERT INTO terminology (term, alias, target_table, source) VALUES ('org', 'organization', 'orgs', 'user')`)
	store.DB().Exec(`INSERT INTO terminology (term, alias, target_table, source) VALUES ('org', 'organization account', 'orgs', 'user')`)

	scores := ComputeLexicalScores(store, "org")
	if scores["orgs"] <= 0 {
		t.Errorf("expected terminology match when a short query is a substring of a longer alias, scores = %v", scores)
	}
}

func TestTerminologyMatch_AbbreviationFloorAvoidsNoise(t *testing.T) {
	// A one/two-character query fragment shouldn't score-match just
	// because it happens to appear inside some unrelated long alias.
	// Calls terminologyMatch directly (not ComputeLexicalScores) to
	// isolate this signal from unrelated fuzzy table-name matching, which
	// operates independently and would otherwise mask what this test
	// checks.
	store := seedTestStore(t)
	if err := store.InitTerminologySchema(); err != nil {
		t.Fatalf("InitTerminologySchema: %v", err)
	}
	store.DB().Exec(`INSERT INTO terminology (term, alias, target_table, source) VALUES ('org', 'organization', 'orgs', 'user')`)

	scores := terminologyMatch(store, "or")
	if scores["orgs"] > 0 {
		t.Errorf("expected no terminology match for a too-short query fragment, scores = %v", scores)
	}
}

func TestTerminologyMatch_QueryContainsAliasScoresHigherThanAbbreviation(t *testing.T) {
	// The full-phrase-match direction is stronger evidence than the
	// short-abbreviation direction, so it should be weighted higher.
	store := seedTestStore(t)
	if err := store.InitTerminologySchema(); err != nil {
		t.Fatalf("InitTerminologySchema: %v", err)
	}
	store.DB().Exec(`INSERT INTO terminology (term, alias, target_table, source) VALUES ('org', 'organization', 'orgs', 'user')`)

	full := terminologyMatch(store, "organization")
	short := terminologyMatch(store, "org")
	if full["orgs"] <= short["orgs"] {
		t.Errorf("full-alias match (%v) should score higher than short-abbreviation match (%v)", full["orgs"], short["orgs"])
	}
}

func TestTerminologyMatch_CaseInsensitive(t *testing.T) {
	store := seedTestStore(t)
	if err := store.InitTerminologySchema(); err != nil {
		t.Fatalf("InitTerminologySchema: %v", err)
	}
	store.DB().Exec(`INSERT INTO terminology (term, alias, target_table, source) VALUES ('pr', 'pull request', 'pull_requests', 'user')`)

	scores := ComputeLexicalScores(store, "PULL REQUEST status")
	if scores["pull_requests"] <= 0 {
		t.Errorf("expected case-insensitive terminology match, scores = %v", scores)
	}
}

func TestQuery_ScoreOrdering(t *testing.T) {
	store := seedTestStore(t)

	result, err := Query(store, "reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	// First matched result should have a positive score
	for _, tbl := range result.Tables {
		if tbl.IsMatch {
			if tbl.MatchScore <= 0 {
				t.Errorf("first match %s has score %.2f, want > 0", tbl.TableName, tbl.MatchScore)
			}
			break
		}
	}
}

func TestShortenType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"character varying", "varchar"},
		{"character varying(255)", "character varying(255)"}, // ShortenType doesn't handle params
		{"character", "char"},
		{"timestamp with time zone", "tstz"},
		{"timestamp without time zone", "ts"},
		{"time with time zone", "ttz"},
		{"time without time zone", "tt"},
		{"double precision", "float8"},
		{"real", "float4"},
		{"boolean", "bool"},
		{"integer", "int"},
		{"bigint", "int8"},
		{"smallint", "int2"},
		{"numeric", "num"},
		{"decimal", "num"},
		{"text", "text"},
		{"jsonb", "jsonb"},
		{"json", "json"},
		{"uuid", "uuid"},
		{"bytea", "bytea"},
		{"unknown_type", "unknown_type"}, // unknown types pass through
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ShortenType(tt.input)
			if got != tt.expected {
				t.Errorf("ShortenType(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestShortenTypeParam(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"character varying(255)", "varchar(255)"},
		{"character varying(50)", "varchar(50)"},
		{"character(10)", "char(10)"},
		{"numeric(10,2)", "num(10,2)"},
		{"integer", "int"},
		{"text", "text"},
		{"timestamp with time zone", "tstz"},
		{"unknown(100)", "unknown(100)"}, // unknown base type with params passes through
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ShortenTypeParam(tt.input)
			if got != tt.expected {
				t.Errorf("ShortenTypeParam(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
