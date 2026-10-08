//go:build windows

package platform

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func portOwnerPID(port int) (int, bool) {
	command := exec.Command("netstat", "-ano", "-p", "tcp")
	// A GUI launch has no console to inherit, so netstat would flash its own.
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	output, err := command.Output()
	if err != nil {
		return 0, false
	}
	wanted := ":" + strconv.Itoa(port)
	for line := range strings.SplitSeq(string(output), "\n") {
		pid, found := listeningPID(line, wanted)
		if found {
			return pid, true
		}
	}
	return 0, false
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
