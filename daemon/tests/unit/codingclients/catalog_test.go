package codingclients_test

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

func TestCatalogMergeKeepsTheRestOfTheFile(t *testing.T) {
	base := "http://127.0.0.1:10201"
	models := []codingclients.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o"}}
	cases := []struct {
		kind     string
		existing string
		want     string
		address  string
	}{
		{codingclients.FilePi, `{"theme":"dark","providers":{"openai":{"api":"openai"}}}` + "\n", "RELO_PI_API_KEY", base + "/v1"},
		{codingclients.FileZCode, `{"providers":{"other":{}}}` + "\n", "$RELO_ZCODE_API_KEY", base + "/v1"},
		{codingclients.FileClineProviders, `{"providers":{"openai":{"settings":{}}}}` + "\n", "$RELO_CLINE_API_KEY", base},
		{codingclients.FileClineModels, "", "Relo", base},
		{codingclients.FileOMP, "theme: dark\nproviders:\n  openai:\n    api: openai\n", "RELO_OMP_API_KEY", base + "/v1"},
		{codingclients.FileDSH, "- id: other\n  config:\n    providers:\n      openai:\n        baseURL: http://example\n", "apiKeyEnv: RELO_DSH_API_KEY", base + "/v1"},
		{codingclients.FileMCode, "editor: vim\n", "${RELO_MCODE_API_KEY}", base},
		{codingclients.FileRaycast, "providers:\n  - id: openai\n    name: OpenAI\n", "Relo", base},
		{codingclients.FileKimi, "[providers.other]\nbase_url = \"http://example\"\n", "${RELO_KIMI_API_KEY}", base + "/v1"},
	}
	for _, testCase := range cases {
		t.Run(testCase.kind, func(t *testing.T) {
			merged, err := codingclients.MergeKind(testCase.kind, testCase.existing, base, models)
			if err != nil {
				t.Fatalf("MergeKind() error = %v", err)
			}
			if !codingclients.KindPointsAt(testCase.kind, merged, base, models) {
				t.Fatalf("merged file does not point at Relo:\n%s", merged)
			}
			if testCase.want != "" && !strings.Contains(merged, testCase.want) {
				t.Fatalf("merged = %s, want %s", merged, testCase.want)
			}
			if !strings.Contains(merged, testCase.address) {
				t.Fatalf("merged = %s, want address %s", merged, testCase.address)
			}
			if strings.Contains(merged, "rlo_ak_secret") {
				t.Fatalf("merged carries a raw token:\n%s", merged)
			}
			if testCase.existing != "" && !strings.Contains(merged, siblingMarker(testCase.existing)) {
				t.Fatalf("merged = %s, want the original content kept", merged)
			}
			stripped, err := codingclients.StripKind(testCase.kind, merged)
			if err != nil {
				t.Fatalf("StripKind() error = %v", err)
			}
			if strings.TrimSpace(stripped) == "" {
				t.Fatal("StripKind() removed the file's contents, want the file kept")
			}
			present, err := codingclients.KindHasRelo(testCase.kind, stripped)
			if err != nil {
				t.Fatalf("KindHasRelo() error = %v", err)
			}
			if present {
				t.Fatalf("stripped file still has a relo provider:\n%s", stripped)
			}
			if testCase.existing != "" && !strings.Contains(stripped, siblingMarker(testCase.existing)) {
				t.Fatalf("stripped = %s, want the original content kept", stripped)
			}
		})
	}
}

