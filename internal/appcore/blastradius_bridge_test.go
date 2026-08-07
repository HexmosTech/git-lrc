package appcore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HexmosTech/blastradius"
	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	"github.com/HexmosTech/git-lrc/internal/reviewopts"
)

func ptr(f float64) *float64 { return &f }

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return b
}

func TestSortedHunksByBlastRadius(t *testing.T) {
	hunks := []reviewmodel.DiffReviewHunk{
		{NewStartLine: 10, NewLineCount: 2, BlastRadius: ptr(5)},  // low score
		{NewStartLine: 40, NewLineCount: 3},                       // unscored
		{NewStartLine: 20, NewLineCount: 5, BlastRadius: ptr(90)}, // high score
	}

	got := sortedHunksByBlastRadius(hunks)
	if len(got) != 3 {
		t.Fatalf("expected 3 hunks, got %d", len(got))
	}
	wantOrder := []int{20, 10, 40} // high score, low score, unscored last
	for i, h := range got {
		if h.NewStartLine != wantOrder[i] {
			t.Fatalf("position %d: NewStartLine = %d, want %d (order: %v)", i, h.NewStartLine, wantOrder[i], got)
		}
	}

	// Input slice must not be mutated.
	if hunks[0].NewStartLine != 10 || hunks[1].NewStartLine != 40 || hunks[2].NewStartLine != 20 {
		t.Fatalf("input hunks slice was mutated: %+v", hunks)
	}
}

func TestSortedHunksByBlastRadiusAllUnscoredPreservesOrder(t *testing.T) {
	hunks := []reviewmodel.DiffReviewHunk{
		{NewStartLine: 10, NewLineCount: 2},
		{NewStartLine: 20, NewLineCount: 5},
	}
	got := sortedHunksByBlastRadius(hunks)
	if got[0].NewStartLine != 10 || got[1].NewStartLine != 20 {
		t.Fatalf("expected original diff order preserved when nothing is scored, got %+v", got)
	}
}

func TestStartBlastRadiusScoringDisabled(t *testing.T) {
	h := startBlastRadiusScoring(reviewopts.Options{BlastRadius: false}, "", nil, false)
	if h != nil {
		t.Fatalf("expected nil handle when scoring is disabled, got %+v", h)
	}
	// A nil handle must be safe to wait on and apply from.
	if report := h.wait(0); report != nil {
		t.Fatalf("nil handle wait should return nil report, got %+v", report)
	}
	files := []reviewmodel.DiffReviewFileResult{
		{FilePath: "foo.go", Hunks: []reviewmodel.DiffReviewHunk{{NewStartLine: 1, NewLineCount: 1}}},
	}
	applyBlastRadiusFromHandle(h, files)
	if files[0].Hunks[0].BlastRadius != nil {
		t.Fatalf("expected BlastRadius to stay nil when scoring is disabled, got %v", files[0].Hunks[0].BlastRadius)
	}
}

func TestApplyBlastScoresToFilesJoinsByKey(t *testing.T) {
	files := []reviewmodel.DiffReviewFileResult{
		{FilePath: "a.go", Hunks: []reviewmodel.DiffReviewHunk{
			{NewStartLine: 5, NewLineCount: 3},
			{NewStartLine: 40, NewLineCount: 2},
		}},
		{FilePath: "b.go", Hunks: []reviewmodel.DiffReviewHunk{{NewStartLine: 5, NewLineCount: 3}}},
	}
	scores := map[string]float64{
		blastRadiusKey("a.go", 5, 3): 77.5,
		blastRadiusKey("b.go", 5, 3): 12.0,
		blastRadiusKey("c.go", 1, 1): 99.0, // no matching hunk - ignored
	}
	applyBlastScoresToFiles(scores, files)
	if files[0].Hunks[0].BlastRadius == nil || *files[0].Hunks[0].BlastRadius != 77.5 {
		t.Fatalf("a.go hunk 1 = %v, want 77.5", files[0].Hunks[0].BlastRadius)
	}
	if files[0].Hunks[1].BlastRadius != nil {
		t.Fatalf("a.go hunk 2 should stay nil, got %v", files[0].Hunks[1].BlastRadius)
	}
	if files[1].Hunks[0].BlastRadius == nil || *files[1].Hunks[0].BlastRadius != 12.0 {
		t.Fatalf("b.go hunk = %v, want 12.0", files[1].Hunks[0].BlastRadius)
	}
}

func TestBlastScoringHandleOrderIndependence(t *testing.T) {
	// Report-first vs review-first is just "who reads the handle when":
	// a completed handle must serve its report immediately, and an
	// uncompleted one must time out cleanly without blocking forever.
	completed := &blastScoringHandle{done: make(chan struct{})}
	close(completed.done)
	if completed.wait(0) != nil {
		t.Fatal("completed handle with nil report should return nil")
	}

	pending := &blastScoringHandle{done: make(chan struct{})}
	if report := pending.wait(10 * time.Millisecond); report != nil {
		t.Fatalf("pending handle should time out with nil report, got %+v", report)
	}
}

