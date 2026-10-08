// Secret store detection: keychain or encrypted fallback.
package secrets

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
)

// Options names the secret backend one process should use.
type Options struct {
	// Keychain asks for the operating system keychain. It is opt-in, so a
	// daemon never prompts the keychain unless the operator asked for it.
	Keychain bool
	// KeyFile names the vault key. A relative name is resolved inside
	// RELO_HOME.
	KeyFile string
}

// Report is what a diagnostic knows about a state directory's secret store
// without creating or changing anything.
type Report struct {
	Mode SecretMode
	// KeyPending is true when the vault holds no secrets yet, so the key is
	// still to be written on the next start rather than missing.
	KeyPending bool
}

// Detect returns the secret backend this process should use. The OS keychain
// takes the secrets when the operator asked for it and the host has one;
// otherwise the encrypted vault does, with its key read or created.
func Detect(home string, opts Options, logger *slog.Logger) (SecretStore, error) {
	if opts.Keychain && probeKeychain(logger) {
		return NewKeychainStore(), nil
	}
	key, err := loadOrCreateKey(home, KeyPath(home, opts.KeyFile), logger)
	if err != nil {
		return nil, err
	}
	return NewEncryptedStore(key, SecretsPath(home))
}

// Inspect reports the secret backend a state directory would use, creating
// nothing. It is what a diagnostic asks.
func Inspect(home string, opts Options) (Report, error) {
	if opts.Keychain && probeKeychain(discardLogger()) {
		return Report{Mode: ModeKeychain}, nil
	}
	path := KeyPath(home, opts.KeyFile)
	_, err := readKeyFile(path)
	if err == nil {
		return Report{Mode: ModeEncryptedFile}, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return Report{}, err
	}
	if _, err := os.Stat(SecretsPath(home)); err == nil {
		return Report{}, fmt.Errorf("%s: %w", path, ErrKeyMissing)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Report{}, fmt.Errorf("inspect secrets file %s: %w", SecretsPath(home), err)
	}
	return Report{Mode: ModeEncryptedFile, KeyPending: true}, nil
}

// Wipe deletes the secrets a state directory stored in the operating system
// keychain, so removing Relo leaves no credential behind. It does nothing when
// this home keeps its secrets in the encrypted vault: they are in the home the
// caller deletes anyway, and the keychain holds nothing of theirs.
func Wipe(home string, opts Options, refs []string) error {
	if !opts.Keychain || len(refs) == 0 {
		return nil
	}
	store := NewKeychainStore()
	for _, ref := range refs {
		if err := store.Delete(ref); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
