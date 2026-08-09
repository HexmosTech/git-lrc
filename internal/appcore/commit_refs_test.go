package appcore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HexmosTech/git-lrc/internal/reviewopts"
)

// initTestRepoWithCommits creates a throwaway git repo with two commits and
// chdirs the test process into it (restoring the original cwd on cleanup),
// since RunGitCommand shells out to `git` in the process's current
// directory. Returns the two commit SHAs in commit order.
func initTestRepoWithCommits(t *testing.T) (first, second string) {
	t.Helper()

	dir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(origWD); err != nil {
			t.Fatalf("failed to restore cwd: %v", err)
		}
	})

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	run("init", "-q")
	run("config", "commit.gpgsign", "false")

	// Built via plumbing (write-tree/commit-tree), not `git commit` --
	// this machine has a global LiveReview commit gate installed
	// (dogfooding git-lrc itself) that intercepts the `commit` porcelain
	// command specifically; commit-tree bypasses it entirely and is a more
	// direct way to build fixture commits for a test like this anyway.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	run("add", "a.txt")
	tree1 := run("write-tree")
	first = commitTree(t, dir, tree1, "first commit")

	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	run("add", "b.txt")
	tree2 := run("write-tree")
	second = commitTree(t, dir, tree2, "second commit", "-p", first)

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir into test repo: %v", err)
	}
	return first, second
}

// commitTree runs `git commit-tree <tree> [extraArgs...]` with the given
// message piped on stdin, returning the resulting commit SHA.
func commitTree(t *testing.T, dir, tree, message string, extraArgs ...string) string {
	t.Helper()
	args := append([]string{"commit-tree", tree}, extraArgs...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	cmd.Stdin = strings.NewReader(message)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestResolveCommitRefs_SingleCommit(t *testing.T) {
	first, second := initTestRepoWithCommits(t)
	_ = first

	refs, err := resolveCommitRefs(reviewopts.Options{DiffSource: "commit", CommitVal: second})
	if err != nil {
		t.Fatalf("resolveCommitRefs returned error: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d: %+v", len(refs), refs)
	}
	if refs[0].Ref != second || refs[0].Type != "commit" {
		t.Errorf("unexpected ref: %+v (want sha=%s type=commit)", refs[0], second)
	}
}

func TestResolveCommitRefs_Range(t *testing.T) {
	first, second := initTestRepoWithCommits(t)

	refs, err := resolveCommitRefs(reviewopts.Options{DiffSource: "range", RangeVal: first + ".." + second})
	if err != nil {
		t.Fatalf("resolveCommitRefs returned error: %v", err)
	}

	var gotRange bool
	commits := map[string]bool{}
	for _, r := range refs {
		if r.Type == "range" {
			gotRange = true
			if r.Ref != first+".."+second {
				t.Errorf("range ref mismatch: %q", r.Ref)
			}
		} else {
			commits[r.Ref] = true
		}
	}
	if !gotRange {
		t.Error("expected a literal range ref in the result")
	}
	if !commits[second] {
		t.Errorf("expected expanded commits to include %s, got %+v", second, refs)
	}
	// `first..second` in git log/rev-list semantics excludes `first` itself.
	if commits[first] {
		t.Errorf("range a..b should not include a itself, got %+v", refs)
	}
}

func TestResolveCommitRefs_CommitFlagWithRangeSyntax(t *testing.T) {
	first, second := initTestRepoWithCommits(t)

	// --commit accepts a "a..b" value too (existing collectDiffWithOptions
	// behavior); resolveCommitRefs must expand it the same way --range does.
	refs, err := resolveCommitRefs(reviewopts.Options{DiffSource: "commit", CommitVal: first + ".." + second})
	if err != nil {
		t.Fatalf("resolveCommitRefs returned error: %v", err)
	}
	if len(refs) < 2 {
		t.Fatalf("expected at least a range ref + one commit ref, got %+v", refs)
	}
}

func TestResolveCommitRefs_NoCommitYet(t *testing.T) {
	for _, ds := range []string{"staged", "working", "file"} {
		refs, err := resolveCommitRefs(reviewopts.Options{DiffSource: ds})
		if err != nil {
			t.Fatalf("resolveCommitRefs(%s) returned error: %v", ds, err)
		}
		if refs != nil {
			t.Errorf("resolveCommitRefs(%s) = %+v, want nil (no commit exists yet)", ds, refs)
		}
	}
}
