package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestRepositoriesReportAClosedDatabase(t *testing.T) {
	db := testkit.OpenTestDB(t)
	catalog := sqlite.NewCatalogRepo(db)
	credentials := sqlite.NewCredentialRepo(db)
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	ctx := context.Background()
	provider := sqlite.ProviderRow{ID: "openai", Origin: "custom", Label: "OpenAI", Auth: "api_key"}
	model := sqlite.ModelRow{ProviderID: "openai", ModelID: "gpt-4o", Source: "manual"}
	group := sqlite.GroupRow{ID: "fast", Strategy: "priority"}
	calls := map[string]func() error{
		"list providers":  func() error { _, err := catalog.ListProviders(ctx); return err },
		"get provider":    func() error { _, err := catalog.GetProvider(ctx, "openai"); return err },
		"save provider":   func() error { return catalog.SaveProvider(ctx, provider) },
		"list models":     func() error { _, err := catalog.ListModels(ctx); return err },
		"get model":       func() error { _, err := catalog.GetModel(ctx, "openai", "gpt-4o"); return err },
		"save model":      func() error { return catalog.SaveModel(ctx, model) },
		"list groups":     func() error { _, err := catalog.ListGroups(ctx); return err },
		"save group":      func() error { return catalog.SaveGroup(ctx, group) },
		"delete group":    func() error { return catalog.DeleteGroup(ctx, "fast") },
		"modelsdev state": func() error { _, _, err := catalog.GetModelsDevState(ctx); return err },
		"save model facts": func() error {
			return catalog.SaveModelFacts(ctx, sqlite.ModelFactsRow{ProviderID: "openai", ModelID: "gpt-4o", Layer: "override"})
		},
		"list credentials": func() error {
			_, err := credentials.List(ctx)
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Fatalf("%s on a closed database returned no error", name)
			}
		})
	}
}