func TestPiAndOMPWriteResolvedLimits(t *testing.T) {
	window := int64(400_000)
	output := int64(32_000)
	models := []codingclients.ModelRef{
		{ID: "relo-openai-gpt-4o", ContextWindow: &window, MaxOutput: &output},
		{ID: "relo-openai-bare"},
		{ID: "relo-openai-window-only", ContextWindow: &window},
	}
	base := "http://127.0.0.1:10201"
	for _, kind := range []string{codingclients.FilePi, codingclients.FileOMP} {
		t.Run(kind, func(t *testing.T) {
			merged, err := codingclients.MergeKind(kind, "", base, models)
			if err != nil {
				t.Fatalf("MergeKind() error = %v", err)
			}
			if !codingclients.KindPointsAt(kind, merged, base, models) {
				t.Fatalf("merged file does not point at Relo:\n%s", merged)
			}
			written := catalogLimits(t, kind, merged)
			sized := written["relo-openai-gpt-4o"]
			if sized.context != window || sized.output != output || !sized.hasOutput {
				t.Fatalf("sized = %+v, want context %d and max output %d", sized, window, output)
			}
			if bare := written["relo-openai-bare"]; bare.hasContext || bare.hasOutput {
				t.Fatalf("bare = %+v, want both limits omitted", bare)
			}
			if only := written["relo-openai-window-only"]; only.context != window || only.hasOutput {
				t.Fatalf("window only = %+v, want context %d and no max output", only, window)
			}
		})
	}
}

func TestPiAndOMPNameTheKeyVariable(t *testing.T) {
	base := "http://127.0.0.1:10201"
	models := []codingclients.ModelRef{{ID: "relo-openai-gpt-4o"}}
	cases := []struct {
		kind string
		want string
	}{
		{codingclients.FilePi, "RELO_PI_API_KEY"},
		{codingclients.FileOMP, "RELO_OMP_API_KEY"},
	}
	for _, testCase := range cases {
		t.Run(testCase.kind, func(t *testing.T) {
			merged, err := codingclients.MergeKind(testCase.kind, "", base, models)
			if err != nil {
				t.Fatalf("MergeKind() error = %v", err)
			}
			key := catalogKey(t, testCase.kind, merged)
			if key != testCase.want {
				t.Fatalf("apiKey = %q, want %q", key, testCase.want)
			}
			if strings.ContainsAny(key, "${}$") {
				t.Fatalf("apiKey = %q, want a bare variable name", key)
			}
		})
	}
}

// catalogKey reads the key one client catalog names for Relo. A client
// resolves that value as the name of an environment variable, a shell
// command, or a literal, so a template reaches the upstream as the secret
// itself and every request is refused.
func catalogKey(t *testing.T, kind, merged string) string {
	t.Helper()
	type provider struct {
		APIKey string `json:"apiKey" yaml:"apiKey"`
	}
	var document struct {
		Providers map[string]provider `json:"providers" yaml:"providers"`
	}
	switch kind {
	case codingclients.FilePi:
		if err := json.Unmarshal([]byte(merged), &document); err != nil {
			t.Fatalf("parse Pi catalog: %v\n%s", err, merged)
		}
	case codingclients.FileOMP:
		if err := yaml.Unmarshal([]byte(merged), &document); err != nil {
			t.Fatalf("parse Oh My Pi catalog: %v\n%s", err, merged)
		}
	default:
		t.Fatalf("kind %s has no provider catalog", kind)
	}
	return document.Providers["relo"].APIKey
}

