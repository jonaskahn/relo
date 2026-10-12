package codingclients_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

func TestDSHMergeKeepsTheOperatorPatch(t *testing.T) {
	base := "http://127.0.0.1:10201"
	models := []codingclients.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o"}}
	existing := strings.Join([]string{
		"# the operator's own patch",
		"- id: other-patch",
		"  config:",
		"    enabled: true",
		"- id: llm-pi-ai",
		"  config:",
		"    providers:",
		"      openai:",
		"        baseURL: http://example",
		"      relo:",
		"        baseUrl: http://stale",
		"",
	}, "\n")

	merged, err := codingclients.MergeKind(codingclients.FileDSH, existing, base, models)
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	for _, kept := range []string{"# the operator's own patch", "- id: other-patch", "enabled: true", "openai:", "http://example"} {
		if !strings.Contains(merged, kept) {
			t.Fatalf("merged = %s, want the operator content %q kept", merged, kept)
		}
	}
	if strings.Contains(merged, "http://stale") {
		t.Fatalf("merged = %s, want the stale relo entry replaced", merged)
	}
	if !codingclients.KindPointsAt(codingclients.FileDSH, merged, base, models) {
		t.Fatalf("merged file does not point at Relo:\n%s", merged)
	}
	if has, err := codingclients.KindHasRelo(codingclients.FileDSH, merged); err != nil || !has {
		t.Fatalf("KindHasRelo() = %v, %v, want the patch to name Relo", has, err)
	}
	if count := strings.Count(merged, "- id: llm-pi-ai"); count != 1 {
		t.Fatalf("merged carries the patch item %d times, want exactly one:\n%s", count, merged)
	}
}

func TestDSHMergeIsIdempotent(t *testing.T) {
	base := "http://127.0.0.1:10201"
	models := []codingclients.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o"}}

	first, err := codingclients.MergeKind(codingclients.FileDSH, "", base, models)
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	second, err := codingclients.MergeKind(codingclients.FileDSH, first, base, models)
	if err != nil {
		t.Fatalf("MergeKind() on its own output error = %v", err)
	}
	if first != second {
		t.Fatalf("merged twice differs:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestDSHPointsAtRefusesAChangedModel(t *testing.T) {
	base := "http://127.0.0.1:10201"
	models := []codingclients.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o"}}
	merged, err := codingclients.MergeKind(codingclients.FileDSH, "", base, models)
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	changed := []codingclients.ModelRef{{ID: "relo-openai-gpt-4o-mini", Name: "GPT-4o mini"}}
	if codingclients.KindPointsAt(codingclients.FileDSH, merged, base, changed) {
		t.Fatal("KindPointsAt() accepted a patch written for another model list")
	}
}

func TestDSHStripLeavesTheOperatorsPatch(t *testing.T) {
	existing := "- id: other-patch\n  config:\n    enabled: true\n- id: llm-pi-ai\n  config:\n    providers:\n      openai:\n        baseURL: http://example\n      relo:\n        baseURL: http://relo\n"
	stripped, err := codingclients.StripKind(codingclients.FileDSH, existing)
	if err != nil {
		t.Fatalf("StripKind() error = %v", err)
	}
	if strings.Contains(stripped, "relo") {
		t.Fatalf("stripped = %s, want no relo entry", stripped)
	}
	if !strings.Contains(stripped, "other-patch") || !strings.Contains(stripped, "openai") {
		t.Fatalf("stripped = %s, want the operator content kept", stripped)
	}
	if has, err := codingclients.KindHasRelo(codingclients.FileDSH, stripped); err != nil || has {
		t.Fatalf("KindHasRelo() = %v, %v, want no relo entry", has, err)
	}
}

func TestDSHCarriesLimitsEffortsAndTheEnvKey(t *testing.T) {
	window, output := int64(200000), int64(64000)
	vision, on, off, toggle := true, true, false, true
	cases := []struct {
		name   string
		model  codingclients.ModelRef
		want   []string
		absent []string
	}{
		{
			name:   "a plain model carries no effort ladder",
			model:  codingclients.ModelRef{ID: "relo-openai-gpt-4o", Name: "GPT-4o"},
			want:   []string{"reasoningEfforts: false", "input: [text]"},
			absent: []string{"reasoningEfforts:\n", "off:"},
		},
		{
			name:  "a vision model declares its input",
			model: codingclients.ModelRef{ID: "relo-openai-gpt-4o", Name: "GPT-4o", Vision: &vision},
			want:  []string{"input: [text, image]"},
		},
		{
			name: "a reasoning model carries its limits and ladder",
			model: codingclients.ModelRef{
				ID: "relo-openai-o3", Name: "o3", ContextWindow: &window, MaxOutput: &output,
				Reasoning: &on, ReasoningEfforts: []string{"low", "high"}, ReasoningToggle: &toggle,
			},
			want: []string{
				"contextWindow: 200000", "maxTokens: 64000",
				"reasoningEfforts:\n              off:\n              low: low\n              high: high\n",
			},
		},
		{
			name: "a reasoning model without a toggle offers no off level",
			model: codingclients.ModelRef{
				ID: "relo-openai-o3", Name: "o3", Reasoning: &on,
				ReasoningEfforts: []string{"low", "high"}, ReasoningToggle: &off,
			},
			want:   []string{"reasoningEfforts:\n              low: low\n              high: high\n"},
			absent: []string{"off:"},
		},
		{
			name:  "a deepseek model marks its thinking format",
			model: codingclients.ModelRef{ID: "relo-deepseek-chat", Name: "DeepSeek Chat"},
			want:  []string{"compat: { thinkingFormat: deepseek }"},
		},
	}
	base := "http://127.0.0.1:10201"
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			merged, err := codingclients.MergeKind(codingclients.FileDSH, "", base, []codingclients.ModelRef{testCase.model})
			if err != nil {
				t.Fatalf("MergeKind() error = %v", err)
			}
			for _, want := range testCase.want {
				if !strings.Contains(merged, want) {
					t.Fatalf("merged = %s, want %q", merged, want)
				}
			}
			for _, absent := range testCase.absent {
				if strings.Contains(merged, absent) {
					t.Fatalf("merged = %s, want no %q", merged, absent)
				}
			}
		})
	}
}

