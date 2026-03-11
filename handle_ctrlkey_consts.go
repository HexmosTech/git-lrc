package main

import "github.com/HexmosTech/git-lrc/interactive/keybinding"

const (
	ctrlCKey byte = keybinding.CtrlCKey
	ctrlSKey byte = keybinding.CtrlSKey
	ctrlVKey byte = keybinding.CtrlVKey
	ctrlYKey byte = keybinding.CtrlYKey
)

func mapControlKeyToDecision(key byte, allowEnter bool) (int, bool) {
	code, ok := keybinding.MapControlKeyToDecision(key, allowEnter)
	if !ok {
		return 0, false
	}
	return mapKeybindingDecisionToMain(code), true
}

func mapKeybindingDecisionToMain(code int) int {
	switch code {
	case keybinding.DecisionAbort:
		return decisionAbort
	case keybinding.DecisionSkip:
		return decisionSkip
	case keybinding.DecisionVouch:
		return decisionVouch
	default:
		return decisionCommit
	}
}
