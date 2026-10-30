// Connections settings: the opt-in for the keyless free connections.
package config

import (
	"errors"
	"io/fs"
	"os"
	"strconv"
)

// UpdateConnectionsConfig stores whether the keyless free connections are
// enabled, keeping the rest of the startup file.
func UpdateConnectionsConfig(path string, free bool) error {
	appearanceMu.Lock()
	defer appearanceMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return writeFileAtomically(path, upsertTableKey(string(data), "connections", "free", strconv.FormatBool(free)))
}

// ReadFreeConnections reads the free-connections opt-in from the startup
// file on every call, so saving the choice takes effect at once. An unreadable
// file reads as off: consent is never assumed from a failure.
func ReadFreeConnections(path string) bool {
	cfg, err := ReadFile(path)
	if err != nil {
		return false
	}
	return cfg.Connections.Free
}
