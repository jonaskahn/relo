// Package access owns the client keys that authenticate inbound inference
// requests: how one is minted, how its secret is digested, and which coding
// clients Relo can hand one to. Nothing here touches the database, so both
// the command line and the server build keys from the same place.
package access

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	// Prefix marks a Relo client key, so a leaked string is recognizable and
	// a scanner can find one.
	Prefix = "rlo_ak_"
	// IDBytes is the entropy behind a key identifier.
	IDBytes = 16
	// SecretBytes is the entropy behind a key secret.
	SecretBytes = 32
	// hintPrefix and hintSuffix are the readable ends a stored hint keeps, so
	// an operator can tell two keys apart without either being stored.
	hintPrefix = 4
	hintSuffix = 2
	hintGap    = "..."
	hintFloor  = hintPrefix + hintSuffix
	// separator splits the identifier from the secret. The identifier is hex,
	// so the first separator is always the one this package wrote.
	separator = "_"
	// LegacyTokenFile is the state-directory file older builds generated the
	// single data-plane bearer token into. Nothing reads it any more; it is
	// named here so a diagnostic can tell an operator what to retire.
	LegacyTokenFile = "data-plane-token"
)

// ErrMalformed reports a string that is not a Relo client key.
var ErrMalformed = errors.New("not a relo client key")

// Token is one minted client key: the value to hand the operator once, and
// the pieces a store keeps instead of it.
type Token struct {
	Raw    string
	ID     string
	Secret string
	Digest string
	Hint   string
}

// NewID returns a fresh key identifier.
func NewID() (string, error) {
	raw := make([]byte, IDBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate an access key id: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// Mint returns a fresh token for the given identifier.
func Mint(id string) (Token, error) {
	raw := make([]byte, SecretBytes)
	if _, err := rand.Read(raw); err != nil {
		return Token{}, fmt.Errorf("generate an access key secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	return Token{
		Raw: Prefix + id + separator + secret,
		ID:  id, Secret: secret, Digest: Digest(secret), Hint: Hint(secret),
	}, nil
}

// Parse splits a presented token into the identifier that names the stored
// key and the secret to verify against it. The identifier is not trusted as
// a credential on its own: it only says which digest to compare.
func Parse(raw string) (string, string, error) {
	rest, found := strings.CutPrefix(strings.TrimSpace(raw), Prefix)
	if !found {
		return "", "", ErrMalformed
	}
	id, secret, found := strings.Cut(rest, separator)
	if !found || secret == "" || len(id) != IDBytes*2 {
		return "", "", ErrMalformed
	}
	if _, err := hex.DecodeString(id); err != nil {
		return "", "", ErrMalformed
	}
	return id, secret, nil
}

// Digest renders the stored form of a secret. A digest is what makes the
// stored key useless to whoever reads the database.
func Digest(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// Hint renders the readable ends of a secret, which identify a key to a
// human without being one.
func Hint(secret string) string {
	if len(secret) <= hintFloor {
		return strings.Repeat("*", len(secret))
	}
	return secret[:hintPrefix] + hintGap + secret[len(secret)-hintSuffix:]
}
