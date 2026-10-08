package codingclients_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/catalog"
)

func TestMergeCodexConfigPointsAtTheCatalog(t *testing.T) {
	catalog := filepath.Join(t.TempDir(), "relo-model-catalog.json")
	existing := "model_catalog_json = \"/operator/catalog.json\"\napproval_policy = \"never\"\n"
	merged, err := codingclients.MergeCodexConfig(existing, baseURL, catalog, helperFile, true)
	if err != nil {
		t.Fatalf("MergeCodexConfig() error = %v", err)
	}
	document := map[string]any{}
	if err := toml.Unmarshal([]byte(merged), &document); err != nil {
		t.Fatalf("the merged file does not parse: %v\n%s", err, merged)
	}
	if document["model_catalog_json"] != catalog {
		t.Fatalf("model_catalog_json = %v, want %s", document["model_catalog_json"], catalog)
	}
	if strings.Count(merged, "model_catalog_json") != 1 {
		t.Fatalf("catalog assignments = %d, want one\n%s", strings.Count(merged, "model_catalog_json"), merged)
	}
	again, err := codingclients.MergeCodexConfig(merged, baseURL, catalog, helperFile, true)
	if err != nil {
		t.Fatalf("MergeCodexConfig() again error = %v", err)
	}
	if again != merged {
		t.Fatalf("a second merge changed the file:\n%s\n---\n%s", merged, again)
	}
	stripped := codingclients.StripCodexConfig(merged)
	if strings.Contains(stripped, "relo-model-catalog.json") || strings.Contains(stripped, "model_catalog_json") {
		t.Fatalf("stripped file still names the catalog:\n%s", stripped)
	}
	if !strings.Contains(stripped, "approval_policy") {
		t.Fatalf("stripped file lost the operator's row:\n%s", stripped)
	}
}

