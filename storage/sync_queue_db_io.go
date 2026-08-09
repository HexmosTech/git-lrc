package storage

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// SyncQueueColumns is the canonical column list/order for sync_queue rows,
// shared by every SELECT so callers can scan with a fixed set of dest vars.
const SyncQueueColumns = `id, repo_path, remote_url, branch, commit_sha, tree_hash, review_id, api_url, api_key,
	status, attempts, last_attempt_at, next_attempt_at, last_error, created_at, synced_at`

// formatTimestamp is the single place sync_queue timestamps are serialized,
// so every column round-trips through the exact same format.
func formatTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// parseTimestamp is the single place sync_queue timestamps are parsed.
// Errors are returned to the caller (never silently swallowed) -- a value
// that fails to parse here means data this package itself wrote is
// corrupt, which is always worth surfacing.
func parseTimestamp(raw string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse timestamp %q: %w", raw, err)
	}
	return t, nil
}

// nullStringToNullTime converts a scanned nullable TEXT timestamp to
// sql.NullTime. A non-NULL value that fails to parse is logged (it means
// this package's own previously-written data is corrupt) and degrades to
// NULL rather than failing the whole read -- callers like `sync status`
// should stay usable even if one row has a malformed timestamp.
func nullStringToNullTime(v sql.NullString) sql.NullTime {
	if !v.Valid || v.String == "" {
		return sql.NullTime{}
	}
	t, err := parseTimestamp(v.String)
	if err != nil {
		log.Printf("storage: sync_queue: %v (treating as unset)", err)
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

// InsertSyncQueueItem inserts a new pending sync_queue row. Idempotent via
// ON CONFLICT(review_id, commit_sha) DO NOTHING.
func InsertSyncQueueItem(db *sql.DB, repoPath, remoteURL, branch, commitSHA, treeHash, reviewID, apiURL, apiKey string, createdAt time.Time) error {
	if db == nil {
		return fmt.Errorf("failed to insert sync queue item: nil database handle")
	}
	_, err := ExecSQL(db, `
		INSERT INTO sync_queue (repo_path, remote_url, branch, commit_sha, tree_hash, review_id, api_url, api_key, status, attempts, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', 0, ?)
		ON CONFLICT(review_id, commit_sha) DO NOTHING`,
		repoPath, remoteURL, branch, commitSHA, treeHash, reviewID, apiURL, apiKey, formatTimestamp(createdAt),
	)
	if err != nil {
		return fmt.Errorf("failed to insert sync queue item: %w", err)
	}
	return nil
}

// QuerySyncQueueDue returns pending rows due for a retry (never attempted,
// or whose next_attempt_at has passed), oldest first. Caller must close.
func QuerySyncQueueDue(db *sql.DB, status string, now time.Time) (*sql.Rows, error) {
	if db == nil {
		return nil, fmt.Errorf("failed to query due sync queue items: nil database handle")
	}
	rows, err := db.Query(
		`SELECT `+SyncQueueColumns+` FROM sync_queue
		 WHERE status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
		 ORDER BY id ASC`,
		status, formatTimestamp(now),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query due sync queue items: %w", err)
	}
	return rows, nil
}

// QuerySyncQueueCountDue is a cheap count of QuerySyncQueueDue's result set.
func QuerySyncQueueCountDue(db *sql.DB, status string, now time.Time) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("failed to count due sync queue items: nil database handle")
	}
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sync_queue WHERE status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)`,
		status, formatTimestamp(now),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count due sync queue items: %w", err)
	}
	return count, nil
}

// QuerySyncQueueList returns sync_queue rows, optionally filtered by status
// (empty string = all), newest first. Caller must close.
func QuerySyncQueueList(db *sql.DB, status string) (*sql.Rows, error) {
	if db == nil {
		return nil, fmt.Errorf("failed to list sync queue items: nil database handle")
	}
	query := `SELECT ` + SyncQueueColumns + ` FROM sync_queue`
	var args []any
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY id DESC`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list sync queue items: %w", err)
	}
	return rows, nil
}

// UpdateSyncQueueSynced marks an item successfully synced.
func UpdateSyncQueueSynced(db *sql.DB, id int64, ts time.Time) error {
	if db == nil {
		return fmt.Errorf("failed to mark sync queue item synced: nil database handle")
	}
	tsStr := formatTimestamp(ts)
	_, err := ExecSQL(db,
		`UPDATE sync_queue SET status = 'synced', synced_at = ?, last_attempt_at = ?, last_error = NULL WHERE id = ?`,
		tsStr, tsStr, id,
	)
	if err != nil {
		return fmt.Errorf("failed to mark sync queue item %d synced: %w", id, err)
	}
	return nil
}

