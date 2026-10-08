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
	Bind          *string
	Port          *int
	OpenAIPort    *int
	AnthropicPort *int
	GeminiPort    *int
}

// UpdateServerConfig stores where the management listener and the data plane
// answer. The whole candidate configuration is checked first, so a bind or a
// port that would leave the next start unaccepted is refused. The rest of the
// startup file stays.
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
	if patch.OpenAIPort != nil {
		candidate.Server.DataPlane.OpenAI = *patch.OpenAIPort
	}
	if patch.AnthropicPort != nil {
		candidate.Server.DataPlane.Anthropic = *patch.AnthropicPort
	}
	if patch.GeminiPort != nil {
		candidate.Server.DataPlane.Gemini = *patch.GeminiPort
	}
}

func writeServerPatch(updated string, patch ServerConfigPatch) string {
	if patch.Bind != nil {
		updated = upsertTableKey(updated, "server", "bind", fmt.Sprintf("%q", *patch.Bind))
	}
	if patch.Port != nil {
		updated = upsertTableKey(updated, "server", "port", strconv.Itoa(*patch.Port))
	}
	if patch.OpenAIPort != nil {
		updated = upsertTableKey(updated, "server.data_plane", "openai", strconv.Itoa(*patch.OpenAIPort))
	}
	if patch.AnthropicPort != nil {
		updated = upsertTableKey(updated, "server.data_plane", "anthropic", strconv.Itoa(*patch.AnthropicPort))
	}
	if patch.GeminiPort != nil {
		updated = upsertTableKey(updated, "server.data_plane", "gemini", strconv.Itoa(*patch.GeminiPort))
	}
	return updated
}
