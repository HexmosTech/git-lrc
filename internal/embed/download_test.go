package embed

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func makeTarGz(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.tgz")

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func makeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.zip")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	zw.Close()

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractTarGzMember(t *testing.T) {
	archive := makeTarGz(t, map[string]string{
		"pkg/lib/libonnxruntime.so.1.29.0": "fake shared library bytes",
		"pkg/README.md":                    "irrelevant",
	})
	dest := filepath.Join(t.TempDir(), "out.so")

	if err := extractMember(archive, archiveTarGz, "pkg/lib/libonnxruntime.so.1.29.0", dest); err != nil {
		t.Fatalf("extractMember: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "fake shared library bytes" {
		t.Errorf("extracted content = %q", got)
	}
}

func TestExtractTarGzMember_NotFound(t *testing.T) {
	archive := makeTarGz(t, map[string]string{"a": "b"})
	dest := filepath.Join(t.TempDir(), "out.so")
	if err := extractMember(archive, archiveTarGz, "nonexistent", dest); err == nil {
		t.Fatal("expected error for missing member")
	}
}

func TestExtractZipMember(t *testing.T) {
	archive := makeZip(t, map[string]string{
		"pkg/lib/onnxruntime.dll": "fake windows dll bytes",
		"pkg/README.md":           "irrelevant",
	})
	dest := filepath.Join(t.TempDir(), "out.dll")

	if err := extractMember(archive, archiveZip, "pkg/lib/onnxruntime.dll", dest); err != nil {
		t.Fatalf("extractMember: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "fake windows dll bytes" {
		t.Errorf("extracted content = %q", got)
	}
}

func TestExtractZipMember_NotFound(t *testing.T) {
	archive := makeZip(t, map[string]string{"a": "b"})
	dest := filepath.Join(t.TempDir(), "out.dll")
	if err := extractMember(archive, archiveZip, "nonexistent", dest); err == nil {
		t.Fatal("expected error for missing member")
	}
}

func TestCurrentPlatformRuntimeAsset_KnownPlatformsPinned(t *testing.T) {
	// Sanity-check the pinned asset table itself, independent of the host
	// platform running the test.
	for key, a := range runtimeAssets {
		if a.URL == "" || a.SHA256 == "" || a.Size == 0 || a.MemberPath == "" || a.LocalName == "" {
			t.Errorf("runtimeAssets[%s] has an empty required field: %+v", key, a)
		}
		if len(a.SHA256) != 64 {
			t.Errorf("runtimeAssets[%s].SHA256 length = %d, want 64 hex chars", key, len(a.SHA256))
		}
	}
}
