package blastradius

import (
	"encoding/json"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// These tests replay real reviews rather than hand-built hunks. The two
// review 11632 fixtures pin the formula change described in
// testdata/reviews/README.md:
//
//   - review_11632_report.json is the original artifact (linear roll-up),
//     the same one LiveReview's Math Mode tests assert against.
//   - review_11632_current_report.json is the same diff re-scored by the
//     current code (sqrt roll-up), against a v0.9.0 moderate index so the
//     per-symbol inputs still match the original exactly.
//
// review 10566 (31 files, 207 hunks) is the largest captured review and is
// replayed from the design-harness fixture.

func loadReplayReport(t *testing.T, path string) Report {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return r
}

// loadUIFixtureReport loads tools/uidev/fixtures/blastradius.json, which
// wraps the report under "report" alongside the captured status envelope.
func loadUIFixtureReport(t *testing.T, path string) Report {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var wrapped struct {
		Report Report `json:"report"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return wrapped.Report
}

func replayHunk(t *testing.T, r Report, file string, newStart int) HunkReport {
	t.Helper()
	for _, f := range r.Files {
		if f.Path != file {
			continue
		}
		for _, h := range f.Hunks {
			if h.NewStart == newStart {
				return h
			}
		}
	}
	t.Fatalf("hunk %s:%d not found", file, newStart)
	return HunkReport{}
}

func symbolSums(h HunkReport) (blast, priority float64) {
	for _, s := range h.Symbols {
		blast += s.BlastRadiusRaw
		priority += s.ReviewPriorityRaw
	}
	return blast, priority
}

func assertNear(t *testing.T, label string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Errorf("%s = %v, want %v (±%v)", label, got, want, tolerance)
	}
}

func TestReview11632ReplayMatchesLiveReviewGolden(t *testing.T) {
	r := loadReplayReport(t, "testdata/reviews/review_11632_report.json")
	if r.Project != "home-lovestaco-hex-lr-LiveReview" {
		t.Fatalf("Project = %q", r.Project)
	}
	if got := len(r.Files); got != 2 {
		t.Fatalf("files = %d, want 2", got)
	}

	// The multi-symbol hunk LiveReview's TestComputeMathMode_Review11632
	// MultiSymbolMax pins its numbers to.
	h := replayHunk(t, r, "ui/src/pages/Chatbot/rebucketChart.ts", 557)
	if h.SymbolCombine != "" {
		t.Fatalf("legacy fixture SymbolCombine = %q, want empty", h.SymbolCombine)
	}
	assertNear(t, "hunk 557 BlastRadiusRaw", h.BlastRadiusRaw, 30.702042046643918, 1e-9)
	assertNear(t, "hunk 557 ReviewPriorityRaw", h.ReviewPriorityRaw, 69.7, 1e-9)
	assertNear(t, "hunk 557 Combined", h.Combined, 100, 1e-9)

	wantBlast := map[string]float64{
		"SlopeEncodingParts":  1.0,
		"SlopeStats":          1.7320508075688772,
		"computeHeatmapStats": 6.8728982993122445,
		"computeSlopeStats":   6.8728982993122445,
		"getSlopeEncoding":    7.091488720002977,
		"isSlopeGraph":        6.8728982993122445,
	}
	if len(h.Symbols) != len(wantBlast) {
		t.Fatalf("hunk 557 symbols = %d, want %d", len(h.Symbols), len(wantBlast))
	}
	for _, s := range h.Symbols {
		want, ok := wantBlast[s.Name]
		if !ok {
			t.Errorf("unexpected symbol %q", s.Name)
			continue
		}
		assertNear(t, "hunk 557 "+s.Name+" BlastRadiusRaw", s.BlastRadiusRaw, want, 1e-9)
	}

	// The legacy roll-up is a plain sum of symbols plus hunk signals. If the
	// fixture is ever mistaken for a current (sqrt) report, this fails.
	blastSum, prioritySum := symbolSums(h)
	hunkBlast := sumSignalPoints(h.Signals, blastRadiusCategories)
	hunkPriority := sumSignalPoints(h.Signals, reviewPriorityCategories)
	assertNear(t, "hunk 557 linear blast roll-up", blastSum+hunkBlast, h.BlastRadiusRaw, 1e-9)
	assertNear(t, "hunk 557 linear priority roll-up", prioritySum+hunkPriority, h.ReviewPriorityRaw, 1e-9)
}

func TestReview11632CurrentReplayUsesSaturatingCombine(t *testing.T) {
	r := loadReplayReport(t, "testdata/reviews/review_11632_current_report.json")

	for _, f := range r.Files {
		for _, h := range f.Hunks {
			if h.SymbolCombine != "sqrt" {
				t.Fatalf("%s:%d SymbolCombine = %q, want sqrt", f.Path, h.NewStart, h.SymbolCombine)
			}
			blastSum, prioritySum := symbolSums(h)
			hunkBlast := sumSignalPoints(h.Signals, blastRadiusCategories)
			hunkPriority := sumSignalPoints(h.Signals, reviewPriorityCategories)
			assertNear(t, f.Path+": blast sqrt roll-up", math.Sqrt(blastSum)+hunkBlast, h.BlastRadiusRaw, 1e-9)
			assertNear(t, f.Path+": priority sqrt roll-up", math.Sqrt(prioritySum)+hunkPriority, h.ReviewPriorityRaw, 1e-9)
		}
	}

	h := replayHunk(t, r, "ui/src/pages/Chatbot/rebucketChart.ts", 557)
	assertNear(t, "current hunk 557 BlastRadiusRaw", h.BlastRadiusRaw, 5.763008763747323, 1e-9)
	assertNear(t, "current hunk 557 ReviewPriorityRaw", h.ReviewPriorityRaw, 8.34865258589672, 1e-9)
	assertNear(t, "current hunk 557 Combined", h.Combined, 100, 1e-9)

	single := replayHunk(t, r, "ui/src/pages/Chatbot/ChatConversation.tsx", 841)
	assertNear(t, "current single-symbol BlastRadiusRaw", single.BlastRadiusRaw,
		math.Sqrt(6.980266605821071), 1e-9)
	assertNear(t, "current single-symbol ReviewPriorityRaw", single.ReviewPriorityRaw,
		math.Sqrt(38.1), 1e-9)
}

// TestReview11632ReplaySymbolsMatchOriginal is the "same review, same
// inputs" check: the re-run's per-symbol scores, caller counts, and signal
// lists must match the original artifact exactly. Only the hunk roll-up is
// allowed to differ, and only in the documented sqrt way.
func relativeQN(project, qn string) string {
	return strings.TrimPrefix(qn, project+".")
}

func TestReview11632ReplaySymbolsMatchOriginal(t *testing.T) {
	legacy := loadReplayReport(t, "testdata/reviews/review_11632_report.json")
	current := loadReplayReport(t, "testdata/reviews/review_11632_current_report.json")

	key := func(file string, start int) string { return file + ":" + strconv.Itoa(start) }
	currentByKey := map[string]HunkReport{}
	for _, f := range current.Files {
		for _, h := range f.Hunks {
			currentByKey[key(f.Path, h.NewStart)] = h
		}
	}

	compared := 0
	for _, f := range legacy.Files {
		for _, want := range f.Hunks {
			got, ok := currentByKey[key(f.Path, want.NewStart)]
			if !ok {
				t.Fatalf("re-run is missing %s:%d", f.Path, want.NewStart)
			}
			if len(got.Symbols) != len(want.Symbols) {
				t.Fatalf("%s:%d symbols = %d, want %d", f.Path, want.NewStart, len(got.Symbols), len(want.Symbols))
			}
			for i := range want.Symbols {
				w, g := want.Symbols[i], got.Symbols[i]
				if g.Name != w.Name || relativeQN(current.Project, g.QualifiedName) != relativeQN(legacy.Project, w.QualifiedName) {
					t.Fatalf("%s:%d symbol %d = %q/%q, want %q/%q",
						f.Path, want.NewStart, i, g.Name, g.QualifiedName, w.Name, w.QualifiedName)
				}
				assertNear(t, f.Path+":"+strconv.Itoa(want.NewStart)+" "+w.Name+" blast", g.BlastRadiusRaw, w.BlastRadiusRaw, 1e-9)
				assertNear(t, f.Path+":"+strconv.Itoa(want.NewStart)+" "+w.Name+" priority", g.ReviewPriorityRaw, w.ReviewPriorityRaw, 1e-9)
				if g.DirectCount != w.DirectCount || g.TransitiveCount != w.TransitiveCount {
					t.Fatalf("%s:%d %s callers = %d/%d, want %d/%d",
						f.Path, want.NewStart, w.Name, g.DirectCount, g.TransitiveCount, w.DirectCount, w.TransitiveCount)
				}
				if len(g.Signals) != len(w.Signals) {
					t.Fatalf("%s:%d %s signals = %d, want %d",
						f.Path, want.NewStart, w.Name, len(g.Signals), len(w.Signals))
				}
				for j := range w.Signals {
					assertNear(t, f.Path+":"+strconv.Itoa(want.NewStart)+" "+w.Name+" signal "+w.Signals[j].Name,
						g.Signals[j].Points, w.Signals[j].Points, 1e-9)
				}
				compared++
			}
		}
	}
	if compared != 8 {
		t.Fatalf("compared %d symbols, want 8", compared)
	}
}

func TestReview10566ReplayIsLegacyLinear(t *testing.T) {
	r := loadUIFixtureReport(t, "../tools/uidev/fixtures/blastradius.json")
	if r.Project != "home-shrsv-bin-LiveReview" {
		t.Fatalf("Project = %q", r.Project)
	}

	hunks := 0
	withSymbols := 0
	for _, f := range r.Files {
		for _, h := range f.Hunks {
			hunks++
			if len(h.Symbols) > 0 {
				withSymbols++
			}
			blastSum, prioritySum := symbolSums(h)
			hunkBlast := sumSignalPoints(h.Signals, blastRadiusCategories)
			hunkPriority := sumSignalPoints(h.Signals, reviewPriorityCategories)
			assertNear(t, f.Path+": linear blast roll-up", blastSum+hunkBlast, h.BlastRadiusRaw, 1e-6)
			assertNear(t, f.Path+": linear priority roll-up", prioritySum+hunkPriority, h.ReviewPriorityRaw, 1e-6)
		}
	}
	if hunks != 207 {
		t.Fatalf("hunks = %d, want 207", hunks)
	}
	if withSymbols != 45 {
		t.Fatalf("hunks with symbols = %d, want 45", withSymbols)
	}

	// A hunk with 15 symbols and one with none pinned from the capture, so a
	// fixture mix-up cannot pass by matching only trivial hunks.
	nav := replayHunk(t, r, "ui/src/components/Navbar/NavMegaMenu.tsx", 1)
	if len(nav.Symbols) != 15 {
		t.Fatalf("NavMegaMenu symbols = %d, want 15", len(nav.Symbols))
	}
	assertNear(t, "NavMegaMenu BlastRadiusRaw", nav.BlastRadiusRaw, 45.57003102438757, 1e-9)
	lock := replayHunk(t, r, "go.sum", 122)
	if len(lock.Symbols) != 0 {
		t.Fatalf("go.sum symbols = %d, want 0", len(lock.Symbols))
	}
}
