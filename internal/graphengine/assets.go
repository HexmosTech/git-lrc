package graphengine

import (
	"bufio"
	"fmt"
	"strings"
)

// AssetName maps GOOS/GOARCH to the release asset filename. Linux uses the
// "-portable" (statically linked) build so the install works on musl and
// older-glibc systems alike; darwin/windows have a single variant each.
// The "-ui-" assets bundle the graph-visualization server and are
// deliberately not used.
func AssetName(goos, goarch string) (string, error) {
	if goarch != "amd64" && goarch != "arm64" {
		return "", fmt.Errorf("unsupported architecture %q for %s", goarch, BinaryName)
	}
	switch goos {
	case "linux":
		return fmt.Sprintf("%s-linux-%s-portable.tar.gz", BinaryName, goarch), nil
	case "darwin":
		return fmt.Sprintf("%s-darwin-%s.tar.gz", BinaryName, goarch), nil
	case "windows":
		return fmt.Sprintf("%s-windows-%s.zip", BinaryName, goarch), nil
	default:
		return "", fmt.Errorf("unsupported platform %q for %s", goos, BinaryName)
	}
}

// archiveBinaryMember is the name of the executable entry inside a release
// archive (archives are flat: binary + LICENSE + install script + notices).
func archiveBinaryMember(goos string) string {
	if goos == "windows" {
		return BinaryName + ".exe"
	}
	return BinaryName
}

// parseChecksums parses a standard `sha256sum` style checksums.txt
// ("<hex>  <filename>" per line) into filename -> lowercase hex digest.
func parseChecksums(data []byte) map[string]string {
	sums := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != 64 {
			continue
		}
		sums[fields[1]] = strings.ToLower(fields[0])
	}
	return sums
}

// releaseAssetURL builds the download URL for an asset of the given release
// tag.
func releaseAssetURL(versionTag, assetName string) string {
	return fmt.Sprintf("%s/%s/%s", releaseBaseURL, versionTag, assetName)
}
