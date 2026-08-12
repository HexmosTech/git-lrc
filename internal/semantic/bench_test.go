package semantic

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/shrsv/dbctx/internal/search"
	"github.com/shrsv/dbctx/internal/testutil"
)

// These benchmarks use a 50-table synthetic schema (see
// testutil.NewLargeStore) rather than the 4-table hand-written fixture
// used elsewhere — dbctx's own stated performance target is 50+ table
// databases (see README "Performance"), and brute-force cosine similarity
// is only "entirely acceptable" (the design's own words) at that kind of
// scale, not a handful of tables. They use the deterministic fake
// embedder (see fake_embedder_test.go), not the real ONNX backend, so
// they measure this package's own retrieval/storage overhead in
// isolation from model inference cost — see internal/embed's benchmarks
// for real embedding latency.

const benchTableCount = 50

func BenchmarkBuildIndex_Large(b *testing.B) {
	// NewLargeStore's in-memory databases share a single SQLite
	// "file::memory:?cache=shared" connection namespace, so each iteration
	// must fully close its store before the next one opens — otherwise
	// row IDs collide against data still alive in the shared cache.
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		store := testutil.NewLargeStore(b, benchTableCount)
		emb := newFakeEmbedder()
		b.StartTimer()

		if _, err := BuildIndex(store, emb, io.Discard); err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		store.Close()
		b.StartTimer()
	}
}

func BenchmarkBuildIndex_Large_Incremental(b *testing.B) {
	store := testutil.NewLargeStore(b, benchTableCount)
	emb := newFakeEmbedder()
	if _, err := BuildIndex(store, emb, io.Discard); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Unchanged schema: every candidate is reused, exercising only the
		// diff/hash-comparison path, not re-embedding.
		if _, err := BuildIndex(store, emb, io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScorer_Score_Large(b *testing.B) {
	store := testutil.NewLargeStore(b, benchTableCount)
	emb := newFakeEmbedder()
	if _, err := BuildIndex(store, emb, io.Discard); err != nil {
		b.Fatal(err)
	}
	scorer, err := NewScorer(store, emb)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, _, err := scorer.Score("cancelled subscriptions status update"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQuery_Large_LexicalOnly(b *testing.B) {
	store := testutil.NewLargeStore(b, benchTableCount)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := search.Query(store, "cancelled subscriptions status update"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQuery_Large_Hybrid(b *testing.B) {
	store := testutil.NewLargeStore(b, benchTableCount)
	emb := newFakeEmbedder()
	if _, err := BuildIndex(store, emb, io.Discard); err != nil {
		b.Fatal(err)
	}
	scorer, err := NewScorer(store, emb)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := search.QueryHybrid(store, "cancelled subscriptions status update", scorer); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkOpenAndQuery_WithSemanticIndex_FileBacked measures the cost of
// reopening a .dtx file that already has a populated semantic index and
// immediately querying it — the actual per-process startup + first-query
// cost a CLI invocation or freshly-started service pays, as opposed to the
// in-memory benchmarks above.
func BenchmarkOpenAndQuery_WithSemanticIndex_FileBacked(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "bench.dtx")

	store := testutil.NewLargeStoreAt(b, benchTableCount, path)
	emb := newFakeEmbedder()
	if _, err := BuildIndex(store, emb, io.Discard); err != nil {
		b.Fatal(err)
	}
	store.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reopened := testutil.OpenExistingStore(b, path)
		scorer, err := NewScorer(reopened, emb)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := search.QueryHybrid(reopened, "cancelled subscriptions status update", scorer); err != nil {
			b.Fatal(err)
		}
		reopened.Close()
	}
}
