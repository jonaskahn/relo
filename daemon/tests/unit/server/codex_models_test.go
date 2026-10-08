package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestListModelsAnswersTheCodexCatalogShape is what Codex's picker reads:
// GET /v1/models returns Relo's catalog rows, so the connection the model
// runs on reaches the client with no query required.
func TestListModelsAnswersTheCodexCatalogShape(t *testing.T) {
	harness := newHarness(t)
	response := harness.dataPlane(http.MethodGet, "/v1/models", "", nil)
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
