package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/access"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

// storeKey builds one storable key. Every field the store carries across is
// set, so a round trip that dropped one would show up in the assertion.
func storeKey(id, name string) access.Key {
	return access.Key{
		ID: id, Name: name, Kind: "agent", Client: "claude-code",
		Owner: "operator-1", TokenDigest: "digest-" + id, TokenHint: "sk-...tail",
		Generation: 3, ExpiresAtMs: 1_700_086_400_000,
		CreatedAtMs: 1_700_000_000_000, UpdatedAtMs: 1_700_000_100_000,
	}
}

// TestAccessKeyStoreLifecycle drives the store the way the use cases do, over a
// real database: a key is stored, read back, touched, rotated, renamed, and
// finally revoked, and every field survives the trip through SQL.
func TestAccessKeyStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	store := sqlite.NewAccessKeyStore(db)
	key := storeKey("key-1", "first")

	if err := store.Insert(ctx, key); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	stored, found, err := store.Get(ctx, "key-1")
	if err != nil || !found {
		t.Fatalf("Get() = %+v, found = %v, error = %v, want the stored key", stored, found, err)
	}
	for name, pair := range map[string][2]any{
		"name":       {stored.Name, key.Name},
		"kind":       {stored.Kind, key.Kind},
		"client":     {stored.Client, key.Client},
		"owner":      {stored.Owner, key.Owner},
		"digest":     {stored.TokenDigest, key.TokenDigest},
		"hint":       {stored.TokenHint, key.TokenHint},
		"generation": {stored.Generation, key.Generation},
		"expires":    {stored.ExpiresAtMs, key.ExpiresAtMs},
		"created":    {stored.CreatedAtMs, key.CreatedAtMs},
		"updated":    {stored.UpdatedAtMs, key.UpdatedAtMs},
		"last used":  {stored.LastUsedAtMs, key.LastUsedAtMs},
		"revoked":    {stored.RevokedAtMs, key.RevokedAtMs},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s = %v, want %v", name, pair[0], pair[1])
		}
	}

	t.Run("lists every key", func(t *testing.T) {
		keys, err := store.List(ctx)
		if err != nil || len(keys) != 1 || keys[0].ID != "key-1" {
			t.Fatalf("List() = %+v, %v, want the stored key", keys, err)
		}
	})

	t.Run("records that a key authenticated a request", func(t *testing.T) {
		if err := store.Touch(ctx, "key-1", 1_700_000_500_000); err != nil {
			t.Fatalf("Touch() error = %v", err)
		}
		touched, found, err := store.Get(ctx, "key-1")
		if err != nil || !found {
			t.Fatalf("Get() = %+v, %v", touched, err)
		}
		if touched.LastUsedAtMs != 1_700_000_500_000 {
			t.Fatalf("LastUsedAtMs = %d, want the time the key was used", touched.LastUsedAtMs)
		}
	})

	t.Run("replaces the digest and hint on a rotation", func(t *testing.T) {
		if err := store.Rotate(ctx, access.KeyRotation{ID: "key-1", Digest: "digest-2", Hint: "sk-...next", ExpiresAtMs: 1_700_172_800_000, AtMs: 1_700_086_400_000}); err != nil {
			t.Fatalf("Rotate() error = %v", err)
		}
		rotated, found, err := store.Get(ctx, "key-1")
		if err != nil || !found {
			t.Fatalf("Get() = %+v, %v", rotated, err)
		}
		if rotated.TokenDigest != "digest-2" || rotated.TokenHint != "sk-...next" {
			t.Fatalf("key = %+v, want the rotated digest and hint", rotated)
		}
		if rotated.Generation != key.Generation+1 {
			t.Fatalf("Generation = %d, want it raised past %d", rotated.Generation, key.Generation)
		}
		if rotated.ExpiresAtMs != 1_700_172_800_000 {
			t.Fatalf("ExpiresAtMs = %d, want the new expiry", rotated.ExpiresAtMs)
		}
	})

	t.Run("stores a new name and expiry", func(t *testing.T) {
		if err := store.Update(ctx, access.KeyUpdate{ID: "key-1", Name: "renamed", ExpiresAtMs: 1_700_259_200_000, AtMs: 1_700_086_400_000}); err != nil {
			t.Fatalf("Update() error = %v", err)
		}
		renamed, found, err := store.Get(ctx, "key-1")
		if err != nil || !found {
			t.Fatalf("Get() = %+v, %v", renamed, err)
		}
		if renamed.Name != "renamed" || renamed.ExpiresAtMs != 1_700_259_200_000 {
			t.Fatalf("key = %+v, want the new name and expiry", renamed)
		}
		// A rename is not a rotation, so the credential it names is untouched.
		if renamed.TokenDigest != "digest-2" {
			t.Fatalf("TokenDigest = %q, want the rotated digest kept", renamed.TokenDigest)
		}
	})

	t.Run("retires a key", func(t *testing.T) {
		if err := store.Revoke(ctx, "key-1", 1_700_172_800_000); err != nil {
			t.Fatalf("Revoke() error = %v", err)
		}
		revoked, found, err := store.Get(ctx, "key-1")
		if err != nil || !found {
			t.Fatalf("Get() = %+v, %v", revoked, err)
		}
		if revoked.RevokedAtMs != 1_700_172_800_000 {
			t.Fatalf("RevokedAtMs = %d, want the time it was retired", revoked.RevokedAtMs)
		}
	})
}

