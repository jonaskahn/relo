// Runtime file: the running daemon's port and instance.
package platform

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

const (
	// runtimeFileName is what a headless daemon publishes about itself, so
	// the lifecycle commands can find, address, and stop it again.
	runtimeFileName = "runtime.json"
	// RuntimeFileMode keeps the published address and instance to the
	// operator who started the daemon.
	RuntimeFileMode = 0o600
)

// Runtime is what a headless daemon tells the lifecycle commands: where it
// listens, which run it is, and the process that owns it.
type Runtime struct {
	Address    string `json:"address"`
	InstanceID string `json:"instance_id"`
	PID        int    `json:"pid"`
	// Listeners are every address this process bound, management included.
	// A file written before they were published leaves this empty, and a
	// stop then waits only for the management listener.
	Listeners []string `json:"listeners,omitempty"`
}

// RuntimePath returns the path of the runtime file in a state directory.
func RuntimePath(home string) string {
	return filepath.Join(home, runtimeFileName)
}

// Port returns the port the published address listens on, and zero when the
// address does not name one.
func (r Runtime) Port() int {
	_, port, err := net.SplitHostPort(r.Address)
	if err != nil {
		return 0
	}
	value, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return value
}

// ReadRuntime reads what a daemon published, and reports whether a usable
// file is there.
func ReadRuntime(home string) (Runtime, bool) {
	content, err := os.ReadFile(RuntimePath(home))
	if err != nil {
		return Runtime{}, false
	}
	var published Runtime
	if err := json.Unmarshal(content, &published); err != nil {
		return Runtime{}, false
	}
	if published.Address == "" || published.InstanceID == "" {
		return Runtime{}, false
	}
	return published, true
}

// RemoveRuntime deletes the runtime file, ignoring one that is already gone.
func RemoveRuntime(home string) {
	_ = os.Remove(RuntimePath(home))
}

func removeOwnRuntime(home, instanceID string) {
	published, found := ReadRuntime(home)
	if !found {
		return
	}
	if published.InstanceID != instanceID || published.PID != os.Getpid() {
		return
	}
	RemoveRuntime(home)
}

func publishRuntime(home string, published Runtime) error {
	content, err := json.Marshal(published)
	if err != nil {
		return err
	}
	path := RuntimePath(home)
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, RuntimeFileMode)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(temporary)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return os.Rename(temporary, path)
}
