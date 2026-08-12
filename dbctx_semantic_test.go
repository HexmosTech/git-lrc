package dbctx

import (
	"io"
	"testing"

	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/embed"
	"github.com/shrsv/dbctx/internal/search"
	"github.com/shrsv/dbctx/internal/semantic"
)

// newECommerceStore builds a small e-commerce-flavored schema — the kind of
// domain the task's example queries ("buyers" -> customers, "purchases" ->
// orders) are drawn from — directly via SQL, independent of testutil's
// fixture (which is hand-tuned for the code-review-domain 4-table fixture
// used by most other tests and isn't meant to be a general-purpose builder).
func newECommerceStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.OpenStore("")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.InitSchema(); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	if err := store.InitFTS(); err != nil {
		t.Fatalf("InitFTS: %v", err)
	}

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := store.DB().Exec(q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}

	// customers
	exec(`INSERT INTO tables (id, schema, name, row_estimate) VALUES (1, 'public', 'customers', 1000)`)
	exec(`INSERT INTO columns (table_id, name, type, nullable, position) VALUES (1, 'id', 'integer', 0, 1)`)
	exec(`INSERT INTO columns (table_id, name, type, nullable, position) VALUES (1, 'email', 'text', 0, 2)`)
	exec(`INSERT INTO columns (table_id, name, type, nullable, position) VALUES (1, 'created_at', 'timestamptz', 0, 3)`)
	exec(`INSERT INTO primary_keys (table_id, column_name) VALUES (1, 'id')`)

	// orders
	exec(`INSERT INTO tables (id, schema, name, row_estimate) VALUES (2, 'public', 'orders', 5000)`)
	exec(`INSERT INTO columns (table_id, name, type, nullable, position) VALUES (2, 'id', 'integer', 0, 1)`)
	exec(`INSERT INTO columns (table_id, name, type, nullable, position) VALUES (2, 'customer_id', 'integer', 0, 2)`)
	exec(`INSERT INTO columns (table_id, name, type, nullable, position) VALUES (2, 'total', 'numeric', 0, 3)`)
	exec(`INSERT INTO columns (table_id, name, type, nullable, position) VALUES (2, 'status', 'text', 0, 4)`)
	exec(`INSERT INTO columns (table_id, name, type, nullable, position) VALUES (2, 'created_at', 'timestamptz', 0, 5)`)
	exec(`INSERT INTO primary_keys (table_id, column_name) VALUES (2, 'id')`)
	exec(`INSERT INTO foreign_keys (table_id, src_columns, ref_table_id, dst_columns, constraint_name) VALUES (2, 'customer_id', 1, 'id', 'orders_customer_fk')`)
	var statusColID int64
	store.DB().QueryRow(`SELECT id FROM columns WHERE table_id = 2 AND name = 'status'`).Scan(&statusColID)
	exec(`INSERT INTO field_stats (column_id, distinct_count, null_count, is_state_like, is_categorical) VALUES (?, 4, 0, 1, 1)`, statusColID)
	for _, v := range []string{"pending", "paid", "cancelled", "refunded"} {
		exec(`INSERT INTO field_values (column_id, value, frequency) VALUES (?, ?, 100)`, statusColID, v)
	}

	// repositories (unrelated domain object, to test "GitHub repositories" recall)
	exec(`INSERT INTO tables (id, schema, name, row_estimate) VALUES (3, 'public', 'repositories', 200)`)
	exec(`INSERT INTO columns (table_id, name, type, nullable, position) VALUES (3, 'id', 'integer', 0, 1)`)
	exec(`INSERT INTO columns (table_id, name, type, nullable, position) VALUES (3, 'name', 'text', 0, 2)`)
	exec(`INSERT INTO columns (table_id, name, type, nullable, position) VALUES (3, 'provider', 'text', 0, 3)`)
	exec(`INSERT INTO primary_keys (table_id, column_name) VALUES (3, 'id')`)
	var providerColID int64
	store.DB().QueryRow(`SELECT id FROM columns WHERE table_id = 3 AND name = 'provider'`).Scan(&providerColID)
	exec(`INSERT INTO field_stats (column_id, distinct_count, null_count, is_state_like, is_categorical) VALUES (?, 2, 0, 0, 1)`, providerColID)
	for _, v := range []string{"github", "gitlab"} {
		exec(`INSERT INTO field_values (column_id, value, frequency) VALUES (?, ?, 100)`, providerColID, v)
	}

	if err := search.PopulateFTS(store); err != nil {
		t.Fatalf("PopulateFTS: %v", err)
	}
	return store
}

// realEmbedder returns a default embedder backed by whatever is in the
// local model/runtime cache, or skips the test. Like internal/embed's own
// integration tests, this never downloads anything — it's opt-in via a
// populated cache (see internal/embed.CheckCache), keeping ordinary
// `go test ./...` runs fast and network-free.
func realEmbedder(t *testing.T) semantic.Embedder {
	t.Helper()
	st, err := embed.CheckCache()
	if err != nil || !st.ModelReady || !st.RuntimeReady {
		t.Skip("bge-small-en-v1.5 model / onnxruntime library not present locally; skipping semantic recall integration test")
	}
	emb, err := semantic.NewDefaultEmbedder(nil)
	if err != nil {
		t.Fatalf("NewDefaultEmbedder: %v", err)
	}
	return emb
}