func TestWriteCodexCatalogKeepsKnownLimits(t *testing.T) {
	reasoning := true
	plain := false
	window := int64(200000)
	vision := true
	path := codingclients.CodexCatalogPath(filepath.Join(t.TempDir(), "config.toml"))
	err := codingclients.WriteCodexCatalog(path, []codingclients.ModelRef{
		{ID: "thinker", Name: "Thinker", ContextWindow: &window, Reasoning: &reasoning, Vision: &vision},
		{ID: "plain", Name: "Plain", Reasoning: &plain},
		{ID: "  "},
	})
	if err != nil {
		t.Fatalf("WriteCodexCatalog() error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var catalog struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(content, &catalog); err != nil {
		t.Fatalf("decode catalog: %v\n%s", err, content)
	}
	if len(catalog.Models) != 2 {
		t.Fatalf("models = %d, want the two named rows", len(catalog.Models))
	}
	if catalog.Models[0]["slug"] != "thinker" || catalog.Models[0]["context_window"] != float64(window) {
		t.Fatalf("thinker = %#v", catalog.Models[0])
	}
	levels, _ := catalog.Models[0]["supported_reasoning_levels"].([]any)
	if len(levels) != 4 {
		t.Fatalf("reasoning levels = %#v", catalog.Models[0]["supported_reasoning_levels"])
	}
	modalities, _ := catalog.Models[0]["input_modalities"].([]any)
	if len(modalities) != 2 || modalities[1] != "image" {
		t.Fatalf("modalities = %#v", modalities)
	}
	plainLevels, ok := catalog.Models[1]["supported_reasoning_levels"].([]any)
	if !ok || len(plainLevels) != 0 {
		t.Fatalf("plain model reasoning = %#v, want an empty list Codex can parse", catalog.Models[1]["supported_reasoning_levels"])
	}
}

const nativeCache = `{"models":[
 {"slug":"gpt-6-luna","display_name":"GPT-6-Luna","description":"Our fast model.","visibility":"list","priority":3,
  "base_instructions":"You are Codex, a coding agent based on GPT-6. Work with the user.",
  "context_window":272000,"model_messages":{"persistent_instructions":"x"},"service_tiers":[{"id":"priority"}],
  "supports_experimental_context":true,"use_responses_lite":true,"web_search_tool_type":"text",
  "supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"},{"effort":"max"},{"effort":"ultra"}],
  "default_reasoning_level":"low","template_only_field":"kept"},
 {"slug":"anthropic/claude-opus-4-6","display_name":"leftover","description":"Routed via another tool.","visibility":"list"}
]}`

func writeCatalogWithCache(t *testing.T, cache string, models []codingclients.ModelRef) []map[string]any {
	t.Helper()
	dir := t.TempDir()
	if cache != "" {
		if err := os.WriteFile(filepath.Join(dir, "models_cache.json"), []byte(cache), 0o600); err != nil {
			t.Fatalf("write the models cache: %v", err)
		}
	}
	path := codingclients.CodexCatalogPath(filepath.Join(dir, "config.toml"))
	if err := codingclients.WriteCodexCatalog(path, models); err != nil {
		t.Fatalf("WriteCodexCatalog() error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var file struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(content, &file); err != nil {
		t.Fatalf("decode catalog: %v\n%s", err, content)
	}
	return file.Models
}

// TestWriteCodexCatalogClonesANativeTemplate is what makes a routed model
// read like a Codex one: the row keeps the native shape, names its
// connection, and drops what is only true of OpenAI's own models.
func TestWriteCodexCatalogClonesANativeTemplate(t *testing.T) {
	reasoning := true
	window := int64(1000000)
	rows := writeCatalogWithCache(t, nativeCache, []codingclients.ModelRef{{
		ID: "relo-opencode-go-deepseek-flash", Name: "DeepSeek Flash", Upstream: "deepseek-flash",
		Connection: "OpenCode Go", ContextWindow: &window, Reasoning: &reasoning,
	}})
	row := rows[0]
	if row["slug"] != "relo-opencode-go-deepseek-flash" || row["display_name"] != "DeepSeek Flash 1M on OpenCode Go" {
		t.Fatalf("names = %v / %v, want the slug and the model by its connection", row["slug"], row["display_name"])
	}
	if row["description"] != "Routed via Relo" {
		t.Fatalf("description = %v", row["description"])
	}
	if row["template_only_field"] != "kept" {
		t.Fatalf("row = %v, want the template's own fields kept", row)
	}
	for _, field := range []string{"model_messages", "service_tiers", "use_responses_lite", "supports_experimental_context", "web_search_tool_type"} {
		if _, found := row[field]; found {
			t.Fatalf("row still carries %s: %v", field, row)
		}
	}
	instructions, _ := row["base_instructions"].(string)
	if strings.Contains(instructions, "GPT-6") || !strings.Contains(instructions, "powered by the deepseek-flash") {
		t.Fatalf("base_instructions = %q, want the model's own identity", instructions)
	}
	if row["context_window"] != float64(window) || row["default_reasoning_level"] != "medium" {
		t.Fatalf("row = %v, want the window and a medium default", row)
	}
	if levels, _ := row["supported_reasoning_levels"].([]any); len(levels) != 4 {
		t.Fatalf("levels = %v, want low through xhigh", row["supported_reasoning_levels"])
	}
}

// TestWriteCodexCatalogSetsInstructionsWhenTheTemplateUsesModelMessages is
// the row Codex refuses without one of those fields: newer native templates
// keep the prompt under model_messages, which a routed clone must drop.
func TestWriteCodexCatalogSetsInstructionsWhenTheTemplateUsesModelMessages(t *testing.T) {
	cache := `{"models":[
 {"slug":"gpt-6-luna","display_name":"GPT-6-Luna","visibility":"list",
  "model_messages":{"instructions_template":"You are Codex, an agent based on GPT-6. Work with the user."}}
]}`
	rows := writeCatalogWithCache(t, cache, []codingclients.ModelRef{{
		ID: "relo-claude-claude-fable-5", Name: "Claude Fable 5", Upstream: "claude-fable-5",
		Connection: "Claude",
	}})
	row := rows[0]
	if _, found := row["model_messages"]; found {
		t.Fatalf("row still carries model_messages: %v", row)
	}
	instructions, _ := row["base_instructions"].(string)
	if instructions == "" || strings.Contains(instructions, "GPT-6") {
		t.Fatalf("base_instructions = %q, want a routed identity and no GPT claim", instructions)
	}
	if !strings.Contains(instructions, "powered by the claude-fable-5") {
		t.Fatalf("base_instructions = %q, want the model's own identity", instructions)
	}
	for i, got := range writeCatalogWithCache(t, cache, []codingclients.ModelRef{
		{ID: "relo-claude-claude-fable-5", Name: "Fable", Connection: "Claude"},
		{ID: "reloc-combo", Name: "Combo", Connection: catalog.RouteLabelPrefix},
		{ID: "relo-opencode-go-flash", Name: "Flash", Connection: "OpenCode Go"},
	}) {
		text, _ := got["base_instructions"].(string)
		if strings.TrimSpace(text) == "" {
			t.Fatalf("model %d %v missing base_instructions, which is what Codex refuses", i, got["slug"])
		}
		if got["slug"] == "reloc-combo" && text != "You are a coding agent, powered by the Relo Combo." {
			t.Fatalf("base_instructions = %q, want the Relo group identity", text)
		}
		if _, found := got["model_messages"]; found {
			t.Fatalf("model %d still carries model_messages", i)
		}
	}
}

// TestWriteCodexCatalogKeepsAChatGPTModelNative keeps what Codex already says
// about a model the ChatGPT sign-in serves.
func TestWriteCodexCatalogKeepsAChatGPTModelNative(t *testing.T) {
	rows := writeCatalogWithCache(t, nativeCache, []codingclients.ModelRef{{
		ID: "relo-openai-codex-gpt-6-luna", Name: "GPT-6-Luna", Upstream: "gpt-6-luna",
		Connection: "ChatGPT", ChatGPT: true,
	}})
	row := rows[0]
	if row["slug"] != "relo-openai-codex-gpt-6-luna" || row["display_name"] != "GPT-6-Luna on ChatGPT" {
		t.Fatalf("names = %v / %v", row["slug"], row["display_name"])
	}
	if row["description"] != "Our fast model." {
		t.Fatalf("description = %v, want the native one", row["description"])
	}
	if _, found := row["model_messages"]; !found {
		t.Fatalf("row = %v, want the native fields kept", row)
	}
	if levels, _ := row["supported_reasoning_levels"].([]any); len(levels) != 6 {
		t.Fatalf("levels = %v, want the native ladder", row["supported_reasoning_levels"])
	}
}

// TestWriteCodexCatalogFallsBackWithoutACache covers a machine that has never
// run Codex against its own service.
func TestWriteCodexCatalogFallsBackWithoutACache(t *testing.T) {
	rows := writeCatalogWithCache(t, "", []codingclients.ModelRef{{ID: "m", Name: "M", Connection: "Conn"}})
	if rows[0]["slug"] != "m" || rows[0]["shell_type"] != "unified_exec" || rows[0]["display_name"] != "M on Conn" {
		t.Fatalf("row = %v, want the minimal row", rows[0])
	}
}

// TestMergeCodexConfigDropsAModelTheCatalogDoesNotList stops a leftover model
// from another tool being sent to Relo, while one Relo lists stays.
func TestMergeCodexConfigDropsAModelTheCatalogDoesNotList(t *testing.T) {
	dir := t.TempDir()
	catalog := codingclients.CodexCatalogPath(filepath.Join(dir, "config.toml"))
	if err := codingclients.WriteCodexCatalog(catalog, []codingclients.ModelRef{{ID: "relo-a-b", Name: "B"}}); err != nil {
		t.Fatalf("WriteCodexCatalog() error = %v", err)
	}

	stale := "model = \"anthropic/claude-opus-4-6\"\napproval_policy = \"never\"\n"
	merged, err := codingclients.MergeCodexConfig(stale, baseURL, catalog, helperFile, true)
	if err != nil {
		t.Fatalf("MergeCodexConfig() error = %v", err)
	}
	if strings.Contains(merged, "claude-opus-4-6") || !strings.Contains(merged, "approval_policy") {
		t.Fatalf("merged file = %s, want the stale model gone and the rest kept", merged)
	}

	listed := "model = \"relo-a-b\" # mine\n"
	merged, err = codingclients.MergeCodexConfig(listed, baseURL, catalog, helperFile, true)
	if err != nil {
		t.Fatalf("MergeCodexConfig() error = %v", err)
	}
	if !strings.Contains(merged, "model = \"relo-a-b\"") {
		t.Fatalf("merged file = %s, want the listed model kept", merged)
	}
}

func TestSyncCodexCacheKeepsNativesAndDropsNamespacedRows(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "models_cache.json")
	if err := os.WriteFile(cache, []byte(`{"fetched_at":"2026-01-01T00:00:00Z","client_version":"0.159.0","models":[
		{"slug":"gpt-6-luna","display_name":"Luna","description":"native"},
		{"slug":"opencode-go/deepseek-flash","display_name":"old","description":"Routed via opencodex"},
		{"slug":"relo-old-flash","display_name":"stale"}
	]}`), 0o600); err != nil {
		t.Fatalf("write the cache: %v", err)
	}

	if err := codingclients.SyncCodexCache(dir, []codingclients.ModelRef{
		{ID: "relo-opencode-go-flash", Name: "Flash", Connection: "OpenCode Go"},
	}); err != nil {
		t.Fatalf("SyncCodexCache() error = %v", err)
	}

	content, err := os.ReadFile(cache)
	if err != nil {
		t.Fatalf("read the cache: %v", err)
	}
	var file struct {
		FetchedAt string           `json:"fetched_at"`
		Version   string           `json:"client_version"`
		Models    []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(content, &file); err != nil {
		t.Fatalf("decode the cache: %v", err)
	}
	if file.FetchedAt != "2000-01-01T00:00:00Z" || file.Version != "0.0.0" {
		t.Fatalf("stamp = %s / %s, want the cache marked stale", file.FetchedAt, file.Version)
	}
	slugs := make([]string, 0, len(file.Models))
	for _, row := range file.Models {
		slug, _ := row["slug"].(string)
		slugs = append(slugs, slug)
	}
	joined := strings.Join(slugs, ",")
	if !strings.Contains(joined, "gpt-6-luna") || !strings.Contains(joined, "relo-opencode-go-flash") {
		t.Fatalf("slugs = %s, want the native row and Relo's row", joined)
	}
	if strings.Contains(joined, "opencode-go/") || strings.Contains(joined, "relo-old-flash") {
		t.Fatalf("slugs = %s, want the namespaced and stale Relo rows gone", joined)
	}

	if err := codingclients.StripReloFromCodexCache(dir); err != nil {
		t.Fatalf("StripReloFromCodexCache() error = %v", err)
	}
	content, err = os.ReadFile(cache)
	if err != nil {
		t.Fatalf("read the cache after strip: %v", err)
	}
	if err := json.Unmarshal(content, &file); err != nil {
		t.Fatalf("decode the stripped cache: %v", err)
	}
	if len(file.Models) != 1 || file.Models[0]["slug"] != "gpt-6-luna" {
		t.Fatalf("stripped models = %v, want only the native row", file.Models)
	}
}

func TestWriteCodexCatalogUsesAStatedThinkLadder(t *testing.T) {
	reasoning := true
	rows := writeCatalogWithCache(t, "", []codingclients.ModelRef{{
		ID: "relo-xiaomi-mimo", Name: "MiMo", Connection: "Xiaomi", Reasoning: &reasoning,
		ReasoningEfforts: []string{"low", "medium", "high"},
	}})
	row := rows[0]
	if row["default_reasoning_level"] != "medium" {
		t.Fatalf("default = %v, want medium", row["default_reasoning_level"])
	}
	levels, _ := row["supported_reasoning_levels"].([]any)
	if len(levels) != 3 {
		t.Fatalf("levels = %v, want low, medium, and high", levels)
	}
	for i, effort := range []string{"low", "medium", "high"} {
		if levels[i].(map[string]any)["effort"] != effort {
			t.Fatalf("levels = %v, want %s at %d", levels, effort, i)
		}
	}
}

func TestModelsDigestChangesWhenTheListChanges(t *testing.T) {
	first := codingclients.ModelsDigest([]codingclients.ModelRef{{ID: "a", Name: "A", Connection: "One"}})
	second := codingclients.ModelsDigest([]codingclients.ModelRef{{ID: "a", Name: "A", Connection: "Two"}})
	if first == "" || first == second {
		t.Fatalf("digests = %q / %q, want a change when the connection changes", first, second)
	}
	same := codingclients.ModelsDigest([]codingclients.ModelRef{{ID: "a", Name: "A", Connection: "One"}})
	if first != same {
		t.Fatalf("digest = %q, want a stable fingerprint", same)
	}
	narrow := codingclients.ModelsDigest([]codingclients.ModelRef{{
		ID: "a", Name: "A", Connection: "One", ReasoningEfforts: []string{"low", "medium", "high"},
	}})
	if narrow == first {
		t.Fatalf("digest = %q, want a change when the think ladder changes", narrow)
	}
}
