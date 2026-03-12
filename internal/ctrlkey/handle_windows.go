//go:build windows

package ctrlkey

import (
	"errors"

	"github.com/HexmosTech/git-lrc/interactive/keybinding"
	"github.com/HexmosTech/git-lrc/internal/reviewapi"
)

func HandleWithCancel(stop <-chan struct{}, allowEnter bool) (int, error) {
	code, err := keybinding.HandleCtrlKeyWithCancel(stop, allowEnter)
	if errors.Is(err, keybinding.ErrInputCancelled) {
		return 0, reviewapi.ErrInputCancelled
	}
	if err != nil {
		return 0, err
	}
	return mapKeybindingDecisionToDecisionFlow(code), nil
}
