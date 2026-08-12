package embed

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// CacheDirEnv overrides the local model/runtime cache directory. If unset,
// dbctx uses a single fixed location on every platform: "<home>/.dbctx"
// (see CacheDir) rather than each OS's own cache-directory convention, so
// the path is predictable and identical regardless of where it's run.
const CacheDirEnv = "DBCTX_CACHE_DIR"

// OnnxRuntimeLibEnv, if set, overrides the onnxruntime shared library path
// entirely, skipping the download/cache mechanism. Use this to supply a
// system-installed onnxruntime or to support a platform with no prebuilt
// release (e.g. darwin/amd64).
const OnnxRuntimeLibEnv = "DBCTX_ONNXRUNTIME_LIB"

// CacheDir returns the root directory dbctx uses to cache downloaded model
// weights and the onnxruntime shared library. It does not create the
// directory.
//
// This is deliberately "<home>/.dbctx" on every platform (Linux, macOS,
// Windows) rather than each OS's own cache-directory convention
// (XDG_CACHE_HOME, ~/Library/Caches, %LocalAppData%) — a single
// predictable location that's easy to find, back up, or wipe by hand,
// consistent everywhere dbctx runs.
func CacheDir() (string, error) {
	if v := os.Getenv(CacheDirEnv); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir (set %s to override): %w", CacheDirEnv, err)
	}
	return filepath.Join(home, ".dbctx"), nil
}

// ProgressFunc reports download progress: downloaded/total bytes for the
// current asset and a human label for what's being fetched. total may be 0
// if unknown. Implementations must return quickly.
type ProgressFunc func(label string, downloaded, total int64)

// Status reports whether the local cache already has everything needed for
// semantic indexing, without downloading anything.
type Status struct {
	ModelReady   bool
	RuntimeReady bool
	ModelPath    string
	VocabPath    string
	RuntimeLib   string
}

// CheckCache reports what's already present locally, for CLI/status use.
func CheckCache() (*Status, error) {
	dir, err := CacheDir()
	if err != nil {
		return nil, err
	}
	modelDir := filepath.Join(dir, "models", "bge-small-en-v1.5")
	st := &Status{
		ModelPath: filepath.Join(modelDir, modelWeights.Name),
		VocabPath: filepath.Join(modelDir, modelVocab.Name),
	}
	st.ModelReady = fileMatches(st.ModelPath, modelWeights.Size) && fileMatches(st.VocabPath, modelVocab.Size)

	if lib := os.Getenv(OnnxRuntimeLibEnv); lib != "" {
		st.RuntimeLib = lib
		st.RuntimeReady = fileExists(lib)
		return st, nil
	}
	asset, err := currentPlatformRuntimeAsset()
	if err != nil {
		return st, err
	}
	st.RuntimeLib = filepath.Join(dir, "onnxruntime", onnxRuntimeVersion, asset.LocalName)
	st.RuntimeReady = fileMatches(st.RuntimeLib, 0)
	return st, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// fileMatches reports whether path exists and, if wantSize > 0, has exactly
// that size. This is a cheap integrity heuristic for cache hits — full
// checksum verification only happens right after a fresh download.
func fileMatches(path string, wantSize int64) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if wantSize > 0 && info.Size() != wantSize {
		return false
	}
	return true
}

// downloadToFile fetches url into destPath (via a temp file + atomic
// rename), verifying both size and sha256 before committing. On checksum
// mismatch the temp file is removed and an error returned; destPath is
// never left partially written.
func downloadToFile(url, destPath, wantSHA256 string, wantSize int64, label string, progress ProgressFunc) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(destPath), ".dbctx-download-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once renamed

	h := sha256.New()
	total := wantSize
	if resp.ContentLength > 0 {
		total = resp.ContentLength
	}
	var downloaded int64
	buf := make([]byte, 256*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := tmp.Write(buf[:n]); werr != nil {
				tmp.Close()
				return fmt.Errorf("write %s: %w", destPath, werr)
			}
			h.Write(buf[:n])
			downloaded += int64(n)
			if progress != nil {
				progress(label, downloaded, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			tmp.Close()
			return fmt.Errorf("download %s: %w", url, rerr)
		}
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if wantSize > 0 && downloaded != wantSize {
		return fmt.Errorf("download %s: got %d bytes, want %d", url, downloaded, wantSize)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if wantSHA256 != "" && got != wantSHA256 {
		return fmt.Errorf("download %s: sha256 mismatch: got %s, want %s", url, got, wantSHA256)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("install %s: %w", destPath, err)
	}
	return nil
}
