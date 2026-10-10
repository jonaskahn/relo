package antigravity

import (
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		want     string
	}{
		{"a plain field", "version: 2.9.1\npath: app.zip\n", "2.9.1"},
		{"a quoted field", "version: \"3.0.0\"\n", "3.0.0"},
		{"a field after others", "files:\n  - url: a.zip\nversion: 4.1.2\n", "4.1.2"},
		{"an indented field is not the top level one", "files:\n  version: 9.9.9\n", ""},
		{"a windows line ending is dropped", "version: 2.9.1\r\nX-Evil\n", "2.9.1"},
		{"header-shaped junk is refused", "version: 1.0\n", ""},
		{"no field", "path: app.zip\n", ""},
		{"empty", "", ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := ParseVersion(test.manifest); got != test.want {
				t.Fatalf("ParseVersion() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestUserAgentFollowsTheManifest covers the version the header reports: the
// fallback until a manifest is read, then whatever it named, and never an
// empty one.
func TestUserAgentFollowsTheManifest(t *testing.T) {
	t.Cleanup(func() { SetVersion(DefaultVersion) })
	SetVersion(DefaultVersion)
	want := "antigravity/hub/2.8.0 (aidev_client; os_type=darwin; arch=arm64; cl=963137146)"
	if got := UserAgent(); got != want {
		t.Fatalf("UserAgent() = %q, want %q", got, want)
	}
	SetVersion("3.1.4")
	if got := UserAgent(); !strings.Contains(got, "antigravity/hub/3.1.4 ") {
		t.Fatalf("UserAgent() = %q, want the manifest version", got)
	}
	SetVersion("")
	if got := UserAgent(); !strings.Contains(got, "antigravity/hub/3.1.4 ") {
		t.Fatalf("UserAgent() = %q, want an empty version ignored", got)
	}
}
