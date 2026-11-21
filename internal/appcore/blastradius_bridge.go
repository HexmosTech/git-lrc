package appcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HexmosTech/git-lrc/blastradius"
	"github.com/HexmosTech/git-lrc/blastradius/client"
	"github.com/HexmosTech/git-lrc/internal/crossrepo"
	"github.com/HexmosTech/git-lrc/internal/graphengine"
	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	"github.com/HexmosTech/git-lrc/internal/reviewopts"
	"github.com/HexmosTech/git-lrc/network"
	"github.com/HexmosTech/git-lrc/storage"
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
	done chan struct{}
	// finalDone closes after cross-repo enrichment (and the upload, when one
	// was started) finishes. It is separate from done so the local UI can show
	// the baseline report immediately and swap in the enriched one later.
	finalDone chan struct{}
	report    *blastradius.Report
	err       error
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

// waitFinal waits for post-scoring enrichment (cross-repo) to land, then
// returns the current published report. Racing on the finalDone channel is
// safe; the report itself is read through blastStateSnapshot's mutex.
func (h *blastScoringHandle) waitFinal(timeout time.Duration) *blastradius.Report {
	if h == nil {
		return nil
	}
	select {
	case <-h.finalDone:
	case <-time.After(timeout):
	}
	_, report, _ := blastStateSnapshot()
	return report
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

// Cross-repo enrichment has its own sub-state because the baseline report is
// published (and usable) before the match request finishes. "skipped" means
// the feature was off or the inputs were absent; "pending" only appears while
// the finalize goroutine is actually working.
var (
	crossRepoMu      sync.RWMutex
	crossRepoStatus  = "skipped" // skipped | pending | ready | unavailable
	crossRepoErr     string
	crossRepoImpacts int
	crossRepoRelated int
)

func setCrossRepoState(status string, impacts, related int, errMsg string) {
	crossRepoMu.Lock()
	defer crossRepoMu.Unlock()
	crossRepoStatus, crossRepoImpacts, crossRepoRelated, crossRepoErr = status, impacts, related, errMsg
}

func crossRepoStateSnapshot() (status string, impacts, related int, errMsg string) {
	crossRepoMu.RLock()
	defer crossRepoMu.RUnlock()
	return crossRepoStatus, crossRepoImpacts, crossRepoRelated, crossRepoErr
}

// extractErrorMessage tries to parse a response body as JSON and extract
// a human-readable error message. Falls back to the raw body if parsing fails.
func extractErrorMessage(body []byte) string {
	var parsed struct {
		Error    string `json:"error"`
		Message  string `json:"message"`
		Envelope *struct {
			Error string `json:"error"`
		} `json:"envelope"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		if parsed.Error != "" {
			return parsed.Error
		}
		if parsed.Envelope != nil && parsed.Envelope.Error != "" {
			return parsed.Envelope.Error
		}
		if parsed.Message != "" {
			return parsed.Message
		}
	}
	s := string(body)
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
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

	h := &blastScoringHandle{done: make(chan struct{}), finalDone: make(chan struct{})}
	setBlastState("pending", nil, "")
	setCrossRepoState("skipped", 0, 0, "")

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
		publishBlastReport(report)
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

// engineVersionIsOutdated reports whether installed (as reported by
// graphengine.InstalledVersion, e.g. "0.9.0") differs from pinned (a release
// tag like "v0.12.1"). Used to upgrade an existing ~/.lrc/bin install that
// predates a PinnedVersion bump instead of treating "a binary exists" as
// "the right binary exists" (see issue #151).
func engineVersionIsOutdated(installed, pinned string) bool {
	return installed != strings.TrimPrefix(pinned, "v")
}

// computeBlastRadiusReport does the actual work: binary resolution, project
// auto-derivation via index_repository (which creates or incrementally
// refreshes the index and returns the project name), and scoring.
func computeBlastRadiusReport(opts reviewopts.Options, repoRootPath string, diffContent []byte, verbose bool) (*blastradius.Report, error) {
	binary, err := graphengine.Resolve()
	needsInstall := errors.Is(err, graphengine.ErrNotInstalled)

	// Resolve only checks that a binary exists, not which version - an
	// existing user on an older engine would otherwise never get upgraded
	// to PinnedVersion after an lrc release bumps it (see issue #151).
	if err == nil && !needsInstall {
		if managedPath, pathErr := graphengine.InstalledBinaryPath(); pathErr == nil && binary == managedPath {
			if installed, verErr := graphengine.InstalledVersion(binary); verErr == nil {
				if engineVersionIsOutdated(installed, graphengine.PinnedVersion) {
					needsInstall = true
				}
			}
		}
	}

	if needsInstall {
		if verbose {
			log.Print("blast-radius: engine missing or outdated, installing pinned version into ~/.lrc/bin")
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

// publishBlastReport makes report the current /api/blastradius payload and
// applies its hunk scores to the live review state (so the local page's badges
// and risk sort use the same numbers). Safe to call twice: scoring publishes
// the baseline, then finalize publishes the cross-repo-enriched report.
func publishBlastReport(report *blastradius.Report) {
	setBlastState("ready", report, "")
	scores := blastScoresByKey(report)
	reviewStateMu.Lock()
	if currentReviewState != nil {
		currentReviewState.ApplyBlastRadiusScores(scores)
	}
	reviewStateMu.Unlock()
}

// applyBlastRadiusFromHandle joins scores from an in-flight scoring run onto
// files, waiting up to blastResultGrace for it to finish. When cross-repo is
// on, wait for the enriched report instead so batch outputs (--output json,
// --save-json) include the cross-repo bonus. Nil-safe.
func applyBlastRadiusFromHandle(h *blastScoringHandle, files []reviewmodel.DiffReviewFileResult, waitForCrossRepo bool) {
	var report *blastradius.Report
	if waitForCrossRepo {
		report = h.waitFinal(blastResultGrace)
	} else {
		report = h.wait(blastResultGrace)
	}
	if report == nil {
		return
	}
	applyBlastScoresToFiles(blastScoresByKey(report), files)
}

// finalizeBlastRadiusReport waits for scoring, then enriches the report with
// cross-repo route matches (when enabled and possible). Every failure leaves
// the baseline report untouched and records a diagnostic; it never returns an
// error because cross-repo enrichment must never fail a review.
func finalizeBlastRadiusReport(h *blastScoringHandle, apiURL, apiKey, reviewID, repoRemoteURL string, opts reviewopts.Options, verbose bool) *blastradius.Report {
	if h == nil {
		return nil
	}
	report := h.wait(blastIndexTimeout)
	if report == nil {
		setCrossRepoState("unavailable", 0, 0, "blast-radius scoring unavailable")
		return nil
	}
	if !opts.CrossRepo {
		setCrossRepoState("skipped", 0, 0, "")
		return report
	}
	if strings.TrimSpace(repoRemoteURL) == "" {
		setCrossRepoState("skipped", 0, 0, "no repository remote URL; cannot resolve connected repository")
		return report
	}
	if strings.TrimSpace(apiURL) == "" || strings.TrimSpace(apiKey) == "" || strings.TrimSpace(reviewID) == "" {
		setCrossRepoState("skipped", 0, 0, "review server credentials unavailable")
		return report
	}
	reviewIDNum, err := strconv.ParseInt(strings.TrimSpace(reviewID), 10, 64)
	if err != nil || reviewIDNum <= 0 {
		setCrossRepoState("skipped", 0, 0, "invalid review id")
		return report
	}

	binary, err := graphengine.Resolve()
	if err != nil {
		setCrossRepoState("unavailable", 0, 0, "graph engine unavailable")
		report.CrossRepoDiagnostics = append(report.CrossRepoDiagnostics, "cross_repo: graph engine unavailable")
		return report
	}
	symbols := crossrepo.Symbols(report)
	if len(symbols) == 0 {
		setCrossRepoState("ready", 0, 0, "")
		return report
	}

	timeout := opts.CrossRepoTimeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout+15*time.Second)
	defer cancel()

	setCrossRepoState("pending", 0, 0, "")
	calls, err := crossrepo.ExtractOutboundCalls(ctx, binary, report.Project, symbols)
	if err != nil {
		warnBlastRadius(verbose, fmt.Errorf("cross-repo call extraction: %w", err))
		setCrossRepoState("unavailable", 0, 0, err.Error())
		report.CrossRepoDiagnostics = append(report.CrossRepoDiagnostics, "cross_repo: call extraction failed: "+err.Error())
		return report
	}
	request := crossrepo.BuildSymbolCalls(calls)
	if len(request) == 0 {
		setCrossRepoState("ready", 0, 0, "")
		report.CrossRepoDiagnostics = append(report.CrossRepoDiagnostics, "cross_repo: no outbound HTTP calls in touched symbols")
		return report
	}

	resp, err := crossrepo.FetchMatches(ctx, apiURL, apiKey, reviewIDNum, request, crossrepo.DefaultDepth, timeout)
	if err != nil {
		warnBlastRadius(verbose, fmt.Errorf("cross-repo match request: %w", err))
		setCrossRepoState("unavailable", 0, 0, err.Error())
		report.CrossRepoDiagnostics = append(report.CrossRepoDiagnostics, "cross_repo: match request failed: "+err.Error())
		return report
	}
	impacted, chained := crossrepo.Apply(report, resp)
	related := crossrepo.RelatedRepoCount(resp)
	setCrossRepoState("ready", impacted, related, "")
	if verbose {
		log.Printf("blast-radius: cross-repo matched %d symbol(s), %d chain(s) across %d related repo(s)",
			impacted, chained, related)
	}
	publishBlastReport(report)
	return report
}

// finalizeAndUploadBlastRadiusReport is the goroutine started once a review ID
// exists: it waits for scoring, adds cross-repo enrichment, republishes the
// report for the local UI, then uploads the enriched artifact. finalDone closes
// in every path so the local page and --save-blast-radius stop waiting.
func finalizeAndUploadBlastRadiusReport(h *blastScoringHandle, apiURL, apiKey, reviewID, repoRemoteURL string, opts reviewopts.Options, upload bool, verbose bool) {
	if h == nil {
		return
	}
	defer close(h.finalDone)
	report := finalizeBlastRadiusReport(h, apiURL, apiKey, reviewID, repoRemoteURL, opts, verbose)
	if !upload {
		return
	}
	uploadBlastRadiusReport(report, h, apiURL, apiKey, reviewID, verbose)
}

// uploadBlastRadiusReport POSTs the already-finalized report to LiveReview's
// artifact sync channel (see LiveReview's AGENTS.md "Porting from git-lrc"
// section) so the hosted review-details page shows the same data. Failures
// are non-fatal and only update the local upload status.
func uploadBlastRadiusReport(report *blastradius.Report, h *blastScoringHandle, apiURL, apiKey, reviewID string, verbose bool) {
	if report == nil {
		// Caller already flagged "uploading" before spawning this goroutine
		// (see review_runtime.go) - move to a terminal state here too, or
		// the browser's close-guard would think an upload is in flight
		// forever. h.err distinguishes a real scoring failure (e.g. an
		// outdated/missing graph engine) from an actual timeout - reporting
		// every nil report as "did not finish in time" was masking the
		// former and making root-causing scoring issues harder.
		errMsg := "blast-radius scoring did not finish in time; nothing to upload"
		if h != nil && h.err != nil {
			errMsg = fmt.Sprintf("blast-radius scoring failed: %v", h.err)
		}
		setBlastUploadState("failed", errMsg, 0, 0)
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
		uploadErr := fmt.Errorf("blast-radius report upload rejected (status %d): %s", resp.StatusCode, extractErrorMessage(resp.Body))
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

// saveBlastRadiusOutput writes the same shape GET /api/blastradius serves, so
// the file can be asserted in CI or fed straight to tools/uidev.
func saveBlastRadiusOutput(path string, verbose bool) error {
	status, report, errMsg := blastStateSnapshot()
	if report == nil {
		return fmt.Errorf("no blast-radius report available")
	}
	crossStatus, impacts, related, crossErr := crossRepoStateSnapshot()
	uploadStatus, uploadErr, sizeBytes, durationMS := blastUploadStateSnapshot()
	payload := map[string]any{
		"status": status,
		"report": report,
		"cross_repo": map[string]any{
			"status":        crossStatus,
			"impacts":       impacts,
			"related_repos": related,
			"error":         crossErr,
		},
		"upload": map[string]any{
			"status":      uploadStatus,
			"size_bytes":  sizeBytes,
			"duration_ms": durationMS,
			"error":       uploadErr,
		},
	}
	if errMsg != "" {
		payload["error"] = errMsg
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal blast-radius report: %w", err)
	}
	if err := storage.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	if verbose {
		log.Printf("blast-radius report saved to: %s", path)
	}
	return nil
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
