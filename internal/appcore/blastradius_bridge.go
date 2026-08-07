package appcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/HexmosTech/blastradius"
	"github.com/HexmosTech/blastradius/client"
	"github.com/HexmosTech/git-lrc/internal/graphengine"
	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	"github.com/HexmosTech/git-lrc/internal/reviewopts"
	"github.com/HexmosTech/git-lrc/network"
)

// Blast-radius scoring runs concurrently with the server-side review: the
// diff is scored locally against the codebase-memory-mcp knowledge graph in
// a goroutine started right after diff collection, while the review is
// submitted and polled as usual. Whichever finishes first simply lands
// first; the two result sets are combined when both are available. Scoring
// is strictly best-effort and never blocks or fails a review.

// blastIndexTimeout bounds the auto-index + scoring pass. A first-time index
// of a large repository can legitimately take minutes; it runs alongside the
// (also multi-minute) server review.
const blastIndexTimeout = 15 * time.Minute

// blastResultGrace is how long batch outputs (--save-text/--save-json/
// --save-html) wait for scoring after the review itself has completed,
// so a huge first index can't hang a finished review indefinitely.
const blastResultGrace = 30 * time.Second

// blastUploadExitGrace bounds how long the CLI process waits for an
// in-flight blast-radius upload to finish before exiting anyway. Short,
// since this only buys the upload a little extra time on a fast decision
// (e.g. an immediate vouch), not a correctness guarantee - it never blocks
// the CLI's own "never blocks a review" contract. The browser-side
// beforeunload guard (app.js) covers the tab-close case separately.
const blastUploadExitGrace = 5 * time.Second

// blastScoringHandle carries an in-flight scoring run. A nil handle (scoring
// disabled) is valid everywhere and behaves as "no report".
type blastScoringHandle struct {
	done   chan struct{}
	report *blastradius.Report
	err    error
}

// wait blocks until scoring finishes or timeout elapses, returning the
// report (nil when unavailable). Nil-safe.
func (h *blastScoringHandle) wait(timeout time.Duration) *blastradius.Report {
	if h == nil {
		return nil
	}
	select {
	case <-h.done:
		return h.report
	case <-time.After(timeout):
		return nil
	}
}

// Package-level snapshot backing GET /api/blastradius. Kept separate from
// ReviewState so the full signal-rich report never bloats /api/review's
// payload and survives the UI's wholesale files replacement on final fetch.
var (
	blastStateMu    sync.RWMutex
	blastStatus     = "unavailable" // "pending" | "ready" | "unavailable"
	blastReport     *blastradius.Report
	blastErrMessage string
)

func setBlastState(status string, report *blastradius.Report, errMsg string) {
	blastStateMu.Lock()
	defer blastStateMu.Unlock()
	blastStatus, blastReport, blastErrMessage = status, report, errMsg
}

func blastStateSnapshot() (status string, report *blastradius.Report, errMsg string) {
	blastStateMu.RLock()
	defer blastStateMu.RUnlock()
	return blastStatus, blastReport, blastErrMessage
}

// Package-level snapshot backing the "upload" field of GET /api/blastradius.
// Separate mutex/state from the scoring snapshot above since upload only
// starts once a reviewID exists (well after scoring may have finished) and
// tracks a distinct lifecycle: "idle" (not started/not applicable) ->
// "uploading" -> "uploaded" | "failed".
var (
	blastUploadMu         sync.RWMutex
	blastUploadStatus     = "idle"
	blastUploadErrMsg     string
	blastUploadSizeBytes  int64
	blastUploadDurationMS int64
)

func setBlastUploadState(status, errMsg string, sizeBytes, durationMS int64) {
	blastUploadMu.Lock()
	defer blastUploadMu.Unlock()
	blastUploadStatus, blastUploadErrMsg = status, errMsg
	blastUploadSizeBytes, blastUploadDurationMS = sizeBytes, durationMS
}

func blastUploadStateSnapshot() (status, errMsg string, sizeBytes, durationMS int64) {
	blastUploadMu.RLock()
	defer blastUploadMu.RUnlock()
	return blastUploadStatus, blastUploadErrMsg, blastUploadSizeBytes, blastUploadDurationMS
}

