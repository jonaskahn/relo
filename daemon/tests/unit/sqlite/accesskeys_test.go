package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jonaskahn/relo/internal/access"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

// accessKeyRow builds one storable key, which the repo tests vary field by
// field.
func accessKeyRow(id string) sqlite.AccessKeyRow {
	return sqlite.AccessKeyRow{
		ID: id, Name: id, Kind: "agent", Client: "codex",
		TokenDigest: "digest-" + id, TokenHint: "abcd...yz", Generation: 1,
		CreatedAtMs: 1_700_000_000_000, UpdatedAtMs: 1_700_000_000_000,
	}
}

func TestAccessKeyRepo(t *testing.T) {
	t.Run("list is empty on a fresh database", func(t *testing.T) {
		rows, err := sqlite.NewAccessKeyRepo(testkit.OpenTestDB(t)).List(context.Background())
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("List() returned %d rows, want 0", len(rows))
		}
	})

	t.Run("insert then get round-trips every field", func(t *testing.T) {
		repo := sqlite.NewAccessKeyRepo(testkit.OpenTestDB(t))
		ctx := context.Background()
		row := accessKeyRow("key-1")
		row.ExpiresAtMs = 1_800_000_000_000
		if err := repo.Insert(ctx, row); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		stored, found, err := repo.Get(ctx, row.ID)
		if err != nil || !found {
			t.Fatalf("Get() = %+v, %v, %v", stored, found, err)
		}
		if stored != row {
			t.Fatalf("Get() = %+v, want %+v", stored, row)
		}
		// A key that was just issued has never authenticated a request, so the
		// use timestamp is written by a touch rather than by the insert.
		if stored.LastUsedAtMs != 0 {
			t.Fatalf("last used = %d, want an unused key", stored.LastUsedAtMs)
		}
		if _, found, err := repo.Get(ctx, "absent"); err != nil || found {
			t.Fatalf("Get(absent) found = %v, err = %v, want neither", found, err)
		}
	})

	t.Run("an unset option reads back as zero rather than null", func(t *testing.T) {
		repo := sqlite.NewAccessKeyRepo(testkit.OpenTestDB(t))
		ctx := context.Background()
		if err := repo.Insert(ctx, accessKeyRow("key-plain")); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		stored, _, err := repo.Get(ctx, "key-plain")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if stored.ExpiresAtMs != 0 || stored.RevokedAtMs != 0 || stored.LastUsedAtMs != 0 {
			t.Fatalf("row = %+v, want the unset options as zero", stored)
		}
	})

	t.Run("the listing is newest first", func(t *testing.T) {
		repo := sqlite.NewAccessKeyRepo(testkit.OpenTestDB(t))
		ctx := context.Background()
		older := accessKeyRow("older")
		older.CreatedAtMs = 1_000
		newer := accessKeyRow("newer")
		newer.CreatedAtMs = 2_000
		for _, row := range []sqlite.AccessKeyRow{older, newer} {
			if err := repo.Insert(ctx, row); err != nil {
				t.Fatalf("Insert() error = %v", err)
			}
		}
		rows, err := repo.List(ctx)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(rows) != 2 || rows[0].ID != "newer" {
			t.Fatalf("List() = %+v, want the newest first", rows)
		}
	})

	t.Run("a live name is unique whatever its case", func(t *testing.T) {
		repo := sqlite.NewAccessKeyRepo(testkit.OpenTestDB(t))
		ctx := context.Background()
		if err := repo.Insert(ctx, accessKeyRow("first")); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		clash := accessKeyRow("second")
		clash.Name = "FIRST"
		if err := repo.Insert(ctx, clash); !errors.Is(err, sqlite.ErrAccessKeyNameTaken) {
			t.Fatalf("Insert() error = %v, want the name collision", err)
		}
	})

	t.Run("a secret digest is unique", func(t *testing.T) {
		repo := sqlite.NewAccessKeyRepo(testkit.OpenTestDB(t))
		ctx := context.Background()
		if err := repo.Insert(ctx, accessKeyRow("first")); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		clash := accessKeyRow("second")
		clash.TokenDigest = "digest-first"
		if err := repo.Insert(ctx, clash); err == nil {
			t.Fatal("Insert() error = nil, want the digest collision refused")
		}
	})

	t.Run("update renames a live key and refuses a revoked one", func(t *testing.T) {
		repo := sqlite.NewAccessKeyRepo(testkit.OpenTestDB(t))
		ctx := context.Background()
		if err := repo.Insert(ctx, accessKeyRow("live")); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		if err := repo.Update(ctx, access.KeyUpdate{ID: "live", Name: "renamed", ExpiresAtMs: 1_800_000_000_000, AtMs: 1_700_000_100_000}); err != nil {
			t.Fatalf("Update() error = %v", err)
		}
		stored, _, err := repo.Get(ctx, "live")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if stored.Name != "renamed" || stored.ExpiresAtMs != 1_800_000_000_000 || stored.UpdatedAtMs != 1_700_000_100_000 {
			t.Fatalf("row = %+v, want the new name, expiry, and time", stored)
		}
		if err := repo.Update(ctx, access.KeyUpdate{ID: "absent", Name: "x", AtMs: 1}); !errors.Is(err, sqlite.ErrAccessKeyNotFound) {
			t.Fatalf("Update(absent) error = %v, want a not-found failure", err)
		}
	})

	t.Run("rotate bumps the generation and retires the digest", func(t *testing.T) {
		repo := sqlite.NewAccessKeyRepo(testkit.OpenTestDB(t))
		ctx := context.Background()
		if err := repo.Insert(ctx, accessKeyRow("rotating")); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		if err := repo.Rotate(ctx, access.KeyRotation{ID: "rotating", Digest: "new-digest", Hint: "wxyz...ab", AtMs: 1_700_000_200_000}); err != nil {
			t.Fatalf("Rotate() error = %v", err)
		}
		stored, _, err := repo.Get(ctx, "rotating")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if stored.TokenDigest != "new-digest" || stored.TokenHint != "wxyz...ab" || stored.Generation != 2 {
			t.Fatalf("row = %+v, want the rotated secret at generation 2", stored)
		}
		if stored.ExpiresAtMs != 0 {
			t.Fatalf("expiry = %d, want it cleared", stored.ExpiresAtMs)
		}
	})

	t.Run("revoke is recorded once and survives a rename", func(t *testing.T) {
		repo := sqlite.NewAccessKeyRepo(testkit.OpenTestDB(t))
		ctx := context.Background()
		if err := repo.Insert(ctx, accessKeyRow("retiring")); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		if err := repo.Revoke(ctx, "retiring", 1_700_000_300_000); err != nil {
			t.Fatalf("Revoke() error = %v", err)
		}
		stored, _, err := repo.Get(ctx, "retiring")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if stored.RevokedAtMs != 1_700_000_300_000 {
			t.Fatalf("revoked at = %d, want the timestamp", stored.RevokedAtMs)
		}
		if err := repo.Revoke(ctx, "retiring", 1_700_000_400_000); !errors.Is(err, sqlite.ErrAccessKeyNotFound) {
			t.Fatalf("the second Revoke() error = %v, want a not-found failure", err)
		}
		if err := repo.Update(ctx, access.KeyUpdate{ID: "retiring", Name: "after", AtMs: 1}); !errors.Is(err, sqlite.ErrAccessKeyNotFound) {
			t.Fatalf("Update() on a revoked key error = %v, want a not-found failure", err)
		}
	})

	t.Run("revoking frees the name for a new key", func(t *testing.T) {
		repo := sqlite.NewAccessKeyRepo(testkit.OpenTestDB(t))
		ctx := context.Background()
		if err := repo.Insert(ctx, accessKeyRow("recycled")); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		if err := repo.Revoke(ctx, "recycled", 1); err != nil {
			t.Fatalf("Revoke() error = %v", err)
		}
		replacement := accessKeyRow("recycled")
		// A replacement is a different key that happens to reuse the name.
		replacement.ID = "recycled-again"
		replacement.TokenDigest = "digest-recycled-again"
		if err := repo.Insert(ctx, replacement); err != nil {
			t.Fatalf("reusing the name error = %v", err)
		}
	})

	t.Run("touch records a use", func(t *testing.T) {
		repo := sqlite.NewAccessKeyRepo(testkit.OpenTestDB(t))
		ctx := context.Background()
		if err := repo.Insert(ctx, accessKeyRow("used")); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		if err := repo.Touch(ctx, "used", 1_700_000_600_000); err != nil {
			t.Fatalf("Touch() error = %v", err)
		}
		stored, _, err := repo.Get(ctx, "used")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if stored.LastUsedAtMs != 1_700_000_600_000 {
			t.Fatalf("last used = %d, want the touched time", stored.LastUsedAtMs)
		}
	})

	t.Run("the schema refuses a kind it does not define", func(t *testing.T) {
		repo := sqlite.NewAccessKeyRepo(testkit.OpenTestDB(t))
		row := accessKeyRow("odd")
		row.Kind = "robot"
		if err := repo.Insert(context.Background(), row); err == nil {
			t.Fatal("Insert() error = nil, want the check constraint to refuse the kind")
		}
	})

	t.Run("delete expired removes only expired operator keys", func(t *testing.T) {
		repo := sqlite.NewAccessKeyRepo(testkit.OpenTestDB(t))
		ctx := context.Background()
		now := int64(1_700_000_000_000)
		expired := accessKeyRow("expired")
		expired.ExpiresAtMs = now - 1
		active := accessKeyRow("active")
		active.ExpiresAtMs = now + 1
		never := accessKeyRow("never")
		revoked := accessKeyRow("revoked")
		revoked.ExpiresAtMs = now - 1
		owned := accessKeyRow("owned")
		owned.ExpiresAtMs = now - 1
		owned.Owner = "codex"
		for _, row := range []sqlite.AccessKeyRow{expired, active, never, revoked, owned} {
			if err := repo.Insert(ctx, row); err != nil {
				t.Fatalf("Insert(%s) error = %v", row.ID, err)
			}
		}
		if err := repo.Revoke(ctx, "revoked", now); err != nil {
			t.Fatalf("Revoke() error = %v", err)
		}
		deleted, err := repo.DeleteExpired(ctx, now, nil)
		if err != nil {
			t.Fatalf("DeleteExpired() error = %v", err)
		}
		if deleted != 1 {
			t.Fatalf("DeleteExpired() = %d, want one expired operator key", deleted)
		}
		if _, found, err := repo.Get(ctx, "expired"); err != nil || found {
			t.Fatalf("Get(expired) found = %v, err = %v, want the row gone", found, err)
		}
		for _, id := range []string{"active", "never", "revoked", "owned"} {
			if _, found, err := repo.Get(ctx, id); err != nil || !found {
				t.Fatalf("Get(%s) found = %v, err = %v, want the row kept", id, found, err)
			}
		}
	})

	t.Run("delete expired of a named list removes that subset", func(t *testing.T) {
		repo := sqlite.NewAccessKeyRepo(testkit.OpenTestDB(t))
		ctx := context.Background()
		now := int64(1_700_000_000_000)
		first := accessKeyRow("first")
		first.ExpiresAtMs = now - 1
		second := accessKeyRow("second")
		second.ExpiresAtMs = now - 1
		for _, row := range []sqlite.AccessKeyRow{first, second} {
			if err := repo.Insert(ctx, row); err != nil {
				t.Fatalf("Insert(%s) error = %v", row.ID, err)
			}
		}
		deleted, err := repo.DeleteExpired(ctx, now, []string{"first"})
		if err != nil {
			t.Fatalf("DeleteExpired() error = %v", err)
		}
		if deleted != 1 {
			t.Fatalf("DeleteExpired() = %d, want the named key", deleted)
		}
		if _, found, err := repo.Get(ctx, "first"); err != nil || found {
			t.Fatalf("Get(first) found = %v, err = %v, want the row gone", found, err)
		}
		if _, found, err := repo.Get(ctx, "second"); err != nil || !found {
			t.Fatalf("Get(second) found = %v, err = %v, want the unnamed expired key kept", found, err)
		}
	})
}
