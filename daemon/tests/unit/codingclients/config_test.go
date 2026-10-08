package codingclients_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"gopkg.in/yaml.v3"
)

func sampleModels() []codingclients.ModelRef {
	window := int64(128000)
	output := int64(4096)
	vision := true
	return []codingclients.ModelRef{
		{ID: "relo-openai-gpt-4o", Name: "GPT-4o", ContextWindow: &window, MaxOutput: &output, Vision: &vision},
		{ID: "plain", Name: ""},
	}
}

func TestMergeOpenCodeKeepsSiblingKeys(t *testing.T) {
	existing := "{\"theme\":\"dark\",\"provider\":{\"other\":{\"keep\":true}}}\n"
	merged, err := codingclients.MergeOpenCode(existing, baseURL, sampleModels())
	if err != nil {
		t.Fatalf("MergeOpenCode() error = %v", err)
	}
	if !strings.Contains(merged, "\"theme\":\"dark\"") || !strings.Contains(merged, "\"keep\":true") {
		t.Fatalf("merged file lost the operator's keys:\n%s", merged)
	}
	if !strings.Contains(merged, "{env:RELO_OPENCODE_API_KEY}") {
		t.Fatalf("merged file = %s, want the environment reference", merged)
	}
	if strings.Contains(merged, "rlo_ak_") {
		t.Fatalf("merged file carries a secret:\n%s", merged)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(merged), &document); err != nil {
		t.Fatalf("merged file does not parse: %v\n%s", err, merged)
	}
	again, err := codingclients.MergeOpenCode(merged, baseURL, sampleModels())
	if err != nil {
		t.Fatalf("MergeOpenCode() again error = %v", err)
	}
	if again != merged {
		t.Fatalf("a second merge changed the file:\n%s\n---\n%s", merged, again)
	}
	stripped, err := codingclients.StripOpenCode(merged)
	if err != nil {
		t.Fatalf("StripOpenCode() error = %v", err)
	}
	if strings.Contains(stripped, "relo") {
		t.Fatalf("stripped file still names Relo:\n%s", stripped)
	}
	if !strings.Contains(stripped, "\"theme\":\"dark\"") || !strings.Contains(stripped, "\"keep\":true") {
		t.Fatalf("stripped file lost the operator's keys:\n%s", stripped)
	}
}

func TestProviderPointsAtIgnoresTheRestOfTheFile(t *testing.T) {
	models := sampleModels()
	openCode, err := codingclients.MergeOpenCode("{\"theme\":\"dark\"}\n", baseURL, models)
	if err != nil {
		t.Fatalf("MergeOpenCode() error = %v", err)
	}
	openCode = strings.Replace(openCode, "\"theme\":\"dark\"", "\"theme\":\"light\"", 1)
	if !codingclients.ProviderPointsAt(codingclients.ClientOpenCode, openCode, baseURL, models) {
		t.Fatal("ProviderPointsAt(opencode) = false after an edit outside the provider")
	}
	if codingclients.ProviderPointsAt(codingclients.ClientOpenCode, "{\"theme\":\"dark\"}\n", baseURL, models) {
		t.Fatal("ProviderPointsAt(opencode) = true when the provider is missing")
	}

	hermes, err := codingclients.MergeHermes("model: own\n", baseURL, models)
	if err != nil {
		t.Fatalf("MergeHermes() error = %v", err)
	}
	hermes = strings.Replace(hermes, "model: own", "model: other", 1)
	if !codingclients.ProviderPointsAt(codingclients.ClientHermes, hermes, baseURL, models) {
		t.Fatalf("ProviderPointsAt(hermes) = false after an edit outside the block:\n%s", hermes)
	}
	if codingclients.ProviderPointsAt(codingclients.ClientHermes, "model: own\n", baseURL, models) {
		t.Fatal("ProviderPointsAt(hermes) = true when the block is missing")
	}

	openClaw, err := codingclients.MergeOpenClaw("{\"wizard\":true}\n", baseURL, models)
	if err != nil {
		t.Fatalf("MergeOpenClaw() error = %v", err)
	}
	openClaw = strings.Replace(openClaw, "\"wizard\":true", "\"wizard\":false", 1)
	if !codingclients.ProviderPointsAt(codingclients.ClientOpenClaw, openClaw, baseURL, models) {
		t.Fatal("ProviderPointsAt(openclaw) = false after an edit outside the provider")
	}
	broken := strings.Replace(openClaw, baseURL+"/v1", "http://example.test/v1", 1)
	if codingclients.ProviderPointsAt(codingclients.ClientOpenClaw, broken, baseURL, models) {
		t.Fatal("ProviderPointsAt(openclaw) = true after the address changed")
	}
}

