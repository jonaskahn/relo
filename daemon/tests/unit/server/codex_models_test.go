package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestListModelsAnswersTheCodexCatalogShape is what Codex's picker reads:
// GET /v1/models?profile=codex returns Relo's Codex catalog rows.
func TestListModelsAnswersTheCodexCatalogShape(t *testing.T) {
	harness := newHarness(t)
	response := harness.dataPlane(http.MethodGet, "/v1/models?profile=codex", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("catalog listing status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode the catalog listing: %v\n%s", err, response.Body.String())
	}
	if len(payload.Models) == 0 {
		t.Fatalf("catalog listing = %s, want Relo's published rows", response.Body.String())
	}
	row := payload.Models[0]
	slug, _ := row["slug"].(string)
	name, _ := row["display_name"].(string)
	description, _ := row["description"].(string)
	if slug == "" || name == "" || !strings.Contains(description, "Routed via Relo") {
		t.Fatalf("row = %v, want a slug, a display name, and the connection in the description", row)
	}
	if strings.Contains(response.Body.String(), `"object":"list"`) {
		t.Fatalf("catalog listing = %s, want the Codex catalog shape", response.Body.String())
	}
}

// TestListModelsAnswersTheStandardOpenAIEnvelope keeps a generic client on
// the official list shape and puts Relo's extra facts in a namespaced
// extension rather than inventing OpenAI fields.
func TestListModelsAnswersTheStandardOpenAIEnvelope(t *testing.T) {
	harness := newHarness(t)
	response := harness.dataPlane(http.MethodGet, "/v1/models", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("models status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string         `json:"id"`
			Object  string         `json:"object"`
			Created int            `json:"created"`
			OwnedBy string         `json:"owned_by"`
			Relo    map[string]any `json:"relo"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v\n%s", err, response.Body.String())
	}
	if payload.Object != "list" || len(payload.Data) == 0 {
		t.Fatalf("payload = %+v, want a non-empty OpenAI list", payload)
	}
	row := payload.Data[0]
	if row.ID == "" || row.Object != "model" || row.Created != 0 || row.OwnedBy == "" {
		t.Fatalf("row = %+v, want the standard model object", row)
	}
	if _, found := row.Relo["context_window"]; !found {
		t.Fatalf("row = %+v, want the namespaced capability extension", row)
	}
	if strings.Contains(response.Body.String(), `"slug"`) {
		t.Fatalf("standard payload = %s, want no Codex-only fields", response.Body.String())
	}
}
