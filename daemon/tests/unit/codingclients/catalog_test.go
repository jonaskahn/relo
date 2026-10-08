package codingclients_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
		{codingclients.FilePi, `{"theme":"dark","providers":{"openai":{"api":"openai"}}}` + "\n", "$RELO_PI_API_KEY", base + "/v1"},
		{codingclients.FilePrime, "", "$RELO_PRIME_API_KEY", base + "/v1"},
		{codingclients.FileAside, `{"providers":{"local":{}}}` + "\n", "", base + "/v1"},
		{codingclients.FileOMO, "", "", base + "/v1"},
		{codingclients.FileZCode, `{"providers":{"other":{}}}` + "\n", "$RELO_ZCODE_API_KEY", base + "/v1"},
		{codingclients.FileClineProviders, `{"providers":{"openai":{"settings":{}}}}` + "\n", "$RELO_CLINE_API_KEY", base},
		{codingclients.FileClineModels, "", "Relo", base},
		{codingclients.FileOMP, "theme: dark\nproviders:\n  openai:\n    api: openai\n", "${RELO_OMP_API_KEY}", base + "/v1"},
		{codingclients.FileGajae, "", "openai-completions", base + "/v1"},
		{codingclients.FileDSH, "providers:\n  other:\n    baseUrl: http://example\n", "${RELO_DSH_API_KEY}", base},
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

func TestAsideAccountDirectory(t *testing.T) {
	home := t.TempDir()
	paths := codingclients.Paths{Home: home}
	if _, err := paths.ConfigTargets(codingclients.ClientAside); !errors.Is(err, codingclients.ErrAsideAccount) {
		t.Fatalf("ConfigTargets() error = %v, want %v", err, codingclients.ErrAsideAccount)
	}
	account := filepath.Join(home, ".aside", "u", "work")
	if err := os.MkdirAll(account, 0o755); err != nil {
		t.Fatalf("create the account directory: %v", err)
	}
	targets, err := paths.ConfigTargets(codingclients.ClientAside)
	if err != nil {
		t.Fatalf("ConfigTargets() error = %v", err)
	}
	want := filepath.Join(account, "models.json")
	if len(targets) != 1 || targets[0].Path != want {
		t.Fatalf("targets = %+v, want %s", targets, want)
	}
	if err := os.MkdirAll(filepath.Join(home, ".aside", "u", "play"), 0o755); err != nil {
		t.Fatalf("create a second account: %v", err)
	}
	if _, err := paths.ConfigTargets(codingclients.ClientAside); !errors.Is(err, codingclients.ErrAsideAccount) {
		t.Fatalf("ConfigTargets() error = %v, want %v", err, codingclients.ErrAsideAccount)
	}
}
