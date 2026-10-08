//go:build darwin

package desktop

import "os/exec"

func openCommand(url string) (*exec.Cmd, error) {
	return exec.Command("open", url), nil
}
