package dbctx

import (
	"bytes"
	"testing"

	"github.com/shrsv/dbctx/internal/search"
	"github.com/shrsv/dbctx/internal/testutil"
)

// --- Benchmarks ---

func BenchmarkQuery_Short(b *testing.B) {
	s := testutil.NewTestStore(b, search.PopulateFTS)
	idx := &Index{store: s, ready: make(chan struct{})}
	close(idx.ready)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Query("reviews")
	}
}

func BenchmarkQuery_Medium(b *testing.B) {
	s := testutil.NewTestStore(b, search.PopulateFTS)
	idx := &Index{store: s, ready: make(chan struct{})}
	close(idx.ready)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Query("failed reviews last month")
	}
}

func BenchmarkQuery_Fuzzy(b *testing.B) {
	s := testutil.NewTestStore(b, search.PopulateFTS)
	idx := &Index{store: s, ready: make(chan struct{})}
	close(idx.ready)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Query("revews") // misspelled — triggers fuzzy
	}
}

func BenchmarkMatchedText(b *testing.B) {
	s := testutil.NewTestStore(b, search.PopulateFTS)
	idx := &Index{store: s, ready: make(chan struct{})}
	close(idx.ready)
	result, _ := idx.Query("reviews")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result.Matched().Text()
	}
}

func BenchmarkMatchedTextRaw(b *testing.B) {
	s := testutil.NewTestStore(b, search.PopulateFTS)
	idx := &Index{store: s, ready: make(chan struct{})}
	close(idx.ready)
	result, _ := idx.Query("reviews")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result.Matched().TextRaw()
	}
}

func BenchmarkScoredOnlyText(b *testing.B) {
	s := testutil.NewTestStore(b, search.PopulateFTS)
	idx := &Index{store: s, ready: make(chan struct{})}
	close(idx.ready)
	result, _ := idx.Query("reviews")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result.ScoredOnly().Text()
	}
}

func BenchmarkReport(b *testing.B) {
	s := testutil.NewTestStore(b, search.PopulateFTS)
	idx := &Index{store: s, ready: make(chan struct{})}
	close(idx.ready)
	var buf bytes.Buffer
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		idx.Report(&buf)
	}
}

func BenchmarkTables(b *testing.B) {
	s := testutil.NewTestStore(b, search.PopulateFTS)
	idx := &Index{store: s, ready: make(chan struct{})}
	close(idx.ready)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Tables()
	}
}

func BenchmarkTableDetail(b *testing.B) {
	s := testutil.NewTestStore(b, search.PopulateFTS)
	idx := &Index{store: s, ready: make(chan struct{})}
	close(idx.ready)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.TableDetail("reviews")
	}
}

func BenchmarkStats(b *testing.B) {
	s := testutil.NewTestStore(b, search.PopulateFTS)
	idx := &Index{store: s, ready: make(chan struct{})}
	close(idx.ready)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Stats()
	}
}