// dshEffortKeys reads back the thinking levels every model in a patch declares.
func dshEffortKeys(t *testing.T, patch string) []map[string]any {
	t.Helper()
	var document []map[string]any
	if err := yaml.Unmarshal([]byte(patch), &document); err != nil {
		t.Fatalf("the patch is not a YAML list: %v\n%s", err, patch)
	}
	var keys []map[string]any
	for _, item := range document {
		config, _ := item["config"].(map[string]any)
		providers, _ := config["providers"].(map[string]any)
		relo, _ := providers["relo"].(map[string]any)
		models, _ := relo["models"].([]any)
		for _, entry := range models {
			model, _ := entry.(map[string]any)
			efforts, _ := model["reasoningEfforts"].(map[string]any)
			if efforts != nil {
				keys = append(keys, efforts)
			}
		}
	}
	return keys
}

// TestDSHEffortKeysStayInsideTheHarnessVocabulary pins the harness's closed set
// of thinking levels. A key it does not know fails the whole llm-pi-ai entry at
// boot, which silently drops every Relo route rather than one model.
func TestDSHEffortKeysStayInsideTheHarnessVocabulary(t *testing.T) {
	known := map[string]bool{
		"off": true, "minimal": true, "low": true, "medium": true,
		"high": true, "xhigh": true, "max": true,
	}
	on := true
	models := []codingclients.ModelRef{
		{ID: "relo-orcarouter-tencent-hy3-free", Name: "Hy3", Reasoning: &on,
			ReasoningEfforts: []string{"none", "low", "high"}},
		{ID: "relo-openai-codex-gpt-6-luna", Name: "Luna", Reasoning: &on,
			ReasoningEfforts: []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}},
		{ID: "relo-kilo-free-hy4", Name: "Hy4", Reasoning: &on,
			ReasoningEfforts: []string{"none", "high"}},
	}
	merged, err := codingclients.MergeKind(codingclients.FileDSH, "", "http://127.0.0.1:10201", models)
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	efforts := dshEffortKeys(t, merged)
	if len(efforts) != len(models) {
		t.Fatalf("patch carries %d effort ladders, want one per model:\n%s", len(efforts), merged)
	}
	for _, ladder := range efforts {
		for key, wire := range ladder {
			if !known[key] {
				t.Fatalf("merged = %s, want no effort key %q the harness does not know", merged, key)
			}
			if wire == nil && key != "off" {
				t.Fatalf("merged = %s, want a wire value on %q", merged, key)
			}
		}
	}
	if !strings.Contains(merged, "off: none") {
		t.Fatalf("merged = %s, want Relo's none rung written under the harness's off", merged)
	}
}

