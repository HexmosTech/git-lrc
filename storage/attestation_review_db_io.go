package storage

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	_ "modernc.org/sqlite"
)

// sqlIdentifierPattern is what hasColumn/EnsureReviewSessionsCommitSyncColumns
// require of a table/column name before it's allowed anywhere near a query
// string. SQLite has no way to bind identifiers (table/column names) as
// query parameters -- PRAGMA and DDL statements only accept them literally
// -- so this allowlist is the actual injection defense, not the ? bind
// parameters used everywhere else in this file. Both current call sites
// only ever pass hardcoded literals ("review_sessions", "api_url",
// "api_key"), but this makes that a hard invariant rather than an
// assumption that could quietly stop holding if either function is ever
// called with a computed value.
var sqlIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateSQLIdentifier(kind, name string) error {
	if !sqlIdentifierPattern.MatchString(name) {
		return fmt.Errorf("invalid %s name %q: must match %s", kind, name, sqlIdentifierPattern.String())
	}
	return nil
}

const reviewSchemaVersionMarker = "schema_version:1"

// DeleteBranchSessionsOptions controls optional safety behaviors for branch deletes.
// Zero-value options preserve existing behavior.
type DeleteBranchSessionsOptions struct {
	DryRun bool
	Logf   func(format string, args ...any)
}

// DeleteAllSessionsOptions controls optional safety behaviors for full-table deletes.
// Zero-value options preserve existing behavior.
type DeleteAllSessionsOptions struct {
	RequireConfirmation bool
	Confirmed           bool
	Logf                func(format string, args ...any)
}

// EnsureReviewDBDir creates the .git/lrc directory that stores reviews.db.
func EnsureReviewDBDir(lrcDir string) error {
	if err := MkdirAll(lrcDir, 0755); err != nil {
		return fmt.Errorf("failed to create review database directory %s: %w", lrcDir, err)
	}
	return nil
}

// OpenAttestationReviewDB opens the attestation review SQLite database with WAL and busy timeout.
func OpenAttestationReviewDB(dbPath string) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=%d", dbPath, sqliteBusyTimeoutMS())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open review sqlite database %s: %w", dbPath, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to connect review sqlite database %s: %w", dbPath, err)
	}
	return db, nil
}

// InitializeAttestationReviewSchema executes schema SQL for the review sessions table.
func InitializeAttestationReviewSchema(db *sql.DB, schema string) error {
	if db == nil {
		return fmt.Errorf("failed to initialize review schema: nil database handle")
	}
	if _, err := ExecSQL(db, schema); err != nil {
		return fmt.Errorf("failed to initialize review schema (%s): %w", compactSQL(schema), err)
	}
	// Optional schema marker check to keep schema evolution auditable without breaking existing schemas.
	if !strings.Contains(strings.ToLower(schema), reviewSchemaVersionMarker) {
		// Intentionally non-fatal for backward compatibility.
	}
	// review_sessions predates the api_url/api_key columns; `CREATE TABLE IF
	// NOT EXISTS` above is a no-op against an already-existing table, so an
	// explicit column migration is needed for databases created before
	// schema_version:2.
	if err := EnsureReviewSessionsCommitSyncColumns(db); err != nil {
		return fmt.Errorf("failed to migrate review schema: %w", err)
	}
	return nil
}

// InsertAttestationReviewSessionRow inserts a review session row for coverage
// tracking. apiURL/apiKey are the credentials that actually submitted this
// review, snapshotted here (not re-read from ~/.lrc.toml later) so the
// offline commit-sync queue always targets the account a review really
// belongs to, even if global config changes before the resulting commit's
// post-commit hook fires. Empty for "skipped" (no review was submitted).
func InsertAttestationReviewSessionRow(db *sql.DB, treeHash, branch, action, timestamp, diffFilesJSON, reviewID, apiURL, apiKey string) error {
	if db == nil {
		return fmt.Errorf("failed to insert review session: nil database handle")
	}

	const insertSQL = `INSERT INTO review_sessions (tree_hash, branch, action, timestamp, diff_files, review_id, api_url, api_key)
	 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	if _, err := ExecSQL(db, insertSQL, treeHash, branch, action, timestamp, diffFilesJSON, reviewID, apiURL, apiKey); err != nil {
		return fmt.Errorf("failed to insert review session row: %w", err)
	}
	return nil
}

// hasColumn reports whether table has a column named column. table/column
// are validated against sqlIdentifierPattern before being formatted into
// the query -- SQLite's PRAGMA statements can't bind identifiers as query
// parameters, so this validation is the actual defense against injection.
func hasColumn(db *sql.DB, table, column string) (bool, error) {
	if err := validateSQLIdentifier("table", table); err != nil {
		return false, err
	}
	if err := validateSQLIdentifier("column", column); err != nil {
		return false, err
	}
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("failed to inspect table %s: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notNull, pk int
		var name, ctype string
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			return false, fmt.Errorf("failed to scan table_info row for %s: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// EnsureReviewSessionsCommitSyncColumns adds the api_url/api_key columns
// (used by the offline commit-sync queue) to an existing review_sessions
// table if they aren't already there. SQLite has no `ADD COLUMN IF NOT
// EXISTS`, so this checks via PRAGMA table_info first rather than relying
// on driver-specific "duplicate column" error text — safe to call on every
// open, on both fresh and already-migrated databases.
func EnsureReviewSessionsCommitSyncColumns(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("failed to migrate review_sessions: nil database handle")
	}
	for _, col := range []string{"api_url", "api_key"} {
		exists, err := hasColumn(db, "review_sessions", col)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := ExecSQL(db, fmt.Sprintf("ALTER TABLE review_sessions ADD COLUMN %s TEXT", col)); err != nil {
			return fmt.Errorf("failed to add review_sessions.%s column: %w", col, err)
		}
	}
	return nil
}