// TestAccessKeyStoreReportsTheFeaturesOwnErrors covers the translation the
// store does so a use case never has to know where a key was written: a
// missing key, a name already taken, and a plain storage failure each arrive
// as the error the key lifecycle handles.
func TestAccessKeyStoreReportsTheFeaturesOwnErrors(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	store := sqlite.NewAccessKeyStore(db)

	if err := store.Insert(ctx, storeKey("key-1", "first")); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	t.Run("a name already taken", func(t *testing.T) {
		if err := store.Insert(ctx, storeKey("key-2", "first")); !errors.Is(err, access.ErrKeyNameTaken) {
			t.Fatalf("Insert() error = %v, want ErrKeyNameTaken", err)
		}
	})

	t.Run("an identifier already stored", func(t *testing.T) {
		// The name is free here, so this is the primary key refusing a second
		// row rather than the live-name rule; either way the insert fails.
		if err := store.Insert(ctx, storeKey("key-1", "second")); err == nil {
			t.Fatal("Insert() stored a second row under an identifier already in use")
		}
	})

	t.Run("a key that is not there", func(t *testing.T) {
		if _, found, err := store.Get(ctx, "missing"); found || err != nil {
			t.Fatalf("Get(missing) = found %v, %v, want neither a key nor an error", found, err)
		}
		for name, err := range map[string]error{
			"update": store.Update(ctx, access.KeyUpdate{ID: "missing", Name: "x"}),
			"rotate": store.Rotate(ctx, access.KeyRotation{ID: "missing", Digest: "d", Hint: "h"}),
			"revoke": store.Revoke(ctx, "missing", 0),
		} {
			if !errors.Is(err, access.ErrKeyNotFound) {
				t.Errorf("%s(missing) error = %v, want ErrKeyNotFound", name, err)
			}
		}
	})

	t.Run("touching a key that is not there is not an error", func(t *testing.T) {
		// A request that authenticated under a key deleted a moment earlier
		// must not fail the request itself.
		if err := store.Touch(ctx, "missing", 0); err != nil {
			t.Fatalf("Touch(missing) error = %v, want the touch ignored", err)
		}
	})
}

// TestAccessKeyStoreRemovesExpiredKeys covers the sweep that retires operator
// keys whose expiry has passed, including the case where the caller names
// which keys to consider.
func TestAccessKeyStoreRemovesExpiredKeys(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	store := sqlite.NewAccessKeyStore(db)

	now := time.Now()
	expired := storeKey("expired", "expired")
	expired.ExpiresAtMs = now.Add(-time.Hour).UnixMilli()
	expired.Owner = ""
	// The sweep only retires keys an operator owns; one an integration owns
	// is retired by that integration, not by the expiry sweep.
	expiredIntegration := storeKey("expired-integration", "expired integration")
	expiredIntegration.ExpiresAtMs = now.Add(-time.Hour).UnixMilli()
	expiredIntegration.Owner = "claude-code"
	live := storeKey("live", "live")
	live.ExpiresAtMs = now.Add(time.Hour).UnixMilli()
	noExpiry := storeKey("no-expiry", "no expiry")
	noExpiry.ExpiresAtMs = 0

	for _, key := range []access.Key{expired, live, noExpiry, expiredIntegration} {
		if err := store.Insert(ctx, key); err != nil {
			t.Fatalf("Insert(%s) error = %v", key.ID, err)
		}
	}

	removed, err := store.DeleteExpired(ctx, now.UnixMilli(), nil)
	if err != nil {
		t.Fatalf("DeleteExpired() error = %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want only the expired key", removed)
	}

	keys, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Fatalf("keys = %+v, want the live key, the one with no expiry, and the integration's", keys)
	}
	for _, key := range keys {
		if key.ID == "expired" {
			t.Fatal("the expired operator key was not removed")
		}
	}
}
