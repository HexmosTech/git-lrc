package attestation

import "time"

// ReviewSession represents a single review iteration stored in the DB.
type ReviewSession struct {
	ID        int64     `json:"id"`
	TreeHash  string    `json:"tree_hash"`
	Branch    string    `json:"branch"`
	Action    string    `json:"action"`
	Timestamp time.Time `json:"timestamp"`
	DiffFiles string    `json:"diff_files"`
	ReviewID  string    `json:"review_id"`
	// APIURL/APIKey are the credentials that actually submitted this
	// review, snapshotted at submission time -- see
	// storage.InsertAttestationReviewSessionRow. Empty for sessions
	// recorded before this field existed, or for "skipped" (no review was
	// ever submitted).
	APIURL string `json:"api_url,omitempty"`
	APIKey string `json:"api_key,omitempty"`
}

// SyncCandidate is a review_sessions row worth syncing to a commit once one
// is made from this tree: it represents a real backend submission (action
// reviewed|vouched with a known review_id), with the credentials that made
// it, snapshotted at the time.
type SyncCandidate struct {
	ID       int64
	Branch   string
	Action   string
	ReviewID string
	APIURL   string
	APIKey   string
}

// FileEntry is a slim representation of a file diff for storage.
type FileEntry struct {
	FilePath string      `json:"file_path"`
	Hunks    []HunkRange `json:"hunks"`
}

// HunkRange stores line-range info from a hunk.
type HunkRange struct {
	OldStartLine int `json:"old_start_line"`
	OldLineCount int `json:"old_line_count"`
	NewStartLine int `json:"new_start_line"`
	NewLineCount int `json:"new_line_count"`
}

// CoverageResult holds computed coverage statistics.
type CoverageResult struct {
	Iterations       int     `json:"iterations"`
	PriorAICovPct    float64 `json:"prior_ai_coverage_pct"`
	CoveredLines     int     `json:"covered_lines"`
	TotalLines       int     `json:"total_lines"`
	PriorReviewCount int     `json:"prior_review_count"`
}
