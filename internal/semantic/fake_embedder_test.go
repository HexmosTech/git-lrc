package semantic

import (
	"hash/fnv"
	"math"
	"strings"
)

// fakeEmbedder is a deterministic, dependency-free stand-in for a real
// model, used across this package's tests. It hashes words into buckets
// (bag-of-words), so texts sharing words score more similar than texts
// that don't — enough to exercise BuildIndex/Scorer end-to-end without
// needing the real ONNX backend. It cannot demonstrate true paraphrase
// understanding (e.g. "buyers" ~ "customers" with zero word overlap) —
// that's covered separately by internal/embed's real-model integration
// tests and the top-level dbctx semantic recall tests.
type fakeEmbedder struct {
	dims         int
	modelID      string
	passageCalls int
	queryCalls   int
}

func newFakeEmbedder() *fakeEmbedder {
	return &fakeEmbedder{dims: 16, modelID: "fake-test-model/v1"}
}

func (f *fakeEmbedder) Dims() int       { return f.dims }
func (f *fakeEmbedder) ModelID() string { return f.modelID }

func (f *fakeEmbedder) EmbedPassages(texts []string) ([][]float32, error) {
	f.passageCalls++
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = hashEmbed(t, f.dims)
	}
	return out, nil
}

func (f *fakeEmbedder) EmbedQuery(text string) ([]float32, error) {
	f.queryCalls++
	return hashEmbed(text, f.dims), nil
}

func hashEmbed(text string, dims int) []float32 {
	v := make([]float32, dims)
	for _, w := range strings.Fields(strings.ToLower(text)) {
		h := fnv.New32a()
		h.Write([]byte(w))
		idx := int(h.Sum32() % uint32(dims))
		v[idx]++
	}
	var sumSq float64
	for _, x := range v {
		sumSq += float64(x) * float64(x)
	}
	norm := math.Sqrt(sumSq)
	if norm == 0 {
		return v
	}
	for i := range v {
		v[i] = float32(float64(v[i]) / norm)
	}
	return v
}
