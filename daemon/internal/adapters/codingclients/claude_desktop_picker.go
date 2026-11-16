// Claude Desktop's picker profile: a row in the config library whose only
// content is the egress proxy Desktop sends its network traffic through. It is
// exclusive with the gateway profile, which makes the whole app a Relo client,
// so the two never stand selected together.
package codingclients

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

const (
	// desktopPickerID is a UUID because Desktop only reads an applied profile
	// whose id looks like one.
	desktopPickerID   = "b7d3a41e-52c6-4e08-8f19-3a6c0d92e7f5"
	desktopPickerName = "relo-picker"
	pickerStateFile   = "profile-state.json"
)

// ErrDesktopGatewaySelected reports a picker that was asked for while Desktop is
// wired to Relo's gateway profile. The two cannot both be selected.
var ErrDesktopGatewaySelected = errors.New("claude desktop: the gateway profile is selected")

// DesktopPickerStatus is how the library stands against the picker profile.
type DesktopPickerStatus string

// The statuses the picker profile reports.
const (
	DesktopPickerAbsent      DesktopPickerStatus = "absent"
	DesktopPickerApplied     DesktopPickerStatus = "applied"
	DesktopPickerNotSelected DesktopPickerStatus = "not_selected"
	DesktopPickerUnsafe      DesktopPickerStatus = "unsafe"
)

var egressURL = regexp.MustCompile(`^http://127\.0\.0\.1:([1-9][0-9]{0,4})$`)

// DesktopPickerProfilePath is the file the picker profile lives in.
func DesktopPickerProfilePath(library string) string {
	return desktopJoin(desktopPlatform(library), library, desktopPickerID+".json")
}

// DesktopPickerEgressURL is the address the profile names for a port.
func DesktopPickerEgressURL(port int) string {
	return "http://127.0.0.1:" + strconv.Itoa(port)
}

type pickerState struct {
	PreviousAppliedID string `json:"previousAppliedId"`
}

func pickerStatePath(stateDir string) string { return filepath.Join(stateDir, pickerStateFile) }

func readPickerState(stateDir string) (pickerState, error) {
	content, err := os.ReadFile(pickerStatePath(stateDir))
	if os.IsNotExist(err) {
		return pickerState{}, nil
	}
	if err != nil {
		return pickerState{}, err
	}
	var state pickerState
	if err := json.Unmarshal(content, &state); err != nil || (state.PreviousAppliedID != "" && !safeDesktopID(state.PreviousAppliedID)) {
		return pickerState{}, fmt.Errorf("claude desktop: %w (picker state)", ErrUnrecognisedFile)
	}
	return state, nil
}

func isPickerEntry(entry map[string]any) bool {
	return desktopString(entry["id"]) == desktopPickerID || desktopString(entry["name"]) == desktopPickerName
}

func isGatewayApplied(entries []map[string]any, applied string) bool {
	if applied == desktopProfileID || applied == legacyDesktopProfileID {
		return true
	}
	for _, entry := range entries {
		if desktopOwned(entry) && desktopString(entry["id"]) == applied {
			return true
		}
	}
	return false
}

// ApplyDesktopPickerProfile writes the profile for a port and selects it,
// keeping the selection it replaced in stateDir, never in Desktop's own file.
// It refuses while the gateway profile is selected, and when the library cannot
// be read as one.
func ApplyDesktopPickerProfile(library, stateDir string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("claude desktop: %w (port %d)", ErrUnrecognisedFile, port)
	}
	document, err := loadDesktopMeta(library)
	if err != nil {
		return err
	}
	entries := desktopEntries(document)
	applied := desktopString(document["appliedId"])
	if isGatewayApplied(entries, applied) {
		return ErrDesktopGatewaySelected
	}
	state, err := readPickerState(stateDir)
	if err != nil {
		return err
	}
	previous := state.PreviousAppliedID
	if applied != "" && applied != desktopPickerID {
		previous = applied
	}
	content := []byte(`{"egressProxyUrl":"` + DesktopPickerEgressURL(port) + `"}` + "\n")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	if err := writePickerState(stateDir, pickerState{PreviousAppliedID: previous}); err != nil {
		return err
	}
	if err := WriteFileAtomic(DesktopPickerProfilePath(library), content, 0o600); err != nil {
		return err
	}
	document["entries"] = upsertPickerEntry(entries)
	document["appliedId"] = desktopPickerID
	return writeDesktopMeta(library, document)
}

func writePickerState(stateDir string, state pickerState) error {
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return WriteFileAtomic(pickerStatePath(stateDir), append(encoded, '\n'), 0o600)
}

func upsertPickerEntry(entries []map[string]any) []map[string]any {
	owned := map[string]any{"id": desktopPickerID, "name": desktopPickerName}
	for index, entry := range entries {
		if isPickerEntry(entry) {
			entries[index] = owned
			return entries
		}
	}
	return append(entries, owned)
}

// RemoveDesktopPickerProfile takes the profile out and selects what was
// selected before it. Another profile in the library is left where it is.
func RemoveDesktopPickerProfile(library, stateDir string) error {
	document, err := loadDesktopMeta(library)
	if err != nil {
		return err
	}
	state, err := readPickerState(stateDir)
	if err != nil {
		return err
	}
	entries := desktopEntries(document)
	kept := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if !isPickerEntry(entry) {
			kept = append(kept, entry)
		}
	}
	if desktopString(document["appliedId"]) == desktopPickerID {
		if next := desktopRestoredID(kept, state.PreviousAppliedID); next != "" {
			document["appliedId"] = next
		} else {
			delete(document, "appliedId")
		}
	}
	document["entries"] = kept
	if err := writeDesktopMetaAfterRemove(library, document, kept); err != nil {
		return err
	}
	for _, path := range []string{DesktopPickerProfilePath(library), pickerStatePath(stateDir)} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}
	return nil
}

// InspectDesktopPickerProfile reports how the library stands against the picker
// profile, and the port it names when it is applied.
func InspectDesktopPickerProfile(library string) (DesktopPickerStatus, int) {
	document, err := loadDesktopMeta(library)
	if err != nil {
		return DesktopPickerUnsafe, 0
	}
	var present bool
	for _, entry := range desktopEntries(document) {
		present = present || isPickerEntry(entry)
	}
	if !present {
		return DesktopPickerAbsent, 0
	}
	content, err := os.ReadFile(DesktopPickerProfilePath(library))
	if err != nil {
		return DesktopPickerUnsafe, 0
	}
	var profile map[string]string
	if err := json.Unmarshal(content, &profile); err != nil || len(profile) != 1 {
		return DesktopPickerUnsafe, 0
	}
	match := egressURL.FindStringSubmatch(profile["egressProxyUrl"])
	port, _ := strconv.Atoi(firstOf(match))
	if match == nil || port > 65535 {
		return DesktopPickerUnsafe, 0
	}
	if desktopString(document["appliedId"]) != desktopPickerID {
		return DesktopPickerNotSelected, port
	}
	return DesktopPickerApplied, port
}

func firstOf(match []string) string {
	if len(match) < 2 {
		return ""
	}
	return match[1]
}
