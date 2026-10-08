// OS keychain secret store.
package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	keychainService = "relo"
	// probeRefPrefix keeps a probe item recognisable, and out of the way of
	// any real credential reference.
	probeRefPrefix = "__probe_"
	probeValue     = "probe"
)

// KeychainStore keeps secrets in the operating system keychain.
type KeychainStore struct{}

// NewKeychainStore returns a keychain-backed store, which needs no key
// material of its own.
func NewKeychainStore() SecretStore {
	return KeychainStore{}
}

// Set stores secret under ref.
func (KeychainStore) Set(ref string, secret string) error {
	if err := keyring.Set(keychainService, ref, secret); err != nil {
		return fmt.Errorf("store secret %s: %w", ref, err)
	}
	return nil
}

// Get resolves the secret stored under ref.
func (KeychainStore) Get(ref string) (string, error) {
	value, err := keyring.Get(keychainService, ref)
	if err != nil {
		return "", fmt.Errorf("read secret %s: %w", ref, translateKeychainError(err))
	}
	return value, nil
}

// Delete removes the secret stored under ref.
func (KeychainStore) Delete(ref string) error {
	if err := keyring.Delete(keychainService, ref); err != nil {
		return fmt.Errorf("delete secret %s: %w", ref, translateKeychainError(err))
	}
	return nil
}

// Mode reports the keychain backend.
func (KeychainStore) Mode() SecretMode {
	return ModeKeychain
}

func translateKeychainError(err error) error {
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func probeKeychain(logger *slog.Logger) bool {
	ref := probeRefPrefix + probeSuffix()
	if err := keyring.Set(keychainService, ref, probeValue); err != nil {
		logger.Debug("keychain unavailable", "error", err)
		return false
	}
	value, err := keyring.Get(keychainService, ref)
	_ = keyring.Delete(keychainService, ref)
	if err != nil || value != probeValue {
		logger.Debug("keychain probe failed", "error", err)
		return false
	}
	return true
}

func probeSuffix() string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw)
}
