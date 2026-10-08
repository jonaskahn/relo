package secret_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/jonaskahn/relo/internal/adapters/secrets"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestWipe(t *testing.T) {
	keyring.MockInit()

	t.Run("deletes every reference a keychain home stored", func(t *testing.T) {
		home := testkit.TempHome(t)
		store := secrets.NewKeychainStore()
		refs := []string{"apikey/openai/one", "oauth/anthropic/two"}
		for _, ref := range refs {
			if err := store.Set(ref, "secret"); err != nil {
				t.Fatalf("Set(%s) error = %v", ref, err)
			}
		}
		if err := secrets.Wipe(home, secrets.Options{Keychain: true}, refs); err != nil {
			t.Fatalf("Wipe() error = %v", err)
		}
		for _, ref := range refs {
			if _, err := store.Get(ref); !errors.Is(err, secrets.ErrNotFound) {
				t.Fatalf("Get(%s) after Wipe() error = %v, want %v", ref, err, secrets.ErrNotFound)
			}
		}
	})

	t.Run("a reference the keychain never held is not a failure", func(t *testing.T) {
		if err := secrets.Wipe(testkit.TempHome(t),
			secrets.Options{Keychain: true}, []string{"apikey/openai/never-stored"}); err != nil {
			t.Fatalf("Wipe() error = %v", err)
		}
	})

	t.Run("leaves the vault's own secrets to the home that holds them", func(t *testing.T) {
		home := testkit.TempHome(t)
		store := secrets.NewKeychainStore()
		if err := store.Set("apikey/openai/one", "secret"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		if err := secrets.Wipe(home, secrets.Options{}, []string{"apikey/openai/one"}); err != nil {
			t.Fatalf("Wipe() error = %v", err)
		}
		if _, err := store.Get("apikey/openai/one"); err != nil {
			t.Fatalf("the keychain was touched although this home uses the vault: %v", err)
		}
		if _, err := os.Stat(filepath.Join(home, "secrets.enc")); err == nil {
			t.Fatal("the encrypted vault was removed by a keychain wipe")
		}
	})

	t.Run("has nothing to delete without a reference", func(t *testing.T) {
		if err := secrets.Wipe(testkit.TempHome(t), secrets.Options{Keychain: true}, nil); err != nil {
			t.Fatalf("Wipe() error = %v", err)
		}
	})
}
