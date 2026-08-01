package appcore

import (
	"context"
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

// computeBlastRadiusReport does the actual work: binary resolution, project
// auto-derivation via index_repository (which creates or incrementally
// refreshes the index and returns the project name), and scoring.
func computeBlastRadiusReport(opts reviewopts.Options, repoRootPath string, diffContent []byte, verbose bool) (*blastradius.Report, error) {
	binary, err := graphengine.Resolve()
	if err != nil {
		if errors.Is(err, graphengine.ErrNotInstalled) {
			return nil, fmt.Errorf("graph engine not installed; run `lrc graph install` to enable blast-radius scoring")
		}
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
