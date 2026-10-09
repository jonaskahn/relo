//go:build linux

package platform

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var procNetTCP = []string{"/proc/net/tcp", "/proc/net/tcp6"}

const tcpListenState = "0A"

func portOwnerPID(port int) (int, bool) {
	owners := portOwnerPIDs(port)
	if len(owners) == 0 {
		return 0, false
	}
	return owners[0], true
}

func portOwnerPIDs(port int) []int {
	inodes := listeningInodes(port)
	if len(inodes) == 0 {
		return nil
	}
	return pidsHoldingInodes(inodes)
}

func listeningInodes(port int) map[string]bool {
	inodes := map[string]bool{}
	wanted := strconv.FormatUint(uint64(port), 16)
	for _, table := range procNetTCP {
		file, err := os.Open(table)
		if err != nil {
			continue
		}
		for _, inode := range listenInodesFrom(file, wanted) {
			inodes[inode] = true
		}
		_ = file.Close()
	}
	return inodes
}

func listenInodesFrom(file *os.File, wanted string) []string {
	var inodes []string
	scanner := bufio.NewScanner(file)
	for line := 0; scanner.Scan(); line++ {
		if line == 0 {
			continue
		}
		columns := strings.Fields(scanner.Text())
		if len(columns) < 10 || columns[3] != tcpListenState {
			continue
		}
		if localPort(columns[1]) != wanted {
			continue
		}
		inodes = append(inodes, columns[9])
	}
	return inodes
}

func localPort(address string) string {
	_, port, found := strings.Cut(address, ":")
	if !found {
		return ""
	}
	return strings.ToLower(strings.TrimLeft(port, "0"))
}

func pidsHoldingInodes(inodes map[string]bool) []int {
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var owners []int
	seen := map[int]bool{}
	for _, proc := range procs {
		pid, err := strconv.Atoi(proc.Name())
		if err != nil || seen[pid] {
			continue
		}
		if holdsSocket(filepath.Join("/proc", proc.Name(), "fd"), inodes) {
			seen[pid] = true
			owners = append(owners, pid)
		}
	}
	return owners
}

func holdsSocket(fdDir string, inodes map[string]bool) bool {
	descriptors, err := os.ReadDir(fdDir)
	if err != nil {
		return false
	}
	for _, descriptor := range descriptors {
		target, err := os.Readlink(filepath.Join(fdDir, descriptor.Name()))
		if err != nil {
			continue
		}
		if inode, isSocket := socketInode(target); isSocket && inodes[inode] {
			return true
		}
	}
	return false
}

func socketInode(target string) (string, bool) {
	inode, found := strings.CutPrefix(target, "socket:[")
	if !found {
		return "", false
	}
	return strings.TrimSuffix(inode, "]"), true
}
