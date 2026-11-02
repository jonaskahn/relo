package codingclients_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

const codeBuddyBase = "http://127.0.0.1:10201"

type codeBuddyDocument struct {
	AvailableModels []string `json:"availableModels"`
	Models          []struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		Vendor            string `json:"vendor"`
		URL               string `json:"url"`
		APIKey            string `json:"apiKey"`
		UseCustomProtocol *bool  `json:"useCustomProtocol"`
		MaxInputTokens    *int64 `json:"maxInputTokens"`
		MaxOutputTokens   *int64 `json:"maxOutputTokens"`
		SupportsToolCall  *bool  `json:"supportsToolCall"`
		SupportsImages    *bool  `json:"supportsImages"`
		SupportsReasoning *bool  `json:"supportsReasoning"`
		Reasoning         *struct {
			SupportedEfforts []string `json:"supportedEfforts"`
		} `json:"reasoning"`
	} `json:"models"`
}

func decodeCodeBuddyObject(t *testing.T, content string) codeBuddyDocument {
	t.Helper()
	var document codeBuddyDocument
	if err := json.Unmarshal([]byte(content), &document); err != nil {
		t.Fatalf("the file is not an object with models: %v\n%s", err, content)
	}
	return document
}

func TestCodeBuddyMergeWritesTheObjectFormForAnEmptyFile(t *testing.T) {
	for _, kind := range []string{codingclients.FileCodeBuddy, codingclients.FileWorkBuddy} {
		t.Run(kind, func(t *testing.T) {
			merged, err := codingclients.MergeKind(kind, "", codeBuddyBase, []codingclients.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o"}})
			if err != nil {
				t.Fatalf("MergeKind() error = %v", err)
			}
			if !strings.HasPrefix(strings.TrimSpace(merged), "{") {
				t.Fatalf("merged = %s, want the object form", merged)
			}
			document := decodeCodeBuddyObject(t, merged)
			if len(document.Models) != 1 {
				t.Fatalf("models = %d, want one", len(document.Models))
			}
			entry := document.Models[0]
			if entry.URL != codeBuddyBase+"/v1/chat/completions" {
				t.Fatalf("url = %q, want the chat-completions route", entry.URL)
			}
			if entry.APIKey != "${RELO_CODEBUDDY_API_KEY}" {
				t.Fatalf("apiKey = %q, want the environment reference", entry.APIKey)
			}
			if entry.Vendor != "Relo" || entry.Name != "GPT-4o" {
				t.Fatalf("entry = %+v, want the Relo vendor and the catalog name", entry)
			}
			if entry.UseCustomProtocol == nil || *entry.UseCustomProtocol {
				t.Fatalf("useCustomProtocol = %v, want false", entry.UseCustomProtocol)
			}
		})
	}
}

func TestCodeBuddyMergeIsIdempotent(t *testing.T) {
	models := []codingclients.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o"}}
	first, err := codingclients.MergeKind(codingclients.FileCodeBuddy, "", codeBuddyBase, models)
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	second, err := codingclients.MergeKind(codingclients.FileCodeBuddy, first, codeBuddyBase, models)
	if err != nil {
		t.Fatalf("MergeKind() on its own output error = %v", err)
	}
	if first != second {
		t.Fatalf("merged twice differs:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if count := strings.Count(second, `"vendor": "Relo"`); count != 1 {
		t.Fatalf("merged carries %d Relo entries, want one:\n%s", count, second)
	}
}

func TestCodeBuddyMergeKeepsAUserModelWithTheSameID(t *testing.T) {
	existing := `[{"id":"same","vendor":"Someone","name":"Theirs"}]` + "\n"
	merged, err := codingclients.MergeKind(codingclients.FileWorkBuddy, existing, codeBuddyBase, []codingclients.ModelRef{{ID: "same", Name: "Relo's"}})
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	var rows []struct {
		ID     string `json:"id"`
		Vendor string `json:"vendor"`
		Name   string `json:"name"`
	}
	if err := json.Unmarshal([]byte(merged), &rows); err != nil {
		t.Fatalf("merged is not an array: %v\n%s", err, merged)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want the user model and Relo's", len(rows))
	}
	if rows[0].Vendor != "Someone" || rows[0].Name != "Theirs" {
		t.Fatalf("user row = %+v, want it untouched", rows[0])
	}
}

func TestCodeBuddyMergeReplacesAStaleReloEntry(t *testing.T) {
	stale, err := codingclients.MergeKind(codingclients.FileWorkBuddy, "", codeBuddyBase, []codingclients.ModelRef{{ID: "relo-old"}})
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	current := []codingclients.ModelRef{{ID: "relo-new", Name: "New"}}
	merged, err := codingclients.MergeKind(codingclients.FileWorkBuddy, stale, codeBuddyBase, current)
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	if strings.Contains(merged, "relo-old") {
		t.Fatalf("merged = %s, want the stale entry gone", merged)
	}
	if !codingclients.KindPointsAt(codingclients.FileWorkBuddy, merged, codeBuddyBase, current) {
		t.Fatalf("merged file does not point at Relo:\n%s", merged)
	}
}

func TestCodeBuddyStripKeepsTheUserDocument(t *testing.T) {
	existing := `{"availableModels":["mine"],"models":[{"id":"mine","vendor":"Someone"},{"id":"relo-a","vendor":"Relo"}]}` + "\n"
	stripped, err := codingclients.StripKind(codingclients.FileCodeBuddy, existing)
	if err != nil {
		t.Fatalf("StripKind() error = %v", err)
	}
	if !strings.Contains(stripped, `"availableModels":["mine"]`) || !strings.Contains(stripped, `"id": "mine"`) {
		t.Fatalf("stripped = %s, want the operator's model and availableModels", stripped)
	}
	if strings.Contains(stripped, "relo-a") {
		t.Fatalf("stripped = %s, want no Relo entry", stripped)
	}
}

func TestCodeBuddyStripLeavesTheShapeWhenNothingRemains(t *testing.T) {
	object, err := codingclients.StripKind(codingclients.FileCodeBuddy, `{"models":[{"id":"relo-a","vendor":"Relo"}]}`)
	if err != nil {
		t.Fatalf("StripKind(object) error = %v", err)
	}
	if object != "{}\n" {
		t.Fatalf("object strip = %q, want {}", object)
	}
	pretty, err := codingclients.MergeKind(codingclients.FileCodeBuddy, "", codeBuddyBase, []codingclients.ModelRef{{ID: "relo-a"}})
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	prettyStrip, err := codingclients.StripKind(codingclients.FileCodeBuddy, pretty)
	if err != nil {
		t.Fatalf("StripKind(pretty) error = %v", err)
	}
	if prettyStrip != "{}\n" {
		t.Fatalf("pretty object strip = %q, want {}", prettyStrip)
	}
	array, err := codingclients.StripKind(codingclients.FileWorkBuddy, `[{"id":"relo-a","vendor":"Relo"}]`)
	if err != nil {
		t.Fatalf("StripKind(array) error = %v", err)
	}
	if array != "[]\n" {
		t.Fatalf("array strip = %q, want []", array)
	}
	empty, err := codingclients.StripKind(codingclients.FileCodeBuddy, `{"models":[]}`)
	if err != nil {
		t.Fatalf("StripKind(empty models) error = %v", err)
	}
	if strings.TrimSpace(empty) != `{"models":[]}` {
		t.Fatalf("empty models strip = %q, want the operator's file unchanged", empty)
	}
}

func TestCodeBuddyMergeInsertsModelsIntoAnObjectWithoutOne(t *testing.T) {
	merged, err := codingclients.MergeKind(codingclients.FileCodeBuddy, `{"availableModels":["mine"]}`+"\n", codeBuddyBase, []codingclients.ModelRef{{ID: "relo-a"}})
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	if !strings.Contains(merged, `"availableModels":["mine"]`) {
		t.Fatalf("merged = %s, want availableModels kept", merged)
	}
	if !codingclients.KindPointsAt(codingclients.FileCodeBuddy, merged, codeBuddyBase, []codingclients.ModelRef{{ID: "relo-a"}}) {
		t.Fatalf("merged file does not point at Relo:\n%s", merged)
	}
}

func TestCodeBuddyPointsAtIgnoresAChangedAPIKey(t *testing.T) {
	models := []codingclients.ModelRef{{ID: "relo-a", Name: "A"}}
	merged, err := codingclients.MergeKind(codingclients.FileWorkBuddy, "", codeBuddyBase, models)
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	encrypted := strings.Replace(merged, `${RELO_CODEBUDDY_API_KEY}`, "enc:v1:opaque", 1)
	if !codingclients.KindPointsAt(codingclients.FileWorkBuddy, encrypted, codeBuddyBase, models) {
		t.Fatalf("KindPointsAt() = false after WorkBuddy encrypted the key:\n%s", encrypted)
	}
	if codingclients.KindPointsAt(codingclients.FileWorkBuddy, encrypted, codeBuddyBase, []codingclients.ModelRef{{ID: "relo-b"}}) {
		t.Fatal("KindPointsAt() = true for another model list")
	}
}

func TestCodeBuddyPointsAtIsFalseWithoutAReloEntry(t *testing.T) {
	models := []codingclients.ModelRef{{ID: "relo-a"}}
	for _, existing := range []string{"{}", "[]", `{"models":[]}`, `[{"id":"mine","vendor":"Someone"}]`} {
		if codingclients.KindPointsAt(codingclients.FileCodeBuddy, existing, codeBuddyBase, models) {
			t.Fatalf("KindPointsAt(%s) = true, want false without a Relo entry", existing)
		}
	}
}

func TestCodeBuddyRefusesAnUnreadableFile(t *testing.T) {
	cases := map[string]string{
		"invalid json":      `{`,
		"models not a list": `{"models":{}}`,
		"scalar document":   `42`,
	}
	for name, existing := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := codingclients.MergeKind(codingclients.FileCodeBuddy, existing, codeBuddyBase, nil); !errors.Is(err, codingclients.ErrUnrecognisedFile) {
				t.Fatalf("MergeKind() error = %v, want %v", err, codingclients.ErrUnrecognisedFile)
			}
			if _, err := codingclients.StripKind(codingclients.FileCodeBuddy, existing); !errors.Is(err, codingclients.ErrUnrecognisedFile) {
				t.Fatalf("StripKind() error = %v, want %v", err, codingclients.ErrUnrecognisedFile)
			}
		})
	}
}