func topTable(scores map[string]float64) string {
	var top string
	var best float64 = -1
	for t, s := range scores {
		if s > best {
			best, top = s, t
		}
	}
	return top
}

// TestSemanticRecall_Paraphrases is the direct test of this feature's
// stated purpose: recovering results for semantic paraphrases that share
// no vocabulary with the underlying table/column names, which lexical/fuzzy
// matching structurally cannot do.
func TestSemanticRecall_Paraphrases(t *testing.T) {
	store := newECommerceStore(t)
	emb := realEmbedder(t)

	if _, err := semantic.BuildIndex(store, emb, io.Discard); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	scorer, err := semantic.NewScorer(store, emb)
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}

	cases := []struct {
		query string
		want  string
	}{
		{"buyers", "customers"},
		{"purchases", "orders"},
		{"people who bought something", "orders"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			// Sanity check: lexical search alone must NOT find this table —
			// otherwise the test doesn't actually exercise the semantic path.
			lexical := search.ComputeLexicalScores(store, tc.query)
			if lexical[tc.want] > 0 {
				t.Skipf("lexical search already finds %q for %q; this case doesn't exercise semantic recall", tc.want, tc.query)
			}

			result, err := search.QueryHybrid(store, tc.query, scorer)
			if err != nil {
				t.Fatalf("QueryHybrid(%q): %v", tc.query, err)
			}

			found := false
			for _, tbl := range result.Tables {
				if tbl.TableName == tc.want && tbl.IsMatch {
					found = true
				}
			}
			if !found {
				t.Errorf("query %q: expected %q among matched tables via semantic recall, got %+v", tc.query, tc.want, result.Tables)
			}
		})
	}
}

// TestSemanticRecall_ExactMatchStillDominates verifies the "exact database
// identifiers are extremely valuable" requirement: querying the literal
// table name must not be outranked by a semantically-related-but-different
// table, even with semantic search enabled.
func TestSemanticRecall_ExactMatchStillDominates(t *testing.T) {
	store := newECommerceStore(t)
	emb := realEmbedder(t)

	if _, err := semantic.BuildIndex(store, emb, io.Discard); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	scorer, err := semantic.NewScorer(store, emb)
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}

	result, err := search.QueryHybrid(store, "orders", scorer)
	if err != nil {
		t.Fatalf("QueryHybrid: %v", err)
	}
	if len(result.Tables) == 0 {
		t.Fatal("no tables returned")
	}
	top := result.Tables[0]
	for _, tbl := range result.Tables {
		if tbl.MatchScore > top.MatchScore {
			top = tbl
		}
	}
	if top.TableName != "orders" {
		t.Errorf("top match for query 'orders' = %q (score %.2f), want 'orders'", top.TableName, top.MatchScore)
	}
}

// TestSemanticRecall_GitHubRepositories exercises recall against a JSONB
// -style categorical field (repositories.provider) via a natural-language
// description rather than the literal value.
func TestSemanticRecall_GitHubRepositories(t *testing.T) {
	store := newECommerceStore(t)
	emb := realEmbedder(t)

	if _, err := semantic.BuildIndex(store, emb, io.Discard); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	scorer, err := semantic.NewScorer(store, emb)
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}

	scores, _, err := scorer.Score("GitHub repositories")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if topTable(scores) != "repositories" {
		t.Errorf("top semantic table for 'GitHub repositories' = %q, want 'repositories' (scores: %v)", topTable(scores), scores)
	}
}

// TestSemanticRecall_WorksWithoutTerminology verifies semantic and
// terminology are fully independent signals: semantic retrieval must work
// normally on a store that never had any terminology imported (the common
// case, since terminology is opt-in and separate from --semantic).
func TestSemanticRecall_WorksWithoutTerminology(t *testing.T) {
	store := newECommerceStore(t) // never calls store.InitTerminologySchema
	emb := realEmbedder(t)

	if _, err := semantic.BuildIndex(store, emb, io.Discard); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	scorer, err := semantic.NewScorer(store, emb)
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}

	result, err := search.QueryHybrid(store, "buyers", scorer)
	if err != nil {
		t.Fatalf("QueryHybrid: %v", err)
	}
	found := false
	for _, tbl := range result.Tables {
		if tbl.TableName == "customers" && tbl.IsMatch {
			found = true
		}
	}
	if !found {
		t.Error("semantic recall for 'buyers' should work with no terminology dictionary present")
	}
}

// TestSemanticSearch_UnavailableByDefault verifies the core optionality
// requirement: a store that never had a semantic index built behaves
// exactly like lexical-only dbctx, with no attempt to load a model.
func TestSemanticSearch_UnavailableByDefault(t *testing.T) {
	store := newECommerceStore(t)

	scorer, err := semantic.OpenScorer(store, nil)
	if err != nil {
		t.Fatalf("OpenScorer on a store with no semantic index should not error: %v", err)
	}
	if scorer != nil {
		t.Error("OpenScorer should return a nil scorer when no semantic index exists")
	}

	result, err := search.QueryHybrid(store, "orders", scorer)
	if err != nil {
		t.Fatalf("QueryHybrid: %v", err)
	}
	if len(result.Tables) == 0 {
		t.Fatal("expected lexical results even with no semantic scorer")
	}
}
