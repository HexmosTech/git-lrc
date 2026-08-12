package semantic

import (
	"fmt"
	"sort"

	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/search"
)

// topK bounds how many distinct tables the semantic signal contributes to
// a query, keeping noise out of the fused ranking. Raw cosine similarity
// from short-text CLS embeddings has a fairly high "unrelated text" floor
// (empirically ~0.4-0.45 for bge-small), so relative ranking — not the
// absolute cosine value — is what's informative; topK plus the min-max
// normalization in Score are both here to make the signal about relative
// relevance within this query, not an absolute, cross-query-comparable
// score.
const topK = 12

// Scorer implements search.SemanticScorer against a store's persisted
// semantic_objects, using brute-force cosine similarity. This is
// intentionally not an ANN index: dbctx's expected corpus size (schema
// objects for one database) is small enough that scanning every embedding
// on every query is cheap, and brute force is far simpler to reason about
// and debug than an approximate index would be.
type Scorer struct {
	store    *db.Store
	embedder Embedder
}

// NewScorer builds a Scorer for store using embedder, after verifying the
// embedder matches the model/dimensionality the store's embeddings were
// actually built with. Returns an error if there's no usable semantic
// index on store, or if embedder is incompatible with what's persisted —
// callers should treat either as "fall back to lexical-only", not as a
// fatal condition (see Available for a cheap pre-check that avoids
// constructing an embedder at all in the common no-semantic-index case).
func NewScorer(store *db.Store, embedder Embedder) (*Scorer, error) {
	modelID, dims, ok := Available(store)
	if !ok {
		return nil, fmt.Errorf("no semantic index present")
	}
	if modelID != embedder.ModelID() {
		return nil, fmt.Errorf("semantic index was built with model %q, embedder is %q", modelID, embedder.ModelID())
	}
	if dims != embedder.Dims() {
		return nil, fmt.Errorf("semantic index has dimensionality %d, embedder produces %d", dims, embedder.Dims())
	}
	return &Scorer{store: store, embedder: embedder}, nil
}

// Score embeds query and returns per-table relevance scores, min-max
// normalized across this query's candidates into a 0..1 range (so the
// result is about relative relevance within this query, not raw cosine
// magnitude — see the topK doc comment), plus the best-matching object per
// table as inspectable evidence.
func (s *Scorer) Score(query string) (map[string]float64, []search.SemanticHit, error) {
	qvec, err := s.embedder.EmbedQuery(query)
	if err != nil {
		return nil, nil, fmt.Errorf("embed query: %w", err)
	}

	rows, err := s.store.DB().Query(`
		SELECT so.kind, t.name, so.text, so.embedding
		FROM semantic_objects so
		JOIN tables t ON t.id = so.table_id
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("scan semantic_objects: %w", err)
	}
	defer rows.Close()

	type candidateHit struct {
		table string
		kind  string
		text  string
		score float64
	}
	bestPerTable := make(map[string]candidateHit)

	for rows.Next() {
		var kind, table, text string
		var blob []byte
		if err := rows.Scan(&kind, &table, &text, &blob); err != nil {
			continue
		}
		vec := decodeVector(blob)
		if len(vec) != s.embedder.Dims() {
			continue // defensive: skip rows that don't match this embedder's dimensionality
		}
		sim := cosine(qvec, vec)
		if existing, ok := bestPerTable[table]; !ok || sim > existing.score {
			bestPerTable[table] = candidateHit{table: table, kind: kind, text: text, score: sim}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(bestPerTable) == 0 {
		return nil, nil, nil
	}

	all := make([]candidateHit, 0, len(bestPerTable))
	minScore, maxScore := 1.0, -1.0
	for _, h := range bestPerTable {
		all = append(all, h)
		if h.score < minScore {
			minScore = h.score
		}
		if h.score > maxScore {
			maxScore = h.score
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].score > all[j].score })
	if len(all) > topK {
		all = all[:topK]
	}

	spread := maxScore - minScore
	scores := make(map[string]float64, len(all))
	hits := make([]search.SemanticHit, 0, len(all))
	for _, h := range all {
		normalized := 1.0
		if spread > 1e-9 {
			normalized = (h.score - minScore) / spread
		}
		scores[h.table] = normalized
		hits = append(hits, search.SemanticHit{
			TableName: h.table,
			Kind:      h.kind,
			Text:      h.text,
			Score:     h.score,
		})
	}
	return scores, hits, nil
}
