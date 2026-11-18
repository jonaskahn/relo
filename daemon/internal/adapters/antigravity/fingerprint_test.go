package antigravity

import (
	"regexp"
	"testing"
)

// TestUserAgentNamesOneBuild pins the header to a single build: the endpoint
// reads the version and the changelist together, and a version advertised
// beside a changelist from another build is refused.
func TestUserAgentNamesOneBuild(t *testing.T) {
	want := "antigravity/cli/1.5.11 (aidev_client; os_type=darwin; arch=arm64; cl=1024248524; auth_method=consumer)"
	if got := UserAgent(); got != want {
		t.Fatalf("UserAgent() = %q, want %q", got, want)
	}
}

// TestUserAgentKeepsTheFingerprintShape covers the whole header rather than one
// field of it, because the endpoint reads it as one identity.
func TestUserAgentKeepsTheFingerprintShape(t *testing.T) {
	shape := regexp.MustCompile(`^antigravity/[a-z]+/\d+\.\d+\.\d+ \(aidev_client; os_type=[a-z]+; arch=[a-z0-9_]+; cl=\d+; auth_method=[a-z]+\)$`)
	if got := UserAgent(); !shape.MatchString(got) {
		t.Fatalf("UserAgent() = %q, want the Cloud Code Assist fingerprint shape", got)
	}
}
