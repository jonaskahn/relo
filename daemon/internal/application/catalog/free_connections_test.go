package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestFreePoolProbeAndStaleCommitAreRefused(t *testing.T) {
	ctx := context.Background()
	for _, id := range []string{catalog.OpenCodeFreeTemplate, catalog.KiloFreeTemplate} {
		t.Run(id, func(t *testing.T) {
			svc := New(Options{FreeConnections: func() bool { return false }})
			_, err := svc.ProbeProvider(ctx, ProbeRequest{TemplateID: id})
			if !errors.Is(err, ErrFreeConnectionsDisabled) {
				t.Fatalf("probe error = %v", err)
			}
			svc.probes.Store("stale", &probeSession{ID: "stale", ExpiresAt: time.Now().Add(time.Minute), Provider: catalog.ConnectionRecord{TemplateID: id}})
			_, err = svc.CommitProbe(ctx, "stale", CommitProbeRequest{})
			if !errors.Is(err, ErrFreeConnectionsDisabled) {
				t.Fatalf("commit error = %v", err)
			}
		})
	}
}

func TestFreeConnectionChecksReadTheStoredTemplateWhenTheIdIsKnown(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewCatalogRepo(testkit.OpenTestDB(t))
	if err := repo.SaveProvider(ctx, sqlite.ProviderRow{
		ID: "stored-free", TemplateID: catalog.OpenCodeFreeTemplate,
		Origin: "template", Label: "OpenCode Zen Free", Auth: "none",
	}); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	svc := New(Options{Store: repo, FreeConnections: func() bool { return false }})
	if err := svc.checkFreeConnection(ctx, "", "stored-free"); !errors.Is(err, ErrFreeConnectionsDisabled) {
		t.Fatalf("checkFreeConnection() = %v, want the stored template's opt-out", err)
	}
	if err := svc.checkFreeConnection(ctx, "", "missing"); err == nil {
		t.Fatal("checkFreeConnection() on a missing id returned no read failure")
	}
}

func TestSingleFreeConnectionRuleReadsStoredRows(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewCatalogRepo(testkit.OpenTestDB(t))
	for _, row := range []sqlite.ProviderRow{
		{ID: "paid", TemplateID: "custom", Origin: "custom", Label: "Paid", Auth: "api_key"},
		{ID: "free", TemplateID: catalog.KiloFreeTemplate, Origin: "template", Label: "Kilo Free", Auth: "none"},
	} {
		if err := repo.SaveProvider(ctx, row); err != nil {
			t.Fatalf("SaveProvider(%s) error = %v", row.ID, err)
		}
	}
	svc := New(Options{Store: repo, FreeConnections: func() bool { return true }})
	if err := svc.checkSingleFreeConnection(ctx, catalog.KiloFreeTemplate, ""); !errors.Is(err, ErrFreeConnectionExists) {
		t.Fatalf("checkSingleFreeConnection() = %v, want the stored duplicate", err)
	}
	if err := svc.checkSingleFreeConnection(ctx, "custom", ""); err != nil {
		t.Fatalf("checkSingleFreeConnection(custom) = %v, want no rule for a paid row", err)
	}
	if err := svc.checkSingleFreeConnection(ctx, catalog.KiloFreeTemplate, "paid"); err != nil {
		t.Fatalf("checkSingleFreeConnection(with a target) = %v, want adding a key allowed", err)
	}
}
