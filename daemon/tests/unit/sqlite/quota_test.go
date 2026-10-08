package storage_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestQuotaRepo(t *testing.T) {
	t.Run("a window keeps the length it was reported with", func(t *testing.T) {
		repo := sqlite.NewQuotaRepo(testkit.OpenTestDB(t))
		ctx := context.Background()
		observed := time.Now().Truncate(time.Second)
		if err := repo.UpsertSnapshots(ctx, []sqlite.QuotaRow{{
			CredentialID: "credential-1", Window: "5h", UsedPercent: 42,
			Seconds: 5 * 60 * 60, Source: "header", ObservedAt: observed,
		}}); err != nil {
			t.Fatalf("UpsertSnapshots() error = %v", err)
		}
		rows, err := repo.ListSnapshots(ctx, "credential-1")
		if err != nil {
			t.Fatalf("ListSnapshots() error = %v", err)
		}
		if len(rows) != 1 || rows[0].Seconds != 5*60*60 || rows[0].Source != "header" {
			t.Fatalf("rows = %+v, want the window length and source kept", rows)
		}
	})

	t.Run("money readings round-trip", func(t *testing.T) {
		repo := sqlite.NewQuotaRepo(testkit.OpenTestDB(t))
		ctx := context.Background()
		if err := repo.UpsertSnapshots(ctx, []sqlite.QuotaRow{{
			CredentialID: "credential-1", Window: "balance", Amount: new(42.5),
			Currency: "USD", Source: "probe", ObservedAt: time.Now(),
		}}); err != nil {
			t.Fatalf("UpsertSnapshots() error = %v", err)
		}
		rows, err := repo.ListSnapshots(ctx, "credential-1")
		if err != nil {
			t.Fatalf("ListSnapshots() error = %v", err)
		}
		if len(rows) != 1 || rows[0].Amount == nil || *rows[0].Amount != 42.5 || rows[0].Currency != "USD" {
			t.Fatalf("rows = %+v, want the amount and currency kept", rows)
		}
	})

	t.Run("snapshots round-trip and replace", func(t *testing.T) {
		repo := sqlite.NewQuotaRepo(testkit.OpenTestDB(t))
		ctx := context.Background()
		observed := time.Now().Truncate(time.Second)
		if err := repo.UpsertSnapshots(ctx, []sqlite.QuotaRow{
			{CredentialID: "credential-1", Window: "Gem", UsedPercent: 20, ResetAt: 100, Source: "probe", ObservedAt: observed},
			{CredentialID: "credential-1", Window: "Cla", UsedPercent: 75, Source: "probe", ObservedAt: observed},
		}); err != nil {
			t.Fatalf("UpsertSnapshots() error = %v", err)
		}
		if err := repo.UpsertSnapshots(ctx, []sqlite.QuotaRow{
			{CredentialID: "credential-1", Window: "Gem", UsedPercent: 55, Source: "probe", ObservedAt: observed.Add(time.Minute)},
		}); err != nil {
			t.Fatalf("UpsertSnapshots() error = %v", err)
		}
		rows, err := repo.ListSnapshots(ctx, "credential-1")
		if err != nil {
			t.Fatalf("ListSnapshots() error = %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("rows = %+v, want one row per window", rows)
		}
		if rows[0].Window != "Cla" || rows[0].UsedPercent != 75 {
			t.Fatalf("first row = %+v, want the Cla window", rows[0])
		}
		if rows[1].Window != "Gem" || rows[1].UsedPercent != 55 || !rows[1].ObservedAt.Equal(observed.Add(time.Minute)) {
			t.Fatalf("second row = %+v, want the replaced Gem window", rows[1])
		}
		if len(rows[1:]) != 1 || rows[1].Source != "probe" {
			t.Fatalf("row = %+v, want the source kept", rows[1])
		}
	})

	t.Run("history replaces an observation in the same second", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		repo := sqlite.NewQuotaRepo(db)
		observed := time.Now().Truncate(time.Second)
		row := sqlite.QuotaRow{CredentialID: "credential-1", Window: "Gem", UsedPercent: 10, ObservedAt: observed}
		if err := repo.AppendHistory(context.Background(), []sqlite.QuotaRow{row, {CredentialID: "credential-1", Window: "Gem", UsedPercent: 40, ObservedAt: observed}}); err != nil {
			t.Fatalf("AppendHistory() error = %v", err)
		}
		if got := countHistory(t, db); got != 1 {
			t.Fatalf("history rows = %d, want the second observation to replace the first", got)
		}
	})

	t.Run("history trims at the age bound", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		repo := sqlite.NewQuotaRepo(db)
		now := time.Now()
		if err := repo.AppendHistory(context.Background(), []sqlite.QuotaRow{
			{CredentialID: "credential-1", Window: "Gem", ObservedAt: now.Add(-48 * time.Hour)},
			{CredentialID: "credential-1", Window: "Gem", ObservedAt: now},
		}); err != nil {
			t.Fatalf("AppendHistory() error = %v", err)
		}
		if err := repo.TrimHistory(context.Background(), quotaBounds(200, 64, 4096, 24*time.Hour)); err != nil {
			t.Fatalf("TrimHistory() error = %v", err)
		}
		if got := countHistory(t, db); got != 1 {
			t.Fatalf("history rows = %d, want the aged observation trimmed", got)
		}
	})

	t.Run("history trims at the per-credential bound", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		repo := sqlite.NewQuotaRepo(db)
		now := time.Now()
		rows := make([]sqlite.QuotaRow, 0, 5)
		for index := 0; index < 5; index++ {
			rows = append(rows, sqlite.QuotaRow{
				CredentialID: "credential-1", Window: "Gem", UsedPercent: float64(index),
				ObservedAt: now.Add(time.Duration(index) * time.Second),
			})
		}
		if err := repo.AppendHistory(context.Background(), rows); err != nil {
			t.Fatalf("AppendHistory() error = %v", err)
		}
		if err := repo.TrimHistory(context.Background(), quotaBounds(2, 64, 4096, 30*24*time.Hour)); err != nil {
			t.Fatalf("TrimHistory() error = %v", err)
		}
		if got := countHistory(t, db); got != 2 {
			t.Fatalf("history rows = %d, want the newest two kept", got)
		}
	})

	t.Run("history trims at the credential bound", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		repo := sqlite.NewQuotaRepo(db)
		now := time.Now()
		if err := repo.AppendHistory(context.Background(), []sqlite.QuotaRow{
			{CredentialID: "old", Window: "Gem", ObservedAt: now.Add(-time.Hour)},
			{CredentialID: "fresh", Window: "Gem", ObservedAt: now},
		}); err != nil {
			t.Fatalf("AppendHistory() error = %v", err)
		}
		if err := repo.TrimHistory(context.Background(), quotaBounds(200, 1, 4096, 30*24*time.Hour)); err != nil {
			t.Fatalf("TrimHistory() error = %v", err)
		}
		if got := countHistory(t, db); got != 1 {
			t.Fatalf("history rows = %d, want only the most recently observed credential kept", got)
		}
	})

	t.Run("history trims at the total bound", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		repo := sqlite.NewQuotaRepo(db)
		now := time.Now()
		rows := make([]sqlite.QuotaRow, 0, 6)
		for index := 0; index < 6; index++ {
			rows = append(rows, sqlite.QuotaRow{
				CredentialID: fmt.Sprintf("credential-%d", index), Window: "Gem",
				ObservedAt: now.Add(time.Duration(index) * time.Second),
			})
		}
		if err := repo.AppendHistory(context.Background(), rows); err != nil {
			t.Fatalf("AppendHistory() error = %v", err)
		}
		if err := repo.TrimHistory(context.Background(), quotaBounds(200, 64, 2, 30*24*time.Hour)); err != nil {
			t.Fatalf("TrimHistory() error = %v", err)
		}
		if got := countHistory(t, db); got != 2 {
			t.Fatalf("history rows = %d, want the total bound enforced", got)
		}
	})

	t.Run("repository failures are reported", func(t *testing.T) {
		db := testkit.OpenTestDB(t)
		repo := sqlite.NewQuotaRepo(db)
		if err := db.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		ctx := context.Background()
		if err := repo.UpsertSnapshots(ctx, []sqlite.QuotaRow{{CredentialID: "c", Window: "Gem"}}); err == nil {
			t.Fatal("UpsertSnapshots() error = nil, want the closed database failure")
		}
		if _, err := repo.ListSnapshots(ctx, "c"); err == nil {
			t.Fatal("ListSnapshots() error = nil, want the closed database failure")
		}
		if err := repo.AppendHistory(ctx, []sqlite.QuotaRow{{CredentialID: "c", Window: "Gem"}}); err == nil {
			t.Fatal("AppendHistory() error = nil, want the closed database failure")
		}
		if err := repo.TrimHistory(ctx, quotaBounds(1, 1, 1, time.Hour)); err == nil {
			t.Fatal("TrimHistory() error = nil, want the closed database failure")
		}
		if _, err := repo.ListSnapshots(ctx, "absent"); err != nil && !errors.Is(err, sqlite.ErrCredentialNotFound) {
			t.Logf("ListSnapshots() on a closed database = %v", err)
		}
	})
}

func countHistory(t *testing.T, db *sqlite.DB) int {
	t.Helper()
	var count int
	if err := db.SQL().QueryRow("SELECT COUNT(*) FROM quota_history").Scan(&count); err != nil {
		t.Fatalf("count quota history: %v", err)
	}
	return count
}

func quotaBounds(perCredential, credentials, total int, maxAge time.Duration) sqlite.QuotaBounds {
	return sqlite.QuotaBounds{PerCredential: perCredential, Credentials: credentials, Total: total, MaxAge: maxAge}
}
