// Catalog URL setting: validating and storing the models.dev source.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// UpdateCatalogURL stores the model directory the catalog reads, keeping the
// rest of the startup file.
func UpdateCatalogURL(path, raw string) error {
	raw = strings.TrimSpace(raw)
	if err := validateHTTPURL(raw); err != nil {
		return fmt.Errorf("catalog.modelsdev_url: %w", err)
	}
	appearanceMu.Lock()
	defer appearanceMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	updated := upsertTableKey(string(data), "catalog", "modelsdev_url", fmt.Sprintf("%q", raw))
	return writeFileAtomically(path, updated)
}
