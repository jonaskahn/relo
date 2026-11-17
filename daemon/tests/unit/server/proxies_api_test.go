package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type proxyResponseWire struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Scheme      string `json:"scheme"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Enabled     bool   `json:"enabled"`
	HasPassword bool   `json:"has_password"`
	State       string `json:"state"`
}

type proxyListWire struct {
	Items      []proxyResponseWire `json:"items"`
	NextCursor string              `json:"next_cursor"`
}

// TestProxyPoolAPI covers the settings-owned pool routes: a create never
// returns the password, the list paginates, a bad import line does not block
// a good one, and the providers payload names the read-only pool summary.
func TestProxyPoolAPI(t *testing.T) {
	h := newHarness(t)
	created := h.management(http.MethodPost, "/api/v1/settings/proxies", adminToken, strings.NewReader(
		`{"label":"edge","scheme":"http","host":"127.0.0.1","port":7890,"username":"alice","password":"hunter2","enabled":true}`,
	))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", created.Code, created.Body.String())
	}
	if strings.Contains(created.Body.String(), "hunter2") {
		t.Fatal("the create response revealed the password")
	}
	var item proxyResponseWire
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if !item.HasPassword || item.Username != "alice" || item.State != "unverified" {
		t.Fatalf("item = %+v", item)
	}

	list := h.management(http.MethodGet, "/api/v1/settings/proxies?limit=1", adminToken, nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", list.Code, list.Body.String())
	}
	if strings.Contains(list.Body.String(), "hunter2") {
		t.Fatal("the list response revealed the password")
	}
	var page proxyListWire
	if err := json.Unmarshal(list.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != item.ID {
		t.Fatalf("page = %+v", page)
	}

	imported := h.management(http.MethodPost, "/api/v1/settings/proxies/import", adminToken, strings.NewReader(
		`{"text":"# comment\nhttp://127.0.0.1:7891\nbad line\nsocks5://127.0.0.1:1080"}`,
	))
	if imported.Code != http.StatusOK {
		t.Fatalf("import status = %d, body = %s", imported.Code, imported.Body.String())
	}
	var result struct {
		Results []struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"results"`
	}
	if err := json.Unmarshal(imported.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 3 || result.Results[0].Status != "accepted" ||
		result.Results[1].Status != "invalid" || result.Results[2].Status != "accepted" {
		t.Fatalf("import results = %+v", result.Results)
	}

	patch := h.management(http.MethodPatch, "/api/v1/settings/proxies/"+item.ID, adminToken,
		strings.NewReader(`{"enabled":false,"password":""}`))
	if patch.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", patch.Code, patch.Body.String())
	}
	if strings.Contains(patch.Body.String(), "hunter2") {
		t.Fatal("the patch response revealed the password")
	}
	tested := h.management(http.MethodPost, "/api/v1/settings/proxies/"+item.ID+"/test", adminToken, nil)
	if tested.Code != http.StatusOK {
		t.Fatalf("test status = %d, body = %s", tested.Code, tested.Body.String())
	}

	deleted := h.management(http.MethodDelete, "/api/v1/settings/proxies/"+item.ID, adminToken, nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleted.Code, deleted.Body.String())
	}

	settings := h.management(http.MethodGet, "/api/v1/settings", adminToken, nil)
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), `"proxy_pool"`) {
		t.Fatalf("settings = %d %s", settings.Code, settings.Body.String())
	}
	if strings.Contains(settings.Body.String(), `"proxy_url"`) {
		t.Fatal("the settings payload still carries proxy_url")
	}
}
