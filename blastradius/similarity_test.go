package blastradius

import (
	"context"
	"strings"
	"testing"

	"github.com/HexmosTech/blastradius/client"
)

func TestSimilarSymbolsEmptyInput(t *testing.T) {
	got := similarSymbols(context.Background(), nil, nil)
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
}

func TestSimilarSymbolsParsesRows(t *testing.T) {
	q := &fakeMethodsQuerier{result: &client.QueryResult{
		Columns: []string{"a", "b", "jaccard", "same_file"},
		Rows: [][]string{
			{"pkg.Foo", "pkg.other.Bar", "0.950", "false"},
		},
	}}
	got := similarSymbols(context.Background(), q, []string{"pkg.Foo"})
	matches, ok := got["pkg.Foo"]
	if !ok || len(matches) != 1 || matches[0].QualifiedName != "pkg.other.Bar" || matches[0].Jaccard != 0.95 || matches[0].SameFile {
		t.Fatalf("unexpected result: %+v (ok=%v)", matches, ok)
	}
}

func TestDuplicationSignalNilForEmpty(t *testing.T) {
	if duplicationSignal(nil) != nil {
		t.Fatal("expected nil for no matches")
	}
}

func TestDuplicationSignalWeightsCrossFileHigher(t *testing.T) {
	sameFile := duplicationSignal([]SimilarSymbol{{QualifiedName: "pkg.A", Jaccard: 0.9, SameFile: true}})
	crossFile := duplicationSignal([]SimilarSymbol{{QualifiedName: "pkg.A", Jaccard: 0.9, SameFile: false}})
	if sameFile == nil || crossFile == nil {
		t.Fatal("expected non-nil signals")
	}
	if crossFile.Points <= sameFile.Points {
		t.Fatalf("cross-file match should weigh more: same=%v cross=%v", sameFile.Points, crossFile.Points)
	}
	if crossFile.Category != "duplication" {
		t.Fatalf("unexpected category: %s", crossFile.Category)
	}
}

func TestDuplicationSignalCapsListedMatches(t *testing.T) {
	matches := []SimilarSymbol{
		{QualifiedName: "pkg.A", Jaccard: 0.99},
		{QualifiedName: "pkg.B", Jaccard: 0.95},
		{QualifiedName: "pkg.C", Jaccard: 0.90},
		{QualifiedName: "pkg.D", Jaccard: 0.86},
	}
	sig := duplicationSignal(matches)
	if sig == nil {
		t.Fatal("expected non-nil signal")
	}
	if got := sig.Detail; got == "" {
		t.Fatal("expected non-empty detail")
	}
	// 4 matches with a cap of 3 listed should mention "+1 more".
	if !strings.Contains(sig.Detail, "+1 more") {
		t.Fatalf("expected overflow note in detail, got %q", sig.Detail)
	}
}
