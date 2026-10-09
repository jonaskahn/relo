package service_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	approuting "github.com/jonaskahn/relo/internal/application/routing"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestAddAccountRefreshesOneProviderAtATime covers the roster rebuild behind
// account writes: two writes in a row never race two rebuilds at one roster,
// and the write that landed mid-run still gets its rebuild afterwards.
func TestAddAccountRefreshesOneProviderAtATime(t *testing.T) {
	harness := newHarness(t)
	started := make(chan string, 4)
	release := make(chan struct{})
	var running, runs atomic.Int32
	var overlapped atomic.Bool
	service := appaccount.New(appaccount.Options{
		Entries:   sqlite.NewCredentialStore(harness.db),
		Secrets:   testkit.SecretStore(harness.secrets),
		Pools:     harness.pools,
		Catalog:   harness.catalog,
		Providers: &platform.AccountEdges{Catalog: harness.service},
		Refresh: func(providerID string) {
			runs.Add(1)
			if running.Add(1) > 1 {
				overlapped.Store(true)
			}
			started <- providerID
			<-release
			running.Add(-1)
		},
	})

	ctx := context.Background()
	if _, err := service.AddAccount(ctx, appaccount.NewAccount{
		ProviderID: "openai", Label: "one", SecretValue: "sk-first-secret-value",
	}); err != nil {
		t.Fatalf("AddAccount(one) error = %v", err)
	}
	if provider := <-started; provider != "openai" {
		t.Fatalf("refreshed %q, want the provider the account belongs to", provider)
	}

	// The second write lands while the first rebuild is still working.
	if _, err := service.AddAccount(ctx, appaccount.NewAccount{
		ProviderID: "openai", Label: "two", SecretValue: "sk-second-secret-value",
	}); err != nil {
		t.Fatalf("AddAccount(two) error = %v", err)
	}
	if got := runs.Load(); got != 1 {
		t.Fatalf("refresh runs = %d, want one rebuild at a time", got)
	}

	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for runs.Load() != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := runs.Load(); got != 2 {
		t.Fatalf("refresh runs = %d, want the queued rebuild behind the first", got)
	}
	if overlapped.Load() {
		t.Fatal("two roster rebuilds ran at the same provider at once")
	}
	if entries := harness.pools.GetPool("openai").Entries(); len(entries) != 2 {
		t.Fatalf("pool holds %d entries, want both accounts", len(entries))
	}
}

func TestAddAccountStoresCredentialAndSecret(t *testing.T) {
	harness := newHarness(t)
	entry := harness.addAccount("openai", "work", "sk-test-secret-value")

	if entry.ProviderID != "openai" || entry.Label != "work" || entry.Status != account.StatusActive {
		t.Fatalf("entry = %+v, want an active openai credential", entry)
	}
	if len(entry.ID) != 32 {
		t.Fatalf("entry id = %q, want a generated identifier", entry.ID)
	}
	if entry.SecretMask != "sk-t...ue" {
		t.Fatalf("SecretMask = %q, want the masked key", entry.SecretMask)
	}
	value, found := harness.storedSecret("apikey/openai/" + entry.ID)
	if !found || value != "sk-test-secret-value" {
		t.Fatalf("stored secret = %q, %v, want the key under the credential reference", value, found)
	}
	row, found := harness.credentialRow(entry.ID)
	if !found {
		t.Fatalf("credential %s is not in the table", entry.ID)
	}
	if row.SecretRef != "apikey/openai/"+entry.ID || row.Kind != "api_key" {
		t.Fatalf("row = %+v, want the credential the service described", row)
	}
	if entries := harness.pools.GetPool("openai").Entries(); len(entries) != 1 {
		t.Fatalf("pool holds %d entries, want 1", len(entries))
	}

	t.Run("a sign-in credential carries the token its flow produced", func(t *testing.T) {
		oauth, err := harness.accounts.AddAccount(context.Background(), appaccount.NewAccount{
			ProviderID: "claude", Kind: "oauth", SecretValue: `{"access_token":"sk-ant-oauth"}`,
		})
		if err != nil {
			t.Fatalf("AddAccount(oauth) error = %v", err)
		}
		if oauth.Kind != "oauth" || oauth.Label != "default" {
			t.Fatalf("oauth entry = %+v", oauth)
		}
	})
}

