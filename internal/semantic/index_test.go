package semantic

import (
	"io"
	"testing"

	"github.com/shrsv/dbctx/internal/testutil"
)

func TestBuildIndex_CreatesObjects(t *testing.T) {
	store := testutil.NewSeedStore(t)
	emb := newFakeEmbedder()

	stats, err := BuildIndex(store, emb, io.Discard)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if stats.Total == 0 {
		t.Fatal("stats.Total = 0")
	}
	if stats.Embedded != stats.Total {
		t.Errorf("first build: Embedded = %d, want %d (Total)", stats.Embedded, stats.Total)
	}
	if stats.Reused != 0 {
		t.Errorf("first build: Reused = %d, want 0", stats.Reused)
	}

	var count int
	store.DB().QueryRow("SELECT COUNT(*) FROM semantic_objects").Scan(&count)
	if count != stats.Total {
		t.Errorf("semantic_objects row count = %d, want %d", count, stats.Total)
	}
}

func TestBuildIndex_RecordsMetadata(t *testing.T) {
	store := testutil.NewSeedStore(t)
	emb := newFakeEmbedder()

	if _, err := BuildIndex(store, emb, io.Discard); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	modelID, dims, ok := Available(store)
	if !ok {
		t.Fatal("Available() = false after BuildIndex")
	}
	if modelID != emb.ModelID() {
		t.Errorf("modelID = %q, want %q", modelID, emb.ModelID())
	}
	if dims != emb.Dims() {
		t.Errorf("dims = %d, want %d", dims, emb.Dims())
	}
}

func TestBuildIndex_Incremental_NoOpWhenUnchanged(t *testing.T) {
	store := testutil.NewSeedStore(t)
	emb := newFakeEmbedder()

	if _, err := BuildIndex(store, emb, io.Discard); err != nil {
		t.Fatalf("first BuildIndex: %v", err)
	}
	callsAfterFirst := emb.passageCalls

	stats, err := BuildIndex(store, emb, io.Discard)
	if err != nil {
		t.Fatalf("second BuildIndex: %v", err)
	}
	if stats.Embedded != 0 {
		t.Errorf("second build (unchanged schema): Embedded = %d, want 0", stats.Embedded)
	}
	if emb.passageCalls != callsAfterFirst {
		t.Errorf("EmbedPassages was called again (%d -> %d) despite no schema changes", callsAfterFirst, emb.passageCalls)
	}
}

func TestBuildIndex_DetectsNewColumn(t *testing.T) {
	store := testutil.NewSeedStore(t)
	emb := newFakeEmbedder()

	first, err := BuildIndex(store, emb, io.Discard)
	if err != nil {
		t.Fatalf("first BuildIndex: %v", err)
	}

	// Simulate a schema change: add a new state-like column to orgs.
	var orgsID int64
	store.DB().QueryRow("SELECT id FROM tables WHERE name = 'orgs'").Scan(&orgsID)
	res, err := store.DB().Exec(
		"INSERT INTO columns (table_id, name, type, nullable, position) VALUES (?, 'billing_status', 'text', 0, 99)",
		orgsID,
	)
	if err != nil {
		t.Fatalf("insert column: %v", err)
	}
	colID, _ := res.LastInsertId()
	store.DB().Exec(
		"INSERT INTO field_stats (column_id, distinct_count, null_count, is_state_like, is_categorical) VALUES (?, 3, 0, 1, 1)",
		colID,
	)

	second, err := BuildIndex(store, emb, io.Discard)
	if err != nil {
		t.Fatalf("second BuildIndex: %v", err)
	}
	// +1 distinct candidate identity (the new column object). The orgs
	// table object already existed as an identity — its text changed
	// (new column now listed), so it moves from Reused to Embedded, but
	// Total (distinct candidate count) only grows by the new column.
	if second.Total != first.Total+1 {
		t.Errorf("second build Total = %d, want %d (first Total %d + new column)",
			second.Total, first.Total+1, first.Total)
	}
	if second.Embedded < 2 {
		t.Errorf("expected at least 2 re-embedded objects (new column + changed orgs table text), got %d", second.Embedded)
	}
	if second.Reused == 0 {
		t.Error("expected unrelated objects to still be reused")
	}
}

func TestBuildIndex_RemovesStaleObjects(t *testing.T) {
	store := testutil.NewSeedStore(t)
	emb := newFakeEmbedder()

	if _, err := BuildIndex(store, emb, io.Discard); err != nil {
		t.Fatalf("first BuildIndex: %v", err)
	}

	// Simulate a dropped table: remove 'comments' and everything under it.
	var commentsID int64
	store.DB().QueryRow("SELECT id FROM tables WHERE name = 'comments'").Scan(&commentsID)
	store.DB().Exec("DELETE FROM columns WHERE table_id = ?", commentsID)
	store.DB().Exec("DELETE FROM tables WHERE id = ?", commentsID)

	stats, err := BuildIndex(store, emb, io.Discard)
	if err != nil {
		t.Fatalf("second BuildIndex: %v", err)
	}
	if stats.Removed == 0 {
		t.Error("expected stale semantic objects for the dropped table to be removed")
	}

	var count int
	store.DB().QueryRow("SELECT COUNT(*) FROM semantic_objects so JOIN tables t ON t.id = so.table_id WHERE t.name = 'comments'").Scan(&count)
	if count != 0 {
		t.Errorf("comments still has %d semantic_objects rows after being dropped", count)
	}
}

func TestBuildIndex_ModelMismatchTriggersFullRebuild(t *testing.T) {
	store := testutil.NewSeedStore(t)
	embA := newFakeEmbedder()
	embA.modelID = "model-a/v1"

	first, err := BuildIndex(store, embA, io.Discard)
	if err != nil {
		t.Fatalf("first BuildIndex: %v", err)
	}

	embB := newFakeEmbedder()
	embB.modelID = "model-b/v1"

	second, err := BuildIndex(store, embB, io.Discard)
	if err != nil {
		t.Fatalf("second BuildIndex (different model): %v", err)
	}
	if second.Reused != 0 {
		t.Errorf("model change should force a full rebuild: Reused = %d, want 0", second.Reused)
	}
	if second.Embedded != first.Total {
		t.Errorf("model change should re-embed everything: Embedded = %d, want %d", second.Embedded, first.Total)
	}

	modelID, _, ok := Available(store)
	if !ok || modelID != "model-b/v1" {
		t.Errorf("Available() modelID = %q, ok=%v, want model-b/v1", modelID, ok)
	}
}

func TestAvailable_FalseBeforeBuild(t *testing.T) {
	store := testutil.NewSeedStore(t)
	if err := store.InitSemanticSchema(); err != nil {
		t.Fatalf("InitSemanticSchema: %v", err)
	}
	if _, _, ok := Available(store); ok {
		t.Error("Available() = true before any BuildIndex call")
	}
}
