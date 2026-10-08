//go:build windows

package desktop

import "os/exec"

func openCommand(url string) (*exec.Cmd, error) {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url), nil
}
