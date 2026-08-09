package appcore

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HexmosTech/git-lrc/internal/syncqueue"
	"github.com/HexmosTech/git-lrc/storage"
)

func openTestSyncQueueDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := storage.ExecSQL(db, `
		CREATE TABLE IF NOT EXISTS sync_queue (
		    id              INTEGER PRIMARY KEY AUTOINCREMENT,
		    repo_path       TEXT NOT NULL,
		    remote_url      TEXT,
		    branch          TEXT,
		    commit_sha      TEXT NOT NULL,
		    tree_hash       TEXT NOT NULL,
		    review_id       TEXT NOT NULL,
		    api_url         TEXT NOT NULL,
		    api_key         TEXT NOT NULL,
		    status          TEXT NOT NULL DEFAULT 'pending',
		    attempts        INTEGER NOT NULL DEFAULT 0,
		    last_attempt_at TEXT,
		    next_attempt_at TEXT,
		    last_error      TEXT,
		    created_at      TEXT NOT NULL,
		    synced_at       TEXT,
		    UNIQUE(review_id, commit_sha)
		);`); err != nil {
		t.Fatalf("failed to init schema: %v", err)
	}
	return db
}

func TestFlushSyncQueueDB_SuccessMarksSynced(t *testing.T) {
	var gotPath, gotAPIKey string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("X-API-Key")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"recorded"}`))
	}))
	defer ts.Close()

	db := openTestSyncQueueDB(t)
	if err := syncqueue.Enqueue(db, syncqueue.EnqueueInput{
		RepoPath: "/repo", CommitSHA: "sha1", TreeHash: "tree1",
		ReviewID: "review-1", APIURL: ts.URL, APIKey: "the-key",
	}); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	if err := flushSyncQueueDB(db, true); err != nil {
		t.Fatalf("flushSyncQueueDB failed: %v", err)
	}

	if gotPath != "/api/v1/diff-review/review-1/commit" {
		t.Errorf("unexpected request path: %s", gotPath)
	}
	if gotAPIKey != "the-key" {
		t.Errorf("unexpected X-API-Key: %s", gotAPIKey)
	}

	items, err := syncqueue.List(db, syncqueue.StatusSynced)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 synced item, got %d", len(items))
	}
}

func TestFlushSyncQueueDB_PermanentFailureStopsRetrying(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer ts.Close()

	db := openTestSyncQueueDB(t)
	if err := syncqueue.Enqueue(db, syncqueue.EnqueueInput{
		RepoPath: "/repo", CommitSHA: "sha1", TreeHash: "tree1",
		ReviewID: "review-1", APIURL: ts.URL, APIKey: "stale-key",
	}); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	if err := flushSyncQueueDB(db, true); err != nil {
		t.Fatalf("flushSyncQueueDB failed: %v", err)
	}

	failed, err := syncqueue.List(db, syncqueue.StatusFailed)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(failed) != 1 {
		t.Fatalf("expected 1 permanently-failed item, got %d", len(failed))
	}
	if failed[0].NextAttemptAt != nil {
		t.Error("a permanently-failed item must not be scheduled for retry")
	}

	// Even much later, it must never come up as "due" again.
	due, err := syncqueue.Due(db, time.Now().Add(365*24*time.Hour))
	if err != nil {
		t.Fatalf("Due failed: %v", err)
	}
	if len(due) != 0 {
		t.Error("a 401 response must permanently stop retries")
	}
}

func TestFlushSyncQueueDB_TransientFailureBacksOff(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"boom"}`))
	}))
	defer ts.Close()

	db := openTestSyncQueueDB(t)
	if err := syncqueue.Enqueue(db, syncqueue.EnqueueInput{
		RepoPath: "/repo", CommitSHA: "sha1", TreeHash: "tree1",
		ReviewID: "review-1", APIURL: ts.URL, APIKey: "the-key",
	}); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	if err := flushSyncQueueDB(db, true); err != nil {
		t.Fatalf("flushSyncQueueDB failed: %v", err)
	}

	pending, err := syncqueue.List(db, syncqueue.StatusPending)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected item to remain pending after a 500, got %d", len(pending))
	}
	if pending[0].Attempts != 1 {
		t.Errorf("expected attempts=1, got %d", pending[0].Attempts)
	}
	if pending[0].NextAttemptAt == nil {
		t.Error("expected a scheduled retry after a transient failure")
	}
}

func TestFlushSyncQueueDB_MultipleItemsIndependentOutcomes(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/diff-review/review-ok/commit":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/diff-review/review-bad/commit":
			w.WriteHeader(http.StatusForbidden)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	db := openTestSyncQueueDB(t)
	if err := syncqueue.Enqueue(db, syncqueue.EnqueueInput{
		RepoPath: "/repo", CommitSHA: "sha1", TreeHash: "tree1",
		ReviewID: "review-ok", APIURL: ts.URL, APIKey: "k",
	}); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if err := syncqueue.Enqueue(db, syncqueue.EnqueueInput{
		RepoPath: "/repo", CommitSHA: "sha2", TreeHash: "tree2",
		ReviewID: "review-bad", APIURL: ts.URL, APIKey: "k",
	}); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	if err := flushSyncQueueDB(db, true); err != nil {
		t.Fatalf("flushSyncQueueDB failed: %v", err)
	}

	synced, _ := syncqueue.List(db, syncqueue.StatusSynced)
	failed, _ := syncqueue.List(db, syncqueue.StatusFailed)
	if len(synced) != 1 || synced[0].ReviewID != "review-ok" {
		t.Errorf("expected review-ok to be synced, got %+v", synced)
	}
	if len(failed) != 1 || failed[0].ReviewID != "review-bad" {
		t.Errorf("expected review-bad to be permanently failed, got %+v", failed)
	}
}
