// Removal takes a state directory off a machine: everything inside it goes,
// and so does what it left in the operating system keychain, which outlives
// the home.
package platform

import (
	"context"
	"fmt"
	"os"

	"github.com/jonaskahn/relo/internal/adapters/secrets"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/config"
)

// WipeState removes a state directory and the credentials the operating
// system keychain holds for it. The keychain outlives the home, so its
// references are read and deleted before the home goes.
func WipeState(home string) error {
	if err := wipeKeychain(home); err != nil {
		return err
	}
	return os.RemoveAll(home)
}

func wipeKeychain(home string) error {
	cfg, err := config.LoadOffline(home)
	if err != nil {
		return err
	}
	if !cfg.Secrets.Keychain {
		return nil
	}
	refs, err := StoredSecretRefs(home)
	if err != nil {
		return err
	}
	return secrets.Wipe(home, secrets.Options{Keychain: true, KeyFile: cfg.Secrets.KeyFile}, refs)
}

// StoredSecretRefs names every credential secret a state directory keeps in
// the keychain. A home with no database has stored none.
func StoredSecretRefs(home string) ([]string, error) {
	path := config.DatabasePath(home)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("inspect the state database of %s: %w", home, err)
	}
	db, err := sqlite.OpenReadOnly(path, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	return sqlite.NewCredentialRepo(db).SecretRefs(context.Background())
}
