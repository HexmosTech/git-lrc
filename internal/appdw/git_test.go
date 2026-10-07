package appdw

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	_ = os.WriteFile(filepath.Join(dir, "first.txt"), []byte("first"), 0o644)
	run("add", "first.txt")
	run("commit", "-q", "-m", "first")
	run("tag", "v1")

	_ = os.WriteFile(filepath.Join(dir, "second.txt"), []byte("second"), 0o644)
	run("add", "second.txt")
	run("commit", "-q", "-m", "second")
	return dir
}

func TestMaterializeRefUsesWorktreeForHead(t *testing.T) {
	dir := initTestRepo(t)
	got, cleanup, err := materializeRef(dir, "HEAD")
	if err != nil {
		t.Fatalf("materializeRef(HEAD) error = %v", err)
	}
	cleanup()
	if filepath.Clean(got) != filepath.Clean(dir) {
		t.Errorf("materializeRef(HEAD) = %q, want the repo dir %q", got, dir)
	}
}

func TestMaterializeRefArchivesNonHeadRef(t *testing.T) {
	dir := initTestRepo(t)
	got, cleanup, err := materializeRef(dir, "v1")
	if err != nil {
		t.Fatalf("materializeRef(v1) error = %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(filepath.Join(got, "first.txt")); err != nil {
		t.Errorf("v1 archive missing first.txt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(got, "second.txt")); !os.IsNotExist(err) {
		t.Errorf("v1 archive should not contain second.txt (err=%v)", err)
	}
}

func TestMaterializeRefMissingRef(t *testing.T) {
	dir := initTestRepo(t)
	_, cleanup, err := materializeRef(dir, "does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing ref")
	}
	cleanup()
}