// TestDSHDropsAnEffortTheHarnessCannotName keeps one unknown rung from taking
// the whole provider entry down at boot.
func TestDSHDropsAnEffortTheHarnessCannotName(t *testing.T) {
	on := true
	model := codingclients.ModelRef{ID: "relo-openai-o3", Name: "o3", Reasoning: &on,
		ReasoningEfforts: []string{"turbo", "low", "high"}}
	merged, err := codingclients.MergeKind(codingclients.FileDSH, "", "http://127.0.0.1:10201", []codingclients.ModelRef{model})
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	if strings.Contains(merged, "turbo") {
		t.Fatalf("merged = %s, want the unknown rung dropped", merged)
	}
	if !strings.Contains(merged, "low: low") || !strings.Contains(merged, "high: high") {
		t.Fatalf("merged = %s, want the known rungs kept", merged)
	}
}

// TestDSHNamesOneOffRungOnce covers a model that states both none and off.
func TestDSHNamesOneOffRungOnce(t *testing.T) {
	on := true
	model := codingclients.ModelRef{ID: "relo-openai-o3", Name: "o3", Reasoning: &on,
		ReasoningEfforts: []string{"none", "off", "low"}}
	merged, err := codingclients.MergeKind(codingclients.FileDSH, "", "http://127.0.0.1:10201", []codingclients.ModelRef{model})
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	if count := strings.Count(merged, "            off:"); count != 1 {
		t.Fatalf("merged carries the off rung %d times, want one:\n%s", count, merged)
	}
}

func TestDSHNeverWritesTheKey(t *testing.T) {
	base := "http://127.0.0.1:10201"
	models := []codingclients.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o"}}
	merged, err := codingclients.MergeKind(codingclients.FileDSH, "", base, models)
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	if !strings.Contains(merged, "apiKeyEnv: RELO_DSH_API_KEY") {
		t.Fatalf("merged = %s, want the env helper", merged)
	}
	if strings.Contains(merged, "apiKey:") {
		t.Fatalf("merged = %s, want no inline apiKey", merged)
	}
}

func TestDSHPatchParsesAsAList(t *testing.T) {
	base := "http://127.0.0.1:10201"
	merged, err := codingclients.MergeKind(codingclients.FileDSH, "", base, []codingclients.ModelRef{{ID: "relo-openai-gpt-4o"}})
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	var document []any
	if err := yaml.Unmarshal([]byte(merged), &document); err != nil {
		t.Fatalf("the patch is not a YAML list: %v\n%s", err, merged)
	}
	if len(document) != 1 {
		t.Fatalf("patch carries %d items, want exactly the one Relo writes", len(document))
	}
}

func TestDSHConfigTargetHonoursDSHHome(t *testing.T) {
	home := t.TempDir()
	desktop := filepath.Join(home, "profiles", "desktop")
	if err := os.MkdirAll(desktop, 0o755); err != nil {
		t.Fatalf("create the desktop profile: %v", err)
	}
	t.Setenv("DSH_HOME", home)
	paths := codingclients.Paths{Home: "/home/op"}
	targets, err := paths.ConfigTargets(codingclients.ClientDSH)
	if err != nil {
		t.Fatalf("ConfigTargets() error = %v", err)
	}
	wanted := map[string]string{
		codingclients.FileDSH:         filepath.Join(home, "profiles", "web", "cordis.patch.yml"),
		codingclients.FileDSHHeadless: filepath.Join(home, "profiles", "headless", "cordis.patch.yml"),
		codingclients.FileDSHDesktop:  filepath.Join(desktop, "cordis.patch.yml"),
	}
	if len(targets) != len(wanted) {
		t.Fatalf("ConfigTargets() = %d targets, want the web, headless, and desktop profiles", len(targets))
	}
	for _, target := range targets {
		if want, found := wanted[target.Kind]; !found || target.Path != want {
			t.Fatalf("ConfigTargets() target %q path = %q, want %q", target.Kind, target.Path, want)
		}
	}
}

// TestDSHSkipsADesktopProfileTheAppHasNotCreated covers the desktop app owning
// its profile outright: writing into a profile Relo created would leave the
// app refusing to start.
func TestDSHSkipsADesktopProfileTheAppHasNotCreated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DSH_HOME", home)
	targets, err := codingclients.Paths{Home: "/home/op"}.ConfigTargets(codingclients.ClientDSH)
	if err != nil {
		t.Fatalf("ConfigTargets() error = %v", err)
	}
	for _, target := range targets {
		if target.Kind == codingclients.FileDSHDesktop {
			t.Fatalf("ConfigTargets() wrote the desktop profile at %q, want it skipped", target.Path)
		}
	}
}

