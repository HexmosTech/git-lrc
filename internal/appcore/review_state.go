package appcore

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/HexmosTech/git-lrc/internal/naming"
	"github.com/HexmosTech/git-lrc/internal/reviewhtml"
	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	"github.com/HexmosTech/git-lrc/result"
)

// ReviewState holds the current state of a review for the web UI
type ReviewState struct {
	mu sync.RWMutex

	// Metadata
	ReviewID      string    `json:"reviewID"`
	FriendlyName  string    `json:"friendlyName"`
	GeneratedTime string    `json:"generatedTime"`
	StartedAt     time.Time `json:"-"`

	// Status
	Status  string `json:"status"` // "in_progress", "completed", "failed", "blocked"
	Blocked bool   `json:"blocked"`

	// Content
	Summary string                             `json:"summary"`
	Files   []reviewmodel.DiffReviewFileResult `json:"files"`

	// Counts
	TotalFiles    int `json:"totalFiles"`
	TotalComments int `json:"totalComments"`

	// UI Config
	Interactive        bool   `json:"interactive"`
	IsPostCommitReview bool   `json:"isPostCommitReview"`
	InitialMsg         string `json:"initialMsg"`

	// API Config (for frontend to know where to poll events)
	APIURL string `json:"apiURL"`

	// Error info
	ErrorSummary string `json:"errorSummary,omitempty"`
}

// ReviewStateSnapshot captures read-only fields needed outside the state lock.
type ReviewStateSnapshot struct {
	ReviewID      string
	Status        string
	TotalFiles    int
	TotalComments int
	StartedAt     time.Time
	Summary       string
	Blocked       bool
}

// NewReviewState creates a new ReviewState with initial values
func NewReviewState(reviewID string, files []reviewmodel.DiffReviewFileResult, interactive, isPostCommitReview bool, initialMsg, apiURL string) *ReviewState {
	return &ReviewState{
		ReviewID:           reviewID,
		FriendlyName:       naming.GenerateFriendlyName(),
		GeneratedTime:      time.Now().Format("2006-01-02 15:04:05 MST"),
		StartedAt:          time.Now(),
		Status:             "in_progress",
		Files:              files,
		TotalFiles:         len(files),
		Interactive:        interactive,
		IsPostCommitReview: isPostCommitReview,
		InitialMsg:         initialMsg,
		APIURL:             apiURL,
	}
}

// UpdateFromResult updates the state from a final review result
// It merges comments into existing files rather than replacing them,
// to preserve the hunk data from the initial diff parsing
func (rs *ReviewState) UpdateFromResult(result *reviewmodel.DiffReviewResponse) {
	if result == nil {
		return
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.Status = result.Status
	rs.Summary = result.Summary

	// Merge comments from result into existing files (preserving hunks)
	totalComments := 0
	for i := range rs.Files {
		for _, resultFile := range result.Files {
			if rs.Files[i].FilePath == resultFile.FilePath {
				rs.Files[i].Comments = resultFile.Comments
				break
			}
		}
		totalComments += len(rs.Files[i].Comments)
	}
	rs.TotalComments = totalComments
}

// SetCompleted marks the review as completed
func (rs *ReviewState) SetCompleted(summary string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.Status = "completed"
	if summary != "" {
		rs.Summary = summary
	}
}

// SetFailed marks the review as failed with an error
func (rs *ReviewState) SetFailed(errorSummary string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.Status = "failed"
	rs.ErrorSummary = errorSummary
}

// SetBlocked marks the review as blocked (e.g. quota exceeded)
func (rs *ReviewState) SetBlocked(blocked bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.Blocked = blocked
}

// AddComments adds comments to the total count
// Note: Comments are associated with files in the poll result,
// so full comment merging happens via UpdateFromResult
func (rs *ReviewState) AddComments(count int) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.TotalComments += count
}

// Snapshot returns a thread-safe copy of key state fields.
func (rs *ReviewState) Snapshot() ReviewStateSnapshot {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return ReviewStateSnapshot{
		ReviewID:      rs.ReviewID,
		Status:        rs.Status,
		TotalFiles:    rs.TotalFiles,
		TotalComments: rs.TotalComments,
		StartedAt:     rs.StartedAt,
		Summary:       rs.Summary,
		Blocked:       rs.Blocked,
	}
}

// GetJSON returns the current state as JSON
func (rs *ReviewState) GetJSON() ([]byte, error) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return json.Marshal(rs)
}

// ServeHTTP implements http.Handler for the /api/review endpoint
func (rs *ReviewState) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")

	rs.mu.RLock()
	defer rs.mu.RUnlock()
	if err := json.NewEncoder(w).Encode(rs); err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
		return
	}
}

// PrepareHTMLData converts ReviewState to HTMLTemplateData for initial page render
func (rs *ReviewState) PrepareHTMLData() *result.HTMLTemplateData {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	files := make([]result.HTMLFileData, len(rs.Files))
	for i, file := range rs.Files {
		files[i] = reviewhtml.PrepareFileData(file)
	}

	return &result.HTMLTemplateData{
		GeneratedTime:      rs.GeneratedTime,
		Summary:            "", // Don't include placeholder summary
		Status:             rs.Status,
		TotalFiles:         rs.TotalFiles,
		TotalComments:      rs.TotalComments,
		Files:              files,
		HasSummary:         false, // Will be set when actual summary arrives
		FriendlyName:       rs.FriendlyName,
		Interactive:        rs.Interactive,
		IsPostCommitReview: rs.IsPostCommitReview,
		InitialMsg:         rs.InitialMsg,
		ReviewID:           rs.ReviewID,
		APIURL:             rs.APIURL,
		APIKey:             "", // Don't expose to frontend
	}
}