// TestUploadBlastRadiusReportNoOpCases verifies the fire-and-forget upload
// never makes an HTTP call when there's nothing to upload — a nil handle
// (scoring disabled) or a completed-but-failed run (nil report).
func TestUploadBlastRadiusReportNoOpCases(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	uploadBlastRadiusReport(nil, srv.URL, "key", "42", false)
	if called {
		t.Fatal("nil handle must not make an HTTP call")
	}

	failed := &blastScoringHandle{done: make(chan struct{})}
	close(failed.done) // report stays nil - scoring failed
	uploadBlastRadiusReport(failed, srv.URL, "key", "42", false)
	if called {
		t.Fatal("a handle with no report must not make an HTTP call")
	}
	if status, errMsg, sizeBytes, durationMS := blastUploadStateSnapshot(); status != "failed" || errMsg == "" {
		t.Fatalf("expected a terminal 'failed' upload state with a message when there's no report to upload, got status=%q errMsg=%q sizeBytes=%d durationMS=%d", status, errMsg, sizeBytes, durationMS)
	}
}

// TestUploadBlastRadiusReportUpdatesUploadState verifies the package-level
// upload state (backing GET /api/blastradius's "upload" field) always
// reaches a terminal state, so the browser's poll loop and beforeunload
// guard never see it stuck at "uploading" forever.
func TestUploadBlastRadiusReportUpdatesUploadState(t *testing.T) {
	report := &blastradius.Report{Project: "demo"}

	wantSizeBytes := int64(len(mustMarshal(t, report)))

	t.Run("success reaches uploaded", func(t *testing.T) {
		setBlastUploadState("idle", "", 0, 0)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		h := &blastScoringHandle{done: make(chan struct{}), report: report}
		close(h.done)
		uploadBlastRadiusReport(h, srv.URL, "key", "42", false)

		status, errMsg, sizeBytes, durationMS := blastUploadStateSnapshot()
		if status != "uploaded" || errMsg != "" {
			t.Fatalf("expected status=uploaded with no error, got status=%q errMsg=%q", status, errMsg)
		}
		if sizeBytes != wantSizeBytes {
			t.Fatalf("sizeBytes = %d, want %d (marshaled report size)", sizeBytes, wantSizeBytes)
		}
		if durationMS < 0 {
			t.Fatalf("durationMS = %d, want >= 0", durationMS)
		}
	})

	t.Run("non-2xx reaches failed with message", func(t *testing.T) {
		setBlastUploadState("idle", "", 0, 0)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		h := &blastScoringHandle{done: make(chan struct{}), report: report}
		close(h.done)
		uploadBlastRadiusReport(h, srv.URL, "key", "42", false)

		status, errMsg, sizeBytes, _ := blastUploadStateSnapshot()
		if status != "failed" || errMsg == "" {
			t.Fatalf("expected a terminal 'failed' state with a message on a non-2xx response, got status=%q errMsg=%q", status, errMsg)
		}
		if sizeBytes != wantSizeBytes {
			t.Fatalf("sizeBytes = %d, want %d even on a rejected upload (size is known before the request is sent)", sizeBytes, wantSizeBytes)
		}
	})

	t.Run("transport error reaches failed with message", func(t *testing.T) {
		setBlastUploadState("idle", "", 0, 0)
		h := &blastScoringHandle{done: make(chan struct{}), report: report}
		close(h.done)
		uploadBlastRadiusReport(h, "http://127.0.0.1:0", "key", "42", false)

		if status, errMsg, _, _ := blastUploadStateSnapshot(); status != "failed" || errMsg == "" {
			t.Fatalf("expected a terminal 'failed' state with a message on a transport error, got status=%q errMsg=%q", status, errMsg)
		}
	})
}

func TestWaitForBlastUploadOrTimeout(t *testing.T) {
	t.Run("returns promptly once done closes", func(t *testing.T) {
		done := make(chan struct{})
		close(done)
		start := time.Now()
		waitForBlastUploadOrTimeout(done, time.Second, false)
		if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
			t.Fatalf("expected an immediate return once done is closed, took %s", elapsed)
		}
	})

	t.Run("gives up after timeout without blocking forever", func(t *testing.T) {
		done := make(chan struct{}) // never closed
		start := time.Now()
		waitForBlastUploadOrTimeout(done, 20*time.Millisecond, false)
		if elapsed := time.Since(start); elapsed < 20*time.Millisecond || elapsed > 500*time.Millisecond {
			t.Fatalf("expected to return around the timeout (~20ms), took %s", elapsed)
		}
	})
}

// TestUploadBlastRadiusReportPostsToArtifactChannel verifies a completed
// scoring run POSTs the report JSON to the generic artifact sync channel
// (see LiveReview's AGENTS.md "Porting from git-lrc" section) with the
// review's API key.
func TestUploadBlastRadiusReportPostsToArtifactChannel(t *testing.T) {
	var gotPath, gotAPIKey, gotMethod string
	var gotBody blastradius.Report
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAPIKey = r.Header.Get("X-API-Key")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	report := &blastradius.Report{Project: "demo"}
	h := &blastScoringHandle{done: make(chan struct{}), report: report}
	close(h.done)

	uploadBlastRadiusReport(h, srv.URL, "test-api-key", "42", false)

	wantPath := "/api/v1/diff-review/42/artifacts/blast-radius"
	if gotPath != wantPath {
		t.Fatalf("expected POST to %s, got %s", wantPath, gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if gotAPIKey != "test-api-key" {
		t.Fatalf("expected X-API-Key header 'test-api-key', got %q", gotAPIKey)
	}
	if gotBody.Project != "demo" {
		t.Fatalf("expected uploaded report Project=demo, got %q", gotBody.Project)
	}
}
