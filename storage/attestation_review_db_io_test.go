package storage

import (
	"database/sql"
	"testing"
)

func openTestReviewDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := `
-- schema_version:1
CREATE TABLE IF NOT EXISTS review_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tree_hash TEXT NOT NULL,
    branch TEXT NOT NULL,
    action TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    diff_files TEXT,
    review_id TEXT
);`
	if err := InitializeAttestationReviewSchema(db, schema); err != nil {
		t.Fatalf("failed to init schema: %v", err)
	}
	return db
}

func seedReviewSession(t *testing.T, db *sql.DB, branch string) {
	t.Helper()
	err := InsertAttestationReviewSessionRow(db, "tree", branch, "reviewed", "2026-03-17T00:00:00Z", "[]", "rid", "https://api.example.com", "test-key")
	if err != nil {
		t.Fatalf("failed to insert row: %v", err)
	}
}

func TestDeleteBranchSessionsDryRunDoesNotDelete(t *testing.T) {
	db := openTestReviewDB(t)
	seedReviewSession(t, db, "main")

	count, err := DeleteAttestationReviewSessionsByBranchWithOptions(db, "main", DeleteBranchSessionsOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run delete failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("unexpected dry-run count: got %d want 1", count)
	}

	remaining, err := QueryAttestationReviewSessionCountByBranch(db, "main")
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("dry-run mutated table: remaining=%d", remaining)
	}
}

func TestDeleteAllSessionsConfirmationGate(t *testing.T) {
	db := openTestReviewDB(t)
	seedReviewSession(t, db, "main")

	if _, err := DeleteAllAttestationReviewSessionsWithOptions(db, DeleteAllSessionsOptions{RequireConfirmation: true, Confirmed: false}); err == nil {
		t.Fatalf("expected confirmation-gate error")
	}

	affected, err := DeleteAllAttestationReviewSessionsWithOptions(db, DeleteAllSessionsOptions{RequireConfirmation: true, Confirmed: true})
	if err != nil {
		t.Fatalf("confirmed delete failed: %v", err)
	}
	if affected != 1 {
		t.Fatalf("unexpected affected rows: got %d want 1", affected)
	}
}

func TestDeleteAllSessionsLegacyAPIStillWorks(t *testing.T) {
	db := openTestReviewDB(t)
	seedReviewSession(t, db, "main")

	affected, err := DeleteAllAttestationReviewSessions(db)
	if err != nil {
		t.Fatalf("legacy delete failed: %v", err)
	}
	if affected != 1 {
		t.Fatalf("unexpected affected rows: got %d want 1", affected)
	}
}
