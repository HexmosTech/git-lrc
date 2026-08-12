package dbctx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerminologyPrompt(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	prompt, err := idx.TerminologyPrompt()
	if err != nil {
		t.Fatalf("TerminologyPrompt: %v", err)
	}
	if prompt == "" {
		t.Fatal("TerminologyPrompt returned empty string")
	}
	if !strings.Contains(prompt, "orgs") {
		t.Error("prompt should embed the actual schema (missing 'orgs')")
	}
}

func TestImportTerminology_JSONBytes(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	data := []byte(`[{"term":"pr","aliases":["pull request"],"targets":["pull_requests"]}]`)
	result, err := idx.ImportTerminology(data)
	if err != nil {
		t.Fatalf("ImportTerminology: %v", err)
	}
	if result.Accepted != 1 {
		t.Fatalf("Accepted = %d, want 1 (rejected: %v)", result.Accepted, result.Rejected)
	}
}

func TestImportTerminology_JSONStringCastToBytes(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	// The documented idiom for "supply a JSON string": cast it to []byte.
	jsonString := `[{"term":"pr","aliases":["pull request"],"targets":["pull_requests"]}]`
	result, err := idx.ImportTerminology([]byte(jsonString))
	if err != nil {
		t.Fatalf("ImportTerminology: %v", err)
	}
	if result.Accepted != 1 {
		t.Fatalf("Accepted = %d, want 1", result.Accepted)
	}
}

func TestImportTerminologyFile(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "terminology.json")
	data := []byte(`[{"term":"pr","aliases":["pull request"],"targets":["pull_requests"]}]`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write terminology file: %v", err)
	}

	result, err := idx.ImportTerminologyFile(path)
	if err != nil {
		t.Fatalf("ImportTerminologyFile: %v", err)
	}
	if result.Accepted != 1 {
		t.Fatalf("Accepted = %d, want 1 (rejected: %v)", result.Accepted, result.Rejected)
	}
}

func TestImportTerminologyFile_MissingFile(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	_, err := idx.ImportTerminologyFile(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err == nil {
		t.Fatal("expected error for a nonexistent file")
	}
}

func TestImportTerminologyGroups(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	groups := []TerminologyGroup{
		{
			Term:    "pr",
			Aliases: []string{"pull request", "pull requests"},
			Targets: []string{"pull_requests"},
		},
		{
			Term:    "org_plan",
			Aliases: []string{"subscription tier"},
			Targets: []string{"orgs.plan"},
		},
	}

	result, err := idx.ImportTerminologyGroups(groups)
	if err != nil {
		t.Fatalf("ImportTerminologyGroups: %v", err)
	}
	if result.Accepted != 3 {
		t.Fatalf("Accepted = %d, want 3 (rejected: %v)", result.Accepted, result.Rejected)
	}

	entries, err := idx.Terminology()
	if err != nil {
		t.Fatalf("Terminology: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("Terminology() returned %d entries, want 3", len(entries))
	}
}

func TestImportTerminologyGroups_RejectsInvalidTarget(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	groups := []TerminologyGroup{
		{Term: "x", Aliases: []string{"widget"}, Targets: []string{"not_a_real_table"}},
	}
	result, err := idx.ImportTerminologyGroups(groups)
	if err != nil {
		t.Fatalf("ImportTerminologyGroups: %v", err)
	}
	if result.Accepted != 0 || len(result.Rejected) != 1 {
		t.Fatalf("expected exactly 1 rejection, got accepted=%d rejected=%v", result.Accepted, result.Rejected)
	}
}

func TestTerminology_EmptyBeforeImport(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	entries, err := idx.Terminology()
	if err != nil {
		t.Fatalf("Terminology: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("Terminology() = %v, want empty before any import", entries)
	}
}

// TestTerminology_AllThreeImportPathsAreEquivalent verifies
// ImportTerminology, ImportTerminologyFile, and ImportTerminologyGroups
// all persist the same result for equivalent input — they're three ways
// to supply the same data, not three different behaviors.
func TestTerminology_AllThreeImportPathsAreEquivalent(t *testing.T) {
	jsonData := []byte(`[{"term":"pr","aliases":["pull request"],"targets":["pull_requests"]}]`)
	groups := []TerminologyGroup{
		{Term: "pr", Aliases: []string{"pull request"}, Targets: []string{"pull_requests"}},
	}

	// Subtests so each Index's deferred Close() runs before the next one
	// opens — NewTestStore uses a shared-cache in-memory SQLite DSN, and
	// leaving multiple open at once collides on row IDs.
	var accepted [3]int

	t.Run("bytes", func(t *testing.T) {
		idx := newTestIndex(t)
		defer idx.Close()
		r, err := idx.ImportTerminology(jsonData)
		if err != nil {
			t.Fatalf("ImportTerminology: %v", err)
		}
		accepted[0] = r.Accepted
	})

	t.Run("file", func(t *testing.T) {
		idx := newTestIndex(t)
		defer idx.Close()
		path := filepath.Join(t.TempDir(), "t.json")
		os.WriteFile(path, jsonData, 0o644)
		r, err := idx.ImportTerminologyFile(path)
		if err != nil {
			t.Fatalf("ImportTerminologyFile: %v", err)
		}
		accepted[1] = r.Accepted
	})

	t.Run("groups", func(t *testing.T) {
		idx := newTestIndex(t)
		defer idx.Close()
		r, err := idx.ImportTerminologyGroups(groups)
		if err != nil {
			t.Fatalf("ImportTerminologyGroups: %v", err)
		}
		accepted[2] = r.Accepted
	})

	if accepted[0] != accepted[1] || accepted[1] != accepted[2] {
		t.Errorf("Accepted differs across import paths: bytes=%d file=%d groups=%d",
			accepted[0], accepted[1], accepted[2])
	}
}
