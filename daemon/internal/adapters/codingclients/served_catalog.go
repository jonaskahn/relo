package codingclients

import (
	"os"
	"path/filepath"
	"strings"
)

// PickerID is the id Claude Code's picker lists a model under.
func PickerID(model ModelRef) string {
	return gatewayPickerID(model)
}

// DropServedCatalog removes the catalogs Claude Code cached from the vendor's
// model_selector endpoint (<configDir>/cache/model-catalog/*-cc.json). The
// client trusts a fresh copy for about an hour, so without this a change to
// the rows Relo adds shows only after the cache expires. Desktop's *-ccd.json
// is left alone. Best effort: it answers how many files it removed, and a file
// that is already gone or locked is skipped.
func DropServedCatalog(configDir string) int {
	dir := filepath.Join(configDir, "cache", "model-catalog")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "-cc.json") {
			continue
		}
		if os.Remove(filepath.Join(dir, entry.Name())) == nil {
			removed++
		}
	}
	return removed
}
