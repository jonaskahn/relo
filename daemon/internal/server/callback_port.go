// Callback port ownership: the port a login flow listens on and its conflicts.
package server

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// ErrCallbackPortBusy reports a callback port another process is already
// listening on. A provider that pinned its redirect address leaves Relo no
// port to fall back to, so the operator has to end that process before the
// login can run at all.
var ErrCallbackPortBusy = errors.New("another process is listening on this sign-in port")

// PortBusyError is the refusal a sign-in starts with: the port that is
// taken, and the process the operator would have to end. The console reads
// the owner out of it to name the choice it offers.
type PortBusyError struct {
	Port  int
	Owner PortOwner
}

// Error names the process holding a callback port in the refusal an
// operator reads, so the console can offer to end it.
func (e *PortBusyError) Error() string {
	return fmt.Sprintf("%s is listening on port %d: %s", e.Owner.Name, e.Port, ErrCallbackPortBusy)
}

// PortOwner is the process holding one loopback port, named so an operator
// can recognise it before ending it.
type PortOwner struct {
	PID  int
	Name string
}

// PortInspector reports which process listens on a port and ends it. A server
// built without one treats every port as free, which is what a build with no
// process table to ask does.
type PortInspector interface {
	Owner(port int) (PortOwner, bool)
	Terminate(port int) error
}

func (s *Server) portConflict(flow string) *PortBusyError {
	port := s.callbackPort(flow)
	if s.opts.Ports == nil || port <= 0 {
		return nil
	}
	owner, found := s.opts.Ports.Owner(port)
	if !found || owner.PID == os.Getpid() {
		return nil
	}
	return &PortBusyError{Port: port, Owner: owner}
}

func (s *Server) callbackPort(flow string) int {
	if s.opts.CallbackPorts == nil {
		return 0
	}
	return s.opts.CallbackPorts.CallbackPortFor(flow)
}

func (s *Server) freeCallbackPort(ctx context.Context, port int) error {
	if s.opts.Ports == nil || port <= 0 {
		return nil
	}
	owner, found := s.opts.Ports.Owner(port)
	if !found {
		return nil
	}
	// This daemon listening on the port is a login already in flight, not a
	// stale listener, so ending it would fail the login the operator is
	// watching rather than clear the way for a new one.
	if owner.PID == os.Getpid() {
		return fmt.Errorf("port %d is held by this daemon: %w", port, ErrCallbackPortBusy)
	}
	if err := s.opts.Ports.Terminate(port); err != nil {
		return err
	}
	return nil
}