func TestDSHConfigTargetDefaultsToTheHomeDirectory(t *testing.T) {
	t.Setenv("DSH_HOME", "")
	paths := codingclients.Paths{Home: "/home/op"}
	targets, err := paths.ConfigTargets(codingclients.ClientDSH)
	if err != nil {
		t.Fatalf("ConfigTargets() error = %v", err)
	}
	want := []string{
		"/home/op/.dsh/profiles/web/cordis.patch.yml",
		"/home/op/.dsh/profiles/headless/cordis.patch.yml",
	}
	if len(targets) != len(want) {
		t.Fatalf("ConfigTargets() = %d targets, want %d", len(targets), len(want))
	}
	for index, target := range targets {
		if target.Path != want[index] {
			t.Fatalf("ConfigTargets() path = %q, want %q", target.Path, want[index])
		}
	}
}

// TestDSHDesktopWritesTheProviderWithoutADefaultModel pins the desktop app
// picking its model in its own picker, the way the web profile does.
func TestDSHDesktopWritesTheProviderWithoutADefaultModel(t *testing.T) {
	base := "http://127.0.0.1:10201"
	merged, err := codingclients.MergeKind(codingclients.FileDSHDesktop, "", base, dshHeadlessModels())
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	if strings.Contains(merged, "agent-default-model") {
		t.Fatalf("the desktop app picks its model itself, merged = %s", merged)
	}
	if !codingclients.KindPointsAt(codingclients.FileDSHDesktop, merged, base, dshHeadlessModels()) {
		t.Fatalf("merged file does not point at Relo:\n%s", merged)
	}
}

func dshHeadlessModels() []codingclients.ModelRef {
	return []codingclients.ModelRef{
		{ID: "relo-openai-gpt-4o-mini", Name: "GPT-4o mini"},
		{ID: "relo-openai-gpt-4o", Name: "GPT-4o"},
	}
}

func TestDSHHeadlessSetsTheDefaultModel(t *testing.T) {
	base := "http://127.0.0.1:10201"
	merged, err := codingclients.MergeKind(codingclients.FileDSHHeadless, "", base, dshHeadlessModels())
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	var document []map[string]any
	if err := yaml.Unmarshal([]byte(merged), &document); err != nil {
		t.Fatalf("the patch is not a YAML list: %v\n%s", err, merged)
	}
	if len(document) != 2 {
		t.Fatalf("patch carries %d items, want the provider and the default model:\n%s", len(document), merged)
	}
	defaults := document[1]
	config, _ := defaults["config"].(map[string]any)
	if defaults["id"] != "agent-default-model" || config["provider"] != "relo" || config["model"] != "relo-openai-gpt-4o" {
		t.Fatalf("default item = %v, want relo and the first listed model:\n%s", defaults, merged)
	}
	if !codingclients.KindPointsAt(codingclients.FileDSHHeadless, merged, base, dshHeadlessModels()) {
		t.Fatalf("merged file does not point at Relo:\n%s", merged)
	}
}

func TestDSHWebWritesNoDefaultModel(t *testing.T) {
	merged, err := codingclients.MergeKind(codingclients.FileDSH, "", "http://127.0.0.1:10201", dshHeadlessModels())
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	if strings.Contains(merged, "agent-default-model") {
		t.Fatalf("the web profile picks its model in the browser, merged = %s", merged)
	}
}

