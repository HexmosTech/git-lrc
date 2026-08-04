package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/HexmosTech/git-lrc/network"
	"github.com/HexmosTech/git-lrc/storage"
	"github.com/gofrs/flock"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

// version is injected by main at startup so update checks compare against the
// exact runtime build version (including ldflags overrides).
var version = "unknown"

// =============================================================================
// SELF-UPDATE FUNCTIONALITY
// =============================================================================

// Pre-compiled regexes for version parsing
var (
	semverRe = regexp.MustCompile(`v?(\d+)\.(\d+)\.(\d+)`)
)

var autoUpdateStartOnce sync.Once

const internalBuildVersionSuffix = "-internal"

// semverParse extracts major, minor, patch from a version string like "v0.1.14"
func semverParse(v string) (int, int, int, bool) {
	match := semverRe.FindStringSubmatch(strings.TrimSpace(v))
	if match == nil {
		return 0, 0, 0, false
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	return major, minor, patch, true
}

// semverCompare compares two version strings, returns:
// 1 if a > b, -1 if a < b, 0 if equal, error if parsing fails
func semverCompare(a, b string) (int, error) {
	a1, a2, a3, okA := semverParse(a)
	b1, b2, b3, okB := semverParse(b)
	if !okA {
		return 0, fmt.Errorf("invalid version format: %q", a)
	}
	if !okB {
		return 0, fmt.Errorf("invalid version format: %q", b)
	}
	if a1 != b1 {
		if a1 > b1 {
			return 1, nil
		}
		return -1, nil
	}
	if a2 != b2 {
		if a2 > b2 {
			return 1, nil
		}
		return -1, nil
	}
	if a3 != b3 {
		if a3 > b3 {
			return 1, nil
		}
		return -1, nil
	}
	return 0, nil
}

func internalSelfUpdateDisabled() bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(version)), internalBuildVersionSuffix)
}

func fetchReleaseManifest(client *network.Client) (*network.ReleaseManifest, error) {
	resp, err := network.SelfUpdateFetchManifest(client)
	if err != nil {
		return nil, fmt.Errorf("manifest request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest request failed with status %d: %s", resp.StatusCode, string(resp.Body))
	}

	var manifest network.ReleaseManifest
	if err := json.Unmarshal(resp.Body, &manifest); err != nil {
		return nil, fmt.Errorf("failed to decode release manifest: %w", err)
	}
	if strings.TrimSpace(manifest.LatestVersion) == "" {
		return nil, fmt.Errorf("release manifest is missing latest_version")
	}
	if manifest.Releases == nil {
		return nil, fmt.Errorf("release manifest is missing releases")
	}

	return &manifest, nil
}

func fetchLatestVersionFromManifest() (string, error) {
	client := network.NewSelfUpdateClient()
	manifest, err := fetchReleaseManifest(client)
	if err != nil {
		return "", err
	}
	return manifest.LatestVersion, nil
}

func selfUpdateStateDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home directory: %w", err)
	}
	return filepath.Join(homeDir, ".lrc", "update"), nil
}

func pendingUpdateStatePath() (string, error) {
	stateDir, err := selfUpdateStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(stateDir, "pending-update.json"), nil
}

func updateLockPath() (string, error) {
	stateDir, err := selfUpdateStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(stateDir, "update.lock"), nil
}

func ensureSelfUpdateStateDir() error {
	stateDir, err := selfUpdateStateDir()
	if err != nil {
		return err
	}
	if err := storage.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create self-update state directory: %w", err)
	}
	return nil
}

func loadPendingUpdateState() (*storage.PendingUpdateState, error) {
	statePath, err := pendingUpdateStatePath()
	if err != nil {
		return nil, err
	}

	data, err := storage.ReadPendingUpdateStateBytes(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read pending update state: %w", err)
	}

	var state storage.PendingUpdateState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse pending update state: %w", err)
	}

	if strings.TrimSpace(state.Version) == "" || strings.TrimSpace(state.StagedBinaryPath) == "" {
		return nil, fmt.Errorf("pending update state is incomplete")
	}

	// Backward compatibility: older state files do not have integrity_hash.
	storedIntegrityHash := strings.TrimSpace(state.IntegrityHash)
	if storedIntegrityHash != "" {
		expectedCanonical := pendingUpdateStateIntegrityHash(&state)
		expectedLegacy := pendingUpdateStateIntegrityHashLegacy(&state)
		if storedIntegrityHash != expectedCanonical && storedIntegrityHash != expectedLegacy {
			return nil, fmt.Errorf("pending update state integrity check failed")
		}
	}

	return &state, nil
}

