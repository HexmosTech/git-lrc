package embed

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// EnsureModel makes sure the BGE-small-en-v1.5 ONNX weights and vocabulary
// are present in the local cache, downloading and verifying them if not.
// It never re-downloads a file that already matches the pinned size.
func EnsureModel(progress ProgressFunc) (modelPath, vocabPath string, err error) {
	dir, err := CacheDir()
	if err != nil {
		return "", "", err
	}
	modelDir := filepath.Join(dir, "models", "bge-small-en-v1.5")
	modelPath = filepath.Join(modelDir, modelWeights.Name)
	vocabPath = filepath.Join(modelDir, modelVocab.Name)

	if !fileMatches(modelPath, modelWeights.Size) {
		if err := downloadToFile(modelWeights.URL, modelPath, modelWeights.SHA256, modelWeights.Size,
			"bge-small-en-v1.5 model weights", progress); err != nil {
			return "", "", err
		}
	}
	if !fileMatches(vocabPath, modelVocab.Size) {
		if err := downloadToFile(modelVocab.URL, vocabPath, modelVocab.SHA256, modelVocab.Size,
			"bge-small-en-v1.5 vocabulary", progress); err != nil {
			return "", "", err
		}
	}
	return modelPath, vocabPath, nil
}

// EnsureRuntimeLibrary makes sure the onnxruntime shared library is present
// locally, downloading and extracting it from the platform's pinned release
// archive if necessary. If DBCTX_ONNXRUNTIME_LIB is set, that path is
// returned unchanged (and must already exist).
func EnsureRuntimeLibrary(progress ProgressFunc) (string, error) {
	if lib := os.Getenv(OnnxRuntimeLibEnv); lib != "" {
		if !fileExists(lib) {
			return "", fmt.Errorf("%s=%s does not exist", OnnxRuntimeLibEnv, lib)
		}
		return lib, nil
	}

	asset, err := currentPlatformRuntimeAsset()
	if err != nil {
		return "", err
	}
	dir, err := CacheDir()
	if err != nil {
		return "", err
	}
	libPath := filepath.Join(dir, "onnxruntime", onnxRuntimeVersion, asset.LocalName)
	if fileMatches(libPath, 0) {
		return libPath, nil
	}

	archivePath := filepath.Join(dir, "onnxruntime", onnxRuntimeVersion, filepath.Base(asset.URL))
	if err := downloadToFile(asset.URL, archivePath, asset.SHA256, asset.Size,
		fmt.Sprintf("onnxruntime %s runtime", onnxRuntimeVersion), progress); err != nil {
		return "", err
	}
	defer os.Remove(archivePath)

	if err := extractMember(archivePath, asset.Kind, asset.MemberPath, libPath); err != nil {
		return "", fmt.Errorf("extract onnxruntime library: %w", err)
	}
	return libPath, nil
}

// extractMember pulls a single named file out of a tar.gz or zip archive
// and writes it to destPath.
func extractMember(archivePath string, kind archiveKind, member, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	tmpDest := destPath + ".tmp"
	defer os.Remove(tmpDest)

	switch kind {
	case archiveTarGz:
		if err := extractTarGzMember(archivePath, member, tmpDest); err != nil {
			return err
		}
	case archiveZip:
		if err := extractZipMember(archivePath, member, tmpDest); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown archive kind %v", kind)
	}
	return os.Rename(tmpDest, destPath)
}

func extractTarGzMember(archivePath, member, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("member %q not found in %s", member, archivePath)
		}
		if err != nil {
			return err
		}
		if hdr.Name != member || hdr.Typeflag != tar.TypeReg {
			continue
		}
		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, tr); err != nil {
			return err
		}
		return nil
	}
}

func extractZipMember(archivePath, member, destPath string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name != member {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, rc); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("member %q not found in %s", member, archivePath)
}
