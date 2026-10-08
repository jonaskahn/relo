package secret_test

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/jonaskahn/relo/internal/adapters/secrets"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestEncryptedStoreContainerDamage(t *testing.T) {
	t.Run("a body that is not json is rejected", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "secrets.enc")
		key := randomKey(t)
		if err := os.WriteFile(path, seal(t, key, []byte("not json at all")), 0o600); err != nil {
			t.Fatalf("write a valid container with an invalid body: %v", err)
		}
		if _, err := encryptedStore(t, key, path).Get("ref"); !errors.Is(err, secrets.ErrDecrypt) {
			t.Fatalf("Get() error = %v, want %v", err, secrets.ErrDecrypt)
		}
	})

	t.Run("an unreadable path is reported", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "secrets.enc")
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatalf("create a directory in place of the secrets file: %v", err)
		}
		store := encryptedStore(t, randomKey(t), path)
		if _, err := store.Get("ref"); err == nil {
			t.Fatal("Get() error = nil, want a read failure")
		}
		if err := store.Set("ref", "value"); err == nil {
			t.Fatal("Set() error = nil, want a read failure")
		}
	})

	t.Run("a missing directory is reported", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "gone", "secrets.enc")
		if err := encryptedStore(t, randomKey(t), path).Set("ref", "value"); err == nil {
			t.Fatal("Set() error = nil, want a write failure")
		}
	})
}

func TestKeychainStoreFailures(t *testing.T) {
	keyring.MockInitWithError(errors.New("keychain is locked"))
	t.Cleanup(keyring.MockInit)

	store := secrets.NewKeychainStore()
	if err := store.Set("ref", "value"); err == nil {
		t.Fatal("Set() error = nil, want a keychain failure")
	}
	if _, err := store.Get("ref"); err == nil {
		t.Fatal("Get() error = nil, want a keychain failure")
	}
	if err := store.Delete("ref"); err == nil {
		t.Fatal("Delete() error = nil, want a keychain failure")
	}
}

// seal builds a valid secrets container around an arbitrary body, which is
// how a corrupted-but-decryptable file looks to the store.
func seal(t *testing.T, key, body []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("create GCM: %v", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("generate nonce: %v", err)
	}
	return gcm.Seal(append([]byte{0x01}, nonce...), nonce, body, nil)
}