func TestCodeBuddyWritesOnlyTheKnownFields(t *testing.T) {
	window, output := int64(200000), int64(64000)
	tools, vision, reasons, toggle, doesNot := true, false, true, true, false
	cases := []struct {
		name   string
		model  codingclients.ModelRef
		want   []string
		absent []string
	}{
		{
			name:   "a plain model carries no capabilities",
			model:  codingclients.ModelRef{ID: "relo-plain"},
			absent: []string{"maxInputTokens", "maxOutputTokens", "supportsToolCall", "supportsImages", "supportsReasoning", "reasoning"},
		},
		{
			name: "a known model carries its limits and capabilities",
			model: codingclients.ModelRef{ID: "relo-known", ContextWindow: &window, MaxOutput: &output,
				Tools: &tools, Vision: &vision, Reasoning: &reasons, ReasoningEfforts: []string{"low", "high"}},
			want: []string{
				`"maxInputTokens": 200000`, `"maxOutputTokens": 64000`,
				`"supportsToolCall": true`, `"supportsImages": false`, `"supportsReasoning": true`,
				`"supportedEfforts": [`,
			},
		},
		{
			name:   "a model that does not reason carries the fact but no ladder",
			model:  codingclients.ModelRef{ID: "relo-off", Reasoning: &doesNot},
			want:   []string{`"supportsReasoning": false`},
			absent: []string{"supportedEfforts"},
		},
		{
			name:   "a toggle-only model carries the fact but no ladder",
			model:  codingclients.ModelRef{ID: "relo-toggle", Reasoning: &reasons, ReasoningEfforts: []string{}, ReasoningToggle: &toggle},
			want:   []string{`"supportsReasoning": true`},
			absent: []string{"supportedEfforts"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			merged, err := codingclients.MergeKind(codingclients.FileWorkBuddy, "", codeBuddyBase, []codingclients.ModelRef{testCase.model})
			if err != nil {
				t.Fatalf("MergeKind() error = %v", err)
			}
			for _, want := range testCase.want {
				if !strings.Contains(merged, want) {
					t.Fatalf("merged = %s, want %s", merged, want)
				}
			}
			for _, absent := range testCase.absent {
				if strings.Contains(merged, absent) {
					t.Fatalf("merged = %s, want no %s", merged, absent)
				}
			}
		})
	}
}

