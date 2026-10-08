package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	"github.com/jonaskahn/relo/internal/catalog"
)

// TestModelsDevOnlyDescribesTheModelsTheProviderServes is the sourcing rule
// the two layers exist for, held against both writers: a refresh of the
// connection takes its identifiers from the provider's own list, and a refresh
// of the catalog rewrites facts about the rows that are already there. A model
// the catalog describes and the provider never listed is not a model of the
// connection, however well the catalog describes it.
func TestModelsDevOnlyDescribesTheModelsTheProviderServes(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	listing := listingServer(t, http.StatusOK, `{"data":[{"id":"live-1","name":"Live One"}]}`)
	catalogDocument := catalogServer(t, map[string]map[string]any{
		"groq": {
			"id": "groq", "name": "Groq", "npm": "@ai-sdk/groq", "api": listing.URL,
			"models": map[string]any{
				"live-1": map[string]any{
					"id": "live-1", "name": "Catalog Live One", "status": "active",
					"limit": map[string]any{"context": 4096, "output": 512},
					"cost":  map[string]any{"input": 1.0, "output": 2.0},
				},
				"catalog-only": map[string]any{
					"id": "catalog-only", "name": "Catalog Only", "status": "active",
					"cost": map[string]any{"input": 3.0, "output": 4.0},
				},
			},
		},
	})
	manager := pageService(t, h, catalogDocument.URL)
	seedRow(t, h, sqlite.ProviderRow{
		ID: "groq", TemplateID: "groq", Origin: string(catalog.OriginTemplate),
		Label: "Groq", Auth: string(catalog.AuthAPIKey),
		APIFormat: string(catalog.FormatOpenAIChat), BaseURL: listing.URL + "/v1",
		ModelsFormat: string(catalog.ModelsOpenAI), ModelsDevProviderID: "groq",
		Enabled: true, Rank: 100,
	})
	h.addAccount("groq", "work", "sk-1")

	result, err := manager.RefreshProviderModels(ctx, "groq")
	if err != nil {
		t.Fatalf("RefreshProviderModels() error = %v", err)
	}
	if result.Listed != 1 || result.Added != 1 {
		t.Fatalf("result = %+v, want the one id the provider listed", result)
	}

	// The listed id is a row, and it carries what the catalog describes about
	// it.
	live, err := manager.Model(ctx, "groq", "live-1")
	if err != nil {
		t.Fatalf("Model(live-1) error = %v", err)
	}
	// The catalog's own 4096, offered down to the whole thousand a client is
	// asked for.
	if live.ContextWindow == nil || *live.ContextWindow != 4_000 {
		t.Fatalf("context window = %v, want the catalog's own 4096 rounded to 4000", live.ContextWindow)
	}
	if live.Prices.ModelsDev.Input == nil || *live.Prices.ModelsDev.Input != 1_000_000 {
		t.Fatalf("catalog rate = %v, want the rate the catalog stated", live.Prices.ModelsDev.Input)
	}

	// A model only the catalog describes is not a model of this connection.
	if _, err := manager.Model(ctx, "groq", "catalog-only"); err == nil {
		t.Fatal("catalog-only is a row, want the provider's own list to decide")
	}

	// Refreshing the catalog rewrites facts, never the roster: the same ids are
	// there afterwards, and the catalog's own extra model is still not one of
	// them.
	before := modelIDsOf(t, manager, "groq")
	if _, err := manager.RefreshModelsDev(ctx); err != nil {
		t.Fatalf("RefreshModelsDev() error = %v", err)
	}
	after := modelIDsOf(t, manager, "groq")
	if len(after) != len(before) {
		t.Fatalf("models = %v, want the roster a catalog refresh found", after)
	}
	for i := range before {
		if after[i] != before[i] {
			t.Fatalf("models = %v, want the roster a catalog refresh found", after)
		}
	}
	if _, err := manager.Model(ctx, "groq", "catalog-only"); err == nil {
		t.Fatal("catalog-only became a row, want the catalog kept to the facts it describes")
	}
	afterRefresh, err := manager.Model(ctx, "groq", "live-1")
	if err != nil {
		t.Fatalf("Model(live-1) error = %v", err)
	}
	if afterRefresh.Prices.ModelsDev.Input == nil {
		t.Fatal("the catalog refresh dropped the facts of a listed model")
	}
}

