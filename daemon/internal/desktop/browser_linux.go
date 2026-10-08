//go:build linux

package desktop

import "os/exec"

func openCommand(url string) (*exec.Cmd, error) {
	return exec.Command("xdg-open", url), nil
}
