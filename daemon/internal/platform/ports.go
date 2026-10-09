// Port inspection: ownership and termination for callbacks.
package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jonaskahn/relo/internal/server"
)

// ErrNoPortOwner reports a port no process is listening on, so there is
// nothing to end.
var ErrNoPortOwner = errors.New("no process is listening on that port")

type portInspector struct{}

var _ server.PortInspector = portInspector{}

// NewPortInspector returns the process table reader this build ships.
func NewPortInspector() server.PortInspector {
	return portInspector{}
}

// Owner reports the process listening on a port, or false when the port is
// free or the platform cannot say. A build that cannot answer reports the
// port as free, which is what a sign-in has always assumed.
func (portInspector) Owner(port int) (server.PortOwner, bool) {
	pid, found := portOwnerPID(port)
	if !found {
		return server.PortOwner{}, false
	}
	return server.PortOwner{PID: pid, Name: processName(pid)}, true
}

// Terminate ends every process listening on a port. The caller has already
// shown the operator which process that is, or is quitting and ending
// everything the configuration asked the run to serve, so this does not
// check what a process is. The current process is spared either way: its own
// listeners are about to be replaced, or it is the caller doing the ending.
func (portInspector) Terminate(port int) error {
	pids := portOwnerPIDs(port)
	if len(pids) == 0 {
		return fmt.Errorf("port %d: %w", port, ErrNoPortOwner)
	}
	var failures []error
	self := os.Getpid()
	for _, pid := range pids {
		if pid == self {
			continue
		}
		if err := endListener(pid, port); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func endListener(pid, port int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find the process on port %d: %w", port, err)
	}
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("end the process on port %d: %w", port, err)
	}
	return nil
}

func processName(pid int) string {
	path, err := processExecutable(pid)
	if err != nil || path == "" {
		return fmt.Sprintf("pid %d", pid)
	}
	return filepath.Base(path)
}
