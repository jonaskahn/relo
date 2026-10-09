//go:build darwin

package platform

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func portOwnerPID(port int) (int, bool) {
	owners := portOwnerPIDs(port)
	if len(owners) == 0 {
		return 0, false
	}
	return owners[0], true
}

func portOwnerPIDs(port int) []int {
	binary, err := exec.LookPath("lsof")
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-t")
	output, err := command.Output()
	// A port with no listener is the answer lsof reports by exiting non-zero,
	// so only a failure to run at all hides the owner.
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return nil
	}
	var owners []int
	seen := map[int]bool{}
	for _, line := range strings.Split(string(output), "\n") {
		pid, convErr := strconv.Atoi(strings.TrimSpace(line))
		if convErr != nil || pid <= 0 || seen[pid] {
			continue
		}
		seen[pid] = true
		owners = append(owners, pid)
	}
	return owners
}
