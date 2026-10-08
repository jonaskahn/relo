package secret_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/jonaskahn/relo/internal/adapters/secrets"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestKeychainStore(t *testing.T) {
	keyring.MockInit()

	t.Run("keychain round-trip set get delete", func(t *testing.T) {
		store := secrets.NewKeychainStore()
		if err := store.Set("apikey/openai/one", "sk-secret"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		value, err := store.Get("apikey/openai/one")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if value != "sk-secret" {
			t.Fatalf("Get() = %q, want the stored secret", value)
		}
		if err := store.Delete("apikey/openai/one"); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		if _, err := store.Get("apikey/openai/one"); !errors.Is(err, secrets.ErrNotFound) {
			t.Fatalf("Get() after Delete() error = %v, want %v", err, secrets.ErrNotFound)
		}
		if err := store.Delete("apikey/openai/one"); !errors.Is(err, secrets.ErrNotFound) {
			t.Fatalf("Delete() of a missing secret error = %v, want %v", err, secrets.ErrNotFound)
		}
		if store.Mode() != secrets.ModeKeychain {
			t.Fatalf("Mode() = %q, want %q", store.Mode(), secrets.ModeKeychain)
		}
	})
}

func TestEncryptedStore(t *testing.T) {
	t.Run("encrypted file round-trip", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "secrets.enc")
		store := encryptedStore(t, randomKey(t), path)
		if err := store.Set("apikey/openai/one", "sk-secret"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		if err := store.Set("apikey/openai/two", "sk-other"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		value, err := store.Get("apikey/openai/one")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if value != "sk-secret" {
			t.Fatalf("Get() = %q, want the stored secret", value)
		}
		if store.Mode() != secrets.ModeEncryptedFile {
			t.Fatalf("Mode() = %q, want %q", store.Mode(), secrets.ModeEncryptedFile)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat secrets file: %v", err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("secrets file mode = %v, want 0600", info.Mode().Perm())
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read secrets file: %v", err)
		}
		if bytes.Contains(payload, []byte("sk-secret")) {
			t.Fatal("the secrets file holds plaintext")
		}
		if err := store.Delete("apikey/openai/two"); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		if _, err := store.Get("apikey/openai/two"); !errors.Is(err, secrets.ErrNotFound) {
			t.Fatalf("Get() after Delete() error = %v, want %v", err, secrets.ErrNotFound)
		}
		if err := store.Delete("apikey/openai/two"); !errors.Is(err, secrets.ErrNotFound) {
			t.Fatalf("Delete() of a missing secret error = %v, want %v", err, secrets.ErrNotFound)
		}
	})

	t.Run("encrypted file atomic write", func(t *testing.T) {
		home := testkit.TempHome(t)
		path := filepath.Join(home, "secrets.enc")
		store := encryptedStore(t, randomKey(t), path)
		if err := store.Set("ref", "first"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		if err := store.Set("ref", "second"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		entries, err := os.ReadDir(home)
		if err != nil {
			t.Fatalf("read home: %v", err)
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".tmp") {
				t.Fatalf("temporary file %s was left behind", entry.Name())
			}
		}
		value, err := store.Get("ref")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if value != "second" {
			t.Fatalf("Get() = %q, want the latest value", value)
		}
	})

	t.Run("encrypted file wrong key fails", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "secrets.enc")
		store := encryptedStore(t, randomKey(t), path)
		if err := store.Set("ref", "value"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		other := encryptedStore(t, randomKey(t), path)
		if _, err := other.Get("ref"); !errors.Is(err, secrets.ErrDecrypt) {
			t.Fatalf("Get() with the wrong key error = %v, want %v", err, secrets.ErrDecrypt)
		}
		if err := other.Delete("ref"); !errors.Is(err, secrets.ErrDecrypt) {
			t.Fatalf("Delete() with the wrong key error = %v, want %v", err, secrets.ErrDecrypt)
		}
	})

	t.Run("encrypted file invalid key length", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "secrets.enc")
		if _, err := secrets.NewEncryptedStore([]byte("too-short"), path); !errors.Is(err, secrets.ErrInvalidKey) {
			t.Fatalf("NewEncryptedStore() error = %v, want %v", err, secrets.ErrInvalidKey)
		}
	})

	t.Run("encrypted file rejects a tampered payload", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "secrets.enc")
		store := encryptedStore(t, randomKey(t), path)
		if err := store.Set("ref", "value"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		if err := os.WriteFile(path, []byte{0x02, 0x01, 0x02}, 0o600); err != nil {
			t.Fatalf("tamper with the secrets file: %v", err)
		}
		if _, err := store.Get("ref"); !errors.Is(err, secrets.ErrDecrypt) {
			t.Fatalf("Get() on a tampered file error = %v, want %v", err, secrets.ErrDecrypt)
		}
	})

	t.Run("encrypted file rejects a plain json payload", func(t *testing.T) {
		path := filepath.Join(testkit.TempHome(t), "secrets.enc")
		store := encryptedStore(t, randomKey(t), path)
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatalf("write a plain payload: %v", err)
		}
		if _, err := store.Get("ref"); !errors.Is(err, secrets.ErrDecrypt) {
			t.Fatalf("Get() error = %v, want %v", err, secrets.ErrDecrypt)
		}
	})
}

