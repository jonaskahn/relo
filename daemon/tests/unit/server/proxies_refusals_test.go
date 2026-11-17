package server_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/server"
)

// TestProxyPoolRefusalsMapToServiceErrors covers the error mapping every pool
// route shares: an id nothing stores, an address the normalizer refuses, a
// duplicate address, and a body the decoder refuses.
func TestProxyPoolRefusalsMapToServiceErrors(t *testing.T) {
	h := newHarness(t)
	for _, testCase := range []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{"patch an id nothing stores", http.MethodPatch, "/api/v1/settings/proxies/missing", `{"enabled":false}`, http.StatusNotFound},
		{"delete an id nothing stores", http.MethodDelete, "/api/v1/settings/proxies/missing", "", http.StatusNotFound},
		{"test an id nothing stores", http.MethodPost, "/api/v1/settings/proxies/missing/test", "", http.StatusNotFound},
		{"create an address the normalizer refuses", http.MethodPost, "/api/v1/settings/proxies", `{"scheme":"ftp","host":"127.0.0.1","port":21}`, http.StatusBadRequest},
		{"create a body the decoder refuses", http.MethodPost, "/api/v1/settings/proxies", `{"port":"not a number"}`, http.StatusBadRequest},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := h.management(testCase.method, testCase.path, adminToken, strings.NewReader(testCase.body))
			if recorder.Code != testCase.status {
				t.Fatalf("%s %s = %d, body %s, want %d", testCase.method, testCase.path,
					recorder.Code, recorder.Body.String(), testCase.status)
			}
		})
	}

	payload := `{"scheme":"http","host":"127.0.0.1","port":7890}`
	if code := h.managementJSON(t, http.MethodPost, "/api/v1/settings/proxies", payload, nil); code != http.StatusCreated {
		t.Fatalf("first create = %d", code)
	}
	if code := h.managementJSON(t, http.MethodPost, "/api/v1/settings/proxies", payload, nil); code != http.StatusConflict {
		t.Fatalf("duplicate create = %d, want a conflict", code)
	}
}

// TestProxyPoolListFiltersAndPaginates covers the list query: the cursor, the
// search, and the state filter all reach the pool, and the last page carries no
// cursor.
func TestProxyPoolListFiltersAndPaginates(t *testing.T) {
	h := newHarness(t)
	for _, port := range []int{7890, 7891, 7892} {
		payload := `{"label":"edge-` + strconv.Itoa(port) + `","scheme":"http","host":"127.0.0.1","port":` +
			strconv.Itoa(port) + `,"enabled":true}`
		if code := h.managementJSON(t, http.MethodPost, "/api/v1/settings/proxies", payload, nil); code != http.StatusCreated {
			t.Fatalf("create port %d = %d", port, code)
		}
	}

	var first proxyListWire
	if code := h.managementJSON(t, http.MethodGet, "/api/v1/settings/proxies?limit=2", "", &first); code != http.StatusOK {
		t.Fatalf("first page = %d", code)
	}
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two rows and a cursor", first)
	}
	var second proxyListWire
	if code := h.managementJSON(t, http.MethodGet, "/api/v1/settings/proxies?limit=2&cursor="+first.NextCursor, "", &second); code != http.StatusOK {
		t.Fatalf("second page = %d", code)
	}
	if len(second.Items) != 1 || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want the last row and no cursor", second)
	}

	var searched proxyListWire
	if code := h.managementJSON(t, http.MethodGet, "/api/v1/settings/proxies?q=edge-7891", "", &searched); code != http.StatusOK {
		t.Fatalf("search = %d", code)
	}
	if len(searched.Items) != 1 || !strings.Contains(searched.Items[0].Label, "7891") {
		t.Fatalf("search = %+v, want one matching row", searched)
	}

	var filtered proxyListWire
	if code := h.managementJSON(t, http.MethodGet, "/api/v1/settings/proxies?status=enabled", "", &filtered); code != http.StatusOK {
		t.Fatalf("status filter = %d", code)
	}
	if len(filtered.Items) != 3 {
		t.Fatalf("status filter = %+v, want three enabled rows", filtered)
	}
}

// TestProxyPoolCreateDefaultsToEnabled covers the optional enabled flag: a
// create that omits it stores an enabled proxy, which is what the console
// sends when the switch starts on.
func TestProxyPoolCreateDefaultsToEnabled(t *testing.T) {
	h := newHarness(t)
	var item proxyResponseWire
	if code := h.managementJSON(t, http.MethodPost, "/api/v1/settings/proxies",
		`{"scheme":"http","host":"127.0.0.1","port":7899}`, &item); code != http.StatusCreated {
		t.Fatalf("create = %d", code)
	}
	if !item.Enabled {
		t.Fatalf("item = %+v, want it enabled by default", item)
	}
}

