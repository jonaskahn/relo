package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
)

func TestFreeConnectionsSettingAndProbeGuard(t *testing.T) {
	h := newHarness(t)
	for _, body := range []string{"{}", "{\"free_enabled\":null}", "{\"free_enabled\":\"yes\"}"} {
		response := h.management(http.MethodPatch, "/api/v1/settings/connections", adminToken, strings.NewReader(body))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid body %s: %d %s", body, response.Code, response.Body)
		}
	}
	for _, enabled := range []bool{false, true, false} {
		body, _ := json.Marshal(map[string]bool{"free_enabled": enabled})
		response := h.management(http.MethodPatch, "/api/v1/settings/connections", adminToken, strings.NewReader(string(body)))
		if response.Code != http.StatusOK {
			t.Fatalf("patch: %d %s", response.Code, response.Body)
		}
		read := h.management(http.MethodGet, "/api/v1/settings", adminToken, nil)

		// The wire uses snake_case, independently of Go field spelling.
		var object map[string]json.RawMessage
		if err := json.Unmarshal(read.Body.Bytes(), &object); err != nil {
			t.Fatal(err)
		}
		var connections map[string]bool
		if err := json.Unmarshal(object["connections"], &connections); err != nil {
			t.Fatal(err)
		}
		if connections["free_enabled"] != enabled {
			t.Fatalf("settings = %s", read.Body)
		}
		templates := h.management(http.MethodGet, "/api/v1/templates", adminToken, nil)
		var listed struct {
			FreeEnabled bool `json:"free_enabled"`
			Items       []struct {
				ID    string `json:"id"`
				Label string `json:"label"`
			} `json:"items"`
		}
		if err := json.Unmarshal(templates.Body.Bytes(), &listed); err != nil {
			t.Fatal(err)
		}
		if listed.FreeEnabled != enabled {
			t.Fatalf("templates flag = %v want %v", listed.FreeEnabled, enabled)
		}
		freeLabels := map[string]string{
			"opencode-free": "OpenCode Zen Free",
			"kilo-free":     "Kilo Code Free",
		}
		seenFree := map[string]bool{}
		for _, item := range listed.Items {
			if want, free := freeLabels[item.ID]; free {
				seenFree[item.ID] = true
				if item.Label != want {
					t.Fatalf("%s label = %q want %q", item.ID, item.Label, want)
				}
			}
		}
		if enabled && len(seenFree) != len(freeLabels) {
			t.Fatalf("enabled returned free templates %v", seenFree)
		}
		if !enabled && len(seenFree) != 0 {
			t.Fatalf("disabled returned free templates %v", seenFree)
		}
		if !enabled {
			probe := h.management(http.MethodPost, "/api/v1/connections/probe", adminToken, strings.NewReader("{\"template_id\":\"opencode-free\"}"))
			if probe.Code != http.StatusForbidden || !strings.Contains(probe.Body.String(), "free_connections_disabled") {
				t.Fatalf("probe: %d %s", probe.Code, probe.Body)
			}
		}
	}
}

func TestDuplicateKeylessFreeConnectionIsRefused(t *testing.T) {
	h := newHarness(t)
	if err := config.UpdateConnectionsConfig(h.settingsPath, true); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewCatalogRepo(h.db)
	if err := repo.SaveProvider(context.Background(), sqlite.ProviderRow{
		ID: catalog.OpenCodeFreeTemplate, TemplateID: catalog.OpenCodeFreeTemplate,
		Origin: "template", Label: "OpenCode Zen Free", Auth: "none",
		APIFormat: "openai-chat", BaseURL: "http://127.0.0.1:1", ModelsFormat: "openai", Enabled: true, Rank: 100,
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.catalog.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}

	probe := h.management(http.MethodPost, "/api/v1/connections/probe", adminToken, strings.NewReader(`{"template_id":"opencode-free"}`))
	if probe.Code != http.StatusConflict || !strings.Contains(probe.Body.String(), "free_connection_exists") {
		t.Fatalf("probe = %d %s, want a localized duplicate refusal", probe.Code, probe.Body)
	}
	stored, err := repo.ListProviders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, row := range stored {
		if row.TemplateID == catalog.OpenCodeFreeTemplate {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("stored free connections = %d, want the existing row untouched", count)
	}
}

func TestDisabledFreeConnectionKeepsTheConsentRefusal(t *testing.T) {
	h := newHarness(t)
	if err := config.UpdateConnectionsConfig(h.settingsPath, false); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewCatalogRepo(h.db)
	if err := repo.SaveProvider(context.Background(), sqlite.ProviderRow{
		ID: catalog.OpenCodeFreeTemplate, TemplateID: catalog.OpenCodeFreeTemplate,
		Origin: "template", Label: "OpenCode Zen Free", Auth: "none",
		APIFormat: "openai-chat", BaseURL: h.upstream.URL(), ModelsFormat: "openai", Enabled: true, Rank: 100,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveModel(context.Background(), sqlite.ModelRow{
		ProviderID: catalog.OpenCodeFreeTemplate, ModelID: "free-model", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.catalog.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}

	response := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken,
		strings.NewReader(`{"model":"free-model","messages":[{"role":"user","content":"hi"}]}`))
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "free_connections_disabled") {
		t.Fatalf("chat = %d %s, want the consent refusal", response.Code, response.Body)
	}
	if h.upstream.requestCount() != 0 {
		t.Fatalf("upstream requests = %d, want no provider contact", h.upstream.requestCount())
	}
}
