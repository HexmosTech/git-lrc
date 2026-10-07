package appdw

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
)

// pickPort binds the first free port at or after preferredPort.
func pickPort(preferredPort int) (net.Listener, int, error) {
	lastPort := 49151
	if preferredPort > lastPort {
		lastPort = 65535
	}
	for candidate := preferredPort; candidate <= lastPort; candidate++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", candidate))
		if err == nil {
			return ln, candidate, nil
		}
	}
	return nil, 0, fmt.Errorf("no available port found in range %d-%d", preferredPort, lastPort)
}

// openURL opens a URL in the default browser.
func openURL(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	return exec.Command(cmd, args...).Start()
}

// trimSpace is a tiny helper mirroring strings.TrimSpace for readability.
func trimSpace(s string) string {
	return strings.TrimSpace(s)
}