func savePendingUpdateState(state *storage.PendingUpdateState) error {
	if state == nil {
		return fmt.Errorf("pending update state is nil")
	}
	if err := ensureSelfUpdateStateDir(); err != nil {
		return err
	}

	statePath, err := pendingUpdateStatePath()
	if err != nil {
		return err
	}

	// Store a tamper-evident hash of key state fields before persisting.
	state.IntegrityHash = pendingUpdateStateIntegrityHash(state)

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode pending update state: %w", err)
	}

	if err := storage.WriteFileAtomically(statePath, data, 0644); err != nil {
		return fmt.Errorf("failed to atomically write pending update state: %w", err)
	}

	return nil
}

func pendingUpdateStateIntegrityHash(state *storage.PendingUpdateState) string {
	return "v2:" + pendingUpdateStateIntegrityHashV2(state)
}

func pendingUpdateStateIntegrityHashLegacy(state *storage.PendingUpdateState) string {
	if state == nil {
		return ""
	}
	payload := strings.TrimSpace(state.Version) + "\n" + strings.TrimSpace(state.StagedBinaryPath) + "\n" + strings.TrimSpace(state.DownloadedAt)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func pendingUpdateStateIntegrityHashV2(state *storage.PendingUpdateState) string {
	if state == nil {
		return ""
	}
	payload := "version=" + strings.TrimSpace(state.Version) + "\n" +
		"staged_binary_path=" + strings.TrimSpace(state.StagedBinaryPath) + "\n" +
		"downloaded_at=" + strings.TrimSpace(state.DownloadedAt)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func clearPendingUpdateState() error {
	statePath, err := pendingUpdateStatePath()
	if err != nil {
		return err
	}
	if err := storage.Remove(statePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to clear pending update state: %w", err)
	}
	return nil
}

func currentUserIdentity() (string, string) {
	usr, err := user.Current()
	if err != nil {
		return "", ""
	}
	return strings.TrimSpace(usr.Uid), strings.TrimSpace(usr.Username)
}

func readUpdateLockMetadata() (*storage.UpdateLockMetadata, error) {
	lockPath, err := updateLockPath()
	if err != nil {
		return nil, err
	}

	data, err := storage.ReadUpdateLockMetadataBytes(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read update lock metadata: %w", err)
	}

	if strings.TrimSpace(string(data)) == "" {
		return nil, nil
	}

	var metadata storage.UpdateLockMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse update lock metadata: %w", err)
	}

	return &metadata, nil
}

func writeUpdateLockMetadata(lockPath string, metadata *storage.UpdateLockMetadata) {
	if metadata == nil {
		return
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return
	}
	_ = storage.WriteFile(lockPath, data, 0644)
}

func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	if runtime.GOOS == "windows" {
		check := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid))
		out, err := check.Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(out), fmt.Sprintf(" %d", pid))
	}

	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func waitForProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isProcessRunning(pid) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return !isProcessRunning(pid)
}

func terminateProcessForForceUnlock(pid int, verbose bool) error {
	if pid <= 0 {
		return fmt.Errorf("invalid lock holder pid: %d", pid)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to resolve lock holder process %d: %w", pid, err)
	}

	if !isProcessRunning(pid) {
		return nil
	}

	if verbose {
		log.Printf("self-update --force: stopping updater process pid=%d", pid)
	}

	if runtime.GOOS == "windows" {
		cmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to terminate updater process %d: %w", pid, err)
		}
		_ = waitForProcessExit(pid, 3*time.Second)
		return nil
	}

	if err := process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("failed to send SIGTERM to updater process %d: %w", pid, err)
	}
	if waitForProcessExit(pid, 2*time.Second) {
		return nil
	}

	if verbose {
		log.Printf("self-update --force: process pid=%d ignored SIGTERM; sending SIGKILL", pid)
	}
	if err := process.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("failed to SIGKILL updater process %d: %w", pid, err)
	}
	_ = waitForProcessExit(pid, 2*time.Second)
	return nil
}

