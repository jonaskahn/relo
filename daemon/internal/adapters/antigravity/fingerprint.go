// Package antigravity carries the client identity and wire rules Cloud Code
// Assist expects: the fingerprint every call sends, the SKU each logical model
// collapses onto, and how a refusal is read.
package antigravity

import (
	"fmt"
	"sync/atomic"
)

const (
	// DefaultVersion is the client version reported until the manifest names
	// a newer one, so nothing breaks while Relo is offline.
	DefaultVersion = "2.8.0"

	// The platform tokens mirror the build the manifest describes rather than
	// this host's, because the backend compares the client family a credential
	// was minted for, not the machine Relo runs on.
	clientOS     = "darwin"
	clientArch   = "arm64"
	clientDevice = "aidev_client"
	changeList   = "963137146"

	// InterleavedThinkingBeta is the beta a Claude turn asks for by name. The
	// vendor gates interleaved thinking behind it, and serves the turn without
	// the reasoning the client paid for when it is missing.
	InterleavedThinkingBeta = "interleaved-thinking-2025-05-14"

	// BetaHeader is where that beta travels.
	BetaHeader = "anthropic-beta"
)

// version holds the client version the fingerprint reports. It is read on
// every request and written by a worker once a day, so it lives behind an
// atomic rather than a lock.
var version atomic.Pointer[string]

// SetVersion records the client version the manifest named. An empty one is
// ignored, so a manifest that could not be read leaves the last good version.
func SetVersion(value string) {
	if value == "" {
		return
	}
	version.Store(&value)
}

// UserAgent returns the User-Agent every Cloud Code Assist call sends:
//
//	antigravity/hub/2.8.0 (aidev_client; os_type=darwin; arch=arm64; cl=963137146)
func UserAgent() string {
	current := DefaultVersion
	if stored := version.Load(); stored != nil {
		current = *stored
	}
	return fmt.Sprintf("antigravity/hub/%s (%s; os_type=%s; arch=%s; cl=%s)",
		current, clientDevice, clientOS, clientArch, changeList)
}
