// Model-list diffing: what changed between a client file and Relo's models.
package codingclients

import "strings"

// ModelsDiff counts what changed between the models a client file lists and
// the ones Relo would write now. Added counts entries the file lacks or names
// differently; Removed counts entries the file holds that Relo no longer
// publishes.
type ModelsDiff struct {
	Added   int
	Removed int
}

// Empty reports a file that already carries the published models.
func (d ModelsDiff) Empty() bool {
	return d.Added == 0 && d.Removed == 0
}

// diffNames counts the entries one file lists against the ones Relo wants.
func diffNames(listed, wanted map[string]string) ModelsDiff {
	var diff ModelsDiff
	for id, name := range wanted {
		if got, found := listed[id]; !found || got != name {
			diff.Added++
		}
	}
	for id := range listed {
		if _, found := wanted[id]; !found {
			diff.Removed++
		}
	}
	return diff
}

// wantedCodexNames maps the published models to the names the catalog carries.
func wantedCodexNames(models []ModelRef) map[string]string {
	wanted := make(map[string]string, len(models))
	for _, model := range models {
		if id := strings.TrimSpace(model.ID); id != "" {
			wanted[id] = ConfigDisplayName(model)
		}
	}
	return wanted
}

// wantedPickerNames maps the published models to the names the picker cache
// carries.
func wantedPickerNames(models []ModelRef) map[string]string {
	wanted := make(map[string]string, len(models))
	for _, model := range models {
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		wanted[gatewayPickerID(model)] = ConfigDisplayName(model)
	}
	return wanted
}
