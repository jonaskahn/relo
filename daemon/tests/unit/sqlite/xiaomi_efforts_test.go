package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/wire/formats"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestXiaomiModelsOnlyOfferThreeThinkLevels(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewCatalogRepo(db)
	on, off := true, false

	providers := []sqlite.ProviderRow{
		{ID: "xiaomi", Origin: "template", Label: "Xiaomi", Auth: "api_key", BaseURL: "https://api.xiaomimimo.com/v1"},
		{ID: "xiaomi-token-plan-sgp", Origin: "template", Label: "Xiaomi Token Plan", Auth: "api_key"},
		{ID: "desk", TemplateID: "xiaomi-token-plan-cn", Origin: "template", Label: "Desk", Auth: "api_key"},
		{ID: "mimo-custom", Origin: "custom", Label: "MiMo", Auth: "api_key", BaseURL: "https://token-plan-ams.xiaomimimo.com/v1"},
		{ID: "openai", Origin: "template", Label: "OpenAI", Auth: "api_key", BaseURL: "https://api.openai.com/v1"},
	}
	for _, provider := range providers {
		if err := repo.SaveProvider(ctx, provider); err != nil {
			t.Fatalf("SaveProvider(%s) error = %v", provider.ID, err)
		}
	}

	models := []sqlite.ModelRow{
		{ProviderID: "xiaomi", ModelID: "mimo-v2.6-pro", Source: "listing", UpdatedAtMs: 1},
		{ProviderID: "xiaomi", ModelID: "mimo-asr", Source: "listing", UpdatedAtMs: 1},
		{ProviderID: "xiaomi-token-plan-sgp", ModelID: "mimo-v2.5-pro", Source: "listing", UpdatedAtMs: 1},
		{ProviderID: "desk", ModelID: "mimo-v2.6-pro", Source: "listing", UpdatedAtMs: 1},
		{ProviderID: "mimo-custom", ModelID: "mimo-v2.6-flash", Source: "listing", UpdatedAtMs: 1},
		{ProviderID: "openai", ModelID: "gpt-5", Source: "listing", UpdatedAtMs: 1},
	}
	for _, model := range models {
		if err := repo.SaveModel(ctx, model); err != nil {
			t.Fatalf("SaveModel(%s/%s) error = %v", model.ProviderID, model.ModelID, err)
		}
	}

	facts := []sqlite.ModelFactsRow{
		{ProviderID: "xiaomi", ModelID: "mimo-v2.6-pro", Layer: "modelsdev", SupportsReasoning: &on, ReasoningEfforts: []string{"low", "xhigh"}},
		{ProviderID: "xiaomi", ModelID: "mimo-asr", Layer: "modelsdev", SupportsReasoning: &off, ReasoningEfforts: []string{"low", "xhigh"}},
		{ProviderID: "xiaomi-token-plan-sgp", ModelID: "mimo-v2.5-pro", Layer: "modelsdev", SupportsReasoning: &on},
		{ProviderID: "desk", ModelID: "mimo-v2.6-pro", Layer: "modelsdev", SupportsReasoning: &on, ReasoningEfforts: []string{"max"}},
		{ProviderID: "mimo-custom", ModelID: "mimo-v2.6-flash", Layer: "provider", SupportsReasoning: &on},
		{ProviderID: "openai", ModelID: "gpt-5", Layer: "modelsdev", SupportsReasoning: &on, ReasoningEfforts: []string{"low", "xhigh"}},
	}
	for _, layer := range facts {
		if err := repo.SaveModelFacts(ctx, layer); err != nil {
			t.Fatalf("SaveModelFacts(%s/%s) error = %v", layer.ProviderID, layer.ModelID, err)
		}
	}

	built := catalog.New(sqlite.NewCatalogReader(db), nil, formats.New())
	if err := built.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	snap, found := built.Snapshot()
	if !found {
		t.Fatal("Reload() stored no snapshot")
	}

	want := []string{"low", "medium", "high"}
	for _, id := range []struct{ provider, model string }{
		{"xiaomi", "mimo-v2.6-pro"},
		{"xiaomi-token-plan-sgp", "mimo-v2.5-pro"},
		{"desk", "mimo-v2.6-pro"},
		{"mimo-custom", "mimo-v2.6-flash"},
	} {
		model, ok := snap.Model(id.provider, id.model)
		if !ok || !sameEfforts(model.ReasoningEfforts, want) {
			t.Fatalf("%s/%s efforts = %v, want %v", id.provider, id.model, model.ReasoningEfforts, want)
		}
	}

	speech, ok := snap.Model("xiaomi", "mimo-asr")
	if !ok || !sameEfforts(speech.ReasoningEfforts, []string{"low", "xhigh"}) {
		t.Fatalf("xiaomi/mimo-asr efforts = %v, want the stored ladder left alone", speech.ReasoningEfforts)
	}
	other, ok := snap.Model("openai", "gpt-5")
	if !ok || !sameEfforts(other.ReasoningEfforts, []string{"low", "xhigh"}) {
		t.Fatalf("openai/gpt-5 efforts = %v, want the stored ladder left alone", other.ReasoningEfforts)
	}
}

func sameEfforts(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
