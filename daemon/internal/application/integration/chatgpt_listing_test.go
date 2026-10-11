// ChatGPT listing: full hyphenated names on both surfaces, one base-tier entry.
package integration

import (
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestChatGPTKeepsFullNameAndSingleWindow pins the save-token listing for a
// ChatGPT connection: the provider's hyphenated name reaches both surfaces
// unchanged, and the model is published once at its base-tier window rather
// than beside a 1M twin that would push clients wide prematurely.
func TestChatGPTKeepsFullNameAndSingleWindow(t *testing.T) {
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "codex-test", Label: "ChatGPT", APIFormat: catalog.FormatOpenAIResp,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"gpt-6.1-sol"}, ModelsFormat: catalog.ModelsNone, ContextWindow: 272_000,
	})
	if _, err := db.SQL().Exec("UPDATE providers SET template_id = 'openai-codex' WHERE id = 'codex-test'"); err != nil {
		t.Fatalf("set provider template: %v", err)
	}
	if _, err := db.SQL().Exec("UPDATE model_facts SET name = 'GPT-6.1-Sol' WHERE provider_id = 'codex-test' AND model_id = 'gpt-6.1-sol'"); err != nil {
		t.Fatalf("name the model: %v", err)
	}
	service := NewService(ServiceOptions{Catalog: testkit.ReloadCatalog(t, db, activeAccounts{})})

	plain := indexModels(t, service.publishedModels())
	if len(plain) != 1 {
		t.Fatalf("published = %+v, want one entry", plain)
	}
	entry, found := plain["relo-codex-test-gpt-6.1-sol"]
	if !found {
		t.Fatalf("published = %+v, want the bare base-tier entry", plain)
	}
	if entry.Name != "GPT-6.1-Sol" || entry.ContextWindow == nil || *entry.ContextWindow != 272_000 {
		t.Fatalf("published = %+v, want the full hyphenated name at 272K", plain)
	}

	claude := indexModels(t, service.claudePickerModels())
	if len(claude) != 1 {
		t.Fatalf("claude = %+v, want one entry", claude)
	}
	for _, ref := range claude {
		if ref.Name != "GPT-6.1-Sol" {
			t.Fatalf("claude = %+v, want the full hyphenated name", claude)
		}
	}
}
