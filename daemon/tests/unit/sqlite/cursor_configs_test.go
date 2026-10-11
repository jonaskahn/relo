package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

func cursorVariant(group, selection, effort, details string) catalog.CursorVariant {
	return catalog.CursorVariant{
		GroupID: group, Selection: selection, Effort: effort,
		ModelID: "sonnet", DetailsID: details,
		Params:  []catalog.CursorParam{{ID: "effort", Value: effort}},
		Context: 200000, Reasoning: true,
	}
}

func TestCursorConfigsRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewCursorConfigRepo(db)
	configs := []catalog.CursorVariant{
		cursorVariant("sonnet", "low", "low", "sonnet-low"),
		cursorVariant("sonnet", "high", "high", "sonnet-high"),
	}
	configs[0].Default = true
	if err := repo.Save(ctx, "cursor", "cred-a", configs); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	stored, err := repo.Load(ctx, "cursor", "cred-a")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	bySelection := map[string]catalog.CursorVariant{}
	for _, config := range stored {
		bySelection[config.Selection] = config
	}
	low, high := bySelection["low"], bySelection["high"]
	if len(stored) != 2 || !low.Default || high.DetailsID != "sonnet-high" {
		t.Fatalf("stored = %+v", stored)
	}
	if len(low.Params) != 1 || low.Params[0].ID != "effort" || low.Context != 200000 || !low.Reasoning {
		t.Fatalf("low = %+v, want params, context, and reasoning kept", low)
	}
	if _, err := repo.Load(ctx, "cursor", "cred-b"); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestCursorConfigsReplace(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewCursorConfigRepo(db)
	if err := repo.Save(ctx, "cursor", "cred-a", []catalog.CursorVariant{
		cursorVariant("sonnet", "low", "low", "sonnet-low"),
		cursorVariant("sonnet", "high", "high", "sonnet-high"),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := repo.Save(ctx, "cursor", "cred-a", []catalog.CursorVariant{
		cursorVariant("sonnet", "low", "low", "sonnet-low"),
		cursorVariant("sonnet", "low", "low", "sonnet-low"),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	stored, err := repo.Load(ctx, "cursor", "cred-a")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored = %+v, want the duplicate dropped and the stale row replaced", stored)
	}
	if err := repo.Save(ctx, "cursor", "cred-a", nil); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	cleared, err := repo.Load(ctx, "cursor", "cred-a")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cleared) != 0 {
		t.Fatalf("cleared = %+v, want an empty refresh to clear the account", cleared)
	}
}

func TestCursorConfigsForget(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewCursorConfigRepo(db)
	if err := repo.Save(ctx, "cursor", "cred-a", []catalog.CursorVariant{
		cursorVariant("sonnet", "low", "low", "sonnet-low"),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := repo.Forget(ctx, "cred-a"); err != nil {
		t.Fatalf("Forget() error = %v", err)
	}
	stored, err := repo.Load(ctx, "cursor", "cred-a")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("stored = %+v, want nothing after forget", stored)
	}
}
