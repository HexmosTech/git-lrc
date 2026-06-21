package reviewquery

import (
	"strings"
	"testing"
)

func sampleResult() QueryResult {
	return QueryResult{
		Columns: []string{"Action", "Commits"},
		Rows: [][]string{
			{"reviewed", "89"},
			{"skipped", "218"},
		},
	}
}

func TestFormatTable(t *testing.T) {
	out := FormatTable(sampleResult())
	for _, want := range []string{"Action", "Commits", "reviewed", "89", "skipped", "218", "----"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q\n---\n%s", want, out)
		}
	}
}

func TestFormatJSON(t *testing.T) {
	out, err := FormatJSON(sampleResult())
	if err != nil {
		t.Fatalf("FormatJSON error: %v", err)
	}
	want := `[{"Action":"reviewed","Commits":"89"},{"Action":"skipped","Commits":"218"}]`
	if out != want {
		t.Errorf("FormatJSON =\n%s\nwant\n%s", out, want)
	}
}

func TestFormatJSONEmpty(t *testing.T) {
	out, err := FormatJSON(QueryResult{Columns: []string{"a"}, Rows: nil})
	if err != nil {
		t.Fatalf("FormatJSON error: %v", err)
	}
	if out != "[]" {
		t.Errorf("FormatJSON empty = %q; want []", out)
	}
}
