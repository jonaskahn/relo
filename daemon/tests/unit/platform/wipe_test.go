package platform_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/tests/testkit"
)

// installedHome is a state directory with a configuration, a migrated
// database, and one credential reference, which is what a wipe has to cover.
func installedHome(t *testing.T) string {
	t.Helper()
	home := testkit.TempHome(t)
	if err := os.WriteFile(config.ConfigPath(home), []byte("[secrets]\nkeychain = false\n"), 0o600); err != nil {
		t.Fatalf("write the config: %v", err)
	}
	for _, name := range []string{"secret.key", "secrets.enc", "admin-token", "data-plane-token"} {
		if err := os.WriteFile(filepath.Join(home, name), []byte("content"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(home, "logs"), 0o700); err != nil {
		t.Fatalf("create the log directory: %v", err)
	}
	db, err := sqlite.OpenDB(config.DatabasePath(home), nil)
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	catalog := sqlite.NewCatalogRepo(db)
	if err := catalog.SaveProvider(context.Background(), sqlite.ProviderRow{
		ID: "openai", Origin: "custom", Label: "openai", Auth: "api_key",
		APIFormat: "openai-chat", KeyHeader: "bearer", ModelsFormat: "none",
		Headers: map[string]string{}, Variables: map[string]string{},
		Enabled: true, Rank: 100, PoolStrategy: sqlite.StrategyLeastLoaded,
	}); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	if err := sqlite.NewCredentialStore(db).Insert(context.Background(), account.PoolEntry{
		ID: "one", ProviderID: "openai", Kind: "api_key",
		SecretRef: "apikey/openai/one", Status: account.StatusActive, Priority: 1,
	}); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return home
}

func TestWipeState(t *testing.T) {
	t.Run("removes the state directory with everything in it", func(t *testing.T) {
		home := installedHome(t)
		if err := platform.WipeState(home); err != nil {
			t.Fatalf("WipeState() error = %v", err)
		}
		if _, err := os.Stat(home); !os.IsNotExist(err) {
			t.Fatalf("the state directory survived the wipe: %v", err)
		}
	})

	t.Run("a home that was never started is not a failure", func(t *testing.T) {
		home := testkit.TempHome(t)
		if err := platform.WipeState(home); err != nil {
			t.Fatalf("WipeState() error = %v", err)
		}
		if _, err := os.Stat(home); !os.IsNotExist(err) {
			t.Fatalf("the state directory survived the wipe: %v", err)
		}
	})

	t.Run("a home that cannot be parsed keeps its files, so nothing is half wiped", func(t *testing.T) {
		home := testkit.TempHome(t)
		if err := os.WriteFile(config.ConfigPath(home), []byte("not = = toml"), 0o600); err != nil {
			t.Fatalf("write the broken config: %v", err)
		}
		if err := platform.WipeState(home); err == nil {
			t.Fatal("WipeState() error = nil, want the parse failure")
		}
		if _, err := os.Stat(home); err != nil {
			t.Fatalf("the state directory of an unparsable home was removed: %v", err)
		}
	})
}

func TestStoredSecretRefs(t *testing.T) {
	t.Run("names every credential the state directory stored", func(t *testing.T) {
		home := installedHome(t)
		refs, err := platform.StoredSecretRefs(home)
		if err != nil {
			t.Fatalf("StoredSecretRefs() error = %v", err)
		}
		if len(refs) != 1 || refs[0] != "apikey/openai/one" {
			t.Fatalf("StoredSecretRefs() = %v, want the stored reference", refs)
		}
	})

	t.Run("a home with no database has stored none", func(t *testing.T) {
		refs, err := platform.StoredSecretRefs(testkit.TempHome(t))
		if err != nil {
			t.Fatalf("StoredSecretRefs() error = %v", err)
		}
		if len(refs) != 0 {
			t.Fatalf("StoredSecretRefs() = %v, want none", refs)
		}
	})
}
