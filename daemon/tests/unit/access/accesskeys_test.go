package access_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/access"
	appaccess "github.com/jonaskahn/relo/internal/application/access"
)

func TestAccessKeyLifecycle(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()

	t.Run("a key is issued with a hint and never a secret", func(t *testing.T) {
		issued, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "laptop-codex", Kind: appaccess.Agent, Client: "codex",
		})
		if err != nil {
			t.Fatalf("CreateAccessKey() error = %v", err)
		}
		if issued.Status != access.StatusActive || issued.Generation != 1 {
			t.Fatalf("issued = %+v, want an active first generation", issued)
		}
		if issued.ExpiresAtMs != 0 {
			t.Fatalf("expiry = %d, want never", issued.ExpiresAtMs)
		}
		if issued.Token == "" || !strings.HasPrefix(issued.Token, access.Prefix) {
			t.Fatalf("token = %q, want a relo client key", issued.Token)
		}
		if issued.Hint == issued.Token || strings.Contains(issued.Hint, issued.Token) {
			t.Fatalf("hint = %q, want a hint that is not the token", issued.Hint)
		}

		keys, err := harness.keys.AccessKeys(ctx)
		if err != nil {
			t.Fatalf("AccessKeys() error = %v", err)
		}
		if len(keys) != 1 {
			t.Fatalf("keys = %+v, want the issued key", keys)
		}
		if strings.Contains(keys[0].Hint, issued.Token) {
			t.Fatal("the listing carries the secret")
		}
	})

	t.Run("a key is found by id and by name", func(t *testing.T) {
		issued, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "by-name", Kind: appaccess.Shared,
		})
		if err != nil {
			t.Fatalf("CreateAccessKey() error = %v", err)
		}
		if key, err := harness.keys.AccessKey(ctx, issued.ID); err != nil || key.ID != issued.ID {
			t.Fatalf("AccessKey(id) = %+v, %v, want the key", key, err)
		}
		if key, err := harness.keys.AccessKey(ctx, "BY-NAME"); err != nil || key.ID != issued.ID {
			t.Fatalf("AccessKey(name) = %+v, %v, want the key", key, err)
		}
		if _, err := harness.keys.AccessKey(ctx, "nothing"); !errors.Is(err, appaccess.ErrNotFound) {
			t.Fatalf("AccessKey(unknown) error = %v, want a not-found failure", err)
		}
	})

	t.Run("the verification view carries the digest and no secret", func(t *testing.T) {
		issued, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "verifiable", Kind: appaccess.Agent, Client: access.CustomClient,
		})
		if err != nil {
			t.Fatalf("CreateAccessKey() error = %v", err)
		}
		id, secret, err := access.Parse(issued.Token)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		record, found, err := harness.keys.LookupAccessKey(ctx, id)
		if err != nil || !found {
			t.Fatalf("LookupAccessKey() = %+v, %v, %v", record, found, err)
		}
		if record.Digest != access.Digest(secret) {
			t.Fatal("the stored digest does not cover the secret")
		}
		if _, found, err := harness.keys.LookupAccessKey(ctx, "unknown-id"); err != nil || found {
			t.Fatalf("LookupAccessKey(unknown) found = %v, err = %v, want neither", found, err)
		}
	})

	t.Run("renaming a key and setting its expiry", func(t *testing.T) {
		issued, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "before", Kind: appaccess.Shared,
		})
		if err != nil {
			t.Fatalf("CreateAccessKey() error = %v", err)
		}
		expiry := harness.clock.Now().AddDate(0, 0, 30).UnixMilli()
		if _, err := harness.keys.UpdateAccessKey(ctx, issued.ID, appaccess.AccessKeyUpdate{
			Name: "after", ExpiresAtMs: expiry,
		}); err != nil {
			t.Fatalf("UpdateAccessKey() error = %v", err)
		}
		key, err := harness.keys.AccessKey(ctx, issued.ID)
		if err != nil {
			t.Fatalf("AccessKey() error = %v", err)
		}
		if key.Name != "after" || key.ExpiresAtMs != expiry || key.Status != access.StatusActive {
			t.Fatalf("key = %+v, want the new name and expiry", key)
		}
	})

	t.Run("expiring a key reports it as expired", func(t *testing.T) {
		issued, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "short", Kind: appaccess.Shared,
			ExpiresAtMs: harness.clock.Now().Add(time.Hour).UnixMilli(),
		})
		if err != nil {
			t.Fatalf("CreateAccessKey() error = %v", err)
		}
		harness.clock.Add(2 * time.Hour)
		key, err := harness.keys.AccessKey(ctx, issued.ID)
		if err != nil {
			t.Fatalf("AccessKey() error = %v", err)
		}
		if key.Status != access.StatusExpired {
			t.Fatalf("status = %q, want expired", key.Status)
		}
	})

	t.Run("rotating keeps the identity and retires the secret", func(t *testing.T) {
		issued, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "rotating", Kind: appaccess.Agent, Client: "cursor",
		})
		if err != nil {
			t.Fatalf("CreateAccessKey() error = %v", err)
		}
		rotated, err := harness.keys.RotateAccessKey(ctx, "rotating")
		if err != nil {
			t.Fatalf("RotateAccessKey() error = %v", err)
		}
		if rotated.ID != issued.ID || rotated.Name != issued.Name || rotated.Client != "cursor" {
			t.Fatalf("rotated = %+v, want the same key", rotated)
		}
		if rotated.Generation != 2 {
			t.Fatalf("generation = %d, want 2", rotated.Generation)
		}
		if rotated.Token == issued.Token {
			t.Fatal("rotation returned the same secret")
		}
		oldID, oldSecret, err := access.Parse(issued.Token)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		record, found, err := harness.keys.LookupAccessKey(ctx, oldID)
		if err != nil || !found {
			t.Fatalf("LookupAccessKey() = %v, %v", found, err)
		}
		if record.Digest == access.Digest(oldSecret) {
			t.Fatal("the retired secret still verifies")
		}
	})

	t.Run("revoking is permanent and recorded once", func(t *testing.T) {
		issued, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "doomed", Kind: appaccess.Shared,
		})
		if err != nil {
			t.Fatalf("CreateAccessKey() error = %v", err)
		}
		if err := harness.keys.RevokeAccessKey(ctx, issued.ID); err != nil {
			t.Fatalf("RevokeAccessKey() error = %v", err)
		}
		key, err := harness.keys.AccessKey(ctx, issued.ID)
		if err != nil {
			t.Fatalf("AccessKey() error = %v", err)
		}
		if key.Status != access.StatusRevoked {
			t.Fatalf("status = %q, want revoked", key.Status)
		}
		if err := harness.keys.RevokeAccessKey(ctx, issued.ID); !errors.Is(err, appaccess.ErrNotFound) {
			t.Fatalf("the second revoke error = %v, want a not-found failure", err)
		}
		if _, err := harness.keys.RotateAccessKey(ctx, issued.ID); !errors.Is(err, appaccess.ErrInvalid) {
			t.Fatalf("rotating a revoked key error = %v, want a refusal", err)
		}
		if _, err := harness.keys.UpdateAccessKey(ctx, issued.ID, appaccess.AccessKeyUpdate{Name: "later"}); !errors.Is(err, appaccess.ErrInvalid) {
			t.Fatalf("updating a revoked key error = %v, want a refusal", err)
		}
	})

	t.Run("marking a key used records the time", func(t *testing.T) {
		issued, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "used", Kind: appaccess.Shared,
		})
		if err != nil {
			t.Fatalf("CreateAccessKey() error = %v", err)
		}
		if err := harness.keys.MarkAccessKeyUsed(ctx, issued.ID); err != nil {
			t.Fatalf("MarkAccessKeyUsed() error = %v", err)
		}
		key, err := harness.keys.AccessKey(ctx, issued.ID)
		if err != nil {
			t.Fatalf("AccessKey() error = %v", err)
		}
		if key.LastUsedAtMs != harness.clock.Now().UnixMilli() {
			t.Fatalf("last used = %d, want %d", key.LastUsedAtMs, harness.clock.Now().UnixMilli())
		}
	})
}

