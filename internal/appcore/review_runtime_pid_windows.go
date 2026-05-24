//go:build windows

package appcore

import "os"

// isProcessAlive on Windows uses os.FindProcess which opens a process handle via
// OpenProcess. An error means the PID does not exist. Note: PID reuse is possible
// but rare in practice for a local listing that refreshes on every page load.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	proc.Release()
	return true
}