func TestDetect(t *testing.T) {
	t.Run("detect prefers keychain", func(t *testing.T) {
		logger, buffer := testkit.TestLogger(t)
		keyring.MockInit()
		store, err := secrets.Detect(testkit.TempHome(t), secrets.Options{Keychain: true}, logger)
		if err != nil {
			t.Fatalf("Detect() error = %v", err)
		}
		if store.Mode() != secrets.ModeKeychain {
			t.Fatalf("Mode() = %q, want %q", store.Mode(), secrets.ModeKeychain)
		}
		if strings.Contains(buffer.String(), "__probe") {
			t.Fatalf("the probe leaked into the log: %s", buffer.String())
		}
	})

	t.Run("detect falls back to the vault when the keychain is broken", func(t *testing.T) {
		keyring.MockInitWithError(errors.New("no keychain here"))
		logger, _ := testkit.TestLogger(t)
		store, err := secrets.Detect(testkit.TempHome(t), secrets.Options{Keychain: true}, logger)
		if err != nil {
			t.Fatalf("Detect() error = %v", err)
		}
		if store.Mode() != secrets.ModeEncryptedFile {
			t.Fatalf("Mode() = %q, want %q", store.Mode(), secrets.ModeEncryptedFile)
		}
	})

	t.Run("the keychain is skipped when it is disabled", func(t *testing.T) {
		keyring.MockInit()
		logger, _ := testkit.TestLogger(t)
		store, err := secrets.Detect(testkit.TempHome(t), secrets.Options{}, logger)
		if err != nil {
			t.Fatalf("Detect() error = %v", err)
		}
		if store.Mode() != secrets.ModeEncryptedFile {
			t.Fatalf("Mode() = %q, want the vault while the keychain is off", store.Mode())
		}
	})

	t.Run("the first detect writes the key", func(t *testing.T) {
		keyring.MockInitWithError(errors.New("no keychain here"))
		home := testkit.TempHome(t)
		logger, buffer := testkit.TestLogger(t)
		if _, err := secrets.Detect(home, secrets.Options{}, logger); err != nil {
			t.Fatalf("Detect() error = %v", err)
		}
		info, err := os.Stat(secrets.KeyPath(home, secrets.DefaultKeyFile))
		if err != nil {
			t.Fatalf("the vault key was not written: %v", err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("key file mode = %v, want 0600", info.Mode().Perm())
		}
		if !strings.Contains(buffer.String(), "created secret key") {
			t.Fatalf("log = %q, want the new key reported", buffer.String())
		}
	})

	t.Run("a second detect opens what the first stored", func(t *testing.T) {
		keyring.MockInitWithError(errors.New("no keychain here"))
		home := testkit.TempHome(t)
		first, err := secrets.Detect(home, secrets.Options{}, nil)
		if err != nil {
			t.Fatalf("Detect() error = %v", err)
		}
		if err := first.Set("apikey/openai/one", "sk-secret"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		second, err := secrets.Detect(home, secrets.Options{}, nil)
		if err != nil {
			t.Fatalf("Detect() error = %v", err)
		}
		value, err := second.Get("apikey/openai/one")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if value != "sk-secret" {
			t.Fatalf("Get() = %q, want the stored secret", value)
		}
	})

	t.Run("a relative key file resolves inside the state directory", func(t *testing.T) {
		keyring.MockInitWithError(errors.New("no keychain here"))
		home := testkit.TempHome(t)
		if _, err := secrets.Detect(home, secrets.Options{KeyFile: "vault.key"}, nil); err != nil {
			t.Fatalf("Detect() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(home, "vault.key")); err != nil {
			t.Fatalf("the configured key file was not used: %v", err)
		}
	})

	t.Run("an absolute key file is used as is", func(t *testing.T) {
		keyring.MockInitWithError(errors.New("no keychain here"))
		path := filepath.Join(testkit.TempHome(t), "vault.key")
		if _, err := secrets.Detect(testkit.TempHome(t), secrets.Options{KeyFile: path}, nil); err != nil {
			t.Fatalf("Detect() error = %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("the absolute key file was not used: %v", err)
		}
	})

	t.Run("a raw 32-byte key file is accepted", func(t *testing.T) {
		keyring.MockInitWithError(errors.New("no keychain here"))
		home := testkit.TempHome(t)
		key := randomKey(t)
		if err := os.WriteFile(secrets.KeyPath(home, secrets.DefaultKeyFile), key, 0o600); err != nil {
			t.Fatalf("write the raw key: %v", err)
		}
		store, err := secrets.Detect(home, secrets.Options{}, nil)
		if err != nil {
			t.Fatalf("Detect() error = %v", err)
		}
		if err := store.Set("ref", "value"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		direct, err := secrets.NewEncryptedStore(key, secrets.SecretsPath(home))
		if err != nil {
			t.Fatalf("NewEncryptedStore() error = %v", err)
		}
		value, err := direct.Get("ref")
		if err != nil || value != "value" {
			t.Fatalf("Get() = %q, %v, want the value the store wrote", value, err)
		}
	})

	t.Run("a key Relo cannot read is refused", func(t *testing.T) {
		keyring.MockInitWithError(errors.New("no keychain here"))
		home := testkit.TempHome(t)
		if err := os.WriteFile(secrets.KeyPath(home, secrets.DefaultKeyFile), []byte("not-a-key"), 0o600); err != nil {
			t.Fatalf("write a broken key: %v", err)
		}
		if _, err := secrets.Detect(home, secrets.Options{}, nil); !errors.Is(err, secrets.ErrInvalidKey) {
			t.Fatalf("Detect() error = %v, want %v", err, secrets.ErrInvalidKey)
		}
	})

	t.Run("a vault without its key is refused rather than rekeyed", func(t *testing.T) {
		keyring.MockInitWithError(errors.New("no keychain here"))
		home := testkit.TempHome(t)
		if err := os.WriteFile(secrets.SecretsPath(home), []byte("ciphertext"), 0o600); err != nil {
			t.Fatalf("write a vault: %v", err)
		}
		if _, err := secrets.Detect(home, secrets.Options{}, nil); !errors.Is(err, secrets.ErrKeyMissing) {
			t.Fatalf("Detect() error = %v, want %v", err, secrets.ErrKeyMissing)
		}
		if _, err := os.Stat(secrets.KeyPath(home, secrets.DefaultKeyFile)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat key = %v, want no fresh key written over a vault", err)
		}
	})
}

func TestInspect(t *testing.T) {
	t.Run("inspect never writes the key", func(t *testing.T) {
		keyring.MockInitWithError(errors.New("no keychain here"))
		home := testkit.TempHome(t)
		report, err := secrets.Inspect(home, secrets.Options{})
		if err != nil {
			t.Fatalf("Inspect() error = %v", err)
		}
		if report.Mode != secrets.ModeEncryptedFile || !report.KeyPending {
			t.Fatalf("report = %+v, want a vault whose key is still to be written", report)
		}
		if _, err := os.Stat(secrets.KeyPath(home, secrets.DefaultKeyFile)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat key = %v, want a diagnostic to write nothing", err)
		}
	})

	t.Run("inspect reports a key that is in place", func(t *testing.T) {
		keyring.MockInitWithError(errors.New("no keychain here"))
		home := testkit.TempHome(t)
		if _, err := secrets.Detect(home, secrets.Options{}, nil); err != nil {
			t.Fatalf("Detect() error = %v", err)
		}
		report, err := secrets.Inspect(home, secrets.Options{})
		if err != nil {
			t.Fatalf("Inspect() error = %v", err)
		}
		if report.Mode != secrets.ModeEncryptedFile || report.KeyPending {
			t.Fatalf("report = %+v, want the vault in place", report)
		}
	})

	t.Run("inspect reports a broken key", func(t *testing.T) {
		keyring.MockInitWithError(errors.New("no keychain here"))
		home := testkit.TempHome(t)
		if err := os.WriteFile(secrets.KeyPath(home, secrets.DefaultKeyFile), []byte("not-a-key"), 0o600); err != nil {
			t.Fatalf("write a broken key: %v", err)
		}
		if _, err := secrets.Inspect(home, secrets.Options{}); !errors.Is(err, secrets.ErrInvalidKey) {
			t.Fatalf("Inspect() error = %v, want %v", err, secrets.ErrInvalidKey)
		}
	})

	t.Run("inspect reports a vault without its key", func(t *testing.T) {
		keyring.MockInitWithError(errors.New("no keychain here"))
		home := testkit.TempHome(t)
		if err := os.WriteFile(secrets.SecretsPath(home), []byte("ciphertext"), 0o600); err != nil {
			t.Fatalf("write a vault: %v", err)
		}
		if _, err := secrets.Inspect(home, secrets.Options{}); !errors.Is(err, secrets.ErrKeyMissing) {
			t.Fatalf("Inspect() error = %v, want %v", err, secrets.ErrKeyMissing)
		}
	})

	t.Run("inspect reports the keychain", func(t *testing.T) {
		keyring.MockInit()
		report, err := secrets.Inspect(testkit.TempHome(t), secrets.Options{Keychain: true})
		if err != nil {
			t.Fatalf("Inspect() error = %v", err)
		}
		if report.Mode != secrets.ModeKeychain {
			t.Fatalf("Mode() = %q, want the keychain", report.Mode)
		}
	})
}

func TestSecretsNeverReachTheLog(t *testing.T) {
	logger, buffer := testkit.TestLogger(t)
	path := filepath.Join(testkit.TempHome(t), "secrets.enc")
	store := encryptedStore(t, randomKey(t), path)
	const marker = "sk-live-do-not-log-me"
	if err := store.Set("apikey/openai/one", marker); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if _, err := store.Get("apikey/openai/one"); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if _, err := secrets.Detect(filepath.Dir(path), secrets.Options{Keychain: true}, logger); err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if strings.Contains(buffer.String(), marker) {
		t.Fatalf("a secret reached the log: %s", buffer.String())
	}
}

func encryptedStore(t *testing.T, key []byte, path string) secrets.SecretStore {
	t.Helper()
	store, err := secrets.NewEncryptedStore(key, path)
	if err != nil {
		t.Fatalf("NewEncryptedStore() error = %v", err)
	}
	return store
}

func randomKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}
