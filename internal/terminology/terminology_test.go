package terminology

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shrsv/dbctx/internal/testutil"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		in      string
		table   string
		column  string
		path    string
		wantErr bool
	}{
		{in: "orders", table: "orders"},
		{in: "orders.total", table: "orders", column: "total"},
		{in: "reviews.metadata:$.provider", table: "reviews", column: "metadata", path: "$.provider"},
		{in: "", wantErr: true},
		{in: ".total", wantErr: true},
		{in: "reviews.metadata:", wantErr: true},
		{in: "reviews:$.provider", wantErr: true}, // path with no column
	}
	for _, tt := range tests {
		table, column, path, err := parseTarget(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseTarget(%q): expected error, got none", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTarget(%q): unexpected error: %v", tt.in, err)
			continue
		}
		if table != tt.table || column != tt.column || path != tt.path {
			t.Errorf("parseTarget(%q) = (%q,%q,%q), want (%q,%q,%q)", tt.in, table, column, path, tt.table, tt.column, tt.path)
		}
	}
}

func TestFormatTarget_RoundTrip(t *testing.T) {
	cases := []string{"orders", "orders.total", "reviews.metadata:$.provider"}
	for _, c := range cases {
		table, column, path, err := parseTarget(c)
		if err != nil {
			t.Fatalf("parseTarget(%q): %v", c, err)
		}
		if got := formatTarget(table, column, path); got != c {
			t.Errorf("formatTarget round-trip = %q, want %q", got, c)
		}
	}
}

func termJSON(t *testing.T, groups []TermGroup) []byte {
	t.Helper()
	b, err := json.Marshal(groups)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestImport_TableLevelTarget(t *testing.T) {
	store := testutil.NewSeedStore(t)
	data := termJSON(t, []TermGroup{
		{Term: "pr", Aliases: []string{"pull request", "pull requests"}, Targets: []string{"pull_requests"}},
	})

	result, err := Import(store, data)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Accepted != 2 {
		t.Errorf("Accepted = %d, want 2", result.Accepted)
	}
	if len(result.Rejected) != 0 {
		t.Errorf("Rejected = %v, want none", result.Rejected)
	}

	entries, err := List(store)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("List returned %d entries, want 2", len(entries))
	}
}

func TestImport_ColumnLevelTarget(t *testing.T) {
	store := testutil.NewSeedStore(t)
	data := termJSON(t, []TermGroup{
		{Term: "org_plan", Aliases: []string{"subscription tier"}, Targets: []string{"orgs.plan"}},
	})

	result, err := Import(store, data)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Accepted != 1 {
		t.Fatalf("Accepted = %d, want 1 (rejected: %v)", result.Accepted, result.Rejected)
	}

	entries, _ := List(store)
	if entries[0].TargetTable != "orgs" || entries[0].TargetColumn != "plan" {
		t.Errorf("entry target = %s/%s, want orgs/plan", entries[0].TargetTable, entries[0].TargetColumn)
	}
}

func TestImport_JSONBPathTarget(t *testing.T) {
	store := testutil.NewSeedStore(t)
	data := termJSON(t, []TermGroup{
		{Term: "vcs", Aliases: []string{"version control provider"}, Targets: []string{"reviews.metadata:$.provider"}},
	})

	result, err := Import(store, data)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Accepted != 1 {
		t.Fatalf("Accepted = %d, want 1 (rejected: %v)", result.Accepted, result.Rejected)
	}

	entries, _ := List(store)
	if entries[0].TargetPath != "$.provider" {
		t.Errorf("entry target path = %q, want $.provider", entries[0].TargetPath)
	}
}

func TestImport_RejectsNonexistentTable(t *testing.T) {
	store := testutil.NewSeedStore(t)
	data := termJSON(t, []TermGroup{
		{Term: "x", Aliases: []string{"widget"}, Targets: []string{"widgets"}},
	})

	result, err := Import(store, data)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Accepted != 0 {
		t.Errorf("Accepted = %d, want 0", result.Accepted)
	}
	if len(result.Rejected) != 1 {
		t.Fatalf("Rejected = %v, want 1 entry", result.Rejected)
	}
	if !strings.Contains(result.Rejected[0].Reason, "does not exist") {
		t.Errorf("rejection reason = %q, want mention of nonexistent table", result.Rejected[0].Reason)
	}
}

