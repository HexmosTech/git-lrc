package semantic

import (
	"fmt"

	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/embed"
	"github.com/shrsv/dbctx/internal/search"
)

// NewDefaultEmbedder builds dbctx's standard embedding backend (BGE-small-
// en-v1.5 via onnxruntime), downloading the model weights and the
// platform's onnxruntime shared library to the local cache first if either
// is missing. progress may be nil.
//
// This is the only place internal/semantic depends on internal/embed
// (and therefore on CGO/onnxruntime) — the rest of this package works
// against the embedder-agnostic Embedder interface. Callers that want a
// different backend can implement Embedder themselves and skip this
// constructor entirely.
func NewDefaultEmbedder(progress embed.ProgressFunc) (Embedder, error) {
	libPath, err := embed.EnsureRuntimeLibrary(progress)
	if err != nil {
		return nil, fmt.Errorf("onnxruntime library: %w", err)
	}
	modelPath, vocabPath, err := embed.EnsureModel(progress)
	if err != nil {
		return nil, fmt.Errorf("model weights: %w", err)
	}
	return embed.NewOnnxEmbedder(libPath, modelPath, vocabPath)
}

// OpenScorer returns a ready-to-use search.SemanticScorer for store if it
// has a usable semantic index, constructing the default embedder (which
// may download model/runtime assets to the local cache) only in that case.
//
// Returns (nil, nil) — not an error — if store simply has no semantic
// index at all, which is the common case for a lexical-only .dtx file or
// one built with semantic indexing disabled. This is what keeps opening
// an ordinary .dtx file free of any download or model-load cost: the
// cheap Available() check runs first, and the (comparatively expensive)
// embedder is only constructed when there's actually something to use it
// for.
func OpenScorer(store *db.Store, progress embed.ProgressFunc) (search.SemanticScorer, error) {
	if _, _, ok := Available(store); !ok {
		return nil, nil
	}
	emb, err := NewDefaultEmbedder(progress)
	if err != nil {
		return nil, err
	}
	return NewScorer(store, emb)
}
