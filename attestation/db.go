package attestation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/storage"
)

// ReviewDBPath resolves the review DB location under .git/lrc/reviews.db.
func ReviewDBPath(resolveGitDir func() (string, error)) (string, error) {
	gitDir, err := resolveGitDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve git dir: %w", err)
	}
	if !filepath.IsAbs(gitDir) {
		gitDir, err = filepath.Abs(gitDir)
		if err != nil {
			return "", fmt.Errorf("failed to absolutize git dir: %w", err)
		}
	}
	lrcDir := filepath.Join(gitDir, "lrc")
	if err := storage.EnsureReviewDBDir(lrcDir); err != nil {
		return "", fmt.Errorf("failed to create lrc directory: %w", err)
	}
	return filepath.Join(lrcDir, "reviews.db"), nil
}

// OpenSQLiteReviewDB opens and initializes the SQLite review DB.
func OpenSQLiteReviewDB(dbPath, schema string) (*sql.DB, error) {
	db, err := storage.OpenAttestationReviewDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open review database: %w", err)
	}

	if err := storage.InitializeAttestationReviewSchema(db, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize review database schema: %w", err)
	}

	// Since schema_version:2, review_sessions holds a plaintext api_key
	// (see storage.EnsureReviewSessionsCommitSyncColumns) -- best-effort,
	// but a failure here is worth surfacing rather than swallowing, same
	// reasoning as syncqueue.Open's hardening of ~/.lrc/sync-queue.db.
	if err := storage.Chmod(dbPath, 0600); err != nil {
		log.Printf("attestation: warning: failed to set permissions on %s: %v", dbPath, err)
	}

	return db, nil
}

// CurrentBranch returns current git branch name, or HEAD when detached.
func CurrentBranch() string {
	out, err := exec.Command("git", "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not determine branch (detached HEAD?): %v\n", err)
		return "HEAD"
	}
	return strings.TrimSpace(string(out))
}

// InsertReviewSession inserts a new review session row. apiURL/apiKey are
// the credentials that actually submitted this review (empty for
// "skipped"), snapshotted here so a later commit-sync attempt never has to
// guess which account a review belongs to -- see storage.
// InsertAttestationReviewSessionRow and internal/syncqueue.
func InsertReviewSession(db *sql.DB, treeHash, branch, action string, files []FileEntry, reviewID, apiURL, apiKey string) error {
	filesJSON, err := json.Marshal(files)
	if err != nil {
		return fmt.Errorf("failed to marshal diff files: %w", err)
	}

	err = storage.InsertAttestationReviewSessionRow(
		db,
		treeHash,
		branch,
		action,
		time.Now().UTC().Format(time.RFC3339),
		string(filesJSON),
		reviewID,
		apiURL,
		apiKey,
	)
	if err != nil {
		return fmt.Errorf("failed to insert review session: %w", err)
	}
	return nil
}

// GetSyncCandidateForTreeHash returns the most recent review_sessions row
// for treeHash that represents a real, syncable backend submission, or
// found=false (not an error) when there's nothing to sync -- e.g. the tree
// was only "skipped", or its session predates the api_url/api_key columns.
func GetSyncCandidateForTreeHash(db *sql.DB, treeHash string) (SyncCandidate, bool, error) {
	id, branch, action, reviewID, apiURL, apiKey, found, err := storage.QueryAttestationSyncCandidateForTreeHash(db, treeHash)
	if err != nil {
		return SyncCandidate{}, false, fmt.Errorf("failed to look up sync candidate: %w", err)
	}
	if !found {
		return SyncCandidate{}, false, nil
	}
	return SyncCandidate{
		ID:       id,
		Branch:   branch,
		Action:   action,
		ReviewID: reviewID,
		APIURL:   apiURL,
		APIKey:   apiKey,
	}, true, nil
}

// CountIterations returns total review session count for a branch.
func CountIterations(db *sql.DB, branch string) (int, error) {
	return storage.QueryAttestationReviewSessionCountByBranch(db, branch)
}

// GetPriorReviewedSessions returns branch sessions with action=reviewed in timestamp order.
func GetPriorReviewedSessions(db *sql.DB, branch string) ([]ReviewSession, error) {
	rows, err := storage.QueryAttestationReviewedSessionsByBranch(db, branch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []ReviewSession
	for rows.Next() {
		var s ReviewSession
		var ts, diffFiles, reviewID string
		if err := rows.Scan(&s.ID, &s.TreeHash, &s.Branch, &s.Action, &ts, &diffFiles, &reviewID); err != nil {
			return nil, err
		}
		parsedTime, parseErr := time.Parse(time.RFC3339, ts)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: malformed timestamp %q in review session %d: %v\n", ts, s.ID, parseErr)
		}
		s.Timestamp = parsedTime
		s.DiffFiles = diffFiles
		s.ReviewID = reviewID
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// CleanupReviewSessions deletes all sessions for a branch.
func CleanupReviewSessions(db *sql.DB, branch string) (int64, error) {
	return storage.DeleteAttestationReviewSessionsByBranch(db, branch)
}

// CleanupReviewSessionsDryRun returns how many rows would be removed for a branch without deleting.
func CleanupReviewSessionsDryRun(db *sql.DB, branch string, logf func(format string, args ...any)) (int64, error) {
	return storage.DeleteAttestationReviewSessionsByBranchWithOptions(db, branch, storage.DeleteBranchSessionsOptions{
		DryRun: true,
		Logf:   logf,
	})
}

// CleanupReviewSessionsWithLog deletes branch sessions and emits optional storage audit logging.
func CleanupReviewSessionsWithLog(db *sql.DB, branch string, logf func(format string, args ...any)) (int64, error) {
	return storage.DeleteAttestationReviewSessionsByBranchWithOptions(db, branch, storage.DeleteBranchSessionsOptions{
		Logf: logf,
	})
}

// CleanupAllSessions deletes all sessions from the review database.
func CleanupAllSessions(db *sql.DB) (int64, error) {
	return storage.DeleteAllAttestationReviewSessions(db)
}

// CleanupAllSessionsConfirmed deletes all sessions and requires explicit caller confirmation.
func CleanupAllSessionsConfirmed(db *sql.DB, confirmed bool, logf func(format string, args ...any)) (int64, error) {
	return storage.DeleteAllAttestationReviewSessionsWithOptions(db, storage.DeleteAllSessionsOptions{
		RequireConfirmation: true,
		Confirmed:           confirmed,
		Logf:                logf,
	})
}