func TestMergeOpenCodeRefusesAnUnreadableFile(t *testing.T) {
	if _, err := codingclients.MergeOpenCode("{", baseURL, nil); !errors.Is(err, codingclients.ErrUnrecognisedFile) {
		t.Fatalf("MergeOpenCode() error = %v, want %v", err, codingclients.ErrUnrecognisedFile)
	}
	if _, err := codingclients.MergeOpenCode("", "", nil); !errors.Is(err, codingclients.ErrUnrecognisedFile) {
		t.Fatalf("MergeOpenCode() with no address error = %v, want %v", err, codingclients.ErrUnrecognisedFile)
	}
	present, err := codingclients.HasReloProvider(codingclients.ClientOpenCode, "{\"provider\":{\"relo\":{}}}")
	if err != nil || !present {
		t.Fatalf("HasReloProvider() = %v, %v, want a relo provider", present, err)
	}
}

func TestMergeOpenCodeIncludesOnlyKnownLimits(t *testing.T) {
	merged, err := codingclients.MergeOpenCode("", baseURL, sampleModels())
	if err != nil {
		t.Fatalf("MergeOpenCode() error = %v", err)
	}
	if !strings.Contains(merged, "\"context\":128000") || !strings.Contains(merged, "\"output\":4096") {
		t.Fatalf("merged file = %s, want the catalog limits", merged)
	}
	if !strings.Contains(merged, "\"plain\":{}") {
		t.Fatalf("merged file = %s, want a model with no invented limit", merged)
	}
}

