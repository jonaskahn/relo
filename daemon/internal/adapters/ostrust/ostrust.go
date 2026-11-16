// Asking the operating system to trust Relo's root. It is only ever done on an
// operator's explicit request, through the OS's own consent prompt, and only for
// a root whose constraint was checked first. Every OS command goes through a
// Runner, so the behavior is tested without touching a real keychain.
package ostrust

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// State is where one root stands in the OS trust store.
type State string

// The states a root reports. Unknown is never treated as trusted: arming on a
// guess would cut an app off from a host whose certificate its OS rejects.
const (
	Trusted     State = "trusted"
	Untrusted   State = "untrusted"
	Unsupported State = "unsupported"
	Unknown     State = "unknown"
)

// ErrDeclined reports a trust request the operator turned down at the OS
// prompt, or that the OS refused.
var ErrDeclined = errors.New("the operating system did not trust the certificate")

// ErrUnsupported reports an OS Relo does not install trust on.
var ErrUnsupported = errors.New("trusting a certificate is not supported on this operating system")

// ErrUnavailable reports an install that nothing on this machine can do by
// itself: no way to ask for the administrator approval it needs, or no tool for
// the store. The operator installs the certificate by hand instead.
var ErrUnavailable = errors.New("this machine has no way to install the certificate automatically")

// Scope is how widely a root is trusted. The user's own store reaches the apps
// that user runs; the system store reaches every app and every user, which is
// where a virtual machine's host-trust copy is read from, and it needs an
// administrator's approval to change.
type Scope string

// The scopes a root is trusted in.
const (
	ScopeUser   Scope = ""
	ScopeSystem Scope = "system"
)

// Root is the certificate an OS is asked to trust, and a leaf it issued, which
// is what proves the trust works for the host the root is for.
type Root struct {
	PEM        []byte
	SHA1       string
	SHA256     string
	CommonName string
	Host       string
	LeafPEM    []byte
	// Scope is where the root is trusted. The zero value is the user's own store.
	Scope Scope
}

// Store reads and changes the OS's trust of a root.
type Store interface {
	Inspect(ctx context.Context, root Root) State
	Trust(ctx context.Context, root Root) error
	Untrust(ctx context.Context, root Root) error
}

// Result is what a command left behind.
type Result struct {
	Stdout string
	Stderr string
	Code   int
}

// Runner runs one OS command. An error means the command could not run at all;
// a command that ran and failed is a Result with a code.
type Runner func(ctx context.Context, name string, args ...string) (Result, error)

// ExecRunner runs a command on this machine.
func ExecRunner(ctx context.Context, name string, args ...string) (Result, error) {
	command := exec.CommandContext(ctx, name, args...)
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return Result{Stdout: stdout.String(), Stderr: stderr.String()}, nil
	case errors.As(err, &exit):
		return Result{Stdout: stdout.String(), Stderr: stderr.String(), Code: exit.ExitCode()}, nil
	default:
		return Result{}, err
	}
}

// New returns the store for an OS. An OS Relo does not support gets one that
// reports so and changes nothing.
func New(goos, home string, run Runner) Store {
	if run == nil {
		run = ExecRunner
	}
	switch goos {
	case "darwin":
		return &keychain{home: home, run: run}
	case "windows":
		return &windowsStore{run: run}
	case "linux":
		return &linuxStore{home: home, run: run}
	default:
		return unsupported{}
	}
}

type unsupported struct{}

func (unsupported) Inspect(context.Context, Root) State { return Unsupported }
func (unsupported) Trust(context.Context, Root) error   { return ErrUnsupported }
func (unsupported) Untrust(context.Context, Root) error { return ErrUnsupported }

// privateCopy writes bytes to a fresh 0700 directory and answers the file and a
// cleanup. The OS command reads that copy rather than a shared path, which
// another process of the same user could swap after the check.
func privateCopy(name string, content []byte) (string, func(), error) {
	dir, err := os.MkdirTemp("", "relo-trust-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}
