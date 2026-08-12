package embed

import (
	"math"
	"testing"
)

// realBackend returns a ready-to-use OnnxEmbedder backed by whatever is in
// the local cache (or DBCTX_ONNXRUNTIME_LIB / DBCTX_CACHE_DIR overrides),
// or skips the test. These tests never download anything themselves — CI
// and ordinary `go test ./...` runs skip them, since the ~130MB model and
// the platform onnxruntime library are opt-in resources, not requirements.
func realBackend(t *testing.T) *OnnxEmbedder {
	t.Helper()
	st, err := CheckCache()
	if err != nil || !st.ModelReady || !st.RuntimeReady {
		t.Skip("bge-small-en-v1.5 model / onnxruntime library not present locally; skipping ONNX integration test")
	}
	emb, err := NewOnnxEmbedder(st.RuntimeLib, st.ModelPath, st.VocabPath)
	if err != nil {
		t.Fatalf("NewOnnxEmbedder: %v", err)
	}
	t.Cleanup(func() { emb.Close() })
	return emb
}

func cosine(a, b []float32) float64 {
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot
}

func TestOnnxEmbedder_Dims(t *testing.T) {
	emb := realBackend(t)
	if emb.Dims() != 384 {
		t.Errorf("Dims() = %d, want 384", emb.Dims())
	}
}

func TestOnnxEmbedder_NormalizedOutput(t *testing.T) {
	emb := realBackend(t)
	vecs, err := emb.EmbedPassages([]string{"orders"})
	if err != nil {
		t.Fatalf("EmbedPassages: %v", err)
	}
	if len(vecs) != 1 || len(vecs[0]) != 384 {
		t.Fatalf("unexpected shape: %d vectors, dim %d", len(vecs), len(vecs[0]))
	}
	norm := math.Sqrt(cosine(vecs[0], vecs[0]))
	if math.Abs(norm-1.0) > 1e-4 {
		t.Errorf("||v|| = %f, want ~1.0 (L2 normalized)", norm)
	}
}

func TestOnnxEmbedder_SemanticOrdering(t *testing.T) {
	emb := realBackend(t)

	queryVec, err := emb.EmbedQuery("buyers")
	if err != nil {
		t.Fatalf("EmbedQuery: %v", err)
	}

	passages := []string{"customers", "purchases", "orders", "a completely unrelated random topic about volcanoes"}
	vecs, err := emb.EmbedPassages(passages)
	if err != nil {
		t.Fatalf("EmbedPassages: %v", err)
	}

	scores := make(map[string]float64, len(passages))
	for i, p := range passages {
		scores[p] = cosine(queryVec, vecs[i])
	}

	if scores["customers"] <= scores["a completely unrelated random topic about volcanoes"] {
		t.Errorf("expected 'buyers' closer to 'customers' (%.4f) than to unrelated text (%.4f)",
			scores["customers"], scores["a completely unrelated random topic about volcanoes"])
	}
	if scores["purchases"] <= scores["a completely unrelated random topic about volcanoes"] {
		t.Errorf("expected 'buyers' closer to 'purchases' (%.4f) than to unrelated text (%.4f)",
			scores["purchases"], scores["a completely unrelated random topic about volcanoes"])
	}
}

func TestOnnxEmbedder_Deterministic(t *testing.T) {
	emb := realBackend(t)
	v1, err := emb.EmbedPassages([]string{"repositories"})
	if err != nil {
		t.Fatalf("EmbedPassages: %v", err)
	}
	v2, err := emb.EmbedPassages([]string{"repositories"})
	if err != nil {
		t.Fatalf("EmbedPassages: %v", err)
	}
	sim := cosine(v1[0], v2[0])
	if sim < 0.9999 {
		t.Errorf("same input produced different embeddings: cosine = %f, want ~1.0", sim)
	}
}

func TestOnnxEmbedder_BatchingConsistency(t *testing.T) {
	emb := realBackend(t)
	texts := []string{"orders", "customers", "reviews", "pull_requests", "repositories"}

	alone, err := emb.EmbedPassages(texts[:1])
	if err != nil {
		t.Fatalf("EmbedPassages(single): %v", err)
	}
	batch, err := emb.EmbedPassages(texts)
	if err != nil {
		t.Fatalf("EmbedPassages(batch): %v", err)
	}
	sim := cosine(alone[0], batch[0])
	if sim < 0.999 {
		t.Errorf("batching changed the embedding for the same text: cosine = %f", sim)
	}
}