func TestOMPWritesTheThinkingLadder(t *testing.T) {
	reasons := true
	doesNot := false
	toggle := true
	cases := []struct {
		name        string
		model       codingclients.ModelRef
		wantReason  bool
		wantEfforts []string
	}{
		{
			name:        "a model that reasons states its ladder",
			model:       codingclients.ModelRef{ID: "relo-a", Reasoning: &reasons, ReasoningEfforts: []string{"high", "low"}},
			wantReason:  true,
			wantEfforts: []string{"low", "high"},
		},
		{
			name:        "a model that does not reason carries no thinking",
			model:       codingclients.ModelRef{ID: "relo-b", Reasoning: &doesNot},
			wantReason:  false,
			wantEfforts: nil,
		},
		{
			name:        "a model the catalog does not know carries no thinking",
			model:       codingclients.ModelRef{ID: "relo-c"},
			wantReason:  false,
			wantEfforts: nil,
		},
		{
			name:        "a model that states no ladder offers the shared one",
			model:       codingclients.ModelRef{ID: "relo-d", Reasoning: &reasons},
			wantReason:  true,
			wantEfforts: []string{"low", "medium", "high", "xhigh", "max"},
		},
		{
			name:        "the off level is not an effort the client may name",
			model:       codingclients.ModelRef{ID: "relo-e", Reasoning: &reasons, ReasoningEfforts: []string{"none", "low"}},
			wantReason:  true,
			wantEfforts: []string{"low"},
		},
		{
			name:        "a model that states no usable effort carries no ladder",
			model:       codingclients.ModelRef{ID: "relo-f", Reasoning: &reasons, ReasoningEfforts: []string{"none"}, ReasoningToggle: &toggle},
			wantReason:  true,
			wantEfforts: nil,
		},
	}
	base := "http://127.0.0.1:10201"
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			merged, err := codingclients.MergeKind(codingclients.FileOMP, "", base, []codingclients.ModelRef{testCase.model})
			if err != nil {
				t.Fatalf("MergeKind() error = %v", err)
			}
			if !codingclients.KindPointsAt(codingclients.FileOMP, merged, base, []codingclients.ModelRef{testCase.model}) {
				t.Fatalf("merged file does not point at Relo:\n%s", merged)
			}
			entry := catalogModel(t, codingclients.FileOMP, merged, testCase.model.ID)
			if entry.Reasoning != testCase.wantReason {
				t.Fatalf("reasoning = %v, want %v", entry.Reasoning, testCase.wantReason)
			}
			if strings.Join(entry.Efforts, ",") != strings.Join(testCase.wantEfforts, ",") {
				t.Fatalf("efforts = %v, want %v", entry.Efforts, testCase.wantEfforts)
			}
			if entry.Mode != "effort" && len(entry.Efforts) > 0 {
				t.Fatalf("mode = %q, want effort", entry.Mode)
			}
			if entry.DefaultLevel != "" {
				t.Fatalf("defaultLevel = %q, want none", entry.DefaultLevel)
			}
		})
	}
}

func TestPiWritesTheThinkingLevels(t *testing.T) {
	reasons := true
	toggle := true
	models := []codingclients.ModelRef{
		{ID: "relo-a", Reasoning: &reasons, ReasoningEfforts: []string{"low", "high"}},
		{ID: "relo-b", Reasoning: &reasons, ReasoningToggle: &toggle},
	}
	base := "http://127.0.0.1:10201"
	merged, err := codingclients.MergeKind(codingclients.FilePi, "", base, models)
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	if !codingclients.KindPointsAt(codingclients.FilePi, merged, base, models) {
		t.Fatalf("merged file does not point at Relo:\n%s", merged)
	}
	ladder := catalogPiLevels(t, merged, "relo-a")
	if ladder["low"] != "low" || ladder["high"] != "high" {
		t.Fatalf("levels = %v, want low and high offered", ladder)
	}
	if ladder["medium"] != nil {
		t.Fatalf("levels = %v, want medium hidden", ladder)
	}
	if _, offered := ladder["off"]; offered {
		t.Fatalf("levels = %v, want no off level", ladder)
	}
	toggled := catalogPiLevels(t, merged, "relo-b")
	if toggled["off"] != "none" {
		t.Fatalf("levels = %v, want an off level", toggled)
	}
}

