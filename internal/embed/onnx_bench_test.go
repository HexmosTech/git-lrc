package embed

import "testing"

// These benchmarks exercise the real ONNX backend and, like the real
// tests in onnx_test.go, are skipped unless the model/runtime are already
// present in the local cache — they never download anything themselves.

func BenchmarkOnnxEmbedder_ColdInit(b *testing.B) {
	st, err := CheckCache()
	if err != nil || !st.ModelReady || !st.RuntimeReady {
		b.Skip("bge-small-en-v1.5 model / onnxruntime library not present locally; skipping")
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		emb, err := NewOnnxEmbedder(st.RuntimeLib, st.ModelPath, st.VocabPath)
		if err != nil {
			b.Fatal(err)
		}
		emb.Close()
	}
}

func BenchmarkOnnxEmbedder_EmbedPassages_Single(b *testing.B) {
	emb := benchEmbedder(b)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := emb.EmbedPassages([]string{"orders customer_id total status created_at"}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOnnxEmbedder_EmbedPassages_Batch16(b *testing.B) {
	emb := benchEmbedder(b)
	texts := make([]string, 16)
	for i := range texts {
		texts[i] = "orders customer_id total status created_at"
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := emb.EmbedPassages(texts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOnnxEmbedder_EmbedQuery(b *testing.B) {
	emb := benchEmbedder(b)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := emb.EmbedQuery("people who bought something"); err != nil {
			b.Fatal(err)
		}
	}
}

func benchEmbedder(b *testing.B) *OnnxEmbedder {
	b.Helper()
	st, err := CheckCache()
	if err != nil || !st.ModelReady || !st.RuntimeReady {
		b.Skip("bge-small-en-v1.5 model / onnxruntime library not present locally; skipping")
	}
	emb, err := NewOnnxEmbedder(st.RuntimeLib, st.ModelPath, st.VocabPath)
	if err != nil {
		b.Fatalf("NewOnnxEmbedder: %v", err)
	}
	b.Cleanup(func() { emb.Close() })
	return emb
}
