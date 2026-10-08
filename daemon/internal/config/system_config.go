// System settings: autostart, logging, and update source.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

// SystemConfigPatch names the [system] choices a settings save carries; a nil
// field keeps the stored value.
type SystemConfigPatch struct {
	Autostart       *bool
	LogLevel        *string
	UpdatesURL      *string
	UpdatesDownload *string
}

// UpdateSystemConfig stores the daemon behavior choices: start at login, log
// level, and the update feed. The rest of the startup file stays.
func UpdateSystemConfig(path string, patch SystemConfigPatch) error {
	if err := validateSystemPatch(patch); err != nil {
		return err
	}
	appearanceMu.Lock()
	defer appearanceMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return writeFileAtomically(path, writeSystemPatch(string(data), patch))
}

func validateSystemPatch(patch SystemConfigPatch) error {
	if patch.LogLevel != nil {
		if err := validateLogLevel(*patch.LogLevel); err != nil {
			return err
		}
	}
	if patch.UpdatesURL != nil {
		if err := validateUpdatesURL("system.updates.url", *patch.UpdatesURL); err != nil {
			return err
		}
	}
	if patch.UpdatesDownload != nil {
		if err := validateUpdatesURL("system.updates.download", *patch.UpdatesDownload); err != nil {
			return err
		}
	}
	return nil
}

func writeSystemPatch(updated string, patch SystemConfigPatch) string {
	if patch.Autostart != nil {
		updated = upsertTableKey(updated, "system", "autostart", strconv.FormatBool(*patch.Autostart))
	}
	if patch.LogLevel != nil {
		updated = upsertTableKey(updated, "system.logging", "level", fmt.Sprintf("%q", *patch.LogLevel))
	}
	if patch.UpdatesURL != nil {
		updated = upsertTableKey(updated, "system.updates", "url", fmt.Sprintf("%q", *patch.UpdatesURL))
	}
	if patch.UpdatesDownload != nil {
		updated = upsertTableKey(updated, "system.updates", "download", fmt.Sprintf("%q", *patch.UpdatesDownload))
	}
	return updated
}

func validateUpdatesURL(path, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if err := validateHTTPURL(raw); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
