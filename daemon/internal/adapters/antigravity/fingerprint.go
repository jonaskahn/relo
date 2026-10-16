// Package antigravity carries the client identity and wire rules Cloud Code
// Assist expects: the fingerprint every call sends, the SKU each logical model
// collapses onto, and how a refusal is read.
package antigravity

import "fmt"

// The fingerprint is one identity: the client family, the version, the
// changelist of the build that version names, and the platform tokens. The
// endpoint reads it as a whole, so a version taken on its own is not a newer
// client, it is a build that never shipped.
//
// The version is therefore pinned to the newest build Relo's request shape is
// known to work against, and not taken from the auto-updater manifest: that
// manifest publishes a version without the changelist belonging to it, and
// pairing the two was refused by every call that tried it. Raising either value
// needs the changelist of the new build and a request-shape check against it.
const (
	clientFamily  = "hub"
	clientVersion = "2.23.0"
	changeList    = "963137146"

	// The platform tokens mirror the build the manifest describes rather than
	// this host's, because the backend compares the client family a credential
	// was minted for, not the machine Relo runs on.
	clientOS     = "darwin"
	clientArch   = "arm64"
	clientDevice = "aidev_client"
)

// UserAgent returns the User-Agent every Cloud Code Assist call sends:
//
//	antigravity/hub/2.23.0 (aidev_client; os_type=darwin; arch=arm64; cl=963137146)
func UserAgent() string {
	return fmt.Sprintf("antigravity/%s/%s (%s; os_type=%s; arch=%s; cl=%s)",
		clientFamily, clientVersion, clientDevice, clientOS, clientArch, changeList)
}