func TestAccessKeyValidation(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		request appaccess.NewAccessKey
		want    error
	}{
		{"no name", appaccess.NewAccessKey{Kind: appaccess.Shared}, appaccess.ErrInvalid},
		{"a name of only spaces", appaccess.NewAccessKey{Name: "  ", Kind: appaccess.Shared}, appaccess.ErrInvalid},
		{"a name past the limit", appaccess.NewAccessKey{
			Name: strings.Repeat("n", 65), Kind: appaccess.Shared}, appaccess.ErrInvalid},
		{"an agent key with no client", appaccess.NewAccessKey{Name: "x", Kind: appaccess.Agent}, appaccess.ErrInvalid},
		{"an agent key with an unknown client", appaccess.NewAccessKey{
			Name: "x", Kind: appaccess.Agent, Client: "nope"}, appaccess.ErrInvalid},
		{"a shared key with a client", appaccess.NewAccessKey{
			Name: "x", Kind: appaccess.Shared, Client: "codex"}, appaccess.ErrInvalid},
		{"an unknown kind", appaccess.NewAccessKey{Name: "x", Kind: "robot"}, appaccess.ErrInvalid},
		{"an expiry in the past", appaccess.NewAccessKey{
			Name: "x", Kind: appaccess.Shared, ExpiresAtMs: 1}, appaccess.ErrInvalid},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := harness.keys.CreateAccessKey(ctx, testCase.request); !errors.Is(err, testCase.want) {
				t.Fatalf("CreateAccessKey() error = %v, want %v", err, testCase.want)
			}
		})
	}

	t.Run("a name an active key already holds is refused whatever its case", func(t *testing.T) {
		if _, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "taken", Kind: appaccess.Shared,
		}); err != nil {
			t.Fatalf("CreateAccessKey() error = %v", err)
		}
		if _, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "TAKEN", Kind: appaccess.Shared,
		}); !errors.Is(err, appaccess.ErrNameTaken) {
			t.Fatalf("CreateAccessKey() error = %v, want the name collision", err)
		}
	})

	t.Run("a retired key frees its name", func(t *testing.T) {
		issued, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "recycled", Kind: appaccess.Shared,
		})
		if err != nil {
			t.Fatalf("CreateAccessKey() error = %v", err)
		}
		if err := harness.keys.RevokeAccessKey(ctx, issued.ID); err != nil {
			t.Fatalf("RevokeAccessKey() error = %v", err)
		}
		if _, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "recycled", Kind: appaccess.Shared,
		}); err != nil {
			t.Fatalf("reusing a retired name error = %v", err)
		}
	})

	t.Run("an update keeps the name it was given and refuses a taken one", func(t *testing.T) {
		first, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "first", Kind: appaccess.Shared,
		})
		if err != nil {
			t.Fatalf("CreateAccessKey() error = %v", err)
		}
		if _, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "second", Kind: appaccess.Shared,
		}); err != nil {
			t.Fatalf("CreateAccessKey() error = %v", err)
		}
		if _, err := harness.keys.UpdateAccessKey(ctx, first.ID, appaccess.AccessKeyUpdate{
			Name: "second",
		}); !errors.Is(err, appaccess.ErrNameTaken) {
			t.Fatalf("UpdateAccessKey() error = %v, want the name collision", err)
		}
		if _, err := harness.keys.UpdateAccessKey(ctx, first.ID, appaccess.AccessKeyUpdate{
			Name: "renamed", ExpiresAtMs: 1,
		}); !errors.Is(err, appaccess.ErrInvalid) {
			t.Fatalf("UpdateAccessKey() error = %v, want the expired write refused", err)
		}
	})
}

