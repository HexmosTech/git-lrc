package semantic

import (
	"io"
	"testing"

	"github.com/shrsv/dbctx/internal/testutil"
)

func TestNewScorer_NoIndex(t *testing.T) {
	store := testutil.NewSeedStore(t)
	emb := newFakeEmbedder()

	_, err := NewScorer(store, emb)
	if err == nil {
		t.Fatal("expected error when no semantic index has been built")
	}
}

func TestNewScorer_ModelMismatch(t *testing.T) {
	store := testutil.NewSeedStore(t)
	buildEmb := newFakeEmbedder()
	buildEmb.modelID = "model-a/v1"
	if _, err := BuildIndex(store, buildEmb, io.Discard); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	queryEmb := newFakeEmbedder()
	queryEmb.modelID = "model-b/v1"
	_, err := NewScorer(store, queryEmb)
	if err == nil {
		t.Fatal("expected error when embedder model differs from persisted model")
	}
}

func TestNewScorer_DimsMismatch(t *testing.T) {
	store := testutil.NewSeedStore(t)
	buildEmb := newFakeEmbedder()
	if _, err := BuildIndex(store, buildEmb, io.Discard); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	queryEmb := newFakeEmbedder()
	queryEmb.modelID = buildEmb.modelID // same model id, different dims: simulate corrupt/misconfigured state
	queryEmb.dims = 32
	_, err := NewScorer(store, queryEmb)
	if err == nil {
		t.Fatal("expected error when embedder dimensionality differs from persisted dims")
	}
}

func TestNewScorer_Compatible(t *testing.T) {
	store := testutil.NewSeedStore(t)
	emb := newFakeEmbedder()
	if _, err := BuildIndex(store, emb, io.Discard); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	if _, err := NewScorer(store, emb); err != nil {
		t.Fatalf("NewScorer with matching embedder: %v", err)
	}
}

func TestScorer_Score_RanksSharedWordsHigher(t *testing.T) {
	store := testutil.NewSeedStore(t)
	emb := newFakeEmbedder()
	if _, err := BuildIndex(store, emb, io.Discard); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	scorer, err := NewScorer(store, emb)
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}

	// "enterprise" is a representative value of orgs.plan/orgs.tier in the
	// fixture; the fake embedder is bag-of-words, so a query containing it
	// should score 'orgs' highest among tables.
	scores, hits, err := scorer.Score("enterprise plan")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if len(scores) == 0 {
		t.Fatal("Score returned no scores")
	}

	var top string
	var topScore float64
	for table, s := range scores {
		if s > topScore {
			topScore = s
			top = table
		}
	}
	if top != "orgs" {
		t.Errorf("top-scoring table = %q, want orgs (scores: %v)", top, scores)
	}
	if len(hits) == 0 {
		t.Error("Score returned no hits (evidence)")
	}
}

func TestScorer_Score_NormalizedRange(t *testing.T) {
	store := testutil.NewSeedStore(t)
	emb := newFakeEmbedder()
	if _, err := BuildIndex(store, emb, io.Discard); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	scorer, err := NewScorer(store, emb)
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}

	scores, _, err := scorer.Score("reviews status completed")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	for table, s := range scores {
		if s < 0 || s > 1 {
			t.Errorf("score for %s = %f, want in [0,1]", table, s)
		}
	}
	// At least one table should be at the top of the normalized range.
	sawTop := false
	for _, s := range scores {
		if s >= 0.999 {
			sawTop = true
		}
	}
	if !sawTop {
		t.Error("expected at least one table normalized to ~1.0 (the best match)")
	}
}

func TestScorer_Score_TopKBound(t *testing.T) {
	store := testutil.NewSeedStore(t)
	emb := newFakeEmbedder()
	if _, err := BuildIndex(store, emb, io.Discard); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	scorer, err := NewScorer(store, emb)
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}

	scores, _, err := scorer.Score("orgs reviews comments pull_requests status plan")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if len(scores) > topK {
		t.Errorf("Score returned %d tables, want <= topK (%d)", len(scores), topK)
	}
}