// TestProxyPoolImportReportsEveryLine covers the import answer: a good line
// carries its stored item, and a bad one carries the reason it was refused.
func TestProxyPoolImportReportsEveryLine(t *testing.T) {
	h := newHarness(t)
	var result struct {
		Results []struct {
			Line   int              `json:"line"`
			Status string           `json:"status"`
			Reason string           `json:"reason"`
			Item   *json.RawMessage `json:"item"`
		} `json:"results"`
	}
	if code := h.managementJSON(t, http.MethodPost, "/api/v1/settings/proxies/import",
		`{"text":"http://127.0.0.1:7900\nnope"}`, &result); code != http.StatusOK {
		t.Fatalf("import = %d", code)
	}
	if len(result.Results) != 2 {
		t.Fatalf("import results = %+v", result.Results)
	}
	if result.Results[0].Status != "accepted" || result.Results[0].Item == nil || result.Results[0].Line != 1 {
		t.Fatalf("accepted line = %+v", result.Results[0])
	}
	if result.Results[1].Status != "invalid" || result.Results[1].Reason == "" || result.Results[1].Item != nil {
		t.Fatalf("refused line = %+v", result.Results[1])
	}
}

// TestProxyPoolRoutesReportAnUnconfiguredPool covers the nil guard every pool
// route shares: without a pool the API refuses rather than panics.
func TestProxyPoolRoutesReportAnUnconfiguredPool(t *testing.T) {
	h := newHarness(t)
	detached := h.newServerWithOptions(h.cfg, nil, func(options *server.Options) {
		options.Proxies = nil
	})
	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/settings/proxies"},
		{http.MethodPost, "/api/v1/settings/proxies"},
		{http.MethodPost, "/api/v1/settings/proxies/import"},
		{http.MethodPatch, "/api/v1/settings/proxies/one"},
		{http.MethodDelete, "/api/v1/settings/proxies/one"},
		{http.MethodPost, "/api/v1/settings/proxies/one/test"},
	} {
		recorder := h.managementOn(detached, request.method, request.path, adminToken, strings.NewReader("{}"))
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s %s = %d, body %s, want 503", request.method, request.path,
				recorder.Code, recorder.Body.String())
		}
	}
}

// TestProxyListReportsACoolingProxy covers the cooldown the console renders as
// a countdown: the state filter finds it and the response names the end.
func TestProxyListReportsACoolingProxy(t *testing.T) {
	h := newHarness(t)
	var item proxyResponseWire
	if code := h.managementJSON(t, http.MethodPost, "/api/v1/settings/proxies",
		`{"scheme":"http","host":"127.0.0.1","port":7901,"enabled":true}`, &item); code != http.StatusCreated {
		t.Fatalf("create = %d", code)
	}
	h.proxyPool.Cool(item.ID, 60_000_000_000)

	var listed struct {
		Items []struct {
			ID           string  `json:"id"`
			State        string  `json:"state"`
			CoolingUntil *string `json:"cooling_until"`
		} `json:"items"`
	}
	if code := h.managementJSON(t, http.MethodGet, "/api/v1/settings/proxies?status=cooling", "", &listed); code != http.StatusOK {
		t.Fatalf("list = %d", code)
	}
	if len(listed.Items) != 1 || listed.Items[0].State != "cooling" || listed.Items[0].CoolingUntil == nil {
		t.Fatalf("listed = %+v, want one cooling row naming its end", listed)
	}

	// A proxy whose check fails reports the state the test found.
	detached := h.newServerWithOptions(h.cfg, nil, func(options *server.Options) {
		options.Proxies = h.failingPool()
	})
	created := h.managementOn(detached, http.MethodPost, "/api/v1/settings/proxies", adminToken,
		strings.NewReader(`{"scheme":"http","host":"127.0.0.1","port":7902}`))
	if created.Code != http.StatusCreated {
		t.Fatalf("create on the failing pool = %d, body %s", created.Code, created.Body.String())
	}
	var createdItem proxyResponseWire
	if err := json.Unmarshal(created.Body.Bytes(), &createdItem); err != nil {
		t.Fatalf("decode the create response: %v", err)
	}
	tested := h.managementOn(detached, http.MethodPost,
		"/api/v1/settings/proxies/"+createdItem.ID+"/test", adminToken, nil)
	if tested.Code != http.StatusOK {
		t.Fatalf("test = %d, body %s", tested.Code, tested.Body.String())
	}
	if !strings.Contains(tested.Body.String(), `"unreachable"`) {
		t.Fatalf("test body = %s, want the unreachable state", tested.Body.String())
	}
}
