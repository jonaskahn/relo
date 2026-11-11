package integration

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestFreeLaneIsNotOfferedForABareTurn pins the signed-out OpenCode lane out
// of an integration test turn. That lane is served to OpenCode's own client,
// and a test turn carries a bare prompt, so the gateway refuses it: the lane
// is neither listed nor selectable here.
func TestFreeLaneIsNotOfferedForABareTurn(t *testing.T) {
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	repo := sqlite.NewCatalogRepo(db)
	if err := repo.SaveProvider(context.Background(), sqlite.ProviderRow{
		ID: catalog.OpenCodeFreeTemplate, TemplateID: catalog.OpenCodeFreeTemplate,
		Origin: "template", Label: "OpenCode Zen Free", Auth: "none",
		APIFormat: "openai-chat", BaseURL: upstream.URL, ModelsFormat: "openai",
		Enabled: true, Rank: 100,
	}); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	if err := repo.SaveModel(context.Background(), sqlite.ModelRow{
		ProviderID: catalog.OpenCodeFreeTemplate, ModelID: "free-model",
		APIFormat: string(catalog.FormatOpenAIChat), Enabled: true,
	}); err != nil {
		t.Fatalf("SaveModel() error = %v", err)
	}
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"gpt-4o"}, ModelsFormat: catalog.ModelsNone, ContextWindow: 128_000,
	})
	built := testkit.ReloadCatalog(t, db, activeAccounts{})
	service := NewService(ServiceOptions{Catalog: built})

	free := catalog.ClientModelID(catalog.OpenCodeFreeTemplate, "free-model")
	paid := catalog.ClientModelID("openai", "gpt-4o")

	listed := service.withoutFreeLaneModels([]IntegrationModel{
		{ID: free, ProviderID: catalog.OpenCodeFreeTemplate},
		{ID: paid, ProviderID: "openai"},
	})
	if len(listed) != 1 || listed[0].ID != paid {
		t.Fatalf("listed = %+v, want the free lane left out", listed)
	}

	if !service.freeLaneModel(free) {
		t.Fatal("freeLaneModel() = false, want the free lane recognized")
	}
	if service.freeLaneModel(paid) {
		t.Fatal("freeLaneModel() = true, want a paid model left alone")
	}

	if _, err := service.IntegrationChat(context.Background(), "codex", IntegrationChatRequest{
		Model: free, Prompt: "hi",
	}); err == nil {
		t.Fatal("IntegrationChat() error = nil, want the free lane refused")
	}
}