func TestAddAccountRejections(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()

	t.Run("unknown provider", func(t *testing.T) {
		_, err := harness.accounts.AddAccount(ctx, appaccount.NewAccount{ProviderID: "nothing", SecretValue: "key"})
		if !errors.Is(err, appaccount.ErrUnknownProvider) {
			t.Fatalf("AddAccount() error = %v, want ErrUnknownProvider", err)
		}
		if !strings.Contains(err.Error(), "nothing") {
			t.Fatalf("AddAccount() error = %v, want it to name the provider", err)
		}
	})

	t.Run("empty api key", func(t *testing.T) {
		_, err := harness.accounts.AddAccount(ctx, appaccount.NewAccount{ProviderID: "openai"})
		if !errors.Is(err, appaccount.ErrEmptySecret) {
			t.Fatalf("AddAccount() error = %v, want ErrEmptySecret", err)
		}
		if len(harness.secrets) != 0 {
			t.Fatalf("a refused credential still wrote %d secrets", len(harness.secrets))
		}
	})

	t.Run("no secret store", func(t *testing.T) {
		_, err := harness.withoutSecrets().accounts.AddAccount(ctx, appaccount.NewAccount{ProviderID: "openai", SecretValue: "key"})
		if !errors.Is(err, appaccount.ErrNoSecretStore) {
			t.Fatalf("AddAccount() error = %v, want ErrNoSecretStore", err)
		}
	})
}