func acquireUpdateLock(force bool, command string, verbose bool) (func(), bool, error) {
	if err := ensureSelfUpdateStateDir(); err != nil {
		return nil, false, err
	}

	lockPath, err := updateLockPath()
	if err != nil {
		return nil, false, err
	}

	lock := flock.New(lockPath)

	tryAcquire := func() (bool, error) {
		locked, err := lock.TryLock()
		if err != nil {
			return false, fmt.Errorf("failed to acquire update lock: %w", err)
		}
		return locked, nil
	}

	locked, err := tryAcquire()
	if err != nil {
		return nil, false, err
	}

	if !locked {
		if !force {
			return nil, false, nil
		}

		metadata, metaErr := readUpdateLockMetadata()
		if metaErr != nil {
			if verbose {
				log.Printf("self-update --force: lock metadata unavailable: %v", metaErr)
			}
			return nil, false, fmt.Errorf("self-update lock is held and owner metadata is unreadable; rerun after current updater exits")
		}

		if metadata == nil || metadata.PID <= 0 {
			return nil, false, fmt.Errorf("self-update lock is held and owner PID is unavailable; rerun after current updater exits")
		}

		currentUID, _ := currentUserIdentity()
		if currentUID != "" && metadata.UID != "" && currentUID != metadata.UID {
			return nil, false, fmt.Errorf("refusing to terminate updater process pid=%d owned by another user (%s)", metadata.PID, metadata.Username)
		}

		if err := terminateProcessForForceUnlock(metadata.PID, verbose); err != nil {
			return nil, false, err
		}

		locked, err = tryAcquire()
		if err != nil {
			return nil, false, err
		}
		if !locked {
			return nil, false, fmt.Errorf("self-update lock is still held after --force recovery attempt")
		}
	}

	uid, username := currentUserIdentity()
	writeUpdateLockMetadata(lockPath, &storage.UpdateLockMetadata{
		PID:       os.Getpid(),
		UID:       uid,
		Username:  username,
		Command:   command,
		Version:   version,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	})

	release := func() {
		_ = lock.Unlock()
	}

	return release, true, nil
}

