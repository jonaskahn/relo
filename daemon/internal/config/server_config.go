// Server settings: validating and storing listener fields.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"

	"github.com/BurntSushi/toml"
)

// ServerConfigPatch names the listener choices a settings save carries; a nil
// field keeps the stored value.
type ServerConfigPatch struct {
	Bind *string
	Port *int
}

// UpdateServerConfig stores where the single listener answers. The whole
// candidate configuration is checked first, so a bind or a port that would
// leave the next start unaccepted is refused. The rest of the startup file
// stays.
func UpdateServerConfig(path string, patch ServerConfigPatch) error {
	appearanceMu.Lock()
	defer appearanceMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	candidate := DefaultConfig()
	if len(data) > 0 {
		if _, err := toml.Decode(string(data), &candidate); err != nil {
			return fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	applyServerPatch(&candidate, patch)
	if err := validateServer(&candidate); err != nil {
		return err
	}
	if err := validateAdmin(&candidate); err != nil {
		return err
	}
	return writeFileAtomically(path, writeServerPatch(string(data), patch))
}

func applyServerPatch(candidate *Config, patch ServerConfigPatch) {
	if patch.Bind != nil {
		candidate.Server.Bind = *patch.Bind
	}
	if patch.Port != nil {
		candidate.Server.Port = *patch.Port
	}
}

func writeServerPatch(updated string, patch ServerConfigPatch) string {
	if patch.Bind != nil {
		updated = upsertTableKey(updated, "server", "bind", fmt.Sprintf("%q", *patch.Bind))
	}
	if patch.Port != nil {
		updated = upsertTableKey(updated, "server", "port", strconv.Itoa(*patch.Port))
	}
	return updated
}
