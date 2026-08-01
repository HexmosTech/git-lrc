package graphengine

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAssetNameMapping(t *testing.T) {
	cases := []struct {
		goos, goarch, want string
	}{
		{"linux", "amd64", "codebase-memory-mcp-linux-amd64-portable.tar.gz"},
		{"linux", "arm64", "codebase-memory-mcp-linux-arm64-portable.tar.gz"},
		{"darwin", "amd64", "codebase-memory-mcp-darwin-amd64.tar.gz"},
		{"darwin", "arm64", "codebase-memory-mcp-darwin-arm64.tar.gz"},
		{"windows", "amd64", "codebase-memory-mcp-windows-amd64.zip"},
		{"windows", "arm64", "codebase-memory-mcp-windows-arm64.zip"},
	}
	for _, c := range cases {
		got, err := AssetName(c.goos, c.goarch)
		if err != nil || got != c.want {
			t.Fatalf("AssetName(%s,%s) = %q, %v; want %q", c.goos, c.goarch, got, err, c.want)
		}
	}
	if _, err := AssetName("plan9", "amd64"); err == nil {
		t.Fatal("expected error for unsupported OS")
	}
	if _, err := AssetName("linux", "386"); err == nil {
		t.Fatal("expected error for unsupported arch")
	}
}

func TestArchiveBinaryMember(t *testing.T) {
	if got := archiveBinaryMember("windows"); got != "codebase-memory-mcp.exe" {
		t.Fatalf("windows member = %q", got)
	}
	if got := archiveBinaryMember("linux"); got != "codebase-memory-mcp" {
		t.Fatalf("linux member = %q", got)
	}
}

func TestParseChecksums(t *testing.T) {
	digest := strings.Repeat("ab", 32)
	data := fmt.Sprintf("%s  file-a.tar.gz\nnot a checksum line\n%s  file-b.zip\n", digest, strings.Repeat("cd", 32))
	sums := parseChecksums([]byte(data))
	if sums["file-a.tar.gz"] != digest {
		t.Fatalf("file-a digest = %q", sums["file-a.tar.gz"])
	}
	if len(sums) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(sums), sums)
	}
}

func writeTarGz(t *testing.T, path string, members map[string][]byte) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range members {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeZip(t *testing.T, path string, members map[string][]byte) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range members {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

func extractTo(t *testing.T, archivePath, assetName, member string) (string, error) {
	t.Helper()
	dst, err := os.CreateTemp(t.TempDir(), "extracted-*")
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	if err := extractBinaryToFile(archivePath, assetName, member, dst); err != nil {
		return "", err
	}
	data, err := os.ReadFile(dst.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data), nil
}

func TestExtractFromTarGz(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "a.tar.gz")
	writeTarGz(t, archive, map[string][]byte{
		"LICENSE":             []byte("license text"),
		"codebase-memory-mcp": []byte("BINARY-PAYLOAD"),
		"../evil":             []byte("traversal attempt"),
		"THIRD_PARTY_NOTICES": []byte("notices"),
	})
	got, err := extractTo(t, archive, "a.tar.gz", "codebase-memory-mcp")
	if err != nil {
		t.Fatal(err)
	}
	if got != "BINARY-PAYLOAD" {
		t.Fatalf("extracted %q", got)
	}
}

func TestExtractFromZip(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "a.zip")
	writeZip(t, archive, map[string][]byte{
		"codebase-memory-mcp.exe": []byte("EXE-PAYLOAD"),
		"install.ps1":             []byte("script"),
	})
	got, err := extractTo(t, archive, "a.zip", "codebase-memory-mcp.exe")
	if err != nil {
		t.Fatal(err)
	}
	if got != "EXE-PAYLOAD" {
		t.Fatalf("extracted %q", got)
	}
}

