package main

const (
	ctrlCKey byte = 0x03
	ctrlSKey byte = 0x13
	ctrlVKey byte = 0x16
	ctrlYKey byte = 0x19
)

// mapControlKeyToDecision maps a single key byte to a decision code.
// Returns (decision, true) when a mapping exists, otherwise (0, false).
func mapControlKeyToDecision(key byte, allowEnter bool) (int, bool) {
	switch key {
	case '\r', '\n': // Enter
		if allowEnter {
			return decisionCommit, true
		}
		return 0, false
	case ctrlCKey: // Ctrl-C (ETX)
		return decisionAbort, true
	case ctrlSKey: // Ctrl-S (XOFF)
		return decisionSkip, true
	case ctrlVKey, ctrlYKey: // Ctrl-V/Ctrl-Y (vouch)
		return decisionVouch, true
	case 's', 'S': // Fallback for terminals that intercept Ctrl-S
		return decisionSkip, true
	case 'v', 'V', 'y', 'Y': // Fallback for terminals that intercept Ctrl-V/Ctrl-Y
		return decisionVouch, true
	default:
		return 0, false
	}
}
