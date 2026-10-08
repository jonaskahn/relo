// Package antigravity carries the client fingerprints Cloud Code Assist
// expects.
//
// Sign-in, project discovery, the model listing, and the quota probe send
// the IDE fingerprint. A chat request sends the CLI fingerprint the current
// Antigravity client uses for generateContent. A routed call whose header
// does not match the client family the credential was minted for is rejected.
package antigravity

import (
	"fmt"
	"strings"
)

const (
	// IDEVersion is the Antigravity IDE language-server version the
	// fingerprint reports, matching the client the credential was minted
	// for. Bump it here when the client moves.
	IDEVersion = "2.5.5"

	cliVersion    = "1.2.13"
	cliChangeList = "989937024"
	cliOS         = "darwin"
	cliArch       = "arm64"
	cliAuth       = "consumer"
)

const (
	// The platform and client tokens mirror the fingerprint the real IDE
	// sends rather than this host's, because the backend compares the client
	// family the credential was minted for, not the machine Relo runs on.
	idePlatform = "windows/amd64"
	ideClient   = "aidev_client"
	ideAuth     = "oauth"
)

// CLIUserAgent returns the User-Agent a chat request sends:
//
//	antigravity/cli/1.2.13 (aidev_client; os_type=darwin; arch=arm64; cl=989937024; auth_method=consumer)
func CLIUserAgent() string {
	return fmt.Sprintf("antigravity/cli/%s (aidev_client; os_type=%s; arch=%s; cl=%s; auth_method=%s)",
		cliVersion, cliOS, cliArch, cliChangeList, cliAuth)
}

// UserAgent returns the User-Agent a sign-in, listing, or quota probe carries:
//
//	antigravity/ide/<version> (os_type=<os>; arch=<arch>; aidev_client; auth_method=oauth)
func UserAgent() string {
	osType, arch, _ := strings.Cut(idePlatform, "/")
	return fmt.Sprintf("antigravity/ide/%s (os_type=%s; arch=%s; %s; auth_method=%s)",
		IDEVersion, osType, arch, ideClient, ideAuth)
}