func selfUpdatePlatformID() (string, error) {
	platformOS := runtime.GOOS
	switch platformOS {
	case "linux", "darwin", "windows":
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	platformArch := ""
	switch runtime.GOARCH {
	case "amd64":
		platformArch = "amd64"
	case "arm64":
		platformArch = "arm64"
	default:
		return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}

	return fmt.Sprintf("%s-%s", platformOS, platformArch), nil
}

func verifyDownloadedBinarySHA256(binaryPath, expectedSHA256 string) error {
	expected := strings.ToLower(strings.TrimSpace(expectedSHA256))
	if expected == "" {
		return fmt.Errorf("release manifest checksum is empty")
	}

	f, err := storage.OpenFileForRead(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to open downloaded binary for checksum verification: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("failed to hash downloaded binary: %w", err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("downloaded binary checksum mismatch")
	}

	return nil
}

func downloadVersionBinaryFromManifest(versionTag string) (string, error) {
	platformID, err := selfUpdatePlatformID()
	if err != nil {
		return "", err
	}

	binaryName := "lrc"
	if runtime.GOOS == "windows" {
		binaryName = "lrc.exe"
	}

	client := network.NewSelfUpdateClient()
	manifest, err := fetchReleaseManifest(client)
	if err != nil {
		return "", err
	}

	versionInfo, ok := manifest.Releases[versionTag]
	if !ok {
		return "", fmt.Errorf("release manifest does not include version %s", versionTag)
	}
	artifact, ok := versionInfo.Platforms[platformID]
	if !ok {
		return "", fmt.Errorf("release manifest does not include platform %s for %s", platformID, versionTag)
	}
	if strings.TrimSpace(artifact.Binary) == "" {
		return "", fmt.Errorf("release manifest binary path is empty for %s/%s", versionTag, platformID)
	}
	if !strings.HasSuffix(artifact.Binary, binaryName) {
		return "", fmt.Errorf("release manifest binary path %q does not match expected binary %q", artifact.Binary, binaryName)
	}

	fullURL := network.SelfUpdateBinaryURL(artifact.Binary)

	tmpFile, err := storage.CreateTemp("", "lrc-update-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file for update: %w", err)
	}
	tmpPath := tmpFile.Name()

	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	if !isTTY {
		fmt.Printf("Downloading lrc %s ...\n", versionTag)
	}
	startTime := time.Now()
	statusCode, err := network.SelfUpdateDownloadBinaryTo(client, fullURL, tmpFile, func(downloaded, total int64) {
		if !isTTY {
			return
		}
		elapsed := time.Since(startTime).Seconds()
		if elapsed < 0.001 {
			elapsed = 0.001
		}
		speedBps := float64(downloaded) / elapsed
		speedMBps := speedBps / (1024 * 1024)
		if total > 0 {
			pct := float64(downloaded) * 100.0 / float64(total)
			var etaStr string
			if speedBps > 0 {
				etaSecs := int(float64(total-downloaded) / speedBps)
				if etaSecs < 60 {
					etaStr = fmt.Sprintf("%ds", etaSecs)
				} else {
					etaStr = fmt.Sprintf("%dm%ds", etaSecs/60, etaSecs%60)
				}
			} else {
				etaStr = "?"
			}
			fmt.Printf("\rDownloading lrc %s ... %.1f%% | %.1f MB/s | ETA %-6s", versionTag, pct, speedMBps, etaStr)
		} else {
			fmt.Printf("\rDownloading lrc %s ... %.1f MB/s | %d KB downloaded", versionTag, speedMBps, downloaded/1024)
		}
	})
	if isTTY {
		fmt.Println()
	}
	if err != nil {
		_ = tmpFile.Close()
		_ = storage.Remove(tmpPath)
		return "", fmt.Errorf("failed to download update binary: %w", err)
	}

	if statusCode != http.StatusOK {
		_ = tmpFile.Close()
		_ = storage.Remove(tmpPath)
		return "", fmt.Errorf("failed to download binary (status %d)", statusCode)
	}
	if err := tmpFile.Close(); err != nil {
		_ = storage.Remove(tmpPath)
		return "", fmt.Errorf("failed to finalize downloaded update binary: %w", err)
	}

	// Verify downloaded artifact integrity before making it executable or staging it.
	if err := verifyDownloadedBinarySHA256(tmpPath, artifact.SHA256); err != nil {
		_ = storage.Remove(tmpPath)
		return "", fmt.Errorf("failed to verify downloaded binary checksum: %w", err)
	}

	if err := storage.Chmod(tmpPath, 0755); err != nil {
		_ = storage.Remove(tmpPath)
		return "", fmt.Errorf("failed to mark downloaded binary executable: %w", err)
	}

	return tmpPath, nil
}

func stageUpdateVersion(versionTag string, force bool, verbose bool) (*storage.PendingUpdateState, error) {
	if internalSelfUpdateDisabled() {
		return nil, fmt.Errorf("self-update is disabled in internal builds")
	}

	release, acquired, err := acquireUpdateLock(force, "self-update-stage", verbose)
	if err != nil {
		return nil, err
	}
	if !acquired {
		if verbose {
			log.Println("self-update lock is held by another process; skipping stage")
		}
		return nil, nil
	}
	defer release()

	if strings.TrimSpace(versionTag) == "" {
		return nil, fmt.Errorf("version tag is empty")
	}

	existing, err := loadPendingUpdateState()
	if err == nil && existing != nil && existing.Version == versionTag && !force {
		if st, statErr := os.Stat(existing.StagedBinaryPath); statErr == nil && st.Size() > 0 {
			if verbose {
				log.Printf("reusing already-downloaded update artifact for %s", versionTag)
			}
			return existing, nil
		}
	}

	stagedBinaryPath, err := downloadVersionBinaryFromManifest(versionTag)
	if err != nil {
		return nil, err
	}

	state := &storage.PendingUpdateState{
		Version:          versionTag,
		StagedBinaryPath: stagedBinaryPath,
		DownloadedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	if err := savePendingUpdateState(state); err != nil {
		_ = storage.Remove(stagedBinaryPath)
		return nil, err
	}

	if verbose {
		log.Printf("staged update binary for %s at %s", versionTag, stagedBinaryPath)
	}

	return state, nil
}

func currentBinaryTargets() (string, string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve current executable path: %w", err)
	}

	resolvedExe, err := filepath.EvalSymlinks(exePath)
	if err == nil && strings.TrimSpace(resolvedExe) != "" {
		exePath = resolvedExe
	}

	installDir := filepath.Dir(exePath)
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}

	return filepath.Join(installDir, "lrc"+ext), filepath.Join(installDir, "git-lrc"+ext), nil
}

