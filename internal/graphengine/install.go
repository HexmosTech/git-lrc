package graphengine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/HexmosTech/git-lrc/network"
	"github.com/HexmosTech/git-lrc/storage"
)

// InstallOptions tunes Install. The zero value installs the pinned version
// into the standard ~/.lrc/bin location.
type InstallOptions struct {
	// Force reinstalls even when the installed version already matches.
	Force bool
	// Version overrides PinnedVersion (a release tag like "v0.9.0").
	Version string
	// Dir overrides the install directory (tests use a temp dir).
	Dir string
	// Progress, when set, receives byte counts while the asset downloads.
	Progress func(downloaded, total int64)
	// SkipSmokeTest disables the post-install `--version` run. Only tests
	// (which install a fake, non-executable payload) set this.
	SkipSmokeTest bool
}

// InstallResult describes what Install did.
type InstallResult struct {
	Path    string
	Version string
	Skipped bool // already installed at the requested version
}

// downloadAsset is the network seam, replaced by unit tests. The production
// implementation streams from GitHub releases with host-allowlist validation
// on every redirect hop.
var downloadAsset = func(url string, dst io.Writer, onProgress func(downloaded, total int64)) error {
	client := network.NewGitHubDownloadClient()
	_, err := network.GitHubDownloadTo(client, url, dst, onProgress)
	return err
}

// Install downloads the codebase-memory-mcp release archive for this
// platform, verifies its sha256 against the release's checksums.txt, extracts
// just the binary, and atomically installs it into ~/.lrc/bin. Idempotent:
// when the installed binary already reports the requested version it does
// nothing (unless opts.Force).
func Install(opts InstallOptions) (InstallResult, error) {
	versionTag := opts.Version
	if versionTag == "" {
		versionTag = PinnedVersion
	}
	wantVersion := strings.TrimPrefix(versionTag, "v")

	installDir := opts.Dir
	if installDir == "" {
		var err error
		installDir, err = InstallDir()
		if err != nil {
			return InstallResult{}, err
		}
	}
	binPath := filepath.Join(installDir, binaryFileName())

	if !opts.Force {
		if installed, err := InstalledVersion(binPath); err == nil && installed == wantVersion {
			return InstallResult{Path: binPath, Version: wantVersion, Skipped: true}, nil
		}
	}

	assetName, err := AssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return InstallResult{}, err
	}

	var checksumsBuf strings.Builder
	if err := downloadAsset(releaseAssetURL(versionTag, "checksums.txt"), &checksumsBuf, nil); err != nil {
		return InstallResult{}, fmt.Errorf("failed to download checksums.txt: %w", err)
	}
	wantSum, ok := parseChecksums([]byte(checksumsBuf.String()))[assetName]
	if !ok {
		return InstallResult{}, fmt.Errorf("checksums.txt for %s has no entry for %s", versionTag, assetName)
	}

	archiveFile, err := storage.CreateTemp("", "lrc-graphengine-*"+archiveExt(assetName))
	if err != nil {
		return InstallResult{}, fmt.Errorf("failed to create temp file for download: %w", err)
	}
	archivePath := archiveFile.Name()
	defer func() { _ = storage.Remove(archivePath) }()

	downloadErr := downloadAsset(releaseAssetURL(versionTag, assetName), archiveFile, opts.Progress)
	closeErr := archiveFile.Close()
	if downloadErr != nil {
		return InstallResult{}, fmt.Errorf("failed to download %s: %w", assetName, downloadErr)
	}
	if closeErr != nil {
		return InstallResult{}, fmt.Errorf("failed to finish writing %s: %w", assetName, closeErr)
	}

	if err := verifySHA256(archivePath, wantSum); err != nil {
		return InstallResult{}, fmt.Errorf("checksum mismatch for %s: %w", assetName, err)
	}

	if err := storage.MkdirAll(installDir, 0755); err != nil {
		return InstallResult{}, fmt.Errorf("failed to create %s: %w", installDir, err)
	}
	// Extract into a temp file in the destination directory so the final
	// rename is atomic (same volume).
	tmpBin, err := storage.CreateTemp(installDir, "."+BinaryName+".new-*")
	if err != nil {
		return InstallResult{}, fmt.Errorf("failed to create temp binary file: %w", err)
	}
	tmpPath := tmpBin.Name()
	cleanupTmp := func() { _ = storage.Remove(tmpPath) }

	if err := extractBinaryToFile(archivePath, assetName, archiveBinaryMember(runtime.GOOS), tmpBin); err != nil {
		_ = tmpBin.Close()
		cleanupTmp()
		return InstallResult{}, err
	}
	if err := tmpBin.Close(); err != nil {
		cleanupTmp()
		return InstallResult{}, fmt.Errorf("failed to finish writing binary: %w", err)
	}
	if err := storage.Chmod(tmpPath, 0755); err != nil {
		cleanupTmp()
		return InstallResult{}, fmt.Errorf("failed to mark binary executable: %w", err)
	}
	if err := storage.Rename(tmpPath, binPath); err != nil {
		cleanupTmp()
		return InstallResult{}, fmt.Errorf("failed to install binary into place: %w", err)
	}

	if !opts.SkipSmokeTest {
		if got, err := InstalledVersion(binPath); err != nil {
			_ = storage.Remove(binPath)
			return InstallResult{}, fmt.Errorf("installed binary failed smoke test: %w", err)
		} else if got != wantVersion {
			return InstallResult{}, fmt.Errorf("installed binary reports version %s, expected %s", got, wantVersion)
		}
	}

	return InstallResult{Path: binPath, Version: wantVersion}, nil
}

// Uninstall removes the lrc-managed engine binary. It never touches a
// user-installed copy elsewhere on PATH.
func Uninstall() error {
	binPath, err := InstalledBinaryPath()
	if err != nil {
		return err
	}
	// storage.Remove wraps the underlying error, so unwrap via errors.Is.
	if err := storage.Remove(binPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to remove %s: %w", binPath, err)
	}
	return nil
}

func archiveExt(assetName string) string {
	if strings.HasSuffix(assetName, ".zip") {
		return ".zip"
	}
	return ".tar.gz"
}

func verifySHA256(filePath, wantHex string) error {
	f, err := storage.OpenFileForRead(filePath)
	if err != nil {
		return fmt.Errorf("failed to open downloaded file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("failed to hash downloaded file: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, strings.TrimSpace(wantHex)) {
		return fmt.Errorf("got %s, want %s", got, wantHex)
	}
	return nil
}
