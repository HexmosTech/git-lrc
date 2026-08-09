// Package syncqueue is the offline-first, user-global queue that bridges
// "lrc review ran, and its tree was later committed" to LiveReview's
// backend. It lives at ~/.lrc/sync-queue.db (see configpath.
// ResolveLRCDataDir) -- one queue across every repo the user touches,
// because credentials/org are global (~/.lrc.toml), not per-repo.
//
// Each item snapshots the api_url/api_key that actually submitted the
// review (captured at review-submission time -- see attestation.
// InsertReviewSession), never re-reading current config, so a retry days
// later always targets the account the review really belongs to.
//
// All actual SQL lives in the storage package (this repo's storage/network
// boundary convention, enforced by internal/architecture's
// TestStorageBoundaryEnforcement) -- this package is business logic on top
// of storage's sync-queue query functions.
package syncqueue

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/configpath"
	"github.com/HexmosTech/git-lrc/storage"
)

const dbFileName = "sync-queue.db"

const schema = `
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
);
CREATE INDEX IF NOT EXISTS idx_sync_queue_status_next ON sync_queue(status, next_attempt_at);
`

// Status values for Item.Status.
const (
	StatusPending = "pending"
	StatusSynced  = "synced"
	StatusFailed  = "failed"
)

// Item is one queued (or resolved) commit-sync entry.
type Item struct {
	ID            int64
	RepoPath      string
	RemoteURL     string
	Branch        string
	CommitSHA     string
	TreeHash      string
	ReviewID      string
	APIURL        string
	APIKey        string
	Status        string
	Attempts      int
	LastAttemptAt *time.Time
	NextAttemptAt *time.Time
	LastError     string
	CreatedAt     time.Time
	SyncedAt      *time.Time
}

// EnqueueInput is what a caller (git-lrc's post-commit hook path) knows
// right after a commit is made.
type EnqueueInput struct {
	RepoPath  string
	RemoteURL string
	Branch    string
	CommitSHA string
	TreeHash  string
	ReviewID  string
	APIURL    string
	APIKey    string
}

// Path returns the canonical ~/.lrc/sync-queue.db path.
func Path() (string, error) {
	dataDir, err := configpath.ResolveLRCDataDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve ~/.lrc directory: %w", err)
	}
	return filepath.Join(dataDir, dbFileName), nil
}

// Open opens (creating if needed) the global sync queue database, ensuring
// the schema exists. The file is created 0600 -- it holds a plaintext API
// key, the same trust model ~/.lrc.toml already uses.
func Open() (*sql.DB, error) {
	dataDir, err := configpath.ResolveLRCDataDir()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve ~/.lrc directory: %w", err)
	}
	if err := storage.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(dataDir, dbFileName)
	db, err := storage.OpenSQLite(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sync queue database: %w", err)
	}
	if _, err := storage.ExecSQL(db, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize sync queue schema: %w", err)
	}
	// Best-effort: not fatal (some filesystems don't support Unix
	// permissions), but this file holds a plaintext API key, so a failure
	// here is worth surfacing rather than swallowing outright -- it means
	// the "matches ~/.lrc.toml's trust model" assumption above may not
	// actually hold for this file.
	if err := storage.Chmod(dbPath, 0600); err != nil {
		log.Printf("syncqueue: warning: failed to set permissions on %s: %v", dbPath, err)
	}
	return db, nil
}

// Enqueue inserts a new pending item. Idempotent: enqueueing the same
// (review_id, commit_sha) pair again is a harmless no-op.
func Enqueue(db *sql.DB, in EnqueueInput) error {
	return storage.InsertSyncQueueItem(db,
		in.RepoPath, in.RemoteURL, in.Branch, in.CommitSHA, in.TreeHash, in.ReviewID, in.APIURL, in.APIKey,
		time.Now(),
	)
}

func scanItem(scan func(dest ...any) error) (Item, error) {
	var it Item
	var remoteURL, branch, lastError sql.NullString
	var lastAttemptAt, nextAttemptAt, createdAt, syncedAt sql.NullString

	if err := scan(
		&it.ID, &it.RepoPath, &remoteURL, &branch, &it.CommitSHA, &it.TreeHash, &it.ReviewID, &it.APIURL, &it.APIKey,
		&it.Status, &it.Attempts, &lastAttemptAt, &nextAttemptAt, &lastError, &createdAt, &syncedAt,
	); err != nil {
		return Item{}, err
	}

	it.RemoteURL = remoteURL.String
	it.Branch = branch.String
	it.LastError = lastError.String
	it.LastAttemptAt = parseNullTime(lastAttemptAt)
	it.NextAttemptAt = parseNullTime(nextAttemptAt)
	it.SyncedAt = parseNullTime(syncedAt)
	if t := parseNullTime(createdAt); t != nil {
		it.CreatedAt = *t
	}
	return it, nil
}

// parseNullTime parses a nullable RFC3339 column scanned from a full sync_queue
// row (see scanItem). A non-NULL value that fails to parse is logged -- it
// means this package's own previously-written data is corrupt -- and
// degrades to nil rather than failing the whole row.
func parseNullTime(v sql.NullString) *time.Time {
	if !v.Valid || v.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, v.String)
	if err != nil {
		log.Printf("syncqueue: failed to parse timestamp %q: %v (treating as unset)", v.String, err)
		return nil
	}
	return &t
}