// QuerySyncQueueAttemptsAndCreatedAt reads the fields needed to decide a
// failure's backoff/give-up outcome. createdAtKnown=false (with err=nil)
// means the row's created_at failed to parse (should never happen -- this
// package always writes RFC3339 itself); callers should treat that as
// "age unknown" rather than aborting the whole failure-recording flow.
func QuerySyncQueueAttemptsAndCreatedAt(db *sql.DB, id int64) (attempts int, createdAt time.Time, createdAtKnown bool, err error) {
	if db == nil {
		return 0, time.Time{}, false, fmt.Errorf("failed to read sync queue item: nil database handle")
	}
	var createdAtStr string
	if scanErr := db.QueryRow(`SELECT attempts, created_at FROM sync_queue WHERE id = ?`, id).Scan(&attempts, &createdAtStr); scanErr != nil {
		return 0, time.Time{}, false, fmt.Errorf("failed to read sync queue item %d: %w", id, scanErr)
	}
	parsed, parseErr := parseTimestamp(createdAtStr)
	if parseErr != nil {
		log.Printf("storage: sync_queue: item %d: %v (treating age as unknown)", id, parseErr)
		return attempts, time.Time{}, false, nil
	}
	return attempts, parsed, true, nil
}

// UpdateSyncQueueGiveUp marks an item permanently failed: no next_attempt_at,
// never retried again.
func UpdateSyncQueueGiveUp(db *sql.DB, id int64, ts time.Time, errMsg string) error {
	if db == nil {
		return fmt.Errorf("failed to mark sync queue item failed: nil database handle")
	}
	_, err := ExecSQL(db,
		`UPDATE sync_queue SET status = 'failed', attempts = attempts + 1, last_attempt_at = ?, last_error = ?, next_attempt_at = NULL WHERE id = ?`,
		formatTimestamp(ts), errMsg, id,
	)
	if err != nil {
		return fmt.Errorf("failed to mark sync queue item %d failed: %w", id, err)
	}
	return nil
}

// UpdateSyncQueueRetryScheduled records a transient failure and schedules
// the next retry attempt; the item's status stays 'pending'.
func UpdateSyncQueueRetryScheduled(db *sql.DB, id int64, ts, nextAttemptAt time.Time, errMsg string) error {
	if db == nil {
		return fmt.Errorf("failed to schedule sync queue retry: nil database handle")
	}
	_, err := ExecSQL(db,
		`UPDATE sync_queue SET attempts = attempts + 1, last_attempt_at = ?, last_error = ?, next_attempt_at = ? WHERE id = ?`,
		formatTimestamp(ts), errMsg, formatTimestamp(nextAttemptAt), id,
	)
	if err != nil {
		return fmt.Errorf("failed to schedule retry for sync queue item %d: %w", id, err)
	}
	return nil
}

// DeleteSyncQueueItem permanently removes one item.
func DeleteSyncQueueItem(db *sql.DB, id int64) error {
	if db == nil {
		return fmt.Errorf("failed to delete sync queue item: nil database handle")
	}
	if _, err := ExecSQL(db, `DELETE FROM sync_queue WHERE id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete sync queue item %d: %w", id, err)
	}
	return nil
}

// QuerySyncQueueStatusCounts returns one (status, count) row per status
// present in the table. Caller must close.
func QuerySyncQueueStatusCounts(db *sql.DB) (*sql.Rows, error) {
	if db == nil {
		return nil, fmt.Errorf("failed to compute sync queue status counts: nil database handle")
	}
	rows, err := db.Query(`SELECT status, COUNT(*) FROM sync_queue GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("failed to compute sync queue status counts: %w", err)
	}
	return rows, nil
}

// QuerySyncQueueOldestPendingCreatedAt returns MIN(created_at) among rows
// with the given status (pending), or an invalid/NULL result if none (or
// if that value fails to parse -- logged, never silently misreported).
func QuerySyncQueueOldestPendingCreatedAt(db *sql.DB, status string) (sql.NullTime, error) {
	if db == nil {
		return sql.NullTime{}, fmt.Errorf("failed to compute oldest pending sync queue item: nil database handle")
	}
	var v sql.NullString
	if err := db.QueryRow(`SELECT MIN(created_at) FROM sync_queue WHERE status = ?`, status).Scan(&v); err != nil {
		return sql.NullTime{}, fmt.Errorf("failed to compute oldest pending sync queue item: %w", err)
	}
	return nullStringToNullTime(v), nil
}

// QuerySyncQueueLastAttemptAt returns MAX(last_attempt_at) across the whole
// table, or an invalid/NULL result if no attempt has ever been made (or if
// that value fails to parse -- logged, never silently misreported).
func QuerySyncQueueLastAttemptAt(db *sql.DB) (sql.NullTime, error) {
	if db == nil {
		return sql.NullTime{}, fmt.Errorf("failed to compute last sync queue attempt time: nil database handle")
	}
	var v sql.NullString
	if err := db.QueryRow(`SELECT MAX(last_attempt_at) FROM sync_queue`).Scan(&v); err != nil {
		return sql.NullTime{}, fmt.Errorf("failed to compute last sync queue attempt time: %w", err)
	}
	return nullStringToNullTime(v), nil
}
