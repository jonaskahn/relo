//go:build darwin

package platform

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

func portOwnerPID(port int) (int, bool) {
	binary, err := exec.LookPath("lsof")
	if err != nil {
		return 0, false
	}
	command := exec.Command(binary, "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-t")
	output, err := command.Output()
	// A port with no listener is the answer lsof reports by exiting non-zero,
	// so only a failure to run at all hides the owner.
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return 0, false
	}
	for _, line := range strings.Split(string(output), "\n") {
		if pid, convErr := strconv.Atoi(strings.TrimSpace(line)); convErr == nil && pid > 0 {
			return pid, true
		}
	}
	return 0, false
}
