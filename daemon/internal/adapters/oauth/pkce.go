// PKCE and state generation for authorization-code logins.
package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

const (
	pkceVerifierBytes = 96
	stateBytes        = 32
	uuidBytes         = 16
)

// GeneratePKCE returns an S256 verifier and the challenge derived from
// it. The verifier is 128 base64url characters, inside the 43 to 128
// range RFC 7636 allows.
func GeneratePKCE() (verifier, challenge string) {
	verifier = randomToken(pkceVerifierBytes)
	return verifier, ChallengeForVerifier(verifier)
}

// ChallengeForVerifier returns the S256 challenge of a verifier.
func ChallengeForVerifier(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

// NewState returns a fresh, unguessable authorization state.
func NewState() string {
	return randomToken(stateBytes)
}

func randomToken(size int) string {
	return base64.RawURLEncoding.EncodeToString(randomBytes(size))
}

func randomBytes(size int) []byte {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		panic("oauth: the system random source failed: " + err.Error())
	}
	return raw
}
