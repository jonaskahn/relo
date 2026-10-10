// Reading the client version out of the auto-updater manifest.
package antigravity

import (
	"bufio"
	"regexp"
	"strings"
)

// ManifestURL is where the vendor's auto-updater publishes the current client
// build. Newer models are gated by client version, so a stale one is refused.
const ManifestURL = "https://antigravity-hub-auto-updater-974169037036.us-central1.run.app/manifest/latest-arm64-mac.yml"

var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// ParseVersion reads the top-level version field of the manifest. Only a plain
// dotted version is accepted: the value goes straight into a request header, so
// anything else is refused rather than forwarded. The manifest is read as
// lines instead of YAML because it is the one field Relo needs.
func ParseVersion(manifest string) string {
	scanner := bufio.NewScanner(strings.NewReader(manifest))
	for scanner.Scan() {
		value, found := strings.CutPrefix(scanner.Text(), "version:")
		if !found {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if versionPattern.MatchString(value) {
			return value
		}
	}
	return ""
}
