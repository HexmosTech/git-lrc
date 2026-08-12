package embed

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCacheDir_EnvOverride(t *testing.T) {
	t.Setenv(CacheDirEnv, "/custom/cache/path")
	dir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if dir != "/custom/cache/path" {
		t.Errorf("CacheDir() = %q, want /custom/cache/path", dir)
	}
}

func TestCacheDir_DefaultsUnderHomeDir(t *testing.T) {
	t.Setenv(CacheDirEnv, "")
	dir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	want := filepath.Join(home, ".dbctx")
	if dir != want {
		t.Errorf("CacheDir() = %q, want %q", dir, want)
	}
}

func TestDownloadToFile_Success(t *testing.T) {
	content := []byte("hello dbctx semantic index")
	sum := sha256.Sum256(content)
	hexSum := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "out.bin")

	if err := downloadToFile(srv.URL, dest, hexSum, int64(len(content)), "test asset", nil); err != nil {
		t.Fatalf("downloadToFile: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("downloaded content mismatch: got %q, want %q", got, content)
	}
}

func TestDownloadToFile_ChecksumMismatch(t *testing.T) {
	content := []byte("corrupted or wrong content")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "out.bin")

	err := downloadToFile(srv.URL, dest, "0000000000000000000000000000000000000000000000000000000000000000", int64(len(content)), "test asset", nil)
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Error("destination file should not exist after checksum failure")
	}
}

func TestDownloadToFile_SizeMismatch(t *testing.T) {
	content := []byte("short")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "out.bin")

	err := downloadToFile(srv.URL, dest, "", 99999, "test asset", nil)
	if err == nil {
		t.Fatal("expected size mismatch error, got nil")
	}
}

func TestDownloadToFile_ProgressCallback(t *testing.T) {
	content := make([]byte, 500*1024) // exceed one read-buffer chunk
	for i := range content {
		content[i] = byte(i)
	}
	sum := sha256.Sum256(content)
	hexSum := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "out.bin")

	var calls int
	var lastDownloaded int64
	err := downloadToFile(srv.URL, dest, hexSum, int64(len(content)), "test asset", func(label string, downloaded, total int64) {
		calls++
		lastDownloaded = downloaded
		if label != "test asset" {
			t.Errorf("progress label = %q, want %q", label, "test asset")
		}
	})
	if err != nil {
		t.Fatalf("downloadToFile: %v", err)
	}
	if calls == 0 {
		t.Error("progress callback was never called")
	}
	if lastDownloaded != int64(len(content)) {
		t.Errorf("final downloaded = %d, want %d", lastDownloaded, len(content))
	}
}

func TestFileMatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.bin")
	if fileMatches(path, 0) {
		t.Error("fileMatches should be false for nonexistent file")
	}
	if err := os.WriteFile(path, []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !fileMatches(path, 5) {
		t.Error("fileMatches should be true for matching size")
	}
	if fileMatches(path, 6) {
		t.Error("fileMatches should be false for mismatched size")
	}
	if !fileMatches(path, 0) {
		t.Error("fileMatches with wantSize=0 should only check existence")
	}
}

func TestEnsureRuntimeLibrary_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	fakeLib := filepath.Join(dir, "libonnxruntime.so")
	if err := os.WriteFile(fakeLib, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(OnnxRuntimeLibEnv, fakeLib)

	got, err := EnsureRuntimeLibrary(nil)
	if err != nil {
		t.Fatalf("EnsureRuntimeLibrary: %v", err)
	}
	if got != fakeLib {
		t.Errorf("EnsureRuntimeLibrary() = %q, want %q", got, fakeLib)
	}
}

func TestEnsureRuntimeLibrary_EnvOverrideMissing(t *testing.T) {
	t.Setenv(OnnxRuntimeLibEnv, "/nonexistent/path/libonnxruntime.so")
	_, err := EnsureRuntimeLibrary(nil)
	if err == nil {
		t.Fatal("expected error for missing override path")
	}
}