func TestDeleteExpiredKeys(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()

	t.Run("an expired operator key is removed and a revoked one stays", func(t *testing.T) {
		expired, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "lapsed", Kind: appaccess.Shared,
			ExpiresAtMs: harness.clock.Now().Add(time.Hour).UnixMilli(),
		})
		if err != nil {
			t.Fatalf("CreateAccessKey(expired) error = %v", err)
		}
		revoked, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "retired", Kind: appaccess.Shared,
			ExpiresAtMs: harness.clock.Now().Add(time.Hour).UnixMilli(),
		})
		if err != nil {
			t.Fatalf("CreateAccessKey(revoked) error = %v", err)
		}
		if err := harness.keys.RevokeAccessKey(ctx, revoked.ID); err != nil {
			t.Fatalf("RevokeAccessKey() error = %v", err)
		}
		live, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "live", Kind: appaccess.Shared,
		})
		if err != nil {
			t.Fatalf("CreateAccessKey(live) error = %v", err)
		}
		owned, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "owned-exp", Kind: appaccess.Agent, Client: "codex", Owner: "codex",
			ExpiresAtMs: harness.clock.Now().Add(time.Hour).UnixMilli(),
		})
		if err != nil {
			t.Fatalf("CreateAccessKey(owned) error = %v", err)
		}
		harness.clock.Add(2 * time.Hour)

		deleted, err := harness.keys.DeleteExpiredKeys(ctx, nil)
		if err != nil {
			t.Fatalf("DeleteExpiredKeys() error = %v", err)
		}
		if len(deleted) != 1 || deleted[0] != expired.ID {
			t.Fatalf("deleted = %v, want the expired operator key", deleted)
		}
		if _, err := harness.keys.AccessKey(ctx, expired.ID); !errors.Is(err, appaccess.ErrNotFound) {
			t.Fatalf("AccessKey(expired) error = %v, want a not-found failure", err)
		}
		if key, err := harness.keys.AccessKey(ctx, revoked.ID); err != nil || key.Status != access.StatusRevoked {
			t.Fatalf("revoked key = %+v, %v, want it kept", key, err)
		}
		if key, err := harness.keys.AccessKey(ctx, live.ID); err != nil || key.Status != access.StatusActive {
			t.Fatalf("live key = %+v, %v, want it kept", key, err)
		}
		if key, err := harness.keys.AccessKey(ctx, owned.ID); err != nil || key.Status != access.StatusExpired {
			t.Fatalf("owned key = %+v, %v, want the expired integration key kept", key, err)
		}
	})

	t.Run("a named list refuses a revoked or live key", func(t *testing.T) {
		expired, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "named-lapsed", Kind: appaccess.Shared,
			ExpiresAtMs: harness.clock.Now().Add(time.Hour).UnixMilli(),
		})
		if err != nil {
			t.Fatalf("CreateAccessKey(expired) error = %v", err)
		}
		revoked, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "named-retired", Kind: appaccess.Shared,
		})
		if err != nil {
			t.Fatalf("CreateAccessKey(revoked) error = %v", err)
		}
		if err := harness.keys.RevokeAccessKey(ctx, revoked.ID); err != nil {
			t.Fatalf("RevokeAccessKey() error = %v", err)
		}
		live, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "named-live", Kind: appaccess.Shared,
		})
		if err != nil {
			t.Fatalf("CreateAccessKey(live) error = %v", err)
		}
		owned, err := harness.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
			Name: "named-owned", Kind: appaccess.Agent, Client: "codex", Owner: "codex",
			ExpiresAtMs: harness.clock.Now().Add(time.Hour).UnixMilli(),
		})
		if err != nil {
			t.Fatalf("CreateAccessKey(owned) error = %v", err)
		}
		harness.clock.Add(2 * time.Hour)

		if _, err := harness.keys.DeleteExpiredKeys(ctx, []string{revoked.ID}); !errors.Is(err, appaccess.ErrInvalid) {
			t.Fatalf("DeleteExpiredKeys(revoked) error = %v, want a refusal", err)
		}
		if _, err := harness.keys.DeleteExpiredKeys(ctx, []string{live.ID}); !errors.Is(err, appaccess.ErrInvalid) {
			t.Fatalf("DeleteExpiredKeys(live) error = %v, want a refusal", err)
		}
		if _, err := harness.keys.DeleteExpiredKeys(ctx, []string{owned.ID}); !errors.Is(err, appaccess.ErrOwned) {
			t.Fatalf("DeleteExpiredKeys(owned) error = %v, want an owned-key refusal", err)
		}
		deleted, err := harness.keys.DeleteExpiredKeys(ctx, []string{expired.ID})
		if err != nil {
			t.Fatalf("DeleteExpiredKeys(expired) error = %v", err)
		}
		if len(deleted) != 1 || deleted[0] != expired.ID {
			t.Fatalf("deleted = %v, want the named expired key", deleted)
		}
	})
}