// TestModelsDevCapabilitiesFollowTheAccountKind is the rule for the three
// flags a catalog publishes. A listing that omits them takes tools, reasoning,
// and vision from models.dev, whether the account is an API key or a browser
// sign-in. An override the operator set still wins after a refresh.
func TestModelsDevCapabilitiesFollowTheAccountKind(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	listing := listingServer(t, http.StatusOK, `{"data":[{"id":"live-1","name":"Live One"}]}`)
	described := map[string]any{
		"id": "live-1", "name": "Catalog Live One", "status": "active",
		"tool_call": true, "reasoning": true,
		"modalities": map[string]any{"input": []string{"text", "image"}, "output": []string{"text"}},
		"limit":      map[string]any{"context": 4096, "output": 512},
		"cost":       map[string]any{"input": 1.0, "output": 2.0},
	}
	catalogDocument := catalogServer(t, map[string]map[string]any{
		"groq": {
			"id": "groq", "name": "Groq", "npm": "@ai-sdk/groq", "api": listing.URL,
			"models": map[string]any{"live-1": described},
		},
		"anthropic": {
			"id": "anthropic", "name": "Anthropic",
			"models": map[string]any{"model-1": map[string]any{
				"id": "model-1", "name": "Claude", "status": "active",
				"tool_call": true, "reasoning": true,
				"modalities": map[string]any{"input": []string{"text", "image"}, "output": []string{"text"}},
				"cost":       map[string]any{"input": 3.0, "output": 15.0},
			}},
		},
	})
	manager := pageService(t, h, catalogDocument.URL)

	seedRow(t, h, sqlite.ProviderRow{
		ID: "groq", TemplateID: "groq", Origin: string(catalog.OriginTemplate),
		Label: "Groq", Auth: string(catalog.AuthAPIKey),
		APIFormat: string(catalog.FormatOpenAIChat), BaseURL: listing.URL + "/v1",
		ModelsFormat: string(catalog.ModelsOpenAI), ModelsDevProviderID: "groq",
		Enabled: true, Rank: 100,
	})
	h.addAccount("groq", "work", "sk-1")
	if _, err := manager.RefreshProviderModels(ctx, "groq"); err != nil {
		t.Fatalf("RefreshProviderModels() error = %v", err)
	}
	keyed, err := manager.Model(ctx, "groq", "live-1")
	if err != nil {
		t.Fatalf("Model(live-1) error = %v", err)
	}
	if !capabilityOn(keyed.Capabilities.Tools) || !capabilityOn(keyed.Capabilities.Reasoning) || !capabilityOn(keyed.Capabilities.Vision) {
		t.Fatalf("capabilities = %+v, want models.dev to fill the flags a listing omitted", keyed.Capabilities)
	}
	if keyed.Prices.ModelsDev.Input == nil {
		t.Fatal("the API key refresh dropped the catalog price")
	}

	on := true
	if _, err := manager.PatchModel(ctx, "groq", "live-1", appcatalog.ModelPatch{
		Override: &appcatalog.ModelOverride{Reasoning: &on},
	}); err != nil {
		t.Fatalf("PatchModel() error = %v", err)
	}
	if _, err := manager.RefreshProviderModels(ctx, "groq"); err != nil {
		t.Fatalf("RefreshProviderModels() error = %v", err)
	}
	overridden, err := manager.Model(ctx, "groq", "live-1")
	if err != nil {
		t.Fatalf("Model(live-1) error = %v", err)
	}
	if overridden.Capabilities.Reasoning == nil || !*overridden.Capabilities.Reasoning {
		t.Fatalf("reasoning = %v, want the operator's override to survive the refresh", overridden.Capabilities.Reasoning)
	}
	if !capabilityOn(overridden.Capabilities.Tools) || !capabilityOn(overridden.Capabilities.Vision) {
		t.Fatalf("capabilities = %+v, want models.dev to keep the flags the override did not set", overridden.Capabilities)
	}
	if overridden.CapabilityOverride.Tools != nil || overridden.CapabilityOverride.Vision != nil {
		t.Fatalf("override = %+v, want only reasoning forced", overridden.CapabilityOverride)
	}

	seedRow(t, h, sqlite.ProviderRow{
		ID: "claude", TemplateID: "claude", Origin: string(catalog.OriginSignIn),
		Label: "Claude", Auth: string(catalog.AuthOAuth),
		APIFormat: string(catalog.FormatAnthropic), ModelsDevProviderID: "anthropic",
		Enabled: true, Rank: 100,
	})
	if _, err := manager.RefreshModelsDev(ctx); err != nil {
		t.Fatalf("RefreshModelsDev() error = %v", err)
	}
	browser, err := manager.Model(ctx, "claude", "model-1")
	if err != nil {
		t.Fatalf("Model(model-1) error = %v", err)
	}
	if !capabilityOn(browser.Capabilities.Tools) || !capabilityOn(browser.Capabilities.Reasoning) || !capabilityOn(browser.Capabilities.Vision) {
		t.Fatalf("capabilities = %+v, want the catalog's flags on a browser account", browser.Capabilities)
	}
}

