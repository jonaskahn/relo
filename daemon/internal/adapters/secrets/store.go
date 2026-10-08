// Package secret owns every stored credential: the OS keychain and the
// encrypted vault.
package secrets

import "errors"

// SecretMode names the backend that is currently storing secrets.
type SecretMode string

// Secret modes name where credentials live: the OS keychain, the encrypted
// fallback file, or nowhere yet.
const (
	ModeKeychain      SecretMode = "keychain"
	ModeEncryptedFile SecretMode = "encrypted_file"
	ModeNone          SecretMode = "none"
)

// SecretStore stores and resolves credential secrets by reference name.
type SecretStore interface {
	Set(ref string, secret string) error
	Get(ref string) (string, error)
	Delete(ref string) error
	Mode() SecretMode
}

// Secret errors name the refusals callers map to their own wording: a
// missing entry, a wrong master key, and an unreadable file.
var (
	ErrNotFound   = errors.New("secret not found")
	ErrInvalidKey = errors.New("master key must be exactly 32 bytes")
	ErrDecrypt    = errors.New("failed to decrypt secrets file")
	ErrKeyMissing = errors.New("the secret key file is missing; put the key that encrypted secrets.enc in secrets.key_file")
)