func TestMergeOpenCodeDropsAStalePluralBlock(t *testing.T) {
	existing := "{\"provider\":{\"other\":true},\"providers\":{\"relo\":{\"stale\":true}}}\n"
	merged, err := codingclients.MergeOpenCode(existing, baseURL, sampleModels())
	if err != nil {
		t.Fatalf("MergeOpenCode() error = %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(merged), &document); err != nil {
		t.Fatalf("merged file does not parse: %v\n%s", err, merged)
	}
	if _, ok := document["providers"]; ok {
		t.Fatalf("merged file = %s, want the stale providers block gone", merged)
	}
	if !codingclients.ProviderPointsAt(codingclients.ClientOpenCode, merged, baseURL, sampleModels()) {
		t.Fatalf("ProviderPointsAt() = false after merging:\n%s", merged)
	}
	stripped, err := codingclients.StripProvider(codingclients.ClientOpenCode, existing)
	if err != nil {
		t.Fatalf("StripProvider() error = %v", err)
	}
	if strings.Contains(stripped, "relo") {
		t.Fatalf("stripped file still names Relo:\n%s", stripped)
	}
}

func TestMergeHermesKeepsTheOperatorsProviders(t *testing.T) {
	existing := "providers:\n  openai:\n    api: https://example.test/v1\n"
	merged, err := codingclients.MergeHermes(existing, baseURL, sampleModels())
	if err != nil {
		t.Fatalf("MergeHermes() error = %v", err)
	}
	if !strings.Contains(merged, "openai:") || !strings.Contains(merged, "https://example.test/v1") {
		t.Fatalf("merged file lost the operator's provider:\n%s", merged)
	}
	if !strings.Contains(merged, "${RELO_HERMES_API_KEY}") || !strings.Contains(merged, "supports_vision: true") {
		t.Fatalf("merged file = %s, want the environment reference and the known capability", merged)
	}
	var document map[string]any
	if err := yaml.Unmarshal([]byte(merged), &document); err != nil {
		t.Fatalf("merged file does not parse: %v\n%s", err, merged)
	}
	again, err := codingclients.MergeHermes(merged, baseURL, sampleModels())
	if err != nil {
		t.Fatalf("MergeHermes() again error = %v", err)
	}
	if again != merged {
		t.Fatalf("a second merge changed the file:\n%s\n---\n%s", merged, again)
	}
	foreign, err := codingclients.HasReloProvider(codingclients.ClientHermes, existing+"  relo:\n    api: old\n")
	if err != nil || !foreign {
		t.Fatalf("HasReloProvider() = %v, %v, want the operator's relo provider", foreign, err)
	}
	owned, err := codingclients.HasReloProvider(codingclients.ClientHermes, merged)
	if err != nil || owned {
		t.Fatalf("HasReloProvider() on Relo's own fence = %v, %v, want absent", owned, err)
	}
	stripped, err := codingclients.StripHermes(merged)
	if err != nil {
		t.Fatalf("StripHermes() error = %v", err)
	}
	if strings.Contains(stripped, "relo:") || strings.Contains(stripped, "RELO_HERMES_API_KEY") {
		t.Fatalf("stripped file still names Relo:\n%s", stripped)
	}
	if !strings.Contains(stripped, "openai:") {
		t.Fatalf("stripped file lost the operator's provider:\n%s", stripped)
	}
}

func TestMergeHermesRefusesAnUnreadableFile(t *testing.T) {
	if _, err := codingclients.MergeHermes("providers: [\n", baseURL, nil); !errors.Is(err, codingclients.ErrUnrecognisedFile) {
		t.Fatalf("MergeHermes() error = %v, want %v", err, codingclients.ErrUnrecognisedFile)
	}
	if _, err := codingclients.HasReloProvider(codingclients.ClientHermes, "providers: {openai: {}}\n"); !errors.Is(err, codingclients.ErrUnrecognisedFile) {
		t.Fatalf("HasReloProvider() error = %v, want an inline providers map refused", err)
	}
}

func TestMergeOpenClawKeepsSiblingKeysAndAnExistingMode(t *testing.T) {
	existing := "{\"theme\":\"dark\",\"models\":{\"mode\":\"replace\",\"providers\":{\"other\":{\"baseUrl\":\"https://example.test\"}}}}\n"
	merged, err := codingclients.MergeOpenClaw(existing, baseURL, sampleModels())
	if err != nil {
		t.Fatalf("MergeOpenClaw() error = %v", err)
	}
	if !strings.Contains(merged, "\"theme\":\"dark\"") || !strings.Contains(merged, "\"mode\":\"replace\"") {
		t.Fatalf("merged file lost the operator's keys:\n%s", merged)
	}
	if !strings.Contains(merged, "${RELO_OPENCLAW_API_KEY}") || !strings.Contains(merged, "\"contextWindow\":128000") {
		t.Fatalf("merged file = %s, want the environment reference and the known window", merged)
	}
	if strings.Contains(merged, "\"maxTokens\"") {
		t.Fatalf("merged file invented a max output:\n%s", merged)
	}
	again, err := codingclients.MergeOpenClaw(merged, baseURL, sampleModels())
	if err != nil {
		t.Fatalf("MergeOpenClaw() again error = %v", err)
	}
	if again != merged {
		t.Fatalf("a second merge changed the file:\n%s\n---\n%s", merged, again)
	}
	stripped, err := codingclients.StripOpenClaw(merged)
	if err != nil {
		t.Fatalf("StripOpenClaw() error = %v", err)
	}
	if strings.Contains(stripped, "RELO_OPENCLAW_API_KEY") || strings.Contains(stripped, "\"relo\"") {
		t.Fatalf("stripped file still names Relo:\n%s", stripped)
	}
	if !strings.Contains(stripped, "\"theme\":\"dark\"") || !strings.Contains(stripped, "\"mode\":\"replace\"") {
		t.Fatalf("stripped file lost the operator's keys:\n%s", stripped)
	}
}

func TestClientConfigsTranslateCapabilities(t *testing.T) {
	on, off := true, false
	models := []codingclients.ModelRef{
		{ID: "thinker", Name: "Thinker", Tools: &on, Reasoning: &on, Vision: &on},
		{ID: "plain", Name: "Plain", Tools: &off, Reasoning: &off, Vision: &off},
		{ID: "unknown"},
	}

	merged, err := codingclients.MergeOpenCode("", baseURL, models)
	if err != nil {
		t.Fatalf("MergeOpenCode() error = %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(merged), &document); err != nil {
		t.Fatalf("merged file does not parse: %v\n%s", err, merged)
	}
	v1 := openCodeModelsOf(t, document, "provider")
	if _, ok := document["providers"]; ok {
		t.Fatalf("merged file = %s, want only the singular provider key", merged)
	}
	thinkerV1 := v1["thinker"].(map[string]any)
	if _, ok := thinkerV1["variants"]; ok || thinkerV1["capabilities"] != nil {
		t.Fatalf("thinker = %v, want attachment and modalities without variants", thinkerV1)
	}
	if thinkerV1["attachment"] != true {
		t.Fatalf("attachment = %v, want true", thinkerV1["attachment"])
	}
	modalities := thinkerV1["modalities"].(map[string]any)
	input := modalities["input"].([]any)
	if len(input) != 2 || input[0] != "text" || input[1] != "image" {
		t.Fatalf("input = %v, want text and image", input)
	}
	plain := v1["plain"].(map[string]any)
	if _, ok := plain["variants"]; ok || plain["capabilities"] != nil {
		t.Fatalf("plain = %v, want attachment and modalities without variants", plain)
	}
	if plain["attachment"] != false {
		t.Fatalf("plain attachment = %v, want false", plain["attachment"])
	}
	unknown := v1["unknown"].(map[string]any)
	if len(unknown) != 0 {
		t.Fatalf("unknown = %v, want no invented capabilities", unknown)
	}

	claw, err := codingclients.MergeOpenClaw("", baseURL, models)
	if err != nil {
		t.Fatalf("MergeOpenClaw() error = %v", err)
	}
	var clawDoc map[string]any
	if err := json.Unmarshal([]byte(claw), &clawDoc); err != nil {
		t.Fatalf("openclaw file does not parse: %v\n%s", err, claw)
	}
	rows := clawDoc["models"].(map[string]any)["providers"].(map[string]any)["relo"].(map[string]any)["models"].([]any)
	thinker := clawModel(t, rows, "thinker")
	if thinker["reasoning"] != true {
		t.Fatalf("openclaw thinker = %v, want reasoning on", thinker)
	}
	compat := thinker["compat"].(map[string]any)
	efforts := compat["supportedReasoningEfforts"].([]any)
	if len(efforts) != 5 || compat["thinkingFormat"] != "openai" || compat["supportsTools"] != true {
		t.Fatalf("compat = %v, want tools and the five efforts", compat)
	}
	clawPlain := clawModel(t, rows, "plain")
	if _, ok := clawPlain["reasoning"]; ok {
		t.Fatalf("openclaw plain = %v, want reasoning omitted", clawPlain)
	}
	if _, ok := clawPlain["compat"].(map[string]any)["supportedReasoningEfforts"]; ok || clawPlain["compat"].(map[string]any)["supportsTools"] != false {
		t.Fatalf("openclaw plain = %v, want tools off and no efforts", clawPlain)
	}

	hermes, err := codingclients.MergeHermes("", baseURL, models)
	if err != nil {
		t.Fatalf("MergeHermes() error = %v", err)
	}
	var hermesDoc map[string]any
	if err := yaml.Unmarshal([]byte(hermes), &hermesDoc); err != nil {
		t.Fatalf("hermes file does not parse: %v\n%s", err, hermes)
	}
	hermesModels := hermesDoc["providers"].(map[string]any)["relo"].(map[string]any)["models"].(map[string]any)
	if hermesModels["thinker"].(map[string]any)["supports_vision"] != true {
		t.Fatalf("hermes thinker = %v, want vision on", hermesModels["thinker"])
	}
	if hermesModels["plain"].(map[string]any)["supports_vision"] != false {
		t.Fatalf("hermes plain = %v, want vision off", hermesModels["plain"])
	}
	if _, ok := hermesModels["unknown"].(map[string]any)["supports_vision"]; ok {
		t.Fatalf("hermes unknown = %v, want vision omitted", hermesModels["unknown"])
	}
	if strings.Contains(hermes, "reasoning_effort") || strings.Contains(hermes, "reasoning_overrides") {
		t.Fatalf("hermes invented a thinking budget:\n%s", hermes)
	}
}

func clawModel(t *testing.T, rows []any, id string) map[string]any {
	t.Helper()
	for _, row := range rows {
		model := row.(map[string]any)
		if model["id"] == id {
			return model
		}
	}
	t.Fatalf("openclaw model %s is missing", id)
	return nil
}

func openCodeModelsOf(t *testing.T, document map[string]any, key string) map[string]any {
	t.Helper()
	block := document[key].(map[string]any)["relo"].(map[string]any)
	return block["models"].(map[string]any)
}

func TestConfigPathsRefuseARelativeOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "relative/config")
	t.Setenv("HERMES_HOME", "relative/hermes")
	t.Setenv("OPENCLAW_CONFIG_PATH", "relative/openclaw.json")
	paths := codingclients.Paths{Home: home}
	if _, err := paths.OpenCodeConfig(); !errors.Is(err, codingclients.ErrRelativePath) {
		t.Fatalf("OpenCodeConfig() error = %v, want %v", err, codingclients.ErrRelativePath)
	}
	if _, err := paths.HermesConfig(); !errors.Is(err, codingclients.ErrRelativePath) {
		t.Fatalf("HermesConfig() error = %v, want %v", err, codingclients.ErrRelativePath)
	}
	if _, err := paths.OpenClawConfig(); !errors.Is(err, codingclients.ErrRelativePath) {
		t.Fatalf("OpenClawConfig() error = %v, want %v", err, codingclients.ErrRelativePath)
	}
}

