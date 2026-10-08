package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestRouteSaveAcceptsTheConsoleBody pins the shape the console sends a route
// in. Every management write names its fields in snake_case, so a member the
// decoder does not recognise is refused before it ever reaches the store.
func TestRouteSaveAcceptsTheConsoleBody(t *testing.T) {
	harness := newHarness(t)
	body := strings.NewReader(`{"label":"Fast","strategy":"priority","enabled":true,"listed":true,` +
		`"members":[{"provider_id":"openai","model_id":"gpt-4o","weight":1,"enabled":true}]}`)
	recorder := harness.management(http.MethodPut, "/api/v1/routes/fast", adminToken, body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT /api/v1/routes/fast = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	group := struct {
		ID      string `json:"id"`
		Members []struct {
			ProviderID string `json:"provider_id"`
			ModelID    string `json:"model_id"`
		} `json:"members"`
	}{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &group); err != nil {
		t.Fatalf("decode the saved route: %v", err)
	}
	if group.ID != "fast" || len(group.Members) != 1 || group.Members[0].ProviderID != "openai" {
		t.Fatalf("saved route = %+v, want the console's own member", group)
	}
}

// TestManagementRoutes pins the resource layout the console, the tray app,
// and scripts read, and the absence of every path it replaced.
func TestManagementRoutes(t *testing.T) {
	harness := newHarness(t)

	t.Run("the renamed resources answer", func(t *testing.T) {
		routes := []string{
			"/api/v1/connections",
			"/api/v1/routes",
			"/api/v1/routes/preview?model=gpt-4o",
			"/api/v1/clients/keys",
			"/api/v1/activity/requests",
			"/api/v1/activity/usage?group_by=day",
			"/api/v1/activity/quota",
		}
		for _, path := range routes {
			recorder := harness.management(http.MethodGet, path, adminToken, nil)
			if recorder.Code != http.StatusOK {
				t.Errorf("GET %s = %d, want 200 (body %s)", path, recorder.Code, recorder.Body.String())
			}
		}
	})

	t.Run("the paths these resources replaced are gone", func(t *testing.T) {
		replaced := []string{
			"/api/v1/providers",
			"/api/v1/provider-templates",
			"/api/v1/groups",
			"/api/v1/route?model=gpt-4o",
			"/api/v1/access-keys",
			"/api/v1/logs",
			"/api/v1/usage",
		}
		for _, path := range replaced {
			recorder := harness.management(http.MethodGet, path, adminToken, nil)
			if recorder.Code != http.StatusNotFound {
				t.Errorf("GET %s = %d, want 404", path, recorder.Code)
				continue
			}
			payload := struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}{}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Errorf("GET %s: decode the refusal: %v", path, err)
				continue
			}
			if payload.Error.Code != "not_found" {
				t.Errorf("GET %s: code = %q, want not_found", path, payload.Error.Code)
			}
		}
	})
}
