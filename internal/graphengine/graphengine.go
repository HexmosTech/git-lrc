// Package graphengine manages a local, lrc-owned installation of the
// codebase-memory-mcp binary - the knowledge-graph engine behind blast-radius
// scoring. It downloads the binary from the project's GitHub releases into
// ~/.lrc/bin and nothing else: no PATH edits, no shell rc changes, and it
// never runs the vendor's own `install` subcommand (which auto-modifies
// agent configs for Claude Code, Codex, Gemini, Zed, etc.).
package graphengine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/HexmosTech/git-lrc/configpath"
)

const (
	// BinaryName is the engine executable's base name (without .exe).
	BinaryName = "codebase-memory-mcp"
	// PinnedVersion is the release tag lrc installs. Bumped deliberately with
	// lrc releases rather than tracking "latest", so an lrc build always pairs
	// with an engine version its scoring queries were verified against.
	PinnedVersion = "v0.9.0"

	releaseBaseURL = "https://github.com/DeusData/codebase-memory-mcp/releases/download"
)

// ErrNotInstalled is returned by Resolve when no engine binary can be found.
var ErrNotInstalled = errors.New("codebase-memory-mcp is not installed (run `lrc graph install`)")

// InstallDir returns the lrc-owned directory the engine binary lives in
// (~/.lrc/bin).
func InstallDir() (string, error) {
	dataDir, err := configpath.ResolveLRCDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "bin"), nil
}

func binaryFileName() string {
	if runtime.GOOS == "windows" {
		return BinaryName + ".exe"
	}
	return BinaryName
}

// InstalledBinaryPath returns the path the lrc-managed engine binary would be
// installed at, whether or not it exists yet.
func InstalledBinaryPath() (string, error) {
	dir, err := InstallDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, binaryFileName()), nil
}

// Resolve returns the engine binary path to use: the lrc-managed install in
// ~/.lrc/bin when present, otherwise a user-installed binary found on PATH.
// Returns ErrNotInstalled (wrapped) when neither exists.
func Resolve() (string, error) {
	managed, err := InstalledBinaryPath()
	if err == nil {
		if info, statErr := os.Stat(managed); statErr == nil && !info.IsDir() {
			return managed, nil
		}
	}
	if onPath, lookErr := exec.LookPath(BinaryName); lookErr == nil {
		return onPath, nil
	}
	return "", ErrNotInstalled
}

// InstalledVersion runs `<binary> --version` and returns the reported semver
// (e.g. "0.9.0"). The binary prints a single line like
// "codebase-memory-mcp 0.9.0".
func InstalledVersion(binaryPath string) (string, error) {
	out, err := exec.Command(binaryPath, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("failed to run %s --version: %w", binaryPath, err)
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) == 0 {
		return "", fmt.Errorf("%s --version produced no output", binaryPath)
	}
	return fields[len(fields)-1], nil
}
