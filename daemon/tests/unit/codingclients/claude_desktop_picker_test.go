package codingclients_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

func pickerLibrary(t *testing.T, meta string) (library, state string) {
	t.Helper()
	root := t.TempDir()
	library = filepath.Join(root, "configLibrary")
	if err := os.MkdirAll(library, 0o755); err != nil {
		t.Fatal(err)
	}
	if meta != "" {
		if err := os.WriteFile(filepath.Join(library, "_meta.json"), []byte(meta), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return library, filepath.Join(root, "state")
}

func appliedID(t *testing.T, library string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(library, "_meta.json"))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		AppliedID string `json:"appliedId"`
	}
	_ = json.Unmarshal(content, &meta)
	return meta.AppliedID
}

const operatorMeta = `{"appliedId":"11111111-1111-4111-8111-111111111111","entries":[{"id":"11111111-1111-4111-8111-111111111111","name":"mine"}]}`

// TestThePickerProfileHoldsOnlyTheEgressProxyAndRemembersTheSelectionItReplaced
// pins the write: the profile names the loopback proxy and nothing else, the
// operator's own selection is remembered outside Desktop's file, and removing it
// puts that selection back and leaves the operator's profile where it was.
func TestThePickerProfileHoldsOnlyTheEgressProxyAndRemembersTheSelectionItReplaced(t *testing.T) {
	library, state := pickerLibrary(t, operatorMeta)
	if err := codingclients.ApplyDesktopPickerProfile(library, state, 51234); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	content, _ := os.ReadFile(codingclients.DesktopPickerProfilePath(library))
	if strings.TrimSpace(string(content)) != `{"egressProxyUrl":"http://127.0.0.1:51234"}` {
		t.Fatalf("profile = %s, want only the egress proxy", content)
	}
	status, port := codingclients.InspectDesktopPickerProfile(library)
	if status != codingclients.DesktopPickerApplied || port != 51234 {
		t.Fatalf("inspect = %s %d, want applied on 51234", status, port)
	}
	if strings.Contains(metaText(t, library), "previousAppliedId") {
		t.Fatal("the previous selection was written into Desktop's own metadata")
	}
	if err := codingclients.RemoveDesktopPickerProfile(library, state); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if got := appliedID(t, library); got != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("applied = %q, want the operator's own profile back", got)
	}
	if status, _ := codingclients.InspectDesktopPickerProfile(library); status != codingclients.DesktopPickerAbsent {
		t.Fatalf("status after removal = %s, want absent", status)
	}
	if _, err := os.Stat(codingclients.DesktopPickerProfilePath(library)); !os.IsNotExist(err) {
		t.Fatalf("the profile file is still there: %v", err)
	}
}

func TestApplyingTwiceKeepsTheFirstSelectionAndFollowsAMovedPort(t *testing.T) {
	library, state := pickerLibrary(t, operatorMeta)
	for _, port := range []int{51234, 51999} {
		if err := codingclients.ApplyDesktopPickerProfile(library, state, port); err != nil {
			t.Fatalf("Apply(%d) error = %v", port, err)
		}
	}
	if _, port := codingclients.InspectDesktopPickerProfile(library); port != 51999 {
		t.Fatalf("port = %d, want the moved port", port)
	}
	if err := codingclients.RemoveDesktopPickerProfile(library, state); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if got := appliedID(t, library); got != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("applied = %q, want the operator's profile after two applies", got)
	}
}

// TestThePickerProfileRefusesWhileTheGatewayProfileIsSelected keeps the two
// profiles exclusive: the gateway profile makes the whole app a Relo client, and
// replacing it silently would undo that mode.
func TestThePickerProfileRefusesWhileTheGatewayProfileIsSelected(t *testing.T) {
	library, state := pickerLibrary(t, "")
	if _, _, err := codingclients.WriteDesktopProfile(library, "http://127.0.0.1:10101", "rlo_ak_x", nil); err != nil {
		t.Fatalf("WriteDesktopProfile() error = %v", err)
	}
	if err := codingclients.ApplyDesktopPickerProfile(library, state, 51234); !errors.Is(err, codingclients.ErrDesktopGatewaySelected) {
		t.Fatalf("Apply() error = %v, want %v", err, codingclients.ErrDesktopGatewaySelected)
	}
}

func TestAnUnreadableLibraryIsRefusedAndAForeignPortIsUnsafe(t *testing.T) {
	library, state := pickerLibrary(t, "not json")
	if err := codingclients.ApplyDesktopPickerProfile(library, state, 51234); err == nil {
		t.Fatal("Apply() on an unreadable library error = nil")
	}
	library, state = pickerLibrary(t, operatorMeta)
	if err := codingclients.ApplyDesktopPickerProfile(library, state, 51234); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	profile := codingclients.DesktopPickerProfilePath(library)
	if err := os.WriteFile(profile, []byte(`{"egressProxyUrl":"http://10.0.0.5:8080"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if status, _ := codingclients.InspectDesktopPickerProfile(library); status != codingclients.DesktopPickerUnsafe {
		t.Fatalf("status = %s, want unsafe for a proxy that is not loopback", status)
	}
}

func TestRemovingAPickerThatWasNeverAppliedChangesNothing(t *testing.T) {
	library, state := pickerLibrary(t, operatorMeta)
	if err := codingclients.RemoveDesktopPickerProfile(library, state); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if got := appliedID(t, library); got != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("applied = %q, want the operator's profile untouched", got)
	}
}

func metaText(t *testing.T, library string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(library, "_meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
