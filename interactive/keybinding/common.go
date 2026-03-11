package keybinding

import (
	"errors"
	"os"
	"runtime"
)

var ErrInputCancelled = errors.New("terminal input cancelled")

func openTTY() (*os.File, error) {
	if runtime.GOOS == "windows" {
		return os.OpenFile("CONIN$", os.O_RDWR, 0)
	}
	return os.Open("/dev/tty")
}
