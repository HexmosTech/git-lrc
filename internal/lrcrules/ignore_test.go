package lrcrules

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadIgnorePatternsMissing(t *testing.T) {
	dir := t.TempDir()

	patterns, err := LoadIgnorePatterns(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patterns != nil {
		t.Fatalf("expected nil patterns, got %v", patterns)
	}
}

func TestLoadIgnorePatterns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "ignore"), "# comment\n\nmain.go\n  \n*.log\r\n")

	patterns, err := LoadIgnorePatterns(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"main.go", "*.log"}
	if !reflect.DeepEqual(patterns, want) {
		t.Fatalf("unexpected patterns: got %v, want %v", patterns, want)
	}
}

const sampleTwoFileDiff = `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,3 @@
-old main
+new main
diff --git a/other.go b/other.go
index 3333333..4444444 100644
--- a/other.go
+++ b/other.go
@@ -1,3 +1,3 @@
-old other
+new other
`

func TestFilterDiffNoPatterns(t *testing.T) {
	filtered, excluded := FilterDiff([]byte(sampleTwoFileDiff), nil)
	if string(filtered) != sampleTwoFileDiff {
		t.Fatalf("expected diff unchanged when no patterns given")
	}
	if excluded != nil {
		t.Fatalf("expected no excluded files, got %v", excluded)
	}
}

func TestFilterDiffFullyMatched(t *testing.T) {
	singleFileDiff := `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,3 @@
-old main
+new main
`

	filtered, excluded := FilterDiff([]byte(singleFileDiff), []string{"main.go"})
	if len(filtered) != 0 {
		t.Fatalf("expected empty diff, got %q", string(filtered))
	}
	if !reflect.DeepEqual(excluded, []string{"main.go"}) {
		t.Fatalf("unexpected excluded list: %v", excluded)
	}
}

func TestFilterDiffPartialMatch(t *testing.T) {
	filtered, excluded := FilterDiff([]byte(sampleTwoFileDiff), []string{"main.go"})

	if !reflect.DeepEqual(excluded, []string{"main.go"}) {
		t.Fatalf("unexpected excluded list: %v", excluded)
	}
	if strings.Contains(string(filtered), "main.go") {
		t.Fatalf("filtered diff should not contain main.go section:\n%s", string(filtered))
	}
	if !strings.Contains(string(filtered), "other.go") {
		t.Fatalf("filtered diff should still contain other.go section:\n%s", string(filtered))
	}
}

func TestFilterDiffPreservesPreamble(t *testing.T) {
	diffContent := "\n" + sampleTwoFileDiff

	filtered, excluded := FilterDiff([]byte(diffContent), []string{"main.go"})

	if !reflect.DeepEqual(excluded, []string{"main.go"}) {
		t.Fatalf("unexpected excluded list: %v", excluded)
	}
	if !strings.HasPrefix(string(filtered), "\ndiff --git a/other.go") {
		t.Fatalf("expected preamble to be preserved before other.go section, got:\n%q", string(filtered))
	}
}

func TestFilterDiffEmptyContent(t *testing.T) {
	filtered, excluded := FilterDiff([]byte{}, []string{"main.go"})
	if len(filtered) != 0 {
		t.Fatalf("expected empty diff, got %q", string(filtered))
	}
	if excluded != nil {
		t.Fatalf("expected no excluded files, got %v", excluded)
	}
}

func TestFilterDiffHeadersOnly(t *testing.T) {
	diffContent := "diff --git a/main.go b/main.go\ndiff --git a/other.go b/other.go\n"

	filtered, excluded := FilterDiff([]byte(diffContent), []string{"main.go"})

	if !reflect.DeepEqual(excluded, []string{"main.go"}) {
		t.Fatalf("unexpected excluded list: %v", excluded)
	}
	want := "diff --git a/other.go b/other.go\n"
	if string(filtered) != want {
		t.Fatalf("unexpected filtered diff:\ngot:  %q\nwant: %q", string(filtered), want)
	}
}

func TestFilterDiffGitignorePatternVariety(t *testing.T) {
	diffContent := `diff --git a/build/output.go b/build/output.go
index 1111111..2222222 100644
--- a/build/output.go
+++ b/build/output.go
@@ -1,1 +1,1 @@
-old
+new
diff --git a/build/keep.go b/build/keep.go
index 3333333..4444444 100644
--- a/build/keep.go
+++ b/build/keep.go
@@ -1,1 +1,1 @@
-old
+new
diff --git a/notes.txt b/notes.txt
index 5555555..6666666 100644
--- a/notes.txt
+++ b/notes.txt
@@ -1,1 +1,1 @@
-old
+new
`

	// "build/" excludes everything under build/, "!build/keep.go" carves out
	// an exception, and "/notes.txt" anchors the pattern to the repo root.
	patterns := []string{"build/", "!build/keep.go", "/notes.txt"}

	filtered, excluded := FilterDiff([]byte(diffContent), patterns)

	wantExcluded := []string{"build/output.go", "notes.txt"}
	if !reflect.DeepEqual(excluded, wantExcluded) {
		t.Fatalf("unexpected excluded list: got %v, want %v", excluded, wantExcluded)
	}
	if strings.Contains(string(filtered), "build/output.go") || strings.Contains(string(filtered), "notes.txt") {
		t.Fatalf("filtered diff should not contain excluded sections:\n%s", string(filtered))
	}
	if !strings.Contains(string(filtered), "build/keep.go") {
		t.Fatalf("filtered diff should still contain build/keep.go section:\n%s", string(filtered))
	}
}
