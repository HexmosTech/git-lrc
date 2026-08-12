package semantic

import (
	"io"
	"testing"

	"github.com/shrsv/dbctx/internal/embed"
	"github.com/shrsv/dbctx/internal/search"
	"github.com/shrsv/dbctx/internal/testutil"
)

// realBenchEmbedder returns the real ONNX-backed embedder if the model and
// runtime are already cached locally, or skips — same opt-in-only pattern
// as internal/embed's own real-model tests/benchmarks. These measure this
// feature's actual end-to-end cost (build + query) at the 50-table
// synthetic schema size used by the fake-embedder benchmarks in
// bench_test.go, so the two can be compared directly: how much of the
// hybrid-query/build overhead is this package's own bookkeeping vs. real
// model inference.
func realBenchEmbedder(b *testing.B) Embedder {
	b.Helper()
	st, err := embed.CheckCache()
	if err != nil || !st.ModelReady || !st.RuntimeReady {
		b.Skip("bge-small-en-v1.5 model / onnxruntime library not present locally; skipping")
	}
	emb, err := NewDefaultEmbedder(nil)
	if err != nil {
		b.Fatalf("NewDefaultEmbedder: %v", err)
	}
	return emb
}

func BenchmarkBuildIndex_Large_RealModel(b *testing.B) {
	emb := realBenchEmbedder(b)
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		store := testutil.NewLargeStore(b, benchTableCount)
		b.StartTimer()

		if _, err := BuildIndex(store, emb, io.Discard); err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		store.Close()
		b.StartTimer()
	}
}

func BenchmarkQuery_Large_RealModel_Hybrid(b *testing.B) {
	emb := realBenchEmbedder(b)
	store := testutil.NewLargeStore(b, benchTableCount)
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
