package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyDownloadedBinarySHA256_Success(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "lrc")
	content := []byte("hello lrc")
	if err := os.WriteFile(binPath, content, 0644); err != nil {
		t.Fatalf("failed to write temp binary: %v", err)
	}

	// sha256("hello lrc")
	const expected = "870a3b2e2ed8f4ae1f7c77d8b2eb71d94d0952ca2536dd3e67e993592aa7be99"
	if err := verifyDownloadedBinarySHA256(binPath, expected); err != nil {
		t.Fatalf("expected checksum verification to pass: %v", err)
	}
}

func TestVerifyDownloadedBinarySHA256_Mismatch(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "lrc")
	if err := os.WriteFile(binPath, []byte("tampered"), 0644); err != nil {
		t.Fatalf("failed to write temp binary: %v", err)
	}

	err := verifyDownloadedBinarySHA256(binPath, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

func TestVerifyDownloadedBinarySHA256_EmptyExpected(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "lrc")
	if err := os.WriteFile(binPath, []byte("any"), 0644); err != nil {
		t.Fatalf("failed to write temp binary: %v", err)
	}

	err := verifyDownloadedBinarySHA256(binPath, "")
	if err == nil {
		t.Fatal("expected error when expected checksum is empty")
	}
}