// startBlastRadiusScoring kicks off the concurrent scoring goroutine:
// resolve the engine binary, create/refresh the repo's graph index, score
// every hunk in diffContent, then publish the report for /api/blastradius
// and fold the per-hunk Combined floats into the live ReviewState (so page
// reloads and batch outputs see them). Returns nil when scoring is disabled.
func startBlastRadiusScoring(opts reviewopts.Options, repoRootPath string, diffContent []byte, verbose bool) *blastScoringHandle {
	if !opts.BlastRadius {
		return nil
	}

	h := &blastScoringHandle{done: make(chan struct{})}
	setBlastState("pending", nil, "")

	go func() {
		defer close(h.done)
		report, err := computeBlastRadiusReport(opts, repoRootPath, diffContent, verbose)
		if err != nil {
			h.err = err
			warnBlastRadius(verbose, err)
			setBlastState("unavailable", nil, err.Error())
			return
		}
		h.report = report
		setBlastState("ready", report, "")

		scores := blastScoresByKey(report)
		reviewStateMu.Lock()
		if currentReviewState != nil {
			currentReviewState.ApplyBlastRadiusScores(scores)
		}
		reviewStateMu.Unlock()
	}()
	return h
}

// autoInstallGraphEngine is a one-time, best-effort background download of
// the codebase-memory-mcp binary into ~/.lrc/bin. It exists so existing lrc
// users who upgrade get scoring on their next review without needing to know
// about `lrc graph install` — the engine appears spontaneously, just like the
// install scripts do for new users. Failures are reported but never block
// the review.
func autoInstallGraphEngine(verbose bool) error {
	res, err := graphengine.Install(graphengine.InstallOptions{})
	if err != nil {
		return err
	}
	if res.Skipped {
		if verbose {
			log.Printf("blast-radius: engine already installed at %s (version %s)", res.Path, res.Version)
		}
	} else {
		log.Printf("blast-radius: auto-installed graph engine %s into %s", res.Version, res.Path)
	}
	return nil
}

// computeBlastRadiusReport does the actual work: binary resolution, project
// auto-derivation via index_repository (which creates or incrementally
// refreshes the index and returns the project name), and scoring.
func computeBlastRadiusReport(opts reviewopts.Options, repoRootPath string, diffContent []byte, verbose bool) (*blastradius.Report, error) {
	binary, err := graphengine.Resolve()
	if errors.Is(err, graphengine.ErrNotInstalled) {
		if verbose {
			log.Print("blast-radius: engine not found, attempting one-time install into ~/.lrc/bin")
		}
		if instErr := autoInstallGraphEngine(verbose); instErr != nil {
			return nil, fmt.Errorf("graph engine not installed; run `lrc graph install` to enable blast-radius scoring (auto-install failed: %v)", instErr)
		}
		binary, err = graphengine.Resolve()
		if err != nil {
			return nil, fmt.Errorf("graph engine not installed; run `lrc graph install` to enable blast-radius scoring")
		}
	} else if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), blastIndexTimeout)
	defer cancel()

	project := opts.BlastRadiusProject
	if project == "" {
		if repoRootPath == "" {
			return nil, errors.New("cannot auto-derive the graph project outside a git repository (pass --blast-radius-project)")
		}
		start := time.Now()
		engineClient := &client.Client{Binary: binary}
		project, err = engineClient.IndexRepository(ctx, repoRootPath, "fast")
		if err != nil {
			return nil, fmt.Errorf("failed to index repository for blast-radius scoring: %w", err)
		}
		if verbose {
			log.Printf("blast-radius: index refreshed for project %s in %s", project, time.Since(start).Round(time.Millisecond))
		}
	}

	scoreOpts := blastradius.DefaultOptions()
	scoreOpts.Binary = binary
	return blastradius.ScoreDiff(ctx, diffContent, project, scoreOpts)
}

func blastRadiusKey(filePath string, newStart, newLines int) string {
	return fmt.Sprintf("%s:%d:%d", filePath, newStart, newLines)
}

// blastScoresByKey flattens a report into hunk-key -> Combined score.
func blastScoresByKey(report *blastradius.Report) map[string]float64 {
	if report == nil {
		return nil
	}
	scores := make(map[string]float64)
	for _, f := range report.Files {
		for _, h := range f.Hunks {
			scores[blastRadiusKey(f.Path, h.NewStart, h.NewLines)] = h.Combined
		}
	}
	return scores
}

// applyBlastScoresToFiles writes Combined scores onto matching hunks
// (mutating files in place). Hunks with no match are left nil.
func applyBlastScoresToFiles(scores map[string]float64, files []reviewmodel.DiffReviewFileResult) {
	if len(scores) == 0 {
		return
	}
	for i := range files {
		for j := range files[i].Hunks {
			h := &files[i].Hunks[j]
			score, ok := scores[blastRadiusKey(files[i].FilePath, h.NewStartLine, h.NewLineCount)]
			if !ok {
				continue
			}
			h.BlastRadius = &score
		}
	}
}

