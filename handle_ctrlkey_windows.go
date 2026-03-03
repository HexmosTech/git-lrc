//go:build windows

package main

import (
	"fmt"

	"golang.org/x/term"
)

// Windows console input does not support non-blocking reads via syscall without
// additional APIs, so we fall back to the original blocking implementation.
func handleCtrlKeyWithCancel(stop <-chan struct{}) (int, error) {
	tty, err := openTTY()
	if err != nil {
		return 0, err
	}
	defer tty.Close()

	fd := int(tty.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return 0, err
	}
	defer term.Restore(fd, oldState)

	buf := make([]byte, 1)
	codeChan := make(chan int, 1)
	errChan := make(chan error, 1)

	go func() {
		for {
			n, err := tty.Read(buf)
			if err != nil || n == 0 {
				errChan <- err
				return
			}
			switch buf[0] {
			case ctrlCKey: // Ctrl-C (ETX)
				codeChan <- decisionAbort
				return
			case ctrlSKey: // Ctrl-S (XOFF)
				codeChan <- decisionSkip
				return
			case ctrlVKey: // Ctrl-V (SYN)
				codeChan <- decisionVouch
				return
			}
		}
	}()

	select {
	case code := <-codeChan:
		return code, nil
	case err := <-errChan:
		return 0, err
	case <-stop:
		return 0, fmt.Errorf("cancelled")
	}
}
