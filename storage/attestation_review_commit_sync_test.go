package storage

import (
	"database/sql"
	"testing"
)

// legacySchemaV1 is the pre-commit-sync review_sessions schema (no
// api_url/api_key columns), used to prove the migration path works against
// a database created before those columns existed.
const legacySchemaV1 = `
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

func openLegacyTestReviewDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := InitializeAttestationReviewSchema(db, legacySchemaV1); err != nil {
		t.Fatalf("failed to init legacy schema: %v", err)
	}
	return db
}

func TestHasColumn_RejectsInvalidIdentifiers(t *testing.T) {
	db := openLegacyTestReviewDB(t)

	cases := []struct{ table, column string }{
		{"review_sessions; DROP TABLE review_sessions;--", "api_url"},
		{"review_sessions", "api_url; DROP TABLE review_sessions;--"},
		{"review sessions", "api_url"}, // space
		{"", "api_url"},
		{"review_sessions", ""},
	}
	for _, tc := range cases {
		if _, err := hasColumn(db, tc.table, tc.column); err == nil {
			t.Errorf("hasColumn(%q, %q) should have been rejected as an invalid identifier", tc.table, tc.column)
		}
	}

	// Sanity: the legitimate call still works after the guard is in place.
	if _, err := hasColumn(db, "review_sessions", "review_id"); err != nil {
		t.Errorf("hasColumn with valid identifiers should succeed, got: %v", err)
	}
}

func TestEnsureReviewSessionsCommitSyncColumns_MigratesLegacySchema(t *testing.T) {
	db := openLegacyTestReviewDB(t)

	for _, col := range []string{"api_url", "api_key"} {
		exists, err := hasColumn(db, "review_sessions", col)
		if err != nil {
			t.Fatalf("hasColumn(%s) failed: %v", col, err)
		}
		if !exists {
			t.Errorf("expected column %s to exist after migration", col)
		}
	}

	// Calling it again (as InitializeAttestationReviewSchema always does)
	// must be a no-op, not an error.
	if err := EnsureReviewSessionsCommitSyncColumns(db); err != nil {
		t.Fatalf("second migration call should be a no-op, got error: %v", err)
	}
}

func TestInsertAndQuerySyncCandidate(t *testing.T) {
	db := openLegacyTestReviewDB(t)

	// "skipped" never carries a review_id/api_url/api_key -- must not be a candidate.
	if err := InsertAttestationReviewSessionRow(db, "tree-skip", "main", "skipped", "2026-03-17T00:00:00Z", "[]", "", "", ""); err != nil {
		t.Fatalf("insert skipped row failed: %v", err)
	}
	// "reviewed" with real credentials -- must be a candidate.
	if err := InsertAttestationReviewSessionRow(db, "tree-reviewed", "main", "reviewed", "2026-03-17T00:00:01Z", "[]", "rid-1", "https://api.example.com", "key-1"); err != nil {
		t.Fatalf("insert reviewed row failed: %v", err)
	}
	// A second, later session for the SAME tree -- the query should return this one (most recent).
	if err := InsertAttestationReviewSessionRow(db, "tree-reviewed", "main", "vouched", "2026-03-17T00:00:02Z", "[]", "rid-2", "https://api.example.com", "key-2"); err != nil {
		t.Fatalf("insert vouched row failed: %v", err)
	}

	t.Run("no candidate for a skipped tree", func(t *testing.T) {
		_, _, _, _, _, _, found, err := QueryAttestationSyncCandidateForTreeHash(db, "tree-skip")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if found {
			t.Error("expected no sync candidate for a skipped tree")
		}
	})

	t.Run("no candidate for an unknown tree", func(t *testing.T) {
		_, _, _, _, _, _, found, err := QueryAttestationSyncCandidateForTreeHash(db, "does-not-exist")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if found {
			t.Error("expected no sync candidate for an unknown tree")
		}
	})

	t.Run("returns the most recent syncable session", func(t *testing.T) {
		_, branch, action, reviewID, apiURL, apiKey, found, err := QueryAttestationSyncCandidateForTreeHash(db, "tree-reviewed")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if !found {
			t.Fatal("expected a sync candidate")
		}
		if branch != "main" || action != "vouched" || reviewID != "rid-2" || apiURL != "https://api.example.com" || apiKey != "key-2" {
			t.Errorf("unexpected candidate: branch=%q action=%q reviewID=%q apiURL=%q apiKey=%q", branch, action, reviewID, apiURL, apiKey)
		}
	})
}