// QueryAttestationSyncCandidateForTreeHash returns the most recent
// review_sessions row for treeHash that represents a real backend
// submission worth syncing to a commit (action reviewed|vouched, with a
// non-empty review_id/api_url/api_key). Returns found=false, not an error,
// when there's nothing to sync (e.g. "skipped", or a pre-migration row).
func QueryAttestationSyncCandidateForTreeHash(db *sql.DB, treeHash string) (id int64, branch, action, reviewID, apiURL, apiKey string, found bool, err error) {
	if db == nil {
		return 0, "", "", "", "", "", false, fmt.Errorf("failed to query sync candidate: nil database handle")
	}
	row := db.QueryRow(
		`SELECT id, branch, action, review_id, api_url, api_key
		 FROM review_sessions
		 WHERE tree_hash = ?
		   AND action IN ('reviewed', 'vouched')
		   AND COALESCE(review_id, '') != ''
		   AND COALESCE(api_url, '') != ''
		   AND COALESCE(api_key, '') != ''
		 ORDER BY id DESC
		 LIMIT 1`,
		treeHash,
	)
	scanErr := row.Scan(&id, &branch, &action, &reviewID, &apiURL, &apiKey)
	if scanErr == sql.ErrNoRows {
		return 0, "", "", "", "", "", false, nil
	}
	if scanErr != nil {
		return 0, "", "", "", "", "", false, fmt.Errorf("failed to query sync candidate for tree %q: %w", treeHash, scanErr)
	}
	return id, branch, action, reviewID, apiURL, apiKey, true, nil
}

// QueryAttestationReviewSessionCountByBranch returns total review sessions for one branch.
func QueryAttestationReviewSessionCountByBranch(db *sql.DB, branch string) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("failed to query review session count: nil database handle")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM review_sessions WHERE branch = ?`, branch).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to query review session count for branch %q: %w", branch, err)
	}
	return count, nil
}

// QueryAttestationReviewedSessionsByBranch returns reviewed sessions in timestamp order.
func QueryAttestationReviewedSessionsByBranch(db *sql.DB, branch string) (*sql.Rows, error) {
	if db == nil {
		return nil, fmt.Errorf("failed to query reviewed sessions: nil database handle")
	}
	rows, err := db.Query(
		`SELECT id, tree_hash, branch, action, timestamp, diff_files, review_id
		 FROM review_sessions
		 WHERE branch = ? AND action = 'reviewed'
		 ORDER BY timestamp ASC`,
		branch,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query reviewed sessions for branch %q: %w", branch, err)
	}
	return rows, nil
}

// DeleteAttestationReviewSessionsByBranch deletes branch-local review sessions.
func DeleteAttestationReviewSessionsByBranch(db *sql.DB, branch string) (int64, error) {
	return DeleteAttestationReviewSessionsByBranchWithOptions(db, branch, DeleteBranchSessionsOptions{})
}

// DeleteAttestationReviewSessionsByBranchWithOptions deletes branch-local review sessions with optional dry-run/logging.
// DryRun=true returns the matching row count without mutating the database.
func DeleteAttestationReviewSessionsByBranchWithOptions(db *sql.DB, branch string, opts DeleteBranchSessionsOptions) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("failed to delete branch sessions: nil database handle")
	}

	if opts.DryRun {
		var count int64
		if err := db.QueryRow(`SELECT COUNT(*) FROM review_sessions WHERE branch = ?`, branch).Scan(&count); err != nil {
			return 0, fmt.Errorf("failed to dry-run review session delete for branch %q: %w", branch, err)
		}
		if opts.Logf != nil {
			opts.Logf("storage: dry-run delete for review_sessions branch=%q count=%d", branch, count)
		}
		return count, nil
	}

	result, err := ExecSQL(db, `DELETE FROM review_sessions WHERE branch = ?`, branch)
	if err != nil {
		return 0, fmt.Errorf("failed to delete review sessions for branch %q: %w", branch, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read branch delete rows-affected: %w", err)
	}
	if opts.Logf != nil {
		opts.Logf("storage: deleted review_sessions branch=%q affected=%d", branch, affected)
	}
	return affected, nil
}

// DeleteAllAttestationReviewSessions deletes all review sessions.
func DeleteAllAttestationReviewSessions(db *sql.DB) (int64, error) {
	return DeleteAllAttestationReviewSessionsWithOptions(db, DeleteAllSessionsOptions{})
}

// DeleteAllAttestationReviewSessionsWithOptions deletes all review sessions with optional confirmation/logging.
// Set RequireConfirmation=true and Confirmed=true to enforce caller confirmation without changing default behavior.
func DeleteAllAttestationReviewSessionsWithOptions(db *sql.DB, opts DeleteAllSessionsOptions) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("failed to delete all sessions: nil database handle")
	}
	if opts.RequireConfirmation && !opts.Confirmed {
		return 0, fmt.Errorf("failed to delete all review sessions: caller confirmation required")
	}

	result, err := ExecSQL(db, `DELETE FROM review_sessions`)
	if err != nil {
		return 0, fmt.Errorf("failed to delete all review sessions: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read full delete rows-affected: %w", err)
	}
	if opts.Logf != nil {
		opts.Logf("storage: deleted all review_sessions affected=%d", affected)
	}
	return affected, nil
}

func compactSQL(query string) string {
	trimmedQuery := ""
	for _, line := range strings.Split(query, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if trimmedQuery != "" {
			trimmedQuery += " "
		}
		trimmedQuery += line
	}
	if len(trimmedQuery) > 240 {
		return trimmedQuery[:240] + "..."
	}
	return trimmedQuery
}
