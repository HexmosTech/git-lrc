package appcore

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveReviewRepositoryPathNormalizesRelativePath(t *testing.T) {
	relativePath := filepath.Join("tmp", "repo")
	want, err := filepath.Abs(relativePath)
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}

	got, err := resolveReviewRepositoryPath(func() (string, error) {
		return relativePath, nil
	})
	if err != nil {
		t.Fatalf("resolveReviewRepositoryPath() error = %v", err)
	}

	if got != filepath.Clean(want) {
		t.Fatalf("resolveReviewRepositoryPath() = %q, want %q", got, filepath.Clean(want))
	}
}

func TestResolveReviewRepositoryPathRejectsEmptyPath(t *testing.T) {
	_, err := resolveReviewRepositoryPath(func() (string, error) {
		return "   ", nil
	})
	if err == nil {
		t.Fatal("resolveReviewRepositoryPath() error = nil, want non-nil")
	}
}

func TestResolveReviewRepositoryNameUsesExplicitOverride(t *testing.T) {
	got, err := resolveReviewRepositoryName("custom-name", "/tmp/example", func() (string, error) {
		return "/ignored", nil
	})
	if err != nil {
		t.Fatalf("resolveReviewRepositoryName() error = %v", err)
	}
	if got != "custom-name" {
		t.Fatalf("resolveReviewRepositoryName() = %q, want %q", got, "custom-name")
	}
}

func TestResolveReviewRepositoryNameUsesRepositoryPathBase(t *testing.T) {
	got, err := resolveReviewRepositoryName("", filepath.Join("/tmp", "example-repo"), func() (string, error) {
		return "/ignored", nil
	})
	if err != nil {
		t.Fatalf("resolveReviewRepositoryName() error = %v", err)
	}
	if got != "example-repo" {
		t.Fatalf("resolveReviewRepositoryName() = %q, want %q", got, "example-repo")
	}
}

func TestResolveReviewRepositoryNameFallsBackToNormalizedWorkingDir(t *testing.T) {
	relativeDir := filepath.Join("nested", "worktree")
	wantAbs, err := filepath.Abs(relativeDir)
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}

	got, err := resolveReviewRepositoryName("", "", func() (string, error) {
		return relativeDir, nil
	})
	if err != nil {
		t.Fatalf("resolveReviewRepositoryName() error = %v", err)
	}

	if got != filepath.Base(filepath.Clean(wantAbs)) {
		t.Fatalf("resolveReviewRepositoryName() = %q, want %q", got, filepath.Base(filepath.Clean(wantAbs)))
	}
}

func TestResolveReviewRepositoryNameReturnsWorkingDirError(t *testing.T) {
	wantErr := errors.New("getwd failed")
	_, err := resolveReviewRepositoryName("", "", func() (string, error) {
		return "", wantErr
	})
	if err == nil {
		t.Fatal("resolveReviewRepositoryName() error = nil, want non-nil")
	}
}

func TestResolveRepoRemoteURL(t *testing.T) {
	if got := resolveRepoRemoteURL("https://example.com/org/repo.git", ""); got != "https://example.com/org/repo.git" {
		t.Fatalf("explicit remote = %q", got)
	}
	if got := resolveRepoRemoteURL("https://user:secret-token@example.com/org/repo.git", ""); got != "https://example.com/org/repo.git" {
		t.Fatalf("credential-bearing remote was not sanitized: %q", got)
	}

	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "remote", "add", "origin", "git@example.com:org/repo.git").CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v: %s", err, out)
	}
	if got := resolveRepoRemoteURL("", dir); got != "git@example.com:org/repo.git" {
		t.Fatalf("origin remote = %q", got)
	}
	if got := resolveRepoRemoteURL("", t.TempDir()); got != "" {
		t.Fatalf("remote-less repo = %q, want empty", got)
	}
}
