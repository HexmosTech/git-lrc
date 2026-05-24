//go:build !windows

package appcore

import (
	"os"
	"syscall"
)

// isProcessAlive checks if a process is still running via kill(pid, 0).
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
