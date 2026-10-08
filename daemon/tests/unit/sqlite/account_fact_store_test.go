package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestAccountFactStore covers the store the account-context overrides go
// through: a window an operator set for one model of one account is read back
// for that account, listed across the whole connection, and removed with the
// account rather than left behind.
func TestAccountFactStore(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	store := sqlite.NewAccountFactStore(db)

	window := int64(400_000)
	facts := []account.ContextFact{
		{CredentialID: "cred-1", ProviderID: "openai", ModelID: "gpt-5", ContextWindow: &window, UpdatedAtMs: 1_700_000_000_000},
		{CredentialID: "cred-1", ProviderID: "openai", ModelID: "gpt-5-mini", ContextWindow: &window, UpdatedAtMs: 1_700_000_001_000},
		{CredentialID: "cred-2", ProviderID: "openai", ModelID: "gpt-5", ContextWindow: &window, UpdatedAtMs: 1_700_000_002_000},
		{CredentialID: "cred-3", ProviderID: "anthropic", ModelID: "sonnet", ContextWindow: &window, UpdatedAtMs: 1_700_000_003_000},
	}
	for _, fact := range facts {
		if err := store.Save(ctx, fact); err != nil {
			t.Fatalf("Save(%s/%s) error = %v", fact.CredentialID, fact.ModelID, err)
		}
	}

	t.Run("reads one account's overrides", func(t *testing.T) {
		stored, err := store.List(ctx, "cred-1")
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(stored) != 2 {
			t.Fatalf("List() = %+v, want both models of the account", stored)
		}
		byModel := map[string]account.ContextFact{}
		for _, fact := range stored {
			if fact.CredentialID != "cred-1" {
				t.Fatalf("fact = %+v, want only this account's", fact)
			}
			byModel[fact.ModelID] = fact
		}
		first := byModel["gpt-5"]
		if first.ContextWindow == nil || *first.ContextWindow != window {
			t.Fatalf("gpt-5 window = %v, want the override stored", first.ContextWindow)
		}
		if first.UpdatedAtMs != 1_700_000_000_000 {
			t.Fatalf("UpdatedAtMs = %d, want the time the override was written", first.UpdatedAtMs)
		}
	})

	t.Run("lists every account override of one connection", func(t *testing.T) {
		stored, err := store.ListProvider(ctx, "openai")
		if err != nil {
			t.Fatalf("ListProvider() error = %v", err)
		}
		if len(stored) != 3 {
			t.Fatalf("ListProvider() = %+v, want both accounts of the connection", stored)
		}
		// Listing by connection is how a shared model publishes the smallest
		// window any of its accounts asked for, so every account is named.
		credentials := map[string]bool{}
		for _, fact := range stored {
			credentials[fact.CredentialID] = true
		}
		if !credentials["cred-1"] || !credentials["cred-2"] {
			t.Fatalf("credentials = %v, want both accounts of the connection", credentials)
		}
		for _, fact := range stored {
			if fact.ProviderID != "openai" {
				t.Fatalf("fact = %+v, want only this connection's", fact)
			}
		}
	})

	t.Run("an account with no overrides lists nothing", func(t *testing.T) {
		stored, err := store.List(ctx, "cred-none")
		if err != nil || len(stored) != 0 {
			t.Fatalf("List(unknown) = %+v, %v, want nothing", stored, err)
		}
	})

	t.Run("replaces an override rather than adding a second", func(t *testing.T) {
		narrower := int64(128_000)
		if err := store.Save(ctx, account.ContextFact{
			CredentialID: "cred-1", ProviderID: "openai", ModelID: "gpt-5",
			ContextWindow: &narrower, UpdatedAtMs: 1_700_000_100_000,
		}); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		stored, err := store.List(ctx, "cred-1")
		if err != nil {
			t.Fatal(err)
		}
		for _, fact := range stored {
			if fact.ModelID != "gpt-5" {
				continue
			}
			if fact.ContextWindow == nil || *fact.ContextWindow != narrower {
				t.Fatalf("window = %v, want the replacement", fact.ContextWindow)
			}
			return
		}
		t.Fatal("the override was not stored")
	})

	t.Run("removes one account's overrides with it", func(t *testing.T) {
		if err := store.Delete(ctx, "cred-2"); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		stored, err := store.List(ctx, "cred-2")
		if err != nil || len(stored) != 0 {
			t.Fatalf("List() = %+v, %v, want nothing left for the account", stored, err)
		}
		// Another account's overrides are untouched, which is what keeps one
		// account's window from silently becoming another's.
		if kept, err := store.List(ctx, "cred-1"); err != nil || len(kept) != 2 {
			t.Fatalf("List(cred-1) = %+v, %v, want the other account untouched", kept, err)
		}
	})

	t.Run("clearing a window removes the override rather than storing nothing", func(t *testing.T) {
		// A nil window is not a statement of "no override"; it is the way the
		// account feature says the operator took the override back.
		if err := store.Save(ctx, account.ContextFact{
			CredentialID: "cred-1", ProviderID: "openai", ModelID: "gpt-5-mini",
			UpdatedAtMs: 1_700_000_200_000,
		}); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		stored, err := store.List(ctx, "cred-1")
		if err != nil {
			t.Fatal(err)
		}
		if len(stored) != 1 {
			t.Fatalf("List() = %+v, want the cleared override gone", stored)
		}
		if stored[0].ModelID != "gpt-5" {
			t.Fatalf("List() = %+v, want only the override that remains", stored)
		}
	})
}