func pathDirWritable(path string) bool {
	dir := filepath.Dir(path)
	f, err := storage.CreateTemp(dir, ".lrc-write-check-")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = storage.Remove(name)
	return true
}

func copyFileContents(srcPath, dstPath string, mode fs.FileMode) error {
	src, err := storage.OpenFileForRead(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %w", srcPath, err)
	}
	defer src.Close()

	dst, err := storage.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("failed to open destination file %s: %w", dstPath, err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return fmt.Errorf("failed to copy %s to %s: %w", srcPath, dstPath, err)
	}

	if err := dst.Close(); err != nil {
		return fmt.Errorf("failed to close destination file %s: %w", dstPath, err)
	}

	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := storage.OpenFileForRead(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func binariesMatch(a, b string) (bool, error) {
	ha, err := fileSHA256(a)
	if err != nil {
		return false, err
	}
	hb, err := fileSHA256(b)
	if err != nil {
		return false, err
	}
	return ha == hb, nil
}

// resyncBinary copies srcPath's content over dstPath (via a temp file +
// atomic rename, so a reader never sees a truncated dstPath mid-copy) and
// then verifies the result by checksum rather than just trusting the copy
// call succeeded - a copy that "succeeds" but produces the wrong bytes (a
// concurrent writer, a filesystem quirk) would otherwise look identical to
// a correct sync from the caller's side. Direction-agnostic: the caller
// decides which of the two binaries is the source of truth.
func resyncBinary(srcPath, dstPath string, verbose bool) error {
	if !pathDirWritable(dstPath) {
		return fmt.Errorf("install directory is not writable: %s", filepath.Dir(dstPath))
	}

	tmpPath := filepath.Join(filepath.Dir(dstPath), fmt.Sprintf(".%s.new.%d", filepath.Base(dstPath), time.Now().UnixNano()))
	if err := copyFileContents(srcPath, tmpPath, 0755); err != nil {
		return fmt.Errorf("failed to stage %s resync: %w", filepath.Base(dstPath), err)
	}
	if err := storage.Chmod(tmpPath, 0755); err != nil {
		_ = storage.Remove(tmpPath)
		return fmt.Errorf("failed to set executable permissions on %s: %w", filepath.Base(dstPath), err)
	}
	if err := storage.Rename(tmpPath, dstPath); err != nil {
		_ = storage.Remove(tmpPath)
		return fmt.Errorf("failed to install resynced %s binary: %w", filepath.Base(dstPath), err)
	}

	ok, err := binariesMatch(srcPath, dstPath)
	if err != nil {
		return fmt.Errorf("failed to verify %s resync: %w", filepath.Base(dstPath), err)
	}
	if !ok {
		return fmt.Errorf("%s resync verification failed: content mismatch after copy", filepath.Base(dstPath))
	}
	if verbose {
		log.Printf("%s binary resynced from %s", filepath.Base(dstPath), filepath.Base(srcPath))
	}
	return nil
}

// EnsureGitLRCBinarySynced is a self-healing backstop: it checks whether the
// lrc/git-lrc binaries alongside the currently-running executable actually
// match, and resyncs whichever one is stale. This runs on every invocation
// (see main.go), independent of self-update's own apply step - so a
// mismatch introduced by ANY prior failure mode (a sync step that errored
// partway, an install predating this check, manual tampering) self-heals on
// the very next command, regardless of which binary name was used to invoke
// it.
//
// Direction is decided by modification time, not by name: whichever file
// was written more recently is treated as the source of truth. In normal
// operation that's always lrc (self-update writes it first, then mirrors it
// onto git-lrc), but nothing here assumes that - if git-lrc were ever ahead
// (a manual replacement, a future code path that updates it directly), a
// name-fixed "always copy lrc over git-lrc" rule would silently downgrade
// the newer file instead of repairing the older one.
//
// Cheap in the common (already-synced) case: just two os.Stat calls, no
// file reads, unless a size mismatch actually suggests drift.
func EnsureGitLRCBinarySynced(verbose bool) error {
	if internalSelfUpdateDisabled() {
		return nil
	}

	lrcBinaryPath, gitLRCBinaryPath, err := currentBinaryTargets()
	if err != nil {
		return err
	}

	return ensureBinariesSynced(lrcBinaryPath, gitLRCBinaryPath, verbose)
}

// ensureBinariesSynced holds the actual directionality decision, factored
// out from EnsureGitLRCBinarySynced (which resolves paths via
// os.Executable() and so can't be unit-tested with arbitrary temp paths).
func ensureBinariesSynced(lrcBinaryPath, gitLRCBinaryPath string, verbose bool) error {
	lrcInfo, lrcErr := os.Stat(lrcBinaryPath)
	gitInfo, gitErr := os.Stat(gitLRCBinaryPath)

	switch {
	case lrcErr != nil && gitErr != nil:
		return nil // neither binary is present/readable - nothing this check can fix
	case lrcErr != nil:
		// lrc missing but git-lrc present: recreate lrc from git-lrc.
		if verbose {
			log.Printf("lrc binary missing or unreadable (%v) - resyncing from git-lrc", lrcErr)
		}
		return resyncBinary(gitLRCBinaryPath, lrcBinaryPath, verbose)
	case gitErr != nil:
		if verbose {
			log.Printf("git-lrc binary missing or unreadable (%v) - resyncing from lrc", gitErr)
		}
		return resyncBinary(lrcBinaryPath, gitLRCBinaryPath, verbose)
	}

	if lrcInfo.Size() == gitInfo.Size() {
		// Equal size is a cheap but not conclusive signal (two different
		// builds could coincidentally land on the same byte count) - only
		// skip the (comparatively expensive) hash check when mtimes are
		// also close together, which is what any correct prior sync
		// actually leaves behind (both files touched in the same
		// operation). Mtimes far apart despite equal size means "probably
		// fine, but verify" rather than "definitely fine".
		delta := lrcInfo.ModTime().Sub(gitInfo.ModTime())
		if delta < 0 {
			delta = -delta
		}
		if delta < 5*time.Second {
			return nil
		}
		if match, err := binariesMatch(lrcBinaryPath, gitLRCBinaryPath); err == nil && match {
			return nil
		}
	}

	if lrcInfo.ModTime().After(gitInfo.ModTime()) {
		if verbose {
			log.Printf("git-lrc binary out of sync with lrc (size %d vs %d, lrc is newer) - resyncing", gitInfo.Size(), lrcInfo.Size())
		}
		return resyncBinary(lrcBinaryPath, gitLRCBinaryPath, verbose)
	}
	if verbose {
		log.Printf("lrc binary out of sync with git-lrc (size %d vs %d, git-lrc is newer) - resyncing", lrcInfo.Size(), gitInfo.Size())
	}
	return resyncBinary(gitLRCBinaryPath, lrcBinaryPath, verbose)
}

func runHooksInstallWithBinary(binaryPath string, verbose bool) error {
	cleaned := filepath.Clean(strings.TrimSpace(binaryPath))
	if cleaned == "" {
		return fmt.Errorf("hooks install binary path is empty")
	}
	base := filepath.Base(cleaned)
	if base != "lrc" && base != "lrc.exe" {
		return fmt.Errorf("invalid hooks install binary name: %s", base)
	}
	if _, err := os.Stat(cleaned); err != nil {
		return fmt.Errorf("hooks install binary not accessible: %w", err)
	}

	// Safe: binary path is constrained to basename lrc/lrc.exe and must exist before execution.
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := exec.Command(cleaned, "hooks", "install")
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run hooks install with new binary: %w", err)
	}
	return nil
}

func applyPendingUpdateUnix(state *storage.PendingUpdateState, verbose bool) error {
	lrcBinaryPath, gitLRCBinaryPath, err := currentBinaryTargets()
	if err != nil {
		return err
	}

	if !pathDirWritable(lrcBinaryPath) {
		return fmt.Errorf("install directory is not writable: %s", filepath.Dir(lrcBinaryPath))
	}

	replaceTmpPath := filepath.Join(filepath.Dir(lrcBinaryPath), fmt.Sprintf(".lrc.new.%d", time.Now().UnixNano()))
	if err := copyFileContents(state.StagedBinaryPath, replaceTmpPath, 0755); err != nil {
		return err
	}

	if err := storage.Chmod(replaceTmpPath, 0755); err != nil {
		_ = storage.Remove(replaceTmpPath)
		return fmt.Errorf("failed to set executable permissions on replacement binary: %w", err)
	}

	if err := storage.Rename(replaceTmpPath, lrcBinaryPath); err != nil {
		_ = storage.Remove(replaceTmpPath)
		return fmt.Errorf("failed to replace lrc binary: %w", err)
	}

	if err := resyncBinary(lrcBinaryPath, gitLRCBinaryPath, verbose); err != nil {
		return fmt.Errorf("failed to sync git-lrc binary: %w", err)
	}

	if err := runHooksInstallWithBinary(lrcBinaryPath, verbose); err != nil {
		return err
	}

	_ = storage.Remove(state.StagedBinaryPath)
	if err := clearPendingUpdateState(); err != nil {
		return err
	}

	fmt.Printf("%s✓ Updated to %s and reinstalled global hooks%s\n", colorGreen, state.Version, colorReset)
	return nil
}

func psSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func applyPendingUpdateWindows(state *storage.PendingUpdateState, verbose bool) error {
	lrcBinaryPath, gitLRCBinaryPath, err := currentBinaryTargets()
	if err != nil {
		return err
	}

	statePath, err := pendingUpdateStatePath()
	if err != nil {
		return err
	}

	script := fmt.Sprintf("$src='%s';$dst='%s';$git='%s';$state='%s';for($i=0;$i -lt 120;$i++){try{Move-Item -Force $src $dst;Copy-Item -Force $dst $git;& $dst hooks install *> $null;Remove-Item -Force $state -ErrorAction SilentlyContinue;exit 0}catch{Start-Sleep -Milliseconds 500}};exit 1",
		psSingleQuote(state.StagedBinaryPath),
		psSingleQuote(lrcBinaryPath),
		psSingleQuote(gitLRCBinaryPath),
		psSingleQuote(statePath),
	)

	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	if verbose {
		log.Println("starting Windows self-update helper process")
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Windows update helper: %w", err)
	}

	fmt.Printf("%sUpdate to %s scheduled and will finalize as this process exits.%s\n", colorYellow, state.Version, colorReset)
	return nil
}

func applyPendingUpdateState(state *storage.PendingUpdateState, verbose bool) error {
	if state == nil {
		return nil
	}
	if st, err := os.Stat(state.StagedBinaryPath); err != nil || st.Size() == 0 {
		_ = clearPendingUpdateState()
		if err == nil {
			return fmt.Errorf("staged update binary is empty: %s", state.StagedBinaryPath)
		}
		return fmt.Errorf("staged update binary unavailable: %w", err)
	}

	if runtime.GOOS == "windows" {
		return applyPendingUpdateWindows(state, verbose)
	}
	return applyPendingUpdateUnix(state, verbose)
}

func applyPendingUpdateIfAny(verbose bool) error {
	if internalSelfUpdateDisabled() {
		return nil
	}

	release, acquired, err := acquireUpdateLock(false, "self-update-apply", verbose)
	if err != nil {
		return err
	}
	if !acquired {
		if verbose {
			log.Println("self-update lock is held by another process; skipping apply")
		}
		return nil
	}
	defer release()

	state, err := loadPendingUpdateState()
	if err != nil {
		return err
	}
	if state == nil {
		return nil
	}

	return applyPendingUpdateState(state, verbose)
}

func startAutoUpdateCheck(verbose bool) {
	if internalSelfUpdateDisabled() {
		if verbose {
			log.Printf("auto-update disabled for internal build %s", version)
		}
		return
	}

	autoUpdateStartOnce.Do(func() {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					if verbose {
						log.Printf("auto-update check panicked: %v", r)
					}
				}
			}()

			latestVersion, err := fetchLatestVersionFromManifest()
			if err != nil {
				if verbose {
					log.Printf("auto-update check failed: %v", err)
				}
				return
			}

			cmp, err := semverCompare(version, latestVersion)
			if err != nil {
				if verbose {
					log.Printf("auto-update version compare failed: %v", err)
				}
				return
			}
			if cmp >= 0 {
				return
			}

			_, err = stageUpdateVersion(latestVersion, false, verbose)
			if err != nil {
				if verbose {
					log.Printf("auto-update staging failed: %v", err)
				}
				return
			}

			if verbose {
				log.Printf("auto-update staged version %s for apply-on-exit", latestVersion)
			}
		}()
	})
}

