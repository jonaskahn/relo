// Browser launching for sign-ins, including headless environments.
package oauth

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// Browser opens a URL in the operator's browser.
type Browser interface {
	Open(url string) error
}

// OpenBrowser opens url with the platform URL handler, and reports
// ErrBrowserUnavailable when this host has none.
func OpenBrowser(url string) error {
	return platformBrowser{}.Open(url)
}

// Headless reports whether Relo must assume no browser can open: an
// explicit setting, a CI runner, or Linux without a display.
func Headless(explicit bool) bool {
	if explicit || os.Getenv("CI") != "" {
		return true
	}
	if runtime.GOOS != "linux" {
		return false
	}
	return os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
}

type platformBrowser struct {
	headless bool
}

// Open launches the sign-in page in the operator's browser, refusing where
// no display exists to open one on.
func (p platformBrowser) Open(url string) error {
	if Headless(p.headless) {
		return ErrBrowserUnavailable
	}
	name, args := openCommand(url)
	command := exec.Command(name, args...)
	if err := command.Start(); err != nil {
		return fmt.Errorf("open the browser with %s: %w", name, ErrBrowserUnavailable)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release the browser process: %w", err)
	}
	return nil
}

func openCommand(url string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}

func openBrowser(opts LoginOpts, browser Browser, url string) error {
	if opts.NoBrowser {
		return nil
	}
	if browser == nil {
		browser = platformBrowser{}
	}
	err := browser.Open(url)
	if errors.Is(err, ErrBrowserUnavailable) {
		return nil
	}
	return err
}
