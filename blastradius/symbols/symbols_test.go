package symbols

import (
	"context"
	"testing"

	"github.com/HexmosTech/blastradius/client"
	"github.com/HexmosTech/blastradius/diffparse"
)

type fakeQuerier struct {
	result *client.QueryResult
}

func (f *fakeQuerier) QueryGraph(ctx context.Context, cypher string, maxRows int) (*client.QueryResult, error) {
	return f.result, nil
}

func TestInFileParsesRows(t *testing.T) {
	fq := &fakeQuerier{result: &client.QueryResult{
		Columns: []string{"name", "qualified_name", "label", "start_line", "end_line"},
		Rows: [][]string{
			{"Foo", "pkg.Foo", "Function", "10", "20"},
			{"Bar", "pkg.Bar", "Struct", "22", "30"},
			{"file.go", "pkg.file.go", "File", "0", "0"},
		},
	}}

	got, err := InFile(context.Background(), fq, "pkg/file.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 symbols, got %d: %+v", len(got), got)
	}
	// Columns the fake querier didn't provide should degrade gracefully to
	// zero values rather than erroring.
	if got[0].Complexity != 0 || got[0].IsEntryPoint || got[0].RouteMethod != "" {
		t.Fatalf("expected zero-valued extra fields when columns are absent, got %+v", got[0])
	}
	if got[0].Name != "Foo" || got[0].StartLine != 10 || got[0].EndLine != 20 {
		t.Fatalf("unexpected first symbol: %+v", got[0])
	}
}

func TestInFileParsesNewFields(t *testing.T) {
	fq := &fakeQuerier{result: &client.QueryResult{
		Columns: []string{
			"name", "qualified_name", "label", "start_line", "end_line",
			"complexity", "cognitive", "loop_depth", "out_degree",
			"is_entry_point", "is_exported", "is_test", "route_method", "route_path",
		},
		Rows: [][]string{
			{"Handler", "pkg.Handler", "Function", "1", "5", "7", "12", "2", "3", "true", "true", "false", "GET", "/api/v1/x"},
		},
	}}

	got, err := InFile(context.Background(), fq, "pkg/file.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(got))
	}
	s := got[0]
	if s.Complexity != 7 || s.Cognitive != 12 || s.LoopDepth != 2 || s.OutDegree != 3 {
		t.Fatalf("unexpected numeric fields: %+v", s)
	}
	if !s.IsEntryPoint || !s.IsExported || s.IsTest {
		t.Fatalf("unexpected boolean fields: %+v", s)
	}
	if s.RouteMethod != "GET" || s.RoutePath != "/api/v1/x" {
		t.Fatalf("unexpected route fields: %+v", s)
	}
}

func TestForHunkOverlap(t *testing.T) {
	fileSymbols := []Symbol{
		{Name: "Foo", StartLine: 10, EndLine: 20},
		{Name: "Bar", StartLine: 22, EndLine: 30},
		{Name: "FileNode", StartLine: 0, EndLine: 0},
	}

	cases := []struct {
		name string
		hunk diffparse.Hunk
		want []string
	}{
		{"inside Foo", diffparse.Hunk{NewStart: 12, NewLines: 3}, []string{"Foo"}},
		{"spans Foo and Bar", diffparse.Hunk{NewStart: 18, NewLines: 6}, []string{"Foo", "Bar"}},
		{"no overlap", diffparse.Hunk{NewStart: 35, NewLines: 2}, nil},
		{"pure deletion anchor inside Bar", diffparse.Hunk{NewStart: 25, NewLines: 0}, []string{"Bar"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ForHunk(fileSymbols, tc.hunk)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d symbols %+v, want %v", len(got), got, tc.want)
			}
			for i, s := range got {
				if s.Name != tc.want[i] {
					t.Fatalf("got %+v, want %v", got, tc.want)
				}
			}
		})
	}
}
