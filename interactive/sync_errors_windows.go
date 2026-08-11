func isIgnorableSyncError(err error) bool {
	if err == nil {
		return false
	}

	// Direct match
	if errors.Is(err, windowsErrorInvalidHandle) {
		return true
	}

	// Handle wrapped syscall errors
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == windowsErrorInvalidHandle ||
			errno == syscall.EINVAL ||
			errno == syscall.ENOTSUP
	}

	return false
}