// TestAddAccountRefusesDuplicates covers the rule that one credential never
// enters the pool of a provider twice: the same key value, or the same
// signed-in account behind another set of tokens. An account that needs a
// new sign-in is the exception: the same identity replaces its token.
func TestAddAccountRefusesDuplicates(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	harness.seedProvider("groq", "Groq", string(catalog.AuthAPIKey))
	harness.seedProvider("bedrock", "Bedrock", string(catalog.AuthAWS))

	add := func(providerID, label, secret string) (appaccount.Account, error) {
		t.Helper()
		return harness.accounts.AddAccount(ctx, appaccount.NewAccount{
			ProviderID: providerID, Label: label, SecretValue: secret,
		})
	}

	// refuse asserts that one credential is refused as a duplicate of another
	// and that nothing of it was written.
	refuse := func(t *testing.T, providerID, secret, storedLabel string) {
		t.Helper()
		secrets := len(harness.secrets)
		entries := len(harness.pools.GetPool(providerID).Entries())
		_, err := add(providerID, "again", secret)
		if !errors.Is(err, appaccount.ErrDuplicateAccount) {
			t.Fatalf("AddAccount() error = %v, want ErrDuplicateAccount", err)
		}
		if !strings.Contains(err.Error(), providerID) || !strings.Contains(err.Error(), storedLabel) {
			t.Fatalf("AddAccount() error = %v, want it to name %s and %s", err, providerID, storedLabel)
		}
		if len(harness.secrets) != secrets {
			t.Fatalf("a refused credential wrote a secret: %d stored, want %d", len(harness.secrets), secrets)
		}
		if got := len(harness.pools.GetPool(providerID).Entries()); got != entries {
			t.Fatalf("a refused credential reached the pool: %d entries, want %d", got, entries)
		}
	}

	t.Run("the same key twice on one provider", func(t *testing.T) {
		harness.addAccount("openai", "work", "sk-repeated")
		refuse(t, "openai", "sk-repeated", "work")
		accounts, err := harness.accounts.Accounts(ctx, "openai")
		if err != nil {
			t.Fatalf("Accounts() error = %v", err)
		}
		if len(accounts) != 1 {
			t.Fatalf("Accounts(openai) = %d rows, want the one stored key", len(accounts))
		}
	})

	t.Run("the same key on another provider", func(t *testing.T) {
		if _, err := add("groq", "work", "sk-repeated"); err != nil {
			t.Fatalf("AddAccount() error = %v, want the same key on another provider stored", err)
		}
	})

	t.Run("another key on the same provider", func(t *testing.T) {
		if _, err := add("openai", "second", "sk-distinct"); err != nil {
			t.Fatalf("AddAccount() error = %v, want a distinct key stored", err)
		}
	})

	t.Run("the same account signed in twice", func(t *testing.T) {
		if _, err := add("claude", "work", `{"access_token":"a","account_id":"acct-1"}`); err != nil {
			t.Fatalf("AddAccount() error = %v", err)
		}
		refuse(t, "claude", `{"access_token":"b","account_id":"acct-1"}`, "work")
	})

	t.Run("a sign-in replaces the token of an account that needs one", func(t *testing.T) {
		stored, err := add("claude", "work", `{"access_token":"old","account_id":"acct-reauth"}`)
		if err != nil {
			t.Fatalf("AddAccount() error = %v", err)
		}
		if err := harness.pools.MarkNeedsReauth(ctx, "claude", stored.ID); err != nil {
			t.Fatalf("MarkNeedsReauth() error = %v", err)
		}
		secrets := len(harness.secrets)
		entries := len(harness.pools.GetPool("claude").Entries())
		again, err := add("claude", "renamed", `{"access_token":"new","account_id":"acct-reauth"}`)
		if err != nil {
			t.Fatalf("AddAccount() error = %v, want the existing account refreshed", err)
		}
		if again.ID != stored.ID || again.Status != account.StatusActive || again.Label != "work" {
			t.Fatalf("account = %+v, want %s still labeled work and active", again, stored.ID)
		}
		row, found := harness.credentialRow(stored.ID)
		if !found {
			t.Fatalf("credential %s is not in the table", stored.ID)
		}
		value, found := harness.storedSecret(row.SecretRef)
		if !found || value != `{"access_token":"new","account_id":"acct-reauth"}` {
			t.Fatalf("stored secret = %q, %v, want the new token on the existing reference", value, found)
		}
		if len(harness.secrets) != secrets {
			t.Fatalf("secrets = %d, want %d", len(harness.secrets), secrets)
		}
		if got := len(harness.pools.GetPool("claude").Entries()); got != entries {
			t.Fatalf("pool entries = %d, want %d", got, entries)
		}
		for _, entry := range harness.pools.GetPool("claude").Entries() {
			if entry.ID == stored.ID && entry.Status != account.StatusActive {
				t.Fatalf("pool status = %s, want active", entry.Status)
			}
		}
	})

	t.Run("a paused account is not replaced by another sign-in", func(t *testing.T) {
		stored, err := add("claude", "paused", `{"access_token":"p","account_id":"acct-paused"}`)
		if err != nil {
			t.Fatalf("AddAccount() error = %v", err)
		}
		if err := harness.accounts.PauseAccount(ctx, stored.ID); err != nil {
			t.Fatalf("PauseAccount() error = %v", err)
		}
		refuse(t, "claude", `{"access_token":"p2","account_id":"acct-paused"}`, "paused")
	})

	t.Run("the same address under another spelling", func(t *testing.T) {
		if _, err := add("claude", "mail", `{"access_token":"c","email":"User@Example.com"}`); err != nil {
			t.Fatalf("AddAccount() error = %v", err)
		}
		refuse(t, "claude", `{"access_token":"d","email":"user@example.com"}`, "mail")
	})

	t.Run("another account on the same provider", func(t *testing.T) {
		if _, err := add("claude", "other", `{"access_token":"e","account_id":"acct-2"}`); err != nil {
			t.Fatalf("AddAccount() error = %v, want another account stored", err)
		}
	})

	t.Run("a credential that carries no identity", func(t *testing.T) {
		if _, err := add("claude", "opaque", `{"access_token":"f"}`); err != nil {
			t.Fatalf("AddAccount() error = %v", err)
		}
		if _, err := add("claude", "opaque-again", `{"access_token":"g"}`); err != nil {
			t.Fatalf("AddAccount() error = %v, want an identity-less credential stored", err)
		}
	})

	t.Run("the same AWS keys written in another order", func(t *testing.T) {
		if _, err := add("bedrock", "aws", `{"access_key_id":"AKIA","secret_access_key":"shh"}`); err != nil {
			t.Fatalf("AddAccount() error = %v", err)
		}
		refuse(t, "bedrock", `{"secret_access_key":"shh","access_key_id":"AKIA"}`, "aws")
	})

	t.Run("a key whose stored secret is gone", func(t *testing.T) {
		entry := harness.addAccount("groq", "gone", "sk-gone")
		delete(harness.secrets, "apikey/groq/"+entry.ID)
		if _, err := add("groq", "restored", "sk-gone"); err != nil {
			t.Fatalf("AddAccount() error = %v, want a key whose secret is gone stored again", err)
		}
	})
}
func TestAccountsAndLookup(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	first := harness.addAccount("openai", "one", "sk-one-secret")
	harness.addAccount("claude", "two", "sk-two-secret")

	all, err := harness.accounts.Accounts(ctx, "")
	if err != nil {
		t.Fatalf("Accounts() error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("Accounts() returned %d rows, want 2", len(all))
	}
	openaiOnly, err := harness.accounts.Accounts(ctx, "openai")
	if err != nil {
		t.Fatalf("Accounts(openai) error = %v", err)
	}
	if len(openaiOnly) != 1 || openaiOnly[0].ID != first.ID {
		t.Fatalf("Accounts(openai) = %+v, want only the openai credential", openaiOnly)
	}

	t.Run("by id", func(t *testing.T) {
		entry, err := harness.accounts.Account(ctx, first.ID)
		if err != nil || entry.ID != first.ID {
			t.Fatalf("Account() = %+v, %v", entry, err)
		}
	})

	t.Run("by unique prefix", func(t *testing.T) {
		entry, err := harness.accounts.Account(ctx, first.ID[:8])
		if err != nil || entry.ID != first.ID {
			t.Fatalf("Account(prefix) = %+v, %v", entry, err)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		if _, err := harness.accounts.Account(ctx, "ffffffffffff"); !errors.Is(err, appaccount.ErrAccountNotFound) {
			t.Fatalf("Account() error = %v, want ErrAccountNotFound", err)
		}
	})

	t.Run("a short prefix is not a name", func(t *testing.T) {
		if _, err := harness.accounts.Account(ctx, first.ID[:4]); !errors.Is(err, appaccount.ErrAccountNotFound) {
			t.Fatalf("Account(short prefix) error = %v, want ErrAccountNotFound", err)
		}
	})
}

func TestRemoveAccount(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	entry := harness.addAccount("openai", "work", "sk-removable")

	if err := harness.accounts.RemoveAccount(ctx, entry.ID); err != nil {
		t.Fatalf("RemoveAccount() error = %v", err)
	}
	if _, found := harness.credentialRow(entry.ID); found {
		t.Fatal("the removed credential is still listed")
	}
	if _, found := harness.storedSecret("apikey/openai/" + entry.ID); found {
		t.Fatal("the removed credential's secret is still stored")
	}
	if entries := harness.pools.GetPool("openai").Entries(); len(entries) != 0 {
		t.Fatalf("pool holds %d entries after a removal, want 0", len(entries))
	}
	if _, err := harness.service.Provider(ctx, "openai"); !errors.Is(err, appcatalog.ErrProviderNotFound) {
		t.Fatalf("Provider() after final account removal = %v, want ErrProviderNotFound", err)
	}
	if err := harness.accounts.RemoveAccount(ctx, entry.ID); !errors.Is(err, appaccount.ErrAccountNotFound) {
		t.Fatalf("RemoveAccount() again error = %v, want ErrAccountNotFound", err)
	}
}

func TestRemovingOneOfSeveralAccountsKeepsProvider(t *testing.T) {
	h := newHarness(t)
	first := h.addAccount("openai", "first", "sk-first")
	second := h.addAccount("openai", "second", "sk-second")
	if err := h.accounts.RemoveAccount(context.Background(), first.ID); err != nil {
		t.Fatalf("RemoveAccount() error = %v", err)
	}
	if _, err := h.service.Provider(context.Background(), "openai"); err != nil {
		t.Fatalf("Provider() after nonfinal removal = %v", err)
	}
	if _, found := h.credentialRow(second.ID); !found {
		t.Fatal("the second account was removed")
	}
}

func TestFinalAccountRemovalRefusesReferencedProvider(t *testing.T) {
	h := newHarness(t)
	entry := h.addAccount("openai", "work", "sk-referenced")
	if err := h.routes.Save(context.Background(), "fast", approuting.Write{
		Label: "Fast", Strategy: "priority", Enabled: true, Listed: true,
		Members: []approuting.MemberWrite{{ProviderID: "openai", ModelID: "model-1", Weight: 1, Enabled: true}},
	}); err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}
	if err := h.accounts.RemoveAccount(context.Background(), entry.ID); !errors.Is(err, appcatalog.ErrCatalogConflict) {
		t.Fatalf("RemoveAccount() error = %v, want ErrCatalogConflict", err)
	}
	if _, found := h.credentialRow(entry.ID); !found {
		t.Fatal("a refused removal deleted the account")
	}
	if _, found := h.storedSecret("apikey/openai/" + entry.ID); !found {
		t.Fatal("a refused removal deleted the secret")
	}
	if _, err := h.service.Provider(context.Background(), "openai"); err != nil {
		t.Fatalf("a refused removal deleted the provider: %v", err)
	}
}

func TestKeylessProviderIsExplicitlyRemovable(t *testing.T) {
	h := newHarness(t)
	h.seedProvider("local", "Local", string(catalog.AuthNone))
	if err := h.service.DeleteProvider(context.Background(), "local"); err != nil {
		t.Fatalf("DeleteProvider(keyless) error = %v", err)
	}
	if _, err := h.service.Provider(context.Background(), "local"); !errors.Is(err, appcatalog.ErrProviderNotFound) {
		t.Fatalf("Provider(keyless) error = %v, want ErrProviderNotFound", err)
	}
}

func TestAccountAdditionAndFinalRemovalSerialize(t *testing.T) {
	h := newHarness(t)
	first := h.addAccount("openai", "first", "sk-first")
	var wg sync.WaitGroup
	wg.Add(2)
	var removeErr, addErr error
	go func() {
		defer wg.Done()
		removeErr = h.accounts.RemoveAccount(context.Background(), first.ID)
	}()
	go func() {
		defer wg.Done()
		_, addErr = h.accounts.AddAccount(context.Background(), appaccount.NewAccount{
			ProviderID: "openai", Label: "second", SecretValue: "sk-second",
		})
	}()
	wg.Wait()
	if removeErr != nil {
		t.Fatalf("RemoveAccount() error = %v", removeErr)
	}
	rows, err := h.accounts.Accounts(context.Background(), "openai")
	if err != nil {
		t.Fatalf("Accounts() error = %v", err)
	}
	_, providerErr := h.service.Provider(context.Background(), "openai")
	if addErr == nil {
		if providerErr != nil || len(rows) != 1 {
			t.Fatalf("successful add left provider error %v and %d accounts", providerErr, len(rows))
		}
	} else if !errors.Is(providerErr, appcatalog.ErrProviderNotFound) || len(rows) != 0 {
		t.Fatalf("failed add left provider error %v and %d accounts", providerErr, len(rows))
	}
}

func TestAccountStatusAndPriority(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	entry := harness.addAccount("openai", "work", "sk-status")

	if err := harness.accounts.PauseAccount(ctx, entry.ID); err != nil {
		t.Fatalf("PauseAccount() error = %v", err)
	}
	if row, _ := harness.credentialRow(entry.ID); row.Status != account.StatusPaused {
		t.Fatalf("status = %q, want paused", row.Status)
	}
	if err := harness.accounts.ResumeAccount(ctx, entry.ID); err != nil {
		t.Fatalf("ResumeAccount() error = %v", err)
	}
	if row, _ := harness.credentialRow(entry.ID); row.Status != account.StatusActive {
		t.Fatalf("status = %q, want active", row.Status)
	}
	if err := harness.accounts.SetAccountPriority(ctx, entry.ID, 7); err != nil {
		t.Fatalf("SetAccountPriority() error = %v", err)
	}
	if row, _ := harness.credentialRow(entry.ID); row.Priority != 7 {
		t.Fatalf("priority = %d, want 7", row.Priority)
	}
	if err := harness.accounts.RenameAccount(ctx, entry.ID, "  desk  "); err != nil {
		t.Fatalf("RenameAccount() error = %v", err)
	}
	if row, _ := harness.credentialRow(entry.ID); row.Label != "desk" {
		t.Fatalf("label = %q, want desk", row.Label)
	}
	if err := harness.accounts.RenameAccount(ctx, entry.ID, "   "); err != nil {
		t.Fatalf("RenameAccount() blank error = %v", err)
	}
	if row, _ := harness.credentialRow(entry.ID); row.Label != "default" {
		t.Fatalf("label = %q, want default", row.Label)
	}

	t.Run("out of range", func(t *testing.T) {
		for _, priority := range []int{-101, 101} {
			if err := harness.accounts.SetAccountPriority(ctx, entry.ID, priority); !errors.Is(err, appaccount.ErrPriorityRange) {
				t.Fatalf("SetAccountPriority(%d) error = %v, want ErrPriorityRange", priority, err)
			}
		}
	})

	t.Run("unknown credential", func(t *testing.T) {
		if err := harness.accounts.PauseAccount(ctx, "ffffffffffffffff"); !errors.Is(err, appaccount.ErrAccountNotFound) {
			t.Fatalf("PauseAccount() error = %v, want ErrAccountNotFound", err)
		}
	})
}

func TestAccountsWithoutAPool(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	detached := harness.withoutPool()

	entry, err := detached.accounts.AddAccount(ctx, appaccount.NewAccount{ProviderID: "openai", Label: "repo", SecretValue: "sk-repo"})
	if err != nil {
		t.Fatalf("AddAccount() without a pool error = %v", err)
	}
	if _, found := harness.credentialRow(entry.ID); !found {
		t.Fatal("the credential was not written through the repository")
	}
	if err := detached.accounts.PauseAccount(ctx, entry.ID); err != nil {
		t.Fatalf("PauseAccount() without a pool error = %v", err)
	}
	if err := detached.accounts.SetAccountPriority(ctx, entry.ID, 3); err != nil {
		t.Fatalf("SetAccountPriority() without a pool error = %v", err)
	}
	if err := detached.accounts.RenameAccount(ctx, entry.ID, "repo-2"); err != nil {
		t.Fatalf("RenameAccount() without a pool error = %v", err)
	}
	row, _ := harness.credentialRow(entry.ID)
	if row.Status != account.StatusPaused || row.Priority != 3 || row.Label != "repo-2" {
		t.Fatalf("row = %+v, want the paused rank-3 credential named repo-2", row)
	}
	if err := detached.accounts.RemoveAccount(ctx, entry.ID); err != nil {
		t.Fatalf("RemoveAccount() without a pool error = %v", err)
	}
	if _, found := harness.credentialRow(entry.ID); found {
		t.Fatal("the removed credential is still listed")
	}
}

func TestAccountsWithoutASecretStore(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	detached := harness.withoutSecrets()

	_, err := detached.accounts.AddAccount(ctx, appaccount.NewAccount{
		ProviderID: "claude", Kind: "oauth", SecretValue: `{"access_token":"sk-ant"}`,
	})
	if !errors.Is(err, appaccount.ErrNoSecretStore) {
		t.Fatalf("AddAccount() without a secret store error = %v, want ErrNoSecretStore", err)
	}
}

func TestMaskSecret(t *testing.T) {
	cases := []struct {
		value string
		want  string
	}{
		{"sk-abcdefghijkl", "sk-a...kl"},
		{"short", "*****"},
		{"", "-"},
		{"   ", "-"},
		{"{\"access_token\":\"secret\"}", "oauth"},
	}
	for _, testCase := range cases {
		if got := appaccount.MaskSecret(testCase.value); got != testCase.want {
			t.Fatalf("MaskSecret(%q) = %q, want %q", testCase.value, got, testCase.want)
		}
	}
	if got := appaccount.MaskSecret("sk-abcdefghijkl"); strings.Contains(got, "efghij") {
		t.Fatalf("MaskSecret() = %q, want the middle of the secret hidden", got)
	}
}

// TestCatalogWithoutACatalog covers a build with no catalog attached: a read
// reports the catalog is missing rather than panicking.
func TestCatalogWithoutACatalog(t *testing.T) {
	built := appcatalog.New(appcatalog.Options{})
	if _, err := built.Provider(context.Background(), "openai"); !errors.Is(err, catalog.ErrNoCatalog) {
		t.Fatalf("Provider() error = %v, want ErrNoCatalog", err)
	}
}
