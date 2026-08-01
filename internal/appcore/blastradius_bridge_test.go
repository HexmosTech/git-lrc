package appcore

import (
	"testing"
	"time"

	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	"github.com/HexmosTech/git-lrc/internal/reviewopts"
)

func ptr(f float64) *float64 { return &f }

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
