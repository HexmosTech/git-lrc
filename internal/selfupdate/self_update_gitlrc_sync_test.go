package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBinariesMatch_Identical(t *testing.T) {
	tmpDir := t.TempDir()
	a := filepath.Join(tmpDir, "lrc")
	b := filepath.Join(tmpDir, "git-lrc")
	content := []byte("same content")
	if err := os.WriteFile(a, content, 0755); err != nil {
		t.Fatalf("failed to write %s: %v", a, err)
	}
	if err := os.WriteFile(b, content, 0755); err != nil {
		t.Fatalf("failed to write %s: %v", b, err)
	}

	ok, err := binariesMatch(a, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected identical files to match")
	}
}

func TestBinariesMatch_Different(t *testing.T) {
	tmpDir := t.TempDir()
	a := filepath.Join(tmpDir, "lrc")
	b := filepath.Join(tmpDir, "git-lrc")
	if err := os.WriteFile(a, []byte("new version content"), 0755); err != nil {
		t.Fatalf("failed to write %s: %v", a, err)
	}
	if err := os.WriteFile(b, []byte("stale old content"), 0755); err != nil {
		t.Fatalf("failed to write %s: %v", b, err)
	}

	ok, err := binariesMatch(a, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected different files not to match")
	}
}

// TestResyncGitLRCBinary_RepairsDrift is the regression test for the
// reported bug: a git-lrc binary that has drifted from lrc (e.g. because a
// previous sync step silently failed) gets repaired, and the repair is
// itself verified by checksum rather than just trusted.
func TestResyncGitLRCBinary_RepairsDrift(t *testing.T) {
	tmpDir := t.TempDir()
	lrcPath := filepath.Join(tmpDir, "lrc")
	gitLRCPath := filepath.Join(tmpDir, "git-lrc")

	if err := os.WriteFile(lrcPath, []byte("v0.6.0 binary content"), 0755); err != nil {
		t.Fatalf("failed to write lrc: %v", err)
	}
	if err := os.WriteFile(gitLRCPath, []byte("v0.5.12 stale binary content"), 0755); err != nil {
		t.Fatalf("failed to write git-lrc: %v", err)
	}

	if err := resyncBinary(lrcPath, gitLRCPath, false); err != nil {
		t.Fatalf("resyncBinary failed: %v", err)
	}

	ok, err := binariesMatch(lrcPath, gitLRCPath)
	if err != nil {
		t.Fatalf("unexpected error verifying resync: %v", err)
	}
	if !ok {
		t.Fatal("expected git-lrc to match lrc after resync")
	}

	info, err := os.Stat(gitLRCPath)
	if err != nil {
		t.Fatalf("failed to stat resynced git-lrc: %v", err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Fatal("expected resynced git-lrc to remain executable")
	}
}

func TestResyncGitLRCBinary_MissingGitLRC(t *testing.T) {
	tmpDir := t.TempDir()
	lrcPath := filepath.Join(tmpDir, "lrc")
	gitLRCPath := filepath.Join(tmpDir, "git-lrc") // does not exist yet

	if err := os.WriteFile(lrcPath, []byte("fresh content"), 0755); err != nil {
		t.Fatalf("failed to write lrc: %v", err)
	}

	if err := resyncBinary(lrcPath, gitLRCPath, false); err != nil {
		t.Fatalf("resyncBinary failed: %v", err)
	}

	ok, err := binariesMatch(lrcPath, gitLRCPath)
	if err != nil {
		t.Fatalf("unexpected error verifying resync: %v", err)
	}
	if !ok {
		t.Fatal("expected git-lrc to be created matching lrc")
	}
}

// TestEnsureBinariesSynced_LRCNewer is the reported bug's scenario: lrc was
// updated (newer mtime), git-lrc is stale. The older git-lrc must be
// resynced to match lrc, not the other way around.
func TestEnsureBinariesSynced_LRCNewer(t *testing.T) {
	tmpDir := t.TempDir()
	lrcPath := filepath.Join(tmpDir, "lrc")
	gitLRCPath := filepath.Join(tmpDir, "git-lrc")

	oldTime := time.Now().Add(-1 * time.Hour)
	if err := os.WriteFile(gitLRCPath, []byte("v0.5.12 stale content"), 0755); err != nil {
		t.Fatalf("failed to write git-lrc: %v", err)
	}
	if err := os.Chtimes(gitLRCPath, oldTime, oldTime); err != nil {
		t.Fatalf("failed to set git-lrc mtime: %v", err)
	}
	if err := os.WriteFile(lrcPath, []byte("v0.6.0 fresh content"), 0755); err != nil {
		t.Fatalf("failed to write lrc: %v", err)
	}

	if err := ensureBinariesSynced(lrcPath, gitLRCPath, false); err != nil {
		t.Fatalf("ensureBinariesSynced failed: %v", err)
	}

	got, err := os.ReadFile(gitLRCPath)
	if err != nil {
		t.Fatalf("failed to read git-lrc: %v", err)
	}
	if string(got) != "v0.6.0 fresh content" {
		t.Fatalf("expected git-lrc to be updated to lrc's content, got %q", got)
	}
}

// TestEnsureBinariesSynced_GitLRCNewer is the reverse of the reported bug -
// git-lrc was updated more recently than lrc. Syncing must follow whichever
// file is actually newer, not always overwrite git-lrc with lrc: a
// name-fixed direction would silently downgrade the newer file here.
func TestEnsureBinariesSynced_GitLRCNewer(t *testing.T) {
	tmpDir := t.TempDir()
	lrcPath := filepath.Join(tmpDir, "lrc")
	gitLRCPath := filepath.Join(tmpDir, "git-lrc")

	oldTime := time.Now().Add(-1 * time.Hour)
	if err := os.WriteFile(lrcPath, []byte("v0.6.0 stale content"), 0755); err != nil {
		t.Fatalf("failed to write lrc: %v", err)
	}
	if err := os.Chtimes(lrcPath, oldTime, oldTime); err != nil {
		t.Fatalf("failed to set lrc mtime: %v", err)
	}
	if err := os.WriteFile(gitLRCPath, []byte("v0.6.1 fresh content"), 0755); err != nil {
		t.Fatalf("failed to write git-lrc: %v", err)
	}

	if err := ensureBinariesSynced(lrcPath, gitLRCPath, false); err != nil {
		t.Fatalf("ensureBinariesSynced failed: %v", err)
	}

	got, err := os.ReadFile(lrcPath)
	if err != nil {
		t.Fatalf("failed to read lrc: %v", err)
	}
	if string(got) != "v0.6.1 fresh content" {
		t.Fatalf("expected lrc to be updated to git-lrc's newer content, got %q", got)
	}
}

// TestEnsureBinariesSynced_EqualSizeDifferentContent is the exact edge case
// that surfaced while writing these tests: two different builds can
// coincidentally land on the same byte count, which would fool a size-only
// "already in sync" check into skipping a real resync.
func TestEnsureBinariesSynced_EqualSizeDifferentContent(t *testing.T) {
	tmpDir := t.TempDir()
	lrcPath := filepath.Join(tmpDir, "lrc")
	gitLRCPath := filepath.Join(tmpDir, "git-lrc")

	staleContent := "AAAAAAAAAAAAAAAAAAAA" // same length as freshContent, deliberately
	freshContent := "BBBBBBBBBBBBBBBBBBBB"
	if len(staleContent) != len(freshContent) {
		t.Fatalf("test fixture sizes must match to exercise this edge case: %d vs %d", len(staleContent), len(freshContent))
	}

	oldTime := time.Now().Add(-1 * time.Hour)
	if err := os.WriteFile(gitLRCPath, []byte(staleContent), 0755); err != nil {
		t.Fatalf("failed to write git-lrc: %v", err)
	}
	if err := os.Chtimes(gitLRCPath, oldTime, oldTime); err != nil {
		t.Fatalf("failed to set git-lrc mtime: %v", err)
	}
	if err := os.WriteFile(lrcPath, []byte(freshContent), 0755); err != nil {
		t.Fatalf("failed to write lrc: %v", err)
	}

	if err := ensureBinariesSynced(lrcPath, gitLRCPath, false); err != nil {
		t.Fatalf("ensureBinariesSynced failed: %v", err)
	}

	got, err := os.ReadFile(gitLRCPath)
	if err != nil {
		t.Fatalf("failed to read git-lrc: %v", err)
	}
	if string(got) != freshContent {
		t.Fatalf("expected git-lrc to be resynced despite matching size, got %q", got)
	}
}

func TestEnsureBinariesSynced_AlreadyInSync(t *testing.T) {
	tmpDir := t.TempDir()
	lrcPath := filepath.Join(tmpDir, "lrc")
	gitLRCPath := filepath.Join(tmpDir, "git-lrc")
	content := []byte("identical content")
	if err := os.WriteFile(lrcPath, content, 0755); err != nil {
		t.Fatalf("failed to write lrc: %v", err)
	}
	if err := os.WriteFile(gitLRCPath, content, 0755); err != nil {
		t.Fatalf("failed to write git-lrc: %v", err)
	}

	if err := ensureBinariesSynced(lrcPath, gitLRCPath, false); err != nil {
		t.Fatalf("ensureBinariesSynced failed: %v", err)
	}
}
