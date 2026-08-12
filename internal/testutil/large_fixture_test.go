package testutil

import "testing"

func TestNewLargeStore_BasicShape(t *testing.T) {
	store := NewLargeStore(t, 50)

	var tableCount int
	store.DB().QueryRow("SELECT COUNT(*) FROM tables").Scan(&tableCount)
	if tableCount != 50 {
		t.Errorf("table count = %d, want 50", tableCount)
	}

	var fkCount int
	store.DB().QueryRow("SELECT COUNT(*) FROM foreign_keys").Scan(&fkCount)
	if fkCount == 0 {
		t.Error("expected foreign keys to be generated")
	}

	var stateCount int
	store.DB().QueryRow("SELECT COUNT(*) FROM field_stats WHERE is_state_like = 1").Scan(&stateCount)
	if stateCount != 50 {
		t.Errorf("state-like column count = %d, want 50 (one per table)", stateCount)
	}

	var jsonbCount int
	store.DB().QueryRow("SELECT COUNT(*) FROM jsonb_paths").Scan(&jsonbCount)
	if jsonbCount == 0 {
		t.Error("expected some jsonb_paths to be generated")
	}

	var ftsCount int
	store.DB().QueryRow("SELECT COUNT(*) FROM search_index").Scan(&ftsCount)
	if ftsCount != 50 {
		t.Errorf("search_index row count = %d, want 50", ftsCount)
	}
}
