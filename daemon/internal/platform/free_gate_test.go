package platform

import (
	"context"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	wireformats "github.com/jonaskahn/relo/internal/adapters/wire/formats"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestFreeGatePreservesRowsAcrossOptIn(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	db, err := sqlite.OpenDB(filepath.Join(home, "test.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewCatalogRepo(db)
	for _, id := range []string{catalog.OpenCodeFreeTemplate, catalog.KiloFreeTemplate, "paid"} {
		if err := repo.SaveProvider(ctx, sqlite.ProviderRow{ID: id, TemplateID: id, Label: id, Origin: "template", Auth: "none", APIFormat: "openai-chat", Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SaveModel(ctx, sqlite.ModelRow{ProviderID: catalog.OpenCodeFreeTemplate, ModelID: "free-model", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	path := config.ConfigPath(home)
	reader := freeGate{sqlite.NewCatalogReader(db), path}
	var _ catalog.AccountCapabilityReader = reader
	models := catalog.New(reader, nil, wireformats.New())
	for _, enabled := range []bool{false, true, false, true} {
		if err := config.UpdateConnectionsConfig(path, enabled); err != nil {
			t.Fatal(err)
		}
		rows, err := reader.ListConnections(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := models.Reload(ctx); err != nil {
			t.Fatal(err)
		}
		snapshot, _ := models.Snapshot()
		_, exists := snapshot.Model(catalog.OpenCodeFreeTemplate, "free-model")
		if exists != enabled {
			t.Fatalf("enabled=%v model visible=%v", enabled, exists)
		}
		want := 1
		if enabled {
			want = 3
		}
		if len(rows) != want {
			t.Fatalf("enabled=%v rows=%d want=%d", enabled, len(rows), want)
		}
		stored, err := repo.ListProviders(ctx)
		if err != nil || len(stored) != 3 {
			t.Fatalf("stored rows changed: %v %v", stored, err)
		}
	}
	if err := os.WriteFile(path, []byte("[connections]\nfree = 'broken'"), 0600); err != nil {
		t.Fatal(err)
	}
	rows, err := reader.ListConnections(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("invalid config must fail closed: %v %v", rows, err)
	}
}
