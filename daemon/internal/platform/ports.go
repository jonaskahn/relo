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

// Terminate ends the process listening on a port. The caller has already
// shown the operator which process that is, so this does not check what the
// process is: a stale Relo, a vendor CLI, and a leftover login all need the
// same ending.
func (portInspector) Terminate(port int) error {
	pid, found := portOwnerPID(port)
	if !found {
		return fmt.Errorf("port %d: %w", port, ErrNoPortOwner)
	}
	// A caller that frees the ports its own process serves (a restarted run)
	// must never end itself: the listeners it holds are the ones it is about
	// to replace.
	if pid == os.Getpid() {
		return nil
	}
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
