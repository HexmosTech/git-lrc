package blastradius

import (
	"context"
	"testing"

	"github.com/HexmosTech/blastradius/client"
	"github.com/HexmosTech/blastradius/symbols"
)

func TestTestCoverageCountsEmptyInput(t *testing.T) {
	got := testCoverageCounts(context.Background(), nil, nil)
	if len(got) != 0 {
		t.Fatalf("expected empty map for empty input, got %v", got)
	}
}

func TestTestCoverageCountsParsesRows(t *testing.T) {
	q := &fakeMethodsQuerier{result: &client.QueryResult{
		Columns: []string{"symbol", "test_count"},
		Rows: [][]string{
			{"pkg.Foo", "3"},
			{"pkg.Bar", "0"},
		},
	}}

	got := testCoverageCounts(context.Background(), q, []string{"pkg.Foo", "pkg.Bar"})
	if got["pkg.Foo"] != 3 || got["pkg.Bar"] != 0 {
		t.Fatalf("unexpected counts: %v", got)
	}
	if _, ok := got["pkg.Missing"]; ok {
		t.Fatalf("expected pkg.Missing to be absent, not present with a value")
	}
}

func sumPoints(signals []Signal) float64 {
	total := 0.0
	for _, s := range signals {
		total += s.Points
	}
	return total
}

func TestCodeMetricSignalsCoverageBoost(t *testing.T) {
	zero := symbols.Symbol{}
	base := sumPoints(codeMetricSignals(zero, 0))
	withCoverage := sumPoints(codeMetricSignals(zero, 5))
	if withCoverage >= base {
		t.Fatalf("more test coverage should lower the total: base=%v withCoverage=%v", base, withCoverage)
	}
	if base != 3.0 {
		t.Fatalf("zero-coverage, zero-complexity symbol should score exactly the full +3 coverage boost, got %v", base)
	}
}

func TestCodeMetricSignalsSkipsZeroValues(t *testing.T) {
	zero := symbols.Symbol{}
	signals := codeMetricSignals(zero, 1)
	// Only the always-emitted "Test coverage" signal should appear when every
	// other metric is zero - Complexity/Cognitive/LoopDepth/OutDegree signals
	// are omitted rather than emitted at Points=0, to avoid noise.
	if len(signals) != 1 || signals[0].Name != "Test coverage" {
		t.Fatalf("expected only the Test coverage signal, got %+v", signals)
	}
}