func TestImport_RejectsNonexistentColumn(t *testing.T) {
	store := testutil.NewSeedStore(t)
	data := termJSON(t, []TermGroup{
		{Term: "x", Aliases: []string{"nope"}, Targets: []string{"orgs.nonexistent_column"}},
	})

	result, err := Import(store, data)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Accepted != 0 || len(result.Rejected) != 1 {
		t.Fatalf("expected exactly 1 rejection, got accepted=%d rejected=%v", result.Accepted, result.Rejected)
	}
}

func TestImport_RejectsNonexistentJSONBPath(t *testing.T) {
	store := testutil.NewSeedStore(t)
	data := termJSON(t, []TermGroup{
		{Term: "x", Aliases: []string{"nope"}, Targets: []string{"reviews.metadata:$.nonexistent"}},
	})

	result, err := Import(store, data)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Accepted != 0 || len(result.Rejected) != 1 {
		t.Fatalf("expected exactly 1 rejection, got accepted=%d rejected=%v", result.Accepted, result.Rejected)
	}
}

func TestImport_RejectsMalformedTarget(t *testing.T) {
	store := testutil.NewSeedStore(t)
	data := termJSON(t, []TermGroup{
		{Term: "x", Aliases: []string{"nope"}, Targets: []string{".bad"}},
	})

	result, err := Import(store, data)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Accepted != 0 || len(result.Rejected) != 1 {
		t.Fatalf("expected exactly 1 rejection, got accepted=%d rejected=%v", result.Accepted, result.Rejected)
	}
}

func TestImport_RejectsMissingTermFields(t *testing.T) {
	store := testutil.NewSeedStore(t)
	data := termJSON(t, []TermGroup{
		{Term: "", Aliases: []string{"a"}, Targets: []string{"orgs"}},
		{Term: "no_aliases", Aliases: nil, Targets: []string{"orgs"}},
		{Term: "no_targets", Aliases: []string{"a"}, Targets: nil},
	})

	result, err := Import(store, data)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Accepted != 0 {
		t.Errorf("Accepted = %d, want 0", result.Accepted)
	}
	if len(result.Rejected) != 3 {
		t.Errorf("Rejected count = %d, want 3", len(result.Rejected))
	}
}

func TestImport_DuplicateAliasWithinTerm(t *testing.T) {
	store := testutil.NewSeedStore(t)
	data := termJSON(t, []TermGroup{
		{Term: "x", Aliases: []string{"pull request", "Pull Request"}, Targets: []string{"pull_requests"}},
	})

	result, err := Import(store, data)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Accepted != 1 {
		t.Errorf("Accepted = %d, want 1 (case-insensitive duplicate should be rejected)", result.Accepted)
	}
	if len(result.Rejected) != 1 {
		t.Errorf("Rejected = %v, want 1", result.Rejected)
	}
}

func TestImport_ReimportUpdatesRatherThanDuplicates(t *testing.T) {
	store := testutil.NewSeedStore(t)
	first := termJSON(t, []TermGroup{
		{Term: "pr", Aliases: []string{"pull request"}, Targets: []string{"pull_requests"}},
	})
	if _, err := Import(store, first); err != nil {
		t.Fatalf("first Import: %v", err)
	}

	second := termJSON(t, []TermGroup{
		{Term: "pr_renamed", Aliases: []string{"pull request"}, Targets: []string{"pull_requests"}},
	})
	result, err := Import(store, second)
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if result.Accepted != 1 {
		t.Fatalf("Accepted = %d, want 1", result.Accepted)
	}

	entries, err := List(store)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("List returned %d entries, want 1 (re-import should update, not duplicate)", len(entries))
	}
	if entries[0].Term != "pr_renamed" {
		t.Errorf("term = %q, want pr_renamed (re-import should update the term)", entries[0].Term)
	}
}

func TestList_EmptyWhenNeverImported(t *testing.T) {
	store := testutil.NewSeedStore(t)
	entries, err := List(store)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("List = %v, want empty", entries)
	}
}

func TestGeneratePrompt_IncludesSchemaAndInstructions(t *testing.T) {
	store := testutil.NewSeedStore(t)
	prompt, err := GeneratePrompt(store)
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}

	for _, want := range []string{
		"reviews",                  // actual schema content
		"orgs",                     // actual schema content
		"DO NOT INVENT",            // anti-hallucination rule
		"ASK THE USER",             // interactive-refinement instruction
		"JSON array",               // output format instruction
		"dbctx terminology import", // tells the user how to load results back
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing expected content %q", want)
		}
	}
}