// applyBlastRadiusFromHandle joins scores from an in-flight scoring run onto
// files, waiting up to blastResultGrace for it to finish. Used by the batch
// outputs (--save-text/--save-html) after the review completes. Nil-safe.
func applyBlastRadiusFromHandle(h *blastScoringHandle, files []reviewmodel.DiffReviewFileResult) {
	report := h.wait(blastResultGrace)
	if report == nil {
		return
	}
	applyBlastScoresToFiles(blastScoresByKey(report), files)
}

// uploadBlastRadiusReport waits for the scoring goroutine started by
// startBlastRadiusScoring to finish, then POSTs the report to LiveReview's
// artifact sync channel (see LiveReview's AGENTS.md "Porting from git-lrc"
// section) so the hosted review-details page can show it too. Called as its
// own goroutine once reviewID is known (scoring starts before the review
// submission response arrives, so it can't upload eagerly) — fire-and-forget,
// matching blast-radius scoring's own "never blocks or fails a review"
// contract. A nil handle (scoring disabled) is a silent no-op.
func uploadBlastRadiusReport(h *blastScoringHandle, apiURL, apiKey, reviewID string, verbose bool) {
	if h == nil {
		return
	}
	report := h.wait(blastIndexTimeout)
	if report == nil {
		// Caller already flagged "uploading" before spawning this goroutine
		// (see review_runtime.go) - move to a terminal state here too, or
		// the browser's close-guard would think an upload is in flight
		// forever.
		setBlastUploadState("failed", "blast-radius scoring did not finish in time; nothing to upload", 0, 0)
		return
	}

	reportBytes, err := json.Marshal(report)
	if err != nil {
		uploadErr := fmt.Errorf("failed to marshal blast-radius report: %w", err)
		warnBlastRadius(verbose, uploadErr)
		setBlastUploadState("failed", uploadErr.Error(), 0, 0)
		return
	}
	sizeBytes := int64(len(reportBytes))

	client := network.NewReviewAPIClient(30 * time.Second)
	start := time.Now()
	resp, err := network.ReviewUploadArtifact(client, apiURL, reviewID, "blast-radius", json.RawMessage(reportBytes), apiKey)
	durationMS := time.Since(start).Milliseconds()
	if err != nil {
		uploadErr := fmt.Errorf("failed to upload blast-radius report: %w", err)
		warnBlastRadius(verbose, uploadErr)
		setBlastUploadState("failed", uploadErr.Error(), sizeBytes, durationMS)
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		uploadErr := fmt.Errorf("blast-radius report upload rejected (status %d): %s", resp.StatusCode, string(resp.Body))
		warnBlastRadius(verbose, uploadErr)
		setBlastUploadState("failed", uploadErr.Error(), sizeBytes, durationMS)
		return
	}
	if verbose {
		log.Printf("blast-radius: report uploaded to LiveReview for review %s", reviewID)
	}
	setBlastUploadState("uploaded", "", sizeBytes, durationMS)
}

// waitForBlastUploadOrTimeout waits up to timeout for the upload goroutine
// spawned in review_runtime.go to finish before the CLI process exits, so a
// fast decision doesn't silently kill an in-flight upload. Never blocks
// indefinitely - on timeout it just logs a warning and returns.
func waitForBlastUploadOrTimeout(done <-chan struct{}, timeout time.Duration, verbose bool) {
	select {
	case <-done:
	case <-time.After(timeout):
		warnBlastRadius(verbose, errors.New("blast-radius upload still in progress, exiting anyway"))
	}
}

func warnBlastRadius(verbose bool, err error) {
	msg := fmt.Sprintf("blast-radius scoring skipped: %v", err)
	if verbose {
		log.Print(msg)
		return
	}
	fmt.Fprintln(os.Stderr, "Warning:", msg)
}

// sortedHunksByBlastRadius returns a copy of hunks ordered by descending
// BlastRadius score; hunks with no computed score keep their original
// relative order and sort after every scored hunk. The input slice is never
// mutated.
func sortedHunksByBlastRadius(hunks []reviewmodel.DiffReviewHunk) []reviewmodel.DiffReviewHunk {
	sorted := append([]reviewmodel.DiffReviewHunk(nil), hunks...)
	sort.SliceStable(sorted, func(a, b int) bool {
		ra, rb := sorted[a].BlastRadius, sorted[b].BlastRadius
		if (ra != nil) != (rb != nil) {
			return ra != nil // scored hunks sort before unscored ones
		}
		if ra == nil {
			return false
		}
		return *ra > *rb
	})
	return sorted
}
