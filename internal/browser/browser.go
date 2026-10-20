// Package browser opens URLs with the system's link handler.
package browser

import (
	"errors"
	"os/exec"
	"runtime"
)

// Open starts the system handler for url and returns without waiting. The
// handler's output is discarded so it cannot corrupt the TUI.
func Open(url string) error {
	name, args := command(runtime.GOOS, url)
	if name == "" {
		return errors.New("no browser launcher for " + runtime.GOOS)
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // reap the child
	return nil
}

// command picks the launcher for an OS ("" when unknown).
func command(goos, url string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{url}
	case "linux", "freebsd", "openbsd", "netbsd":
		return "xdg-open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	}
	return "", nil
}
