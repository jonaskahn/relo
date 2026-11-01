package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/wire/formats"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestXiaomiModelsResolveToToggleOnly(t *testing.T) {
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
		{ProviderID: "xiaomi", ModelID: "mimo-v2.6-pro", Layer: "modelsdev", SupportsReasoning: &on, ReasoningEfforts: []string{}, ReasoningToggle: &on},
		{ProviderID: "xiaomi", ModelID: "mimo-asr", Layer: "modelsdev", SupportsReasoning: &off, ReasoningEfforts: []string{"low", "xhigh"}},
		{ProviderID: "xiaomi-token-plan-sgp", ModelID: "mimo-v2.5-pro", Layer: "modelsdev", SupportsReasoning: &on, ReasoningEfforts: []string{}, ReasoningToggle: &on},
		{ProviderID: "desk", ModelID: "mimo-v2.6-pro", Layer: "modelsdev", SupportsReasoning: &on, ReasoningEfforts: []string{}, ReasoningToggle: &on},
		{ProviderID: "mimo-custom", ModelID: "mimo-v2.6-flash", Layer: "provider", SupportsReasoning: &on, ReasoningEfforts: []string{}, ReasoningToggle: &on},
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

	for _, id := range []struct{ provider, model string }{
		{"xiaomi", "mimo-v2.6-pro"},
		{"xiaomi-token-plan-sgp", "mimo-v2.5-pro"},
		{"desk", "mimo-v2.6-pro"},
		{"mimo-custom", "mimo-v2.6-flash"},
	} {
		model, ok := snap.Model(id.provider, id.model)
		if !ok || model.ReasoningEfforts == nil || len(model.ReasoningEfforts) != 0 {
			t.Fatalf("%s/%s efforts = %v, want a stated empty ladder", id.provider, id.model, model.ReasoningEfforts)
		}
		if !model.ReasoningToggle || !model.ReasoningToggleKnown {
			t.Fatalf("%s/%s toggle = %v known %v, want a known toggle", id.provider, id.model, model.ReasoningToggle, model.ReasoningToggleKnown)
		}
	}

	speech, ok := snap.Model("xiaomi", "mimo-asr")
	if !ok || !sameEfforts(speech.ReasoningEfforts, []string{}) {
		t.Fatalf("xiaomi/mimo-asr efforts = %v, want no ladder when reasoning is explicitly unsupported", speech.ReasoningEfforts)
	}
	other, ok := snap.Model("openai", "gpt-5")
	if !ok || !sameEfforts(other.ReasoningEfforts, []string{"low", "xhigh"}) {
		t.Fatalf("openai/gpt-5 efforts = %v, want the stored ladder left alone", other.ReasoningEfforts)
	}
}