func TestConfigPathsHonourAnAbsoluteOverride(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, "xdg")
	hermes := filepath.Join(home, "hermes-home")
	claw := filepath.Join(home, "claw.json")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("HERMES_HOME", hermes)
	t.Setenv("OPENCLAW_CONFIG_PATH", claw)
	paths := codingclients.Paths{Home: home}
	if got, err := paths.OpenCodeConfig(); err != nil || got != filepath.Join(xdg, "opencode", "opencode.json") {
		t.Fatalf("OpenCodeConfig() = %q, %v", got, err)
	}
	if got, err := paths.HermesConfig(); err != nil || got != filepath.Join(hermes, "config.yaml") {
		t.Fatalf("HermesConfig() = %q, %v", got, err)
	}
	if got, err := paths.OpenClawConfig(); err != nil || got != claw {
		t.Fatalf("OpenClawConfig() = %q, %v", got, err)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HERMES_HOME", "")
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	if got, err := paths.OpenCodeConfig(); err != nil || got != filepath.Join(home, ".config", "opencode", "opencode.json") {
		t.Fatalf("OpenCodeConfig() default = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("resolving a path created a directory: %v", err)
	}
}

func TestClientConfigsUseAStatedThinkLadder(t *testing.T) {
	on := true
	models := []codingclients.ModelRef{
		{ID: "mimo", Name: "MiMo", Reasoning: &on, ReasoningEfforts: []string{"low", "medium", "high"}},
		{ID: "thinker", Name: "Thinker", Reasoning: &on},
	}

	merged, err := codingclients.MergeOpenCode("", baseURL, models)
	if err != nil {
		t.Fatalf("MergeOpenCode() error = %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(merged), &document); err != nil {
		t.Fatalf("merged file does not parse: %v\n%s", err, merged)
	}
	v1 := openCodeModelsOf(t, document, "provider")
	for _, id := range []string{"mimo", "thinker"} {
		if _, ok := v1[id].(map[string]any)["variants"]; ok {
			t.Fatalf("opencode %s carries variants", id)
		}
	}

	claw, err := codingclients.MergeOpenClaw("", baseURL, models)
	if err != nil {
		t.Fatalf("MergeOpenClaw() error = %v", err)
	}
	var clawDoc map[string]any
	if err := json.Unmarshal([]byte(claw), &clawDoc); err != nil {
		t.Fatalf("openclaw file does not parse: %v\n%s", err, claw)
	}
	rows := clawDoc["models"].(map[string]any)["providers"].(map[string]any)["relo"].(map[string]any)["models"].([]any)
	mimo := clawModel(t, rows, "mimo")["compat"].(map[string]any)["supportedReasoningEfforts"].([]any)
	if len(mimo) != 3 || mimo[0] != "low" || mimo[1] != "medium" || mimo[2] != "high" {
		t.Fatalf("openclaw mimo efforts = %v, want low, medium, and high", mimo)
	}
	thinker := clawModel(t, rows, "thinker")["compat"].(map[string]any)["supportedReasoningEfforts"].([]any)
	if len(thinker) != 5 {
		t.Fatalf("openclaw thinker efforts = %v, want the shared ladder", thinker)
	}
}