func scanItems(rows *sql.Rows) ([]Item, error) {
	defer rows.Close()
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to scan sync queue item: %w", err)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// Due returns pending items whose next retry is due (or that have never
// been attempted), oldest first.
func Due(db *sql.DB, now time.Time) ([]Item, error) {
	rows, err := storage.QuerySyncQueueDue(db, StatusPending, now)
	if err != nil {
		return nil, err
	}
	return scanItems(rows)
}

// CountDue is a cheap (indexed) count of pending, due items -- used by the
// opportunistic per-invocation trigger to decide whether it's worth
// spawning a flush worker at all.
func CountDue(db *sql.DB, now time.Time) (int, error) {
	return storage.QuerySyncQueueCountDue(db, StatusPending, now)
}

// MarkSynced marks an item as successfully synced.
func MarkSynced(db *sql.DB, id int64, now time.Time) error {
	return storage.UpdateSyncQueueSynced(db, id, now)
}

// maxLastErrorLen caps how much of an error message gets persisted (and
// later printed by `lrc sync list`) -- a pathological response body
// shouldn't be able to bloat the queue database indefinitely.
const maxLastErrorLen = 2000

// sanitizeErrMsg truncates errMsg and strips any occurrence of sensitive
// (secret) substrings before it's ever written to disk or printed --
// defense in depth in case a proxy/gateway ever echoes a request's
// X-API-Key value back in an error response body.
func sanitizeErrMsg(errMsg string, secrets ...string) string {
	for _, s := range secrets {
		if s == "" {
			continue
		}
		errMsg = strings.ReplaceAll(errMsg, s, "[REDACTED]")
	}
	if len(errMsg) > maxLastErrorLen {
		errMsg = errMsg[:maxLastErrorLen] + "... [truncated]"
	}
	return errMsg
}

// RecordFailure records a failed sync attempt for item id. errMsg is
// sanitized (see sanitizeErrMsg) against secrets before being persisted.
//
// permanent=true (auth/ownership rejected -- 401/403/404 from the backend)
// stops retrying immediately: retrying with the same cached credentials
// against the same review_id would only ever fail the same way again.
//
// permanent=false (network error, timeout, 5xx) schedules a backoff retry,
// unless the item has been pending longer than MaxPendingAge, in which case
// it's given up on too (still visible via List/Stats, just not retried).
func RecordFailure(db *sql.DB, id int64, permanent bool, errMsg string, now time.Time, secrets ...string) error {
	errMsg = sanitizeErrMsg(errMsg, secrets...)

	attempts, createdAt, createdAtKnown, err := storage.QuerySyncQueueAttemptsAndCreatedAt(db, id)
	if err != nil {
		return err
	}

	// createdAtKnown=false means the row's created_at didn't parse (already
	// logged by the storage layer) -- treat age as unknown rather than
	// aborting the failure recording, so backoff/attempts tracking still
	// works even for that anomalous row.
	giveUp := permanent || (createdAtKnown && now.Sub(createdAt) > MaxPendingAge)

	if giveUp {
		return storage.UpdateSyncQueueGiveUp(db, id, now, errMsg)
	}

	nextAttempt := now.Add(NextBackoff(attempts))
	return storage.UpdateSyncQueueRetryScheduled(db, id, now, nextAttempt, errMsg)
}

// List returns items, optionally filtered by status ("" = all), newest first.
func List(db *sql.DB, status string) ([]Item, error) {
	rows, err := storage.QuerySyncQueueList(db, status)
	if err != nil {
		return nil, err
	}
	return scanItems(rows)
}

// Forget permanently removes one item (manual "give up on this" escape hatch).
func Forget(db *sql.DB, id int64) error {
	return storage.DeleteSyncQueueItem(db, id)
}

// Stats summarizes queue state for `lrc sync status`.
type Stats struct {
	Pending       int
	Synced        int
	Failed        int
	OldestPending *time.Time
	LastAttemptAt *time.Time
}

// GetStats computes counts per status and the oldest still-pending item's
// created_at (to report its age), plus the most recent attempt across the
// whole queue (to report "last flush attempt").
func GetStats(db *sql.DB) (Stats, error) {
	var s Stats

	rows, err := storage.QuerySyncQueueStatusCounts(db)
	if err != nil {
		return s, err
	}
	func() {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int
			if scanErr := rows.Scan(&status, &count); scanErr != nil {
				err = fmt.Errorf("failed to scan sync queue status counts: %w", scanErr)
				return
			}
			switch status {
			case StatusPending:
				s.Pending = count
			case StatusSynced:
				s.Synced = count
			case StatusFailed:
				s.Failed = count
			}
		}
		if scanErr := rows.Err(); scanErr != nil {
			err = scanErr
		}
	}()
	if err != nil {
		return s, err
	}

	oldestPending, err := storage.QuerySyncQueueOldestPendingCreatedAt(db, StatusPending)
	if err != nil {
		return s, err
	}
	if oldestPending.Valid {
		t := oldestPending.Time
		s.OldestPending = &t
	}

	lastAttempt, err := storage.QuerySyncQueueLastAttemptAt(db)
	if err != nil {
		return s, err
	}
	if lastAttempt.Valid {
		t := lastAttempt.Time
		s.LastAttemptAt = &t
	}

	return s, nil
}
