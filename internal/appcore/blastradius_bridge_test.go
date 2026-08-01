package appcore

import (
	"testing"

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

func TestAnnotateBlastRadiusDisabledByDefault(t *testing.T) {
	files := []reviewmodel.DiffReviewFileResult{
		{FilePath: "foo.go", Hunks: []reviewmodel.DiffReviewHunk{{NewStartLine: 1, NewLineCount: 1}}},
	}
	annotateBlastRadius(reviewopts.Options{}, files, false)
	if files[0].Hunks[0].BlastRadius != nil {
		t.Fatalf("expected BlastRadius to stay nil when opts.BlastRadius is false, got %v", files[0].Hunks[0].BlastRadius)
	}
}
