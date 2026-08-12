// Package semantic adds an optional embedding-based retrieval signal on top
// of dbctx's existing deterministic/lexical index. It builds compact,
// natural-language-ish text for schema objects already known to dbctx
// (tables, meaningful columns, JSONB paths), embeds that text with a local
// model, persists the vectors in the .dtx SQLite store, and at query time
// scores a user's question against those vectors with brute-force cosine
// similarity — small enough corpora (the expected number of schema objects
// in a database) make an ANN index unnecessary.
//
// This package has no dependency on any particular inference backend: it
// defines the [Embedder] interface it needs, and callers (dbctx.go, the
// CLI) supply a concrete implementation such as internal/embed's ONNX
// backend. That keeps CGO/onnxruntime entirely out of this package and out
// of the lexical-only retrieval path in internal/search.
package semantic

import (
	"encoding/binary"
	"math"
)

// Embedder produces fixed-length vectors for text. Implementations should
// be deterministic for a given model/version so that embeddings persisted
// in one build are comparable to embeddings computed for a query later.
//
// BGE-family models (the default backend) are asymmetric: queries and
// indexed passages are embedded differently (a retrieval instruction is
// prepended to queries only), hence the two methods rather than one.
type Embedder interface {
	// EmbedPassages embeds indexed schema-object text.
	EmbedPassages(texts []string) ([][]float32, error)
	// EmbedQuery embeds a single natural-language query.
	EmbedQuery(text string) ([]float32, error)
	// Dims returns the embedding dimensionality.
	Dims() int
	// ModelID returns a stable identifier for the model + backend
	// combination, persisted alongside embeddings for compatibility
	// checking (see Available).
	ModelID() string
}

// Object kinds. Only these three are ever embedded — see objects.go for
// what determines whether a given column or JSONB path is "meaningful"
// enough to include, keeping the corpus small and low-noise rather than
// embedding every piece of metadata independently.
const (
	KindTable     = "table"
	KindColumn    = "column"
	KindJSONBPath = "jsonb_path"
)

// Metadata keys written to the store's generic `metadata` table, recording
// which model produced the persisted embeddings so a mismatched embedder
// (different model, different dimensionality) can be detected cleanly
// rather than silently producing garbage cosine similarities.
const (
	metaModelKey   = "semantic_model"
	metaDimsKey    = "semantic_dims"
	metaBuiltAtKey = "semantic_built_at"
)

// encodeVector packs a float32 vector as little-endian bytes for compact
// BLOB storage — far smaller than a JSON/text encoding of the same values.
func encodeVector(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// decodeVector reverses encodeVector. Returns nil if b's length isn't a
// multiple of 4 (defensive against a corrupt or truncated BLOB).
func decodeVector(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

// cosine returns the cosine similarity of two equal-length vectors. Since
// dbctx's embedder implementations L2-normalize their output, this reduces
// to a dot product; the general form is used here so a future embedder
// need not guarantee normalization.
func cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