func TestCodeBuddyUsesTheSharedEffortLadderWhenTheModelStatesNone(t *testing.T) {
	reasons := true
	merged, err := codingclients.MergeKind(codingclients.FileWorkBuddy, "", codeBuddyBase, []codingclients.ModelRef{{ID: "relo-reasons", Reasoning: &reasons}})
	if err != nil {
		t.Fatalf("MergeKind() error = %v", err)
	}
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		if !strings.Contains(merged, `"`+effort+`"`) {
			t.Fatalf("merged = %s, want the shared effort %s", merged, effort)
		}
	}
}

func TestCodeBuddyTargetsFollowTheInstalledProduct(t *testing.T) {
	t.Run("only WorkBuddy is installed", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("WORKBUDDY_CONFIG_DIR", "")
		t.Setenv("CODEBUDDY_CONFIG_DIR", "")
		if err := os.MkdirAll(filepath.Join(home, ".workbuddy-ai"), 0o755); err != nil {
			t.Fatal(err)
		}
		targets, err := codingclients.Paths{Home: home}.ConfigTargets(codingclients.ClientCodeBuddy)
		if err != nil {
			t.Fatalf("ConfigTargets() error = %v", err)
		}
		if len(targets) != 1 || targets[0].Kind != codingclients.FileWorkBuddy {
			t.Fatalf("targets = %+v, want one WorkBuddy target", targets)
		}
		if targets[0].Path != filepath.Join(home, ".workbuddy-ai", "models.json") {
			t.Fatalf("path = %q", targets[0].Path)
		}
		if _, err := os.Stat(filepath.Join(home, ".codebuddy")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("resolution created the CodeBuddy directory")
		}
	})

	t.Run("both products are installed", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("WORKBUDDY_CONFIG_DIR", "")
		t.Setenv("CODEBUDDY_CONFIG_DIR", "")
		for _, dir := range []string{".workbuddy-ai", ".codebuddy"} {
			if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		targets, err := codingclients.Paths{Home: home}.ConfigTargets(codingclients.ClientCodeBuddy)
		if err != nil {
			t.Fatalf("ConfigTargets() error = %v", err)
		}
		if len(targets) != 2 {
			t.Fatalf("targets = %+v, want both products", targets)
		}
	})

	t.Run("neither product is installed", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("WORKBUDDY_CONFIG_DIR", "")
		t.Setenv("CODEBUDDY_CONFIG_DIR", "")
		targets, err := codingclients.Paths{Home: home}.ConfigTargets(codingclients.ClientCodeBuddy)
		if err != nil {
			t.Fatalf("ConfigTargets() error = %v", err)
		}
		if len(targets) != 1 || targets[0].Kind != codingclients.FileCodeBuddy || targets[0].Dir != filepath.Join(home, ".codebuddy") {
			t.Fatalf("targets = %+v, want the CodeBuddy fallback", targets)
		}
	})

	t.Run("an override names the directory", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, "elsewhere")
		t.Setenv("WORKBUDDY_CONFIG_DIR", dir)
		t.Setenv("CODEBUDDY_CONFIG_DIR", "")
		targets, err := codingclients.Paths{Home: home}.ConfigTargets(codingclients.ClientCodeBuddy)
		if err != nil {
			t.Fatalf("ConfigTargets() error = %v", err)
		}
		if len(targets) != 1 || targets[0].Kind != codingclients.FileWorkBuddy || targets[0].Dir != dir {
			t.Fatalf("targets = %+v, want the WorkBuddy override", targets)
		}
	})

	t.Run("a CodeBuddy override names its own kind", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, "codebuddy-elsewhere")
		t.Setenv("WORKBUDDY_CONFIG_DIR", "")
		t.Setenv("CODEBUDDY_CONFIG_DIR", dir)
		targets, err := codingclients.Paths{Home: home}.ConfigTargets(codingclients.ClientCodeBuddy)
		if err != nil {
			t.Fatalf("ConfigTargets() error = %v", err)
		}
		if len(targets) != 1 || targets[0].Kind != codingclients.FileCodeBuddy || targets[0].Dir != dir {
			t.Fatalf("targets = %+v, want the CodeBuddy override", targets)
		}
	})

	t.Run("overrides naming one directory de-duplicate", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, "shared")
		t.Setenv("WORKBUDDY_CONFIG_DIR", dir)
		t.Setenv("CODEBUDDY_CONFIG_DIR", dir)
		targets, err := codingclients.Paths{Home: home}.ConfigTargets(codingclients.ClientCodeBuddy)
		if err != nil {
			t.Fatalf("ConfigTargets() error = %v", err)
		}
		if len(targets) != 1 || targets[0].Dir != dir {
			t.Fatalf("targets = %+v, want one target for the shared directory", targets)
		}
	})

	t.Run("a relative override is refused", func(t *testing.T) {
		t.Setenv("WORKBUDDY_CONFIG_DIR", "")
		t.Setenv("CODEBUDDY_CONFIG_DIR", "relative/codebuddy")
		if _, err := (codingclients.Paths{Home: t.TempDir()}).ConfigTargets(codingclients.ClientCodeBuddy); !errors.Is(err, codingclients.ErrRelativePath) {
			t.Fatalf("ConfigTargets() error = %v, want %v", err, codingclients.ErrRelativePath)
		}
	})
}
