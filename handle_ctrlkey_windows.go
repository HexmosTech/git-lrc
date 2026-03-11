//go:build windows

package main

import (
	"errors"

	"github.com/HexmosTech/git-lrc/interactive/keybinding"
)

func handleCtrlKeyWithCancel(stop <-chan struct{}, allowEnter bool) (int, error) {
	code, err := keybinding.HandleCtrlKeyWithCancel(stop, allowEnter)
	if errors.Is(err, keybinding.ErrInputCancelled) {
		return 0, errInputCancelled
	}
	if err != nil {
		return 0, err
	}
	return mapKeybindingDecisionToMain(code), nil
}
