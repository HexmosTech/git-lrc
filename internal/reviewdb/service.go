package reviewdb

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/HexmosTech/git-lrc/attestation"
	"github.com/HexmosTech/git-lrc/internal/reviewapi"
	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
)

type reviewSession = attestation.ReviewSession
type attestationFileEntry = attestation.FileEntry
type attestationHunkRange = attestation.HunkRange
type coverageResult = attestation.CoverageResult

const reviewDBSchema = `
-- schema_version:2
-- NOTE: bump schema_version when review_sessions schema changes.
-- v2 added api_url/api_key (see storage.EnsureReviewSessionsCommitSyncColumns,
-- which migrates pre-v2 databases -- CREATE TABLE IF NOT EXISTS below only
-- covers brand-new ones).
CREATE TABLE IF NOT EXISTS review_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tree_hash TEXT NOT NULL,
    branch TEXT NOT NULL,
    action TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    diff_files TEXT,
    review_id TEXT,
    api_url TEXT,
    api_key TEXT
);
CREATE INDEX IF NOT EXISTS idx_review_sessions_branch ON review_sessions(branch);
CREATE INDEX IF NOT EXISTS idx_review_sessions_tree ON review_sessions(tree_hash);
`

func resolveGitDir() (string, error) {
	return reviewapi.ResolveGitDir()
}

func currentTreeHash() (string, error) {
	return reviewapi.CurrentTreeHash()
}

// reviewDBPath returns the path to the review database under .git/lrc/.
func reviewDBPath() (string, error) {
	return attestation.ReviewDBPath(resolveGitDir)
}

// openReviewDB opens (or creates) the SQLite review database.
func openReviewDB() (*sql.DB, error) {
	dbPath, err := reviewDBPath()
	if err != nil {
		return nil, err
	}
	return attestation.OpenSQLiteReviewDB(dbPath, reviewDBSchema)
}

// currentBranch returns the current git branch name, or "HEAD" if detached.
func currentBranch() string {
	return attestation.CurrentBranch()
}

// filesToEntries converts parsed diff file results to slim attestation entries
// (strips Content from hunks to keep DB rows small).
func filesToEntries(files []reviewmodel.DiffReviewFileResult) []attestationFileEntry {
	entries := make([]attestationFileEntry, len(files))
	for i, f := range files {
		hunks := make([]attestationHunkRange, len(f.Hunks))
		for j, h := range f.Hunks {
			hunks[j] = attestationHunkRange{
				OldStartLine: h.OldStartLine,
				OldLineCount: h.OldLineCount,
				NewStartLine: h.NewStartLine,
				NewLineCount: h.NewLineCount,
			}
		}
		entries[i] = attestationFileEntry{
			FilePath: f.FilePath,
			Hunks:    hunks,
		}
	}
	return entries
}

// insertReviewSession inserts a new review session into the database.
func insertReviewSession(db *sql.DB, treeHash, branch, action string, files []attestationFileEntry, reviewID, apiURL, apiKey string) error {
	return attestation.InsertReviewSession(db, treeHash, branch, action, files, reviewID, apiURL, apiKey)
}

// countIterations returns the total number of review sessions for the given branch.
func countIterations(db *sql.DB, branch string) (int, error) {
	return attestation.CountIterations(db, branch)
}

// getPriorReviewedSessions returns all "reviewed" sessions for the branch,
// ordered by timestamp ascending.
func getPriorReviewedSessions(db *sql.DB, branch string) ([]reviewSession, error) {
	return attestation.GetPriorReviewedSessions(db, branch)
}

// cleanupReviewSessions deletes all sessions for the given branch.
// Called after a successful commit to start fresh.
func cleanupReviewSessions(db *sql.DB, branch string) (int64, error) {
	return attestation.CleanupReviewSessions(db, branch)
}

// cleanupAllSessions deletes ALL sessions from the database.
func cleanupAllSessions(db *sql.DB) (int64, error) {
	return attestation.CleanupAllSessions(db)
}

// computePriorCoverage calculates how many lines in the current diff were
// already AI-reviewed in prior iterations (for the same branch).
//
// The algorithm:
//  1. Get all "reviewed" sessions for the current branch
//  2. For each prior session, compute which of the current diff's new-side lines
//     were already covered by that review (i.e., lines that haven't changed since)
//  3. Accumulate coverage across all prior sessions (union of covered lines)
//  4. Return iteration count and coverage percentage
func computePriorCoverage(db *sql.DB, branch, currentTreeHash string, currentFiles []attestationFileEntry) (coverageResult, error) {
	return attestation.ComputePriorCoverage(db, branch, currentTreeHash, currentFiles)
}

// recordAndComputeCoverage is a convenience function that opens the DB,
// records the session, computes coverage, and returns the result.
// It is the main entry point for all review actions (reviewed/skipped/vouched).
func RecordAndComputeCoverage(action string, parsedFiles []reviewmodel.DiffReviewFileResult, reviewID, apiURL, apiKey string, verbose bool) (coverageResult, error) {
	db, err := openReviewDB()
	if err != nil {
		if verbose {
			fmt.Printf("Warning: could not open review DB: %v (coverage tracking disabled)\n", err)
		}
		return coverageResult{Iterations: 1}, nil
	}
	defer db.Close()

	treeHash, err := currentTreeHash()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not determine current tree hash: %v (coverage tracking disabled)\n", err)
		return coverageResult{Iterations: 1}, nil
	}

	branch := currentBranch()
	entries := filesToEntries(parsedFiles)

	// Compute coverage BEFORE inserting current session
	cov, err := computePriorCoverage(db, branch, treeHash, entries)
	if err != nil {
		if verbose {
			fmt.Printf("Warning: coverage computation failed: %v\n", err)
		}
		cov = coverageResult{Iterations: 1}
	}

	// For "reviewed" action, the current review covers 100% of the lines it touches
	// The coverage % reflects how much was ALREADY covered by PRIOR reviews
	// (not including the current one)

	// Insert the current session
	if err := insertReviewSession(db, treeHash, branch, action, entries, reviewID, apiURL, apiKey); err != nil {
		if verbose {
			fmt.Printf("Warning: failed to record review session: %v\n", err)
		}
	}

	return cov, nil
}

// GetSyncCandidateForTreeHash opens the current repo's review DB and looks
// up the most recent syncable review session for treeHash (see
// attestation.GetSyncCandidateForTreeHash). found=false, not an error, means
// there's nothing to sync for this tree.
func GetSyncCandidateForTreeHash(treeHash string) (attestation.SyncCandidate, bool, error) {
	db, err := openReviewDB()
	if err != nil {
		return attestation.SyncCandidate{}, false, fmt.Errorf("could not open review DB: %w", err)
	}
	defer db.Close()

	return attestation.GetSyncCandidateForTreeHash(db, treeHash)
}

// runReviewDBCleanup deletes all review sessions for the current branch.
// Called from the post-commit hook via "lrc review-cleanup".
func RunReviewDBCleanup(verbose bool) error {
	db, err := openReviewDB()
	if err != nil {
		if verbose {
			fmt.Printf("Warning: could not open review DB for cleanup: %v\n", err)
		}
		return nil
	}
	defer db.Close()

	branch := currentBranch()
	affected, err := cleanupReviewSessions(db, branch)
	if err != nil {
		return fmt.Errorf("failed to clean up review sessions: %w", err)
	}
	if verbose && affected > 0 {
		fmt.Printf("lrc: cleaned up %d review session(s) for branch %s\n", affected, branch)
	}
	return nil
}
