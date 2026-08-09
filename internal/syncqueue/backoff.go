package syncqueue

import "time"

// backoffSchedule maps attempt count (0 = first transient failure) to how
// long to wait before the next retry. Capped at the last entry.
var backoffSchedule = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	1 * time.Hour,
	6 * time.Hour,
	24 * time.Hour,
}

// MaxPendingAge is how long a transiently-failing item keeps retrying
// before it's given up on (marked failed instead of pending forever) --
// keeps the queue from growing unbounded for an abandoned repo/credential.
const MaxPendingAge = 30 * 24 * time.Hour

// NextBackoff returns how long to wait before the next retry attempt,
// given how many attempts have already been made.
func NextBackoff(attempts int) time.Duration {
	if attempts < 0 {
		attempts = 0
	}
	if attempts >= len(backoffSchedule) {
		return backoffSchedule[len(backoffSchedule)-1]
	}
	return backoffSchedule[attempts]
}
