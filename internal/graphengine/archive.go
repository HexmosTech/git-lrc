package graphengine

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/HexmosTech/git-lrc/storage"
)

// extractBinaryToFile finds the engine executable member inside the release
// archive at archivePath (tar.gz or zip, decided by the asset name) and
// streams it into dst. Only the exact expected member is extracted - nothing
// else in the archive is ever written to disk - and entries with path
// separators are ignored outright, so a malicious archive cannot traverse
// paths.
func extractBinaryToFile(archivePath, assetName, member string, dst *os.File) error {
	if strings.HasSuffix(assetName, ".zip") {
		return extractFromZip(archivePath, member, dst)
	}
	return extractFromTarGz(archivePath, member, dst)
}

func isExpectedMember(entryName, member string) bool {
	clean := path.Clean(strings.ReplaceAll(entryName, "\\", "/"))
	return clean == member
}

func extractFromTarGz(archivePath, member string, dst *os.File) error {
	f, err := storage.OpenFileForRead(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to read gzip stream: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || !isExpectedMember(hdr.Name, member) {
			continue
		}
		if _, err := io.Copy(dst, tr); err != nil {
			return fmt.Errorf("failed to extract %s: %w", member, err)
		}
		return nil
	}
	return fmt.Errorf("archive does not contain expected member %q", member)
}

func extractFromZip(archivePath, member string, dst *os.File) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open zip archive: %w", err)
	}
	defer zr.Close()

	for _, entry := range zr.File {
		if entry.FileInfo().IsDir() || !isExpectedMember(entry.Name, member) {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			return fmt.Errorf("failed to open zip member %s: %w", member, err)
		}
		_, err = io.Copy(dst, rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("failed to extract %s: %w", member, err)
		}
		return nil
	}
	return fmt.Errorf("archive does not contain expected member %q", member)
}
