//go:build !windows

package main

import (
	"fmt"
	"syscall"
	"time"

	"golang.org/x/term"
)

const (
	readPollInterval = 30 * time.Millisecond
	emptyReadBackoff = 10 * time.Millisecond
)

// handleCtrlKeyWithCancel sets up a non-blocking raw terminal reader to detect Ctrl-S (skip),
// Ctrl-V (vouch), and Ctrl-C (abort). Returns a decision code constant or 0 on cancellation/failure.
func handleCtrlKeyWithCancel(stop <-chan struct{}) (int, error) {
	tty, err := openTTY()
	if err != nil {
		return 0, err
	}

	fd := int(tty.Fd())
	if err := syscall.SetNonblock(fd, true); err != nil {
		tty.Close()
		return 0, err
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		tty.Close()
		return 0, err
	}

	cleanup := func() error {
		var firstErr error
		if err := term.Restore(fd, oldState); err != nil {
			firstErr = err
		}
		if err := syscall.SetNonblock(fd, false); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := tty.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		return firstErr
	}

	codeChan := make(chan int, 1)
	errChan := make(chan error, 1)
	readerDone := make(chan struct{})

	go func() {
		defer close(readerDone)
		buf := make([]byte, 1)
		for {
			select {
			case <-stop:
				return
			default:
			}

			n, err := syscall.Read(fd, buf)
			if err != nil {
				if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
					time.Sleep(readPollInterval)
					continue
				}
				if err == syscall.EINTR {
					continue
				}
				errChan <- err
				return
			}
			if n == 0 {
				time.Sleep(emptyReadBackoff)
				continue
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

	waitDone := func(resultCode int, resultErr error) (int, error) {
		<-readerDone
		if cerr := cleanup(); cerr != nil {
			if resultErr != nil {
				resultErr = fmt.Errorf("%w (cleanup error: %v)", resultErr, cerr)
			} else {
				resultErr = fmt.Errorf("cleanup error: %w", cerr)
			}
		}
		return resultCode, resultErr
	}

	select {
	case code := <-codeChan:
		return waitDone(code, nil)
	case err := <-errChan:
		return waitDone(0, err)
	case <-stop:
		return waitDone(0, fmt.Errorf("cancelled"))
	}
}