func capabilityOn(value *bool) bool {
	return value != nil && *value
}

// TestCapabilityPatchKeepsTheRestOfTheOverride covers a row that sets one
// flag: the context window already stored stays, and clearing the flag does
// not clear it either.
func TestCapabilityPatchKeepsTheRestOfTheOverride(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	ctx := context.Background()

	var decoded appcatalog.ModelPatch
	if err := json.Unmarshal([]byte(`{"reasoning":true}`), &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !decoded.Reasoning.Set || decoded.Reasoning.Value == nil || !*decoded.Reasoning.Value {
		t.Fatalf("reasoning = %+v, want a set true", decoded.Reasoning)
	}
	if decoded.Tools.Set || decoded.Vision.Set {
		t.Fatal("a reasoning write set the other flags")
	}

	window := int64(8192)
	if _, err := manager.PatchModel(ctx, "openai", "model-1", appcatalog.ModelPatch{
		ContextWindow: appcatalog.OptionalInt{Set: true, Value: &window},
	}); err != nil {
		t.Fatalf("PatchModel(context) error = %v", err)
	}
	on := true
	patched, err := manager.PatchModel(ctx, "openai", "model-1", appcatalog.ModelPatch{
		Reasoning: appcatalog.OptionalBool{Set: true, Value: &on},
	})
	if err != nil {
		t.Fatalf("PatchModel(reasoning) error = %v", err)
	}
	if patched.ContextWindow == nil || *patched.ContextWindow != 8_000 {
		t.Fatalf("context = %v, want the window kept, rounded to 8000", patched.ContextWindow)
	}
	if !capabilityOn(patched.Capabilities.Reasoning) || !capabilityOn(patched.CapabilityOverride.Reasoning) {
		t.Fatalf("reasoning = %+v override %+v, want on", patched.Capabilities.Reasoning, patched.CapabilityOverride.Reasoning)
	}

	cleared, err := manager.PatchModel(ctx, "openai", "model-1", appcatalog.ModelPatch{
		Reasoning: appcatalog.OptionalBool{Set: true},
	})
	if err != nil {
		t.Fatalf("PatchModel(clear) error = %v", err)
	}
	if cleared.Capabilities.Reasoning != nil || cleared.CapabilityOverride.Reasoning != nil {
		t.Fatalf("reasoning = %+v, want the override cleared", cleared.Capabilities)
	}
	if cleared.ContextWindow == nil || *cleared.ContextWindow != 8_000 {
		t.Fatalf("context = %v, want the window kept after the flag was cleared", cleared.ContextWindow)
	}
}

// TestPricesPatchKeepsTheRestOfTheOverride covers a row that writes rates from
// its cell: the rewrite replaces the rates rather than the layer, and clearing
// them leaves the price to the provider and models.dev without touching what
// the layer states about anything else.
func TestPricesPatchKeepsTheRestOfTheOverride(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	ctx := context.Background()

	window := int64(8192)
	if _, err := manager.PatchModel(ctx, "openai", "model-1", appcatalog.ModelPatch{
		ContextWindow: appcatalog.OptionalInt{Set: true, Value: &window},
	}); err != nil {
		t.Fatalf("PatchModel(context) error = %v", err)
	}
	input, output := int64(1_000_000), int64(2_000_000)
	priced, err := manager.PatchModel(ctx, "openai", "model-1", appcatalog.ModelPatch{
		Prices: &appcatalog.Prices{Input: &input, Output: &output},
	})
	if err != nil {
		t.Fatalf("PatchModel(prices) error = %v", err)
	}
	if priced.Prices.Override.Input == nil || *priced.Prices.Override.Input != input {
		t.Fatalf("input = %+v, want the rate the operator wrote", priced.Prices.Override.Input)
	}
	if priced.Prices.Effective.Output == nil || *priced.Prices.Effective.Output != output {
		t.Fatalf("output = %+v, want the rate the operator wrote", priced.Prices.Effective.Output)
	}
	if priced.ContextWindow == nil || *priced.ContextWindow != 8_000 {
		t.Fatalf("context = %v, want the window the price write did not touch", priced.ContextWindow)
	}

	next := int64(3_000_000)
	replaced, err := manager.PatchModel(ctx, "openai", "model-1", appcatalog.ModelPatch{
		Prices: &appcatalog.Prices{Input: &next},
	})
	if err != nil {
		t.Fatalf("PatchModel(replace) error = %v", err)
	}
	if replaced.Prices.Override.Input == nil || *replaced.Prices.Override.Input != next {
		t.Fatalf("input = %+v, want the rewritten rate", replaced.Prices.Override.Input)
	}
	if replaced.Prices.Override.Output != nil {
		t.Fatalf("output = %+v, want a rewrite to replace the rates it did not state", replaced.Prices.Override.Output)
	}

	cleared, err := manager.PatchModel(ctx, "openai", "model-1", appcatalog.ModelPatch{
		Prices: &appcatalog.Prices{},
	})
	if err != nil {
		t.Fatalf("PatchModel(clear) error = %v", err)
	}
	if cleared.Prices.Effective.Input != nil || cleared.Prices.Effective.Output != nil {
		t.Fatalf("prices = %+v, want the cleared rates to fall back to the lower layers", cleared.Prices.Effective)
	}
	if cleared.Name != "Model 1" {
		t.Fatalf("name = %q, want the name the layer held to stay", cleared.Name)
	}
	if cleared.ContextWindow == nil || *cleared.ContextWindow != 8_000 {
		t.Fatalf("context = %v, want the window kept after the rates were cleared", cleared.ContextWindow)
	}
}

func TestSetModelsCapabilitiesWritesEveryNamedModel(t *testing.T) {
	h := newHarness(t)
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)
	ctx := context.Background()
	on := true
	err := manager.SetModelsCapabilities(ctx, "openai", []string{"model-1", "missing"},
		appcatalog.CapabilityFlags{Tools: appcatalog.OptionalBool{Set: true, Value: &on}})
	if err != nil {
		t.Fatalf("SetModelsCapabilities() error = %v", err)
	}
	model, err := manager.Model(ctx, "openai", "model-1")
	if err != nil {
		t.Fatalf("Model() error = %v", err)
	}
	if !capabilityOn(model.CapabilityOverride.Tools) || model.CapabilityOverride.Reasoning != nil || model.CapabilityOverride.Vision != nil {
		t.Fatalf("override = %+v, want only tools on", model.CapabilityOverride)
	}
}

// modelIDsOf names the models one connection holds, sorted.
func modelIDsOf(t *testing.T, manager *session, providerID string) []string {
	t.Helper()
	models, _, err := manager.Models(context.Background(), appcatalog.ModelQuery{Provider: providerID})
	if err != nil {
		t.Fatalf("Models(%s) error = %v", providerID, err)
	}
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ModelID)
	}
	sort.Strings(ids)
	return ids
}