// platformInstallCommand returns the appropriate installer command for the current platform
func platformInstallCommand() string {
	if runtime.GOOS == "windows" {
		return `powershell -Command "iwr -useb https://hexmos.com/lrc-install.ps1 | iex"`
	}
	return "curl -fsSL https://hexmos.com/lrc-install.sh | bash"
}

// ANSI color codes for terminal output
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// runSelfUpdate handles the self-update command
func runSelfUpdate(c *cli.Context) error {
	if internalSelfUpdateDisabled() {
		return fmt.Errorf("self-update is disabled in internal builds")
	}

	checkOnly := c.Bool("check")
	force := c.Bool("force")

	fmt.Printf("Current version: %s%s%s\n", colorCyan, version, colorReset)
	fmt.Println("Checking for updates...")

	latestVersion, err := fetchLatestVersionFromManifest()
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	fmt.Printf("Latest version:  %s%s%s\n", colorCyan, latestVersion, colorReset)

	cmp, err := semverCompare(version, latestVersion)
	if err != nil {
		return fmt.Errorf("failed to compare versions: %w", err)
	}
	if cmp >= 0 && !force {
		fmt.Printf("\n%s✓ lrc is already up to date!%s\n", colorGreen, colorReset)
		return nil
	}

	if cmp >= 0 && force {
		fmt.Printf("\n%sForce recovery requested (this may terminate another active lrc self-update process)%s\n", colorYellow, colorReset)
	} else {
		fmt.Printf("\n%s⬆ Update available: %s → %s%s\n", colorYellow, version, latestVersion, colorReset)
		if force {
			fmt.Printf("%sWarning: --force may terminate another active lrc self-update process.%s\n", colorYellow, colorReset)
		}
	}

	if checkOnly {
		fmt.Println("\nRun 'lrc self-update' (without --check) to install the update.")
		return nil
	}

	fmt.Println("Downloading update artifact...")
	state, err := stageUpdateVersion(latestVersion, force, true)
	if err != nil {
		return fmt.Errorf("failed to stage update: %w", err)
	}
	if state == nil {
		fmt.Printf("%sAnother lrc self-update process is active. Re-run with --force to recover.%s\n", colorYellow, colorReset)
		return nil
	}

	fmt.Println("Applying update...")
	if err := applyPendingUpdateState(state, true); err != nil {
		return fmt.Errorf("failed to apply update: %w", err)
	}

	fmt.Printf("\n%s✓ Update complete! Run 'lrc version' to verify.%s\n", colorGreen, colorReset)
	return nil
}

func SetVersion(v string) {
	version = strings.TrimSpace(v)
	if version == "" {
		version = "unknown"
	}
}

func ApplyPendingUpdateIfAny(verbose bool) error {
	return applyPendingUpdateIfAny(verbose)
}

func StartAutoUpdateCheck(verbose bool) {
	startAutoUpdateCheck(verbose)
}

func RunSelfUpdate(c *cli.Context) error {
	return runSelfUpdate(c)
}
