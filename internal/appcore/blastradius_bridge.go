package appcore

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/HexmosTech/blastradius"
	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	"github.com/HexmosTech/git-lrc/internal/reviewopts"
)

func blastRadiusKey(filePath string, newStart, newLines int) string {
	return fmt.Sprintf("%s:%d:%d", filePath, newStart, newLines)
}

// annotateBlastRadius scores every hunk in files against
// opts.BlastRadiusProject using the codebase-memory-mcp-backed blastradius
// library, writing the result directly onto each matching hunk's
// BlastRadius field (mutating files in place). It is strictly best-effort
// and opt-in: when opts.BlastRadius is false it does nothing, and on any
// error (binary missing, project not indexed, timeout) it warns and leaves
// every hunk's BlastRadius nil - this is optional enrichment, never a
// blocker on the review completing.
//
// Once annotated, every consumer (text output, HTML/JSON rendering, the
// live --serve JSON API) can read hunk.BlastRadius directly with no further
// lookup step, since it travels with the hunk itself.
func annotateBlastRadius(opts reviewopts.Options, files []reviewmodel.DiffReviewFileResult, verbose bool) {
	if !opts.BlastRadius {
		return
	}

	var hunks []blastradius.Hunk
	for _, f := range files {
		for _, h := range f.Hunks {
			hunks = append(hunks, blastradius.Hunk{
				FilePath: f.FilePath,
				Header:   fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStartLine, h.OldLineCount, h.NewStartLine, h.NewLineCount),
				NewStart: h.NewStartLine,
				NewLines: h.NewLineCount,
			})
		}
	}
	if len(hunks) == 0 {
		return
	}

	report, err := blastradius.ScoreHunks(context.Background(), opts.BlastRadiusProject, hunks)
	if err != nil {
		warnBlastRadius(verbose, err)
		return
	}

	// Combined blends BlastRadius and ReviewPriority into one 0-100 ranking
	// number (see blastradius.Weights) - git-lrc's UI only surfaces a single
	// score today. Showing the two dimensions separately here is deferred
	// follow-up work, tracked alongside the scoring methodology iteration.
	scores := make(map[string]float64, len(hunks))
	for _, f := range report.Files {
		for _, h := range f.Hunks {
			scores[blastRadiusKey(f.Path, h.NewStart, h.NewLines)] = h.Combined
		}
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
