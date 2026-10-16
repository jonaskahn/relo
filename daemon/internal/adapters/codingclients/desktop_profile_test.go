// The Claude Desktop gateway profile: the no-op write check.
package codingclients

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDesktopProfileUnchangedReportsWhatItCannotCompare covers the two reads
// that stop the check from answering: a profile the updater cannot open, and
// a profile map the encoder cannot render. Both have to report an error rather
// than claim the profile is unchanged, because the caller falls back to writing
// when the answer is not yes.
func TestDesktopProfileUnchangedReportsWhatItCannotCompare(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-profile.json")
	if unchanged, err := desktopProfileUnchanged(missing, map[string]any{}); err == nil || unchanged {
		t.Fatalf("desktopProfileUnchanged(missing) = %v, %v, want an error", unchanged, err)
	}

	path := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write the profile: %v", err)
	}
	// A channel has no JSON form, so the encoder refuses the whole map.
	unrenderable := map[string]any{"inferenceModels": make(chan int)}
	if unchanged, err := desktopProfileUnchanged(path, unrenderable); err == nil || unchanged {
		t.Fatalf("desktopProfileUnchanged(unrenderable) = %v, %v, want an error", unchanged, err)
	}
}

// TestDesktopProfileUnchangedRecognisesTheBytesItWouldWrite pins the answer the
// caller acts on: the profile on disk and the one the updater would write are
// the same bytes, including the trailing newline the writer appends.
func TestDesktopProfileUnchangedRecognisesTheBytesItWouldWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	profile := map[string]any{"modelDiscoveryEnabled": false, "inferenceModels": []any{}}
	written, err := writeDesktopProfile(path, profile)
	if err != nil {
		t.Fatalf("writeDesktopProfile() error = %v", err)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the profile: %v", err)
	}
	if string(onDisk) != string(written) {
		t.Fatalf("the file = %s, want the bytes the writer returned %s", onDisk, written)
	}

	unchanged, err := desktopProfileUnchanged(path, profile)
	if err != nil || !unchanged {
		t.Fatalf("desktopProfileUnchanged() = %v, %v, want the file recognised as its own", unchanged, err)
	}
	changed := map[string]any{"modelDiscoveryEnabled": true, "inferenceModels": []any{}}
	if unchanged, err := desktopProfileUnchanged(path, changed); err != nil || unchanged {
		t.Fatalf("desktopProfileUnchanged(changed) = %v, %v, want a rewrite", unchanged, err)
	}
}