func TestDSHHeadlessMergeIsIdempotent(t *testing.T) {
	base := "http://127.0.0.1:10201"
	first, err := codingclients.MergeKind(codingclients.FileDSHHeadless, "", base, dshHeadlessModels())
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	second, err := codingclients.MergeKind(codingclients.FileDSHHeadless, first, base, dshHeadlessModels())
	if err != nil {
		t.Fatalf("MergeKind() on its own output error = %v", err)
	}
	if first != second {
		t.Fatalf("merged twice differs:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestDSHHeadlessReplacesTheOperatorsDefault(t *testing.T) {
	existing := "- id: agent-default-model\n  config:\n    provider: deepseek-official\n    model: deepseek-flash\n    reasoningEffort: high\n- id: other-patch\n  config:\n    enabled: true\n"
	merged, err := codingclients.MergeKind(codingclients.FileDSHHeadless, existing, "http://127.0.0.1:10201", dshHeadlessModels())
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	if strings.Contains(merged, "deepseek-official") || strings.Contains(merged, "reasoningEffort: high") {
		t.Fatalf("merged = %s, want the old default replaced whole", merged)
	}
	if count := strings.Count(merged, "- id: agent-default-model"); count != 1 {
		t.Fatalf("merged carries the default %d times, want one:\n%s", count, merged)
	}
	if !strings.Contains(merged, "other-patch") {
		t.Fatalf("merged = %s, want the operator's other item kept", merged)
	}
}

func TestDSHStripKeepsADefaultTheOperatorPointedElsewhere(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		wantKept bool
	}{
		{"relo's own default goes", "relo", false},
		{"another provider's default stays", "openai", true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			existing := "- id: agent-default-model\n  config:\n    provider: " + testCase.provider + "\n    model: some-model\n- id: other-patch\n  config:\n    enabled: true\n"
			stripped, err := codingclients.StripKind(codingclients.FileDSHHeadless, existing)
			if err != nil {
				t.Fatalf("StripKind() error = %v", err)
			}
			if kept := strings.Contains(stripped, "agent-default-model"); kept != testCase.wantKept {
				t.Fatalf("stripped = %s, default kept = %v, want %v", stripped, kept, testCase.wantKept)
			}
			if !strings.Contains(stripped, "other-patch") {
				t.Fatalf("stripped = %s, want the operator's other item kept", stripped)
			}
		})
	}
}

func TestDSHHeadlessPointsAtRefusesAChangedDefault(t *testing.T) {
	base := "http://127.0.0.1:10201"
	merged, err := codingclients.MergeKind(codingclients.FileDSHHeadless, "", base, dshHeadlessModels())
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	moved := strings.ReplaceAll(merged, "model: \"relo-openai-gpt-4o\"\n", "model: \"relo-openai-gpt-4o-mini\"\n")
	if moved == merged {
		t.Fatalf("test setup did not change the default:\n%s", merged)
	}
	if codingclients.KindPointsAt(codingclients.FileDSHHeadless, moved, base, dshHeadlessModels()) {
		t.Fatal("KindPointsAt() accepted a patch whose default model moved")
	}
}

func TestDSHStripLeavesAListWhenNothingRemains(t *testing.T) {
	base := "http://127.0.0.1:10201"
	for _, kind := range []string{codingclients.FileDSH, codingclients.FileDSHHeadless, codingclients.FileDSHDesktop} {
		merged, err := codingclients.MergeKind(kind, "", base, dshHeadlessModels())
		if err != nil {
			t.Fatalf("MergeKind(%s) error = %v", kind, err)
		}
		stripped, err := codingclients.StripKind(kind, merged)
		if err != nil {
			t.Fatalf("StripKind(%s) error = %v", kind, err)
		}
		var document []any
		if err := yaml.Unmarshal([]byte(stripped), &document); err != nil || len(document) != 0 {
			t.Fatalf("StripKind(%s) = %q, want an empty YAML list the harness can read (%v)", kind, stripped, err)
		}
	}
}

func TestDSHConfigTargetRefusesARelativeHome(t *testing.T) {
	t.Setenv("DSH_HOME", "relative/dsh")
	paths := codingclients.Paths{Home: "/home/op"}
	if _, err := paths.ConfigTargets(codingclients.ClientDSH); err == nil {
		t.Fatal("ConfigTargets() accepted a relative DSH_HOME")
	}
}

func TestDSHEmptyListStripsAndMergesCleanly(t *testing.T) {
	base := "http://127.0.0.1:10201"
	models := []codingclients.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o"}}
	stripped, err := codingclients.StripKind(codingclients.FileDSH, "[]\n")
	if err != nil {
		t.Fatalf("StripKind() error = %v", err)
	}
	var empty []any
	if err := yaml.Unmarshal([]byte(stripped), &empty); err != nil || len(empty) != 0 {
		t.Fatalf("StripKind() = %q, want an empty YAML list (%v)", stripped, err)
	}
	fromEmpty, err := codingclients.MergeKind(codingclients.FileDSH, "[]\n", base, models)
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	fromScratch, err := codingclients.MergeKind(codingclients.FileDSH, "", base, models)
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	if fromEmpty != fromScratch {
		t.Fatalf("merge from [] differs:\nfrom empty:\n%s\nfrom scratch:\n%s", fromEmpty, fromScratch)
	}
}