// TestXiaomiConnectionsNameTheTextToolDialect pins the per-connection signal a
// chat decoder reads to recover a call the serving template rendered as text.
// Only Xiaomi connections carry it, because only their template does.
func TestXiaomiConnectionsNameTheTextToolDialect(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewCatalogRepo(db)

	providers := []sqlite.ProviderRow{
		{ID: "xiaomi", Origin: "template", Label: "Xiaomi", Auth: "api_key", BaseURL: "https://api.xiaomimimo.com/v1"},
		{ID: "xiaomi-token-plan-sgp", Origin: "template", Label: "Xiaomi Token Plan", Auth: "api_key"},
		{ID: "mimo-custom", Origin: "custom", Label: "MiMo", Auth: "api_key", BaseURL: "https://token-plan-ams.xiaomimimo.com/v1"},
		{ID: "openai", Origin: "template", Label: "OpenAI", Auth: "api_key", BaseURL: "https://api.openai.com/v1"},
	}
	for _, provider := range providers {
		if err := repo.SaveProvider(ctx, provider); err != nil {
			t.Fatalf("SaveProvider(%s) error = %v", provider.ID, err)
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

	for _, id := range []string{"xiaomi", "xiaomi-token-plan-sgp", "mimo-custom"} {
		host, ok := snap.Provider(id)
		if !ok || host.ToolCallDialect != catalog.ToolCallDialectText {
			t.Fatalf("%s dialect = %q, want %q", id, host.ToolCallDialect, catalog.ToolCallDialectText)
		}
	}
	other, ok := snap.Provider("openai")
	if !ok || other.ToolCallDialect != "" {
		t.Fatalf("openai dialect = %q, want empty", other.ToolCallDialect)
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

// TestMiMoFamilyResolvesOnEveryProvider pins that the MiMo rules follow the
// model id rather than the host: a gateway that states an empty ladder or no
// facts at all still resolves thinking as an on/off switch, a gateway that
// states a ladder keeps it, and every MiMo chat model carries the text
// tool-call dialect.
func TestMiMoFamilyResolvesOnEveryProvider(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewCatalogRepo(db)
	on, off := true, false

	providers := []sqlite.ProviderRow{
		{ID: "opencode-go", TemplateID: "opencode-go", Origin: "template", Label: "OpenCode Go", Auth: "api_key"},
		{ID: "opencode-free", TemplateID: "opencode-free", Origin: "template", Label: "OpenCode Free", Auth: "api_key"},
		{ID: "open-design", TemplateID: "open-design", Origin: "template", Label: "OpenDesign", Auth: "api_key"},
		{ID: "nano-gpt", Origin: "custom", Label: "nano-gpt", Auth: "api_key"},
		{ID: "requesty", Origin: "custom", Label: "Requesty", Auth: "api_key"},
	}
	for _, provider := range providers {
		if err := repo.SaveProvider(ctx, provider); err != nil {
			t.Fatalf("SaveProvider(%s) error = %v", provider.ID, err)
		}
	}

	models := []sqlite.ModelRow{
		{ProviderID: "opencode-go", ModelID: "mimo-v2.6-pro", Source: "listing", UpdatedAtMs: 1},
		{ProviderID: "opencode-go", ModelID: "mimo-asr", Source: "listing", UpdatedAtMs: 1},
		{ProviderID: "opencode-go", ModelID: "gpt-5", Source: "listing", UpdatedAtMs: 1},
		{ProviderID: "opencode-free", ModelID: "mimo-v2.6-flash-free", Source: "listing", UpdatedAtMs: 1},
		{ProviderID: "open-design", ModelID: "mimo-v2.6-pro", Source: "listing", UpdatedAtMs: 1},
		{ProviderID: "nano-gpt", ModelID: "xiaomi/mimo-v2.6-pro", Source: "listing", UpdatedAtMs: 1},
		{ProviderID: "requesty", ModelID: "mimo-v2.6-pro", Source: "listing", UpdatedAtMs: 1},
	}
	for _, model := range models {
		if err := repo.SaveModel(ctx, model); err != nil {
			t.Fatalf("SaveModel(%s/%s) error = %v", model.ProviderID, model.ModelID, err)
		}
	}

	facts := []sqlite.ModelFactsRow{
		{ProviderID: "opencode-go", ModelID: "mimo-v2.6-pro", Layer: "modelsdev", SupportsReasoning: &on, ReasoningEfforts: []string{}},
		{ProviderID: "opencode-go", ModelID: "mimo-asr", Layer: "modelsdev", SupportsReasoning: &off, ReasoningEfforts: []string{"low", "xhigh"}},
		{ProviderID: "opencode-go", ModelID: "gpt-5", Layer: "modelsdev", SupportsReasoning: &on, ReasoningEfforts: []string{"low", "xhigh"}},
		{ProviderID: "opencode-free", ModelID: "mimo-v2.6-flash-free", Layer: "modelsdev", SupportsReasoning: &on, ReasoningEfforts: []string{}},
		{ProviderID: "nano-gpt", ModelID: "xiaomi/mimo-v2.6-pro", Layer: "modelsdev", SupportsReasoning: &on, ReasoningEfforts: []string{"none", "high"}},
		{ProviderID: "requesty", ModelID: "mimo-v2.6-pro", Layer: "modelsdev", SupportsReasoning: &on, ReasoningEfforts: []string{"none", "low", "medium", "high", "max"}},
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

	for _, id := range []struct{ provider, model string }{
		{"opencode-go", "mimo-v2.6-pro"},
		{"opencode-free", "mimo-v2.6-flash-free"},
		{"open-design", "mimo-v2.6-pro"},
	} {
		model, ok := snap.Model(id.provider, id.model)
		if !ok || model.ReasoningEfforts == nil || len(model.ReasoningEfforts) != 0 {
			t.Fatalf("%s/%s efforts = %v, want a stated empty ladder", id.provider, id.model, model.ReasoningEfforts)
		}
		if model.SupportsReasoning == nil || !*model.SupportsReasoning || !model.ReasoningToggle || !model.ReasoningToggleKnown {
			t.Fatalf("%s/%s = %+v, want a known toggle on a reasoning model", id.provider, id.model, model)
		}
		if model.ToolCallDialect != catalog.ToolCallDialectText {
			t.Fatalf("%s/%s dialect = %q, want %q", id.provider, id.model, model.ToolCallDialect, catalog.ToolCallDialectText)
		}
	}

	for _, id := range []struct {
		provider, model string
		want            []string
	}{
		{"nano-gpt", "xiaomi/mimo-v2.6-pro", []string{"none", "high"}},
		{"requesty", "mimo-v2.6-pro", []string{"none", "low", "medium", "high", "max"}},
	} {
		model, ok := snap.Model(id.provider, id.model)
		if !ok || !sameEfforts(model.ReasoningEfforts, id.want) {
			t.Fatalf("%s/%s efforts = %v, want the stated ladder %v", id.provider, id.model, model.ReasoningEfforts, id.want)
		}
		if model.ReasoningToggle {
			t.Fatalf("%s/%s toggle = true, want the stated ladder alone", id.provider, id.model)
		}
		if model.ToolCallDialect != catalog.ToolCallDialectText {
			t.Fatalf("%s/%s dialect = %q, want %q", id.provider, id.model, model.ToolCallDialect, catalog.ToolCallDialectText)
		}
	}

	speech, ok := snap.Model("opencode-go", "mimo-asr")
	if !ok || speech.ReasoningToggle || !sameEfforts(speech.ReasoningEfforts, []string{}) {
		t.Fatalf("opencode-go/mimo-asr = %+v, want no toggle", speech)
	}
	if speech.ToolCallDialect != "" {
		t.Fatalf("opencode-go/mimo-asr dialect = %q, want none", speech.ToolCallDialect)
	}
	other, ok := snap.Model("opencode-go", "gpt-5")
	if !ok || !sameEfforts(other.ReasoningEfforts, []string{"low", "xhigh"}) || other.ReasoningToggle {
		t.Fatalf("opencode-go/gpt-5 = %+v, want the stored ladder left alone", other)
	}
	if other.ToolCallDialect != "" {
		t.Fatalf("opencode-go/gpt-5 dialect = %q, want none", other.ToolCallDialect)
	}
}