// catalogModel reads one model entry back out of a client catalog.
func catalogModel(t *testing.T, kind, merged, id string) writtenModel {
	t.Helper()
	type model struct {
		ID        string `json:"id" yaml:"id"`
		Reasoning *bool  `json:"reasoning" yaml:"reasoning"`
		Thinking  *struct {
			Mode         string   `json:"mode" yaml:"mode"`
			DefaultLevel string   `json:"defaultLevel" yaml:"defaultLevel"`
			Efforts      []string `json:"efforts" yaml:"efforts"`
		} `json:"thinking" yaml:"thinking"`
	}
	var document struct {
		Providers map[string]struct {
			Models []model `json:"models" yaml:"models"`
		} `json:"providers" yaml:"providers"`
	}
	switch kind {
	case codingclients.FilePi:
		if err := json.Unmarshal([]byte(merged), &document); err != nil {
			t.Fatalf("parse Pi catalog: %v\n%s", err, merged)
		}
	case codingclients.FileOMP:
		if err := yaml.Unmarshal([]byte(merged), &document); err != nil {
			t.Fatalf("parse Oh My Pi catalog: %v\n%s", err, merged)
		}
	default:
		t.Fatalf("kind %s has no model catalog", kind)
	}
	for _, entry := range document.Providers["relo"].Models {
		if entry.ID != id {
			continue
		}
		written := writtenModel{Reasoning: entry.Reasoning != nil && *entry.Reasoning}
		if entry.Thinking != nil {
			written.Mode = entry.Thinking.Mode
			written.Efforts = entry.Thinking.Efforts
			written.DefaultLevel = entry.Thinking.DefaultLevel
		}
		return written
	}
	t.Fatalf("catalog has no model %q", id)
	return writtenModel{}
}

// writtenModel is the thinking metadata one catalog entry carries.
type writtenModel struct {
	Reasoning    bool
	Mode         string
	Efforts      []string
	DefaultLevel string
}

// catalogPiLevels reads the level map one Pi model entry carries. A level
// the model does not take is null, which is how Pi hides it.
func catalogPiLevels(t *testing.T, merged, id string) map[string]any {
	t.Helper()
	type model struct {
		ID               string         `json:"id"`
		ThinkingLevelMap map[string]any `json:"thinkingLevelMap"`
	}
	var document struct {
		Providers map[string]struct {
			Models []model `json:"models"`
		} `json:"providers"`
	}
	if err := json.Unmarshal([]byte(merged), &document); err != nil {
		t.Fatalf("parse Pi catalog: %v\n%s", err, merged)
	}
	for _, entry := range document.Providers["relo"].Models {
		if entry.ID == id {
			return entry.ThinkingLevelMap
		}
	}
	t.Fatalf("catalog has no model %q", id)
	return nil
}

type writtenLimit struct {
	context    int64
	output     int64
	hasContext bool
	hasOutput  bool
}

func catalogLimits(t *testing.T, kind, merged string) map[string]writtenLimit {
	t.Helper()
	type model struct {
		ID            string `json:"id" yaml:"id"`
		ContextWindow *int64 `json:"contextWindow" yaml:"contextWindow"`
		MaxTokens     *int64 `json:"maxTokens" yaml:"maxTokens"`
	}
	var models []model
	switch kind {
	case codingclients.FilePi:
		var document struct {
			Providers map[string]struct {
				Models []model `json:"models"`
			} `json:"providers"`
		}
		if err := json.Unmarshal([]byte(merged), &document); err != nil {
			t.Fatalf("parse Pi catalog: %v\n%s", err, merged)
		}
		models = document.Providers["relo"].Models
	case codingclients.FileOMP:
		var document struct {
			Providers map[string]struct {
				Models []model `yaml:"models"`
			} `yaml:"providers"`
		}
		if err := yaml.Unmarshal([]byte(merged), &document); err != nil {
			t.Fatalf("parse Oh My Pi catalog: %v\n%s", err, merged)
		}
		models = document.Providers["relo"].Models
	default:
		t.Fatalf("kind %s has no limit catalog", kind)
	}
	limits := make(map[string]writtenLimit, len(models))
	for _, model := range models {
		var limit writtenLimit
		if model.ContextWindow != nil {
			limit.context = *model.ContextWindow
			limit.hasContext = true
		}
		if model.MaxTokens != nil {
			limit.output = *model.MaxTokens
			limit.hasOutput = true
		}
		limits[model.ID] = limit
	}
	return limits
}

func siblingMarker(existing string) string {
	for _, marker := range []string{"dark", "openai", "local", "other", "vim", "example"} {
		if strings.Contains(existing, marker) {
			return marker
		}
	}
	return existing
}
