package keybinding

const (
	CtrlCKey byte = 0x03
	CtrlSKey byte = 0x13
	CtrlVKey byte = 0x16
	CtrlYKey byte = 0x19
)

const (
	DecisionCommit = 0
	DecisionAbort  = 1
	DecisionSkip   = 2
	DecisionVouch  = 4
)

// MapControlKeyToDecision maps a single key byte to a decision code.
// Returns (decision, true) when a mapping exists, otherwise (0, false).
func MapControlKeyToDecision(key byte, allowEnter bool) (int, bool) {
	switch key {
	case '\r', '\n':
		if allowEnter {
			return DecisionCommit, true
		}
		return 0, false
	case CtrlCKey:
		return DecisionAbort, true
	case CtrlSKey:
		return DecisionSkip, true
	case CtrlVKey, CtrlYKey:
		return DecisionVouch, true
	case 's', 'S':
		return DecisionSkip, true
	case 'v', 'V', 'y', 'Y':
		return DecisionVouch, true
	default:
		return 0, false
	}
}
