// Client keys: lifecycle states and status reads.
package access

import (
	"errors"
	"time"
)

var (
	// ErrKeyNotFound reports an identifier no live key carries.
	ErrKeyNotFound = errors.New("access key not found")
	// ErrKeyNameTaken reports a live key that already has a name.
	ErrKeyNameTaken = errors.New("an active access key already has that name")
)

// The statuses a stored key reports.
const (
	StatusActive  = "active"
	StatusExpired = "expired"
	StatusRevoked = "revoked"
)

// KeyUpdate renames a key and moves its expiry, stamping the write it
// came from. A zero expiry clears the key's deadline.
type KeyUpdate struct {
	ID          string
	Name        string
	ExpiresAtMs int64
	AtMs        int64
}

// KeyRotation replaces a key's secret digest and hint, keeping its expiry
// unless the rotation names a new one, and stamps the write it came from.
type KeyRotation struct {
	ID          string
	Digest      string
	Hint        string
	ExpiresAtMs int64
	AtMs        int64
}

// Key is one stored client key: the metadata every surface reports, and the
// digest a presented secret is checked against. The secret itself is never
// stored, so it is never part of this shape.
type Key struct {
	ID           string
	Name         string
	Kind         string
	Client       string
	Owner        string
	TokenDigest  string
	TokenHint    string
	Generation   int
	ExpiresAtMs  int64
	RevokedAtMs  int64
	CreatedAtMs  int64
	UpdatedAtMs  int64
	LastUsedAtMs int64
}

// StatusOf reports whether a key still authenticates a request.
func StatusOf(key Key, now time.Time) string {
	switch {
	case key.RevokedAtMs > 0:
		return StatusRevoked
	case key.ExpiresAtMs > 0 && !now.Before(time.UnixMilli(key.ExpiresAtMs)):
		return StatusExpired
	default:
		return StatusActive
	}
}
