package semantic

import (
	"strings"
	"testing"

	"github.com/shrsv/dbctx/internal/testutil"
)

func TestCollectCandidates_TableObjectIncludesColumnsAndValues(t *testing.T) {
	store := testutil.NewSeedStore(t)

	candidates, err := collectCandidates(store)
	if err != nil {
		t.Fatalf("collectCandidates: %v", err)
	}

	var orgsTable *candidate
	for i := range candidates {
		if candidates[i].Kind == KindTable && candidates[i].TableName == "orgs" {
			orgsTable = &candidates[i]
		}
	}
	if orgsTable == nil {
		t.Fatal("no table object for 'orgs'")
	}
	if !strings.Contains(orgsTable.Text, "plan") || !strings.Contains(orgsTable.Text, "tier") {
		t.Errorf("orgs table text missing column names: %q", orgsTable.Text)
	}
	if !strings.Contains(orgsTable.Text, "enterprise") {
		t.Errorf("orgs table text missing representative value 'enterprise': %q", orgsTable.Text)
	}
}

func TestCollectCandidates_TableObjectIncludesFKTargets(t *testing.T) {
	store := testutil.NewSeedStore(t)

	candidates, err := collectCandidates(store)
	if err != nil {
		t.Fatalf("collectCandidates: %v", err)
	}

	var reviewsTable *candidate
	for i := range candidates {
		if candidates[i].Kind == KindTable && candidates[i].TableName == "reviews" {
			reviewsTable = &candidates[i]
		}
	}
	if reviewsTable == nil {
		t.Fatal("no table object for 'reviews'")
	}
	if !strings.Contains(reviewsTable.Text, "orgs") || !strings.Contains(reviewsTable.Text, "pull_requests") {
		t.Errorf("reviews table text missing FK-related tables: %q", reviewsTable.Text)
	}
}

func TestCollectCandidates_ColumnObjects_OnlyMeaningfulColumns(t *testing.T) {
	store := testutil.NewSeedStore(t)

	candidates, err := collectCandidates(store)
	if err != nil {
		t.Fatalf("collectCandidates: %v", err)
	}

	byIdent := make(map[string]bool)
	for _, c := range candidates {
		if c.Kind == KindColumn {
			byIdent[c.TableName+"."+colNameFromText(c.Text)] = true
		}
	}

	// status is state-like -> should get its own column object.
	if !containsColumnObject(candidates, "reviews", "status") {
		t.Error("expected a column object for reviews.status (state-like)")
	}
	// org_id is a FK column -> should get its own column object.
	if !containsColumnObject(candidates, "reviews", "org_id") {
		t.Error("expected a column object for reviews.org_id (FK)")
	}
	// title is a plain text column with no state/categorical/FK signal ->
	// should NOT get its own column object (kept out of the corpus to
	// avoid noise; still covered by pull_requests' table-level text).
	if containsColumnObject(candidates, "pull_requests", "title") {
		t.Error("did not expect a dedicated column object for pull_requests.title (no state/categorical/FK signal)")
	}
}

func containsColumnObject(candidates []candidate, table, column string) bool {
	for _, c := range candidates {
		if c.Kind == KindColumn && c.TableName == table && strings.HasPrefix(c.Text, table+"."+column+"\n") {
			return true
		}
	}
	return false
}

func colNameFromText(text string) string {
	// first line is "table.column"
	line, _, _ := strings.Cut(text, "\n")
	_, col, ok := strings.Cut(line, ".")
	if !ok {
		return line
	}
	return col
}

func TestCollectCandidates_JSONBPathObjects_OnlyPathsWithSamples(t *testing.T) {
	store := testutil.NewSeedStore(t)

	candidates, err := collectCandidates(store)
	if err != nil {
		t.Fatalf("collectCandidates: %v", err)
	}

	var paths []string
	for _, c := range candidates {
		if c.Kind == KindJSONBPath {
			paths = append(paths, c.Text)
		}
	}
	if len(paths) == 0 {
		t.Fatal("expected at least one jsonb_path object from the reviews.metadata fixture")
	}

	joined := strings.Join(paths, "\n---\n")
	if !strings.Contains(joined, "$.provider") {
		t.Errorf("expected $.provider (has sample_values) to be embedded: %s", joined)
	}
	if strings.Contains(joined, "$.tags\n") {
		t.Errorf("$.tags has no sample_values in the fixture and should be excluded: %s", joined)
	}
}

func TestSpacedWords(t *testing.T) {
	tests := map[string]string{
		"pull_requests":     "pull requests",
		"$.repository.name": "repository name",
		"orders":            "orders",
		"labels[].name":     "labels name",
	}
	for in, want := range tests {
		if got := spacedWords(in); got != want {
			t.Errorf("spacedWords(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSampleValues(t *testing.T) {
	got := parseSampleValues("github(500), gitlab(200)")
	want := []string{"github", "gitlab"}
	if len(got) != len(want) {
		t.Fatalf("parseSampleValues = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseSampleValues[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCandidateHash_ChangesWithText(t *testing.T) {
	c1 := candidate{Text: "orders"}
	c2 := candidate{Text: "orders updated"}
	if c1.hash() == c2.hash() {
		t.Error("different text should produce different hashes")
	}
	c3 := candidate{Text: "orders"}
	if c1.hash() != c3.hash() {
		t.Error("identical text should produce identical hashes")
	}
}