func TestExtractMissingMember(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "a.tar.gz")
	writeTarGz(t, archive, map[string][]byte{"LICENSE": []byte("only license")})
	if _, err := extractTo(t, archive, "a.tar.gz", "codebase-memory-mcp"); err == nil {
		t.Fatal("expected error for missing member")
	}
}

func TestExtractRejectsPathAlias(t *testing.T) {
	// A member named "sub/codebase-memory-mcp" must not satisfy the flat
	// member lookup.
	archive := filepath.Join(t.TempDir(), "a.tar.gz")
	writeTarGz(t, archive, map[string][]byte{"sub/codebase-memory-mcp": []byte("nested")})
	if _, err := extractTo(t, archive, "a.tar.gz", "codebase-memory-mcp"); err == nil {
		t.Fatal("expected error when only a nested lookalike member exists")
	}
}

// stubDownloads replaces the network seam with an in-memory asset map and
// restores it on cleanup.
func stubDownloads(t *testing.T, assets map[string][]byte) {
	t.Helper()
	orig := downloadAsset
	downloadAsset = func(url string, dst io.Writer, onProgress func(int64, int64)) error {
		for name, content := range assets {
			if strings.HasSuffix(url, "/"+name) {
				_, err := dst.Write(content)
				return err
			}
		}
		return fmt.Errorf("stub has no asset for %s", url)
	}
	t.Cleanup(func() { downloadAsset = orig })
}

func buildTestArchive(t *testing.T, payload []byte) (name string, content []byte) {
	t.Helper()
	assetName, err := AssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skipf("unsupported test platform: %v", err)
	}
	dir := t.TempDir()
	archivePath := filepath.Join(dir, assetName)
	members := map[string][]byte{archiveBinaryMember(runtime.GOOS): payload, "LICENSE": []byte("l")}
	if strings.HasSuffix(assetName, ".zip") {
		writeZip(t, archivePath, members)
	} else {
		writeTarGz(t, archivePath, members)
	}
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	return assetName, data
}

func TestInstallEndToEndWithStubbedDownloads(t *testing.T) {
	payload := []byte("FAKE-ENGINE-BINARY")
	assetName, archiveBytes := buildTestArchive(t, payload)
	sum := sha256.Sum256(archiveBytes)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), assetName)
	stubDownloads(t, map[string][]byte{
		"checksums.txt": []byte(checksums),
		assetName:       archiveBytes,
	})

	dir := t.TempDir()
	res, err := Install(InstallOptions{Dir: dir, SkipSmokeTest: true, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped {
		t.Fatal("expected a real install, got Skipped")
	}
	got, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("installed binary content = %q", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(res.Path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0111 == 0 {
			t.Fatalf("installed binary not executable: %v", info.Mode())
		}
	}
}

func TestInstallRejectsChecksumMismatch(t *testing.T) {
	assetName, archiveBytes := buildTestArchive(t, []byte("payload"))
	checksums := fmt.Sprintf("%s  %s\n", strings.Repeat("00", 32), assetName)
	stubDownloads(t, map[string][]byte{
		"checksums.txt": []byte(checksums),
		assetName:       archiveBytes,
	})

	if _, err := Install(InstallOptions{Dir: t.TempDir(), SkipSmokeTest: true, Force: true}); err == nil {
		t.Fatal("expected checksum mismatch error")
	} else if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstallFailsWhenChecksumEntryMissing(t *testing.T) {
	assetName, archiveBytes := buildTestArchive(t, []byte("payload"))
	stubDownloads(t, map[string][]byte{
		"checksums.txt": []byte("deadbeef  some-other-file\n"),
		assetName:       archiveBytes,
	})
	if _, err := Install(InstallOptions{Dir: t.TempDir(), SkipSmokeTest: true, Force: true}); err == nil {
		t.Fatal("expected missing checksum entry error")
	}
}

func TestUninstallMissingBinaryIsNoop(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := Uninstall(); err != nil {
		t.Fatalf("uninstall of absent binary should be a no-op, got %v", err)
	}
}
