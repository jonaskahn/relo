//go:build windows

package platform

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "netstat", "-ano", "-p", "tcp")
	// A GUI launch has no console to inherit, so netstat would flash its own.
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	output, err := command.Output()
	if err != nil {
		return nil
	}
	var owners []int
	seen := map[int]bool{}
	for line := range strings.SplitSeq(string(output), "\n") {
		pid, found := listeningPID(line, ":"+strconv.Itoa(port))
		if !found || seen[pid] {
			continue
		}
		seen[pid] = true
		owners = append(owners, pid)
	}
	return owners
}

func listeningPID(line, port string) (int, bool) {
	columns := strings.Fields(line)
	if len(columns) < 5 || columns[3] != "LISTENING" {
		return 0, false
	}
	if !strings.HasSuffix(columns[1], port) {
		return 0, false
	}
	pid, err := strconv.Atoi(columns[4])
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}
