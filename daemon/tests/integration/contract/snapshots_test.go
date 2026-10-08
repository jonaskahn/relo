package contract_test

import (
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the contract fixtures from the current behavior")

func fixtureDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "fixtures", "contract")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create the fixture dir: %v", err)
	}
	return dir
}

type snapshotCase struct {
	name       string
	method     string
	path       string
	body       string
	wantStatus int
	capture    string
}

func (h *harness) runSnapshots(t *testing.T, dir string, cases []snapshotCase) {
	t.Helper()
	vars := map[string]string{}
	for _, c := range cases {
		path := expand(c.path, vars)
		var body *strings.Reader
		if c.body != "" {
			body = strings.NewReader(expand(c.body, vars))
		}
		response := h.managementWithBody(t, c.method, path, body)
		if response.Code != c.wantStatus {
			t.Fatalf("%s: %s %s = %d, want %d (body %s)", c.name, c.method, path, response.Code, c.wantStatus, response.Body.String())
		}
		normalized := h.normalize(response.Body.Bytes())
		compareFixture(t, dir, c.name+".json", normalized)
		if c.capture != "" {
			var decoded map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
				t.Fatalf("%s: decode the response to capture %q: %v", c.name, c.capture, err)
			}
			text, _ := navigate(decoded, strings.Split(c.capture, ".")).(string)
			if text == "" {
				t.Fatalf("%s: response carries no %q to capture (body %s)", c.name, c.capture, response.Body.String())
			}
			vars[lastSegment(c.capture)] = text
		}
	}
}

func (h *harness) managementWithBody(t *testing.T, method, path string, body *strings.Reader) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = body
	}
	return h.management(method, path, adminToken, reader)
}

func expand(text string, vars map[string]string) string {
	for key, value := range vars {
		text = strings.ReplaceAll(text, "{"+key+"}", value)
	}
	return text
}

func navigate(value any, segments []string) any {
	for _, segment := range segments {
		record, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = record[segment]
	}
	return value
}

func lastSegment(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[i+1:]
	}
	return path
}

func compareFixture(t *testing.T, dir, name string, actual []byte) {
	t.Helper()
	path := filepath.Join(dir, name)
	if *update {
		if err := os.WriteFile(path, append(actual, '\n'), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
		return
	}
	wanted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v (run with -update to record it)", name, err)
	}
	if string(wanted) != string(append(actual, '\n')) {
		t.Fatalf("fixture %s differs from the recorded response (run with -update to inspect, then fix the code, not the fixture)", name)
	}
}

func TestManagementContract(t *testing.T) {
	h := newHarness(t)
	// Every vendor read in this test answers from the fixed models.dev
	// dataset, so no probe, refresh, or search ever reaches the network.
	h.upstream.setFixture("modelsdev.json", false)
	dir := fixtureDir(t)
	h.runSnapshots(t, dir, []snapshotCase{
		{name: "healthz", method: http.MethodGet, path: "/healthz", wantStatus: http.StatusOK},
		{name: "favicon", method: http.MethodGet, path: "/favicon.ico", wantStatus: http.StatusOK},
		{name: "status", method: http.MethodGet, path: "/api/v1/status", wantStatus: http.StatusOK},
		{name: "login-page", method: http.MethodGet, path: "/login", wantStatus: http.StatusOK},
		{name: "callback-unknown", method: http.MethodGet, path: "/callback/unknown", wantStatus: http.StatusBadRequest},
		{name: "callback-status-unknown", method: http.MethodGet, path: "/callback/unknown/status", wantStatus: http.StatusNotFound},

		{name: "accounts-list", method: http.MethodGet, path: "/api/v1/accounts", wantStatus: http.StatusOK},
		{name: "accounts-get", method: http.MethodGet, path: "/api/v1/accounts/one", wantStatus: http.StatusOK},
		{name: "accounts-get-missing", method: http.MethodGet, path: "/api/v1/accounts/missing", wantStatus: http.StatusNotFound},
		{name: "accounts-models", method: http.MethodGet, path: "/api/v1/accounts/one/models", wantStatus: http.StatusOK},
		{name: "accounts-context-get", method: http.MethodGet, path: "/api/v1/accounts/one/models/context", wantStatus: http.StatusOK},

		{name: "keys-list", method: http.MethodGet, path: "/api/v1/clients/keys", wantStatus: http.StatusOK},

		{name: "connections-list", method: http.MethodGet, path: "/api/v1/connections", wantStatus: http.StatusOK},
		{name: "connections-get", method: http.MethodGet, path: "/api/v1/connections/openai", wantStatus: http.StatusOK},
		{name: "connections-get-missing", method: http.MethodGet, path: "/api/v1/connections/missing", wantStatus: http.StatusNotFound},
		{name: "connections-template-settings", method: http.MethodGet, path: "/api/v1/connections/openai/template-settings", wantStatus: http.StatusNotFound},

		{name: "templates-list", method: http.MethodGet, path: "/api/v1/templates", wantStatus: http.StatusOK},
		{name: "modelsdev-state", method: http.MethodGet, path: "/api/v1/modelsdev", wantStatus: http.StatusOK},
		{name: "modelsdev-search", method: http.MethodGet, path: "/api/v1/modelsdev/models?q=gpt", wantStatus: http.StatusOK},
		{name: "catalog-models", method: http.MethodGet, path: "/api/v1/models", wantStatus: http.StatusOK},
		{name: "catalog-model-get", method: http.MethodGet, path: "/api/v1/models/openai/gpt-4o", wantStatus: http.StatusOK},
		{name: "catalog-model-missing", method: http.MethodGet, path: "/api/v1/models/openai/missing", wantStatus: http.StatusNotFound},

		{name: "routes-list", method: http.MethodGet, path: "/api/v1/routes", wantStatus: http.StatusOK},
		{name: "routes-preview", method: http.MethodGet, path: "/api/v1/routes/preview?model=gpt-4o", wantStatus: http.StatusOK},
		{name: "routes-get-missing", method: http.MethodGet, path: "/api/v1/routes/missing", wantStatus: http.StatusInternalServerError},

		{name: "activity-usage", method: http.MethodGet, path: "/api/v1/activity/usage?group_by=day", wantStatus: http.StatusOK},
		{name: "activity-summary", method: http.MethodGet, path: "/api/v1/activity/usage/summary", wantStatus: http.StatusOK},
		{name: "activity-quota", method: http.MethodGet, path: "/api/v1/activity/quota", wantStatus: http.StatusOK},
		{name: "activity-requests", method: http.MethodGet, path: "/api/v1/activity/requests", wantStatus: http.StatusOK},
		{name: "activity-attempts-missing", method: http.MethodGet, path: "/api/v1/activity/requests/999999/attempts", wantStatus: http.StatusOK},
		{name: "activity-captures-missing", method: http.MethodGet, path: "/api/v1/activity/requests/999999/captures", wantStatus: http.StatusOK},
		{name: "activity-capture-body-missing", method: http.MethodGet, path: "/api/v1/activity/requests/999999/captures/999999/body", wantStatus: http.StatusNotFound},

		{name: "logs-daemon", method: http.MethodGet, path: "/api/v1/logs/daemon", wantStatus: http.StatusOK},
		{name: "logs-startups", method: http.MethodGet, path: "/api/v1/logs/startups", wantStatus: http.StatusOK},
		{name: "logs-startup-missing", method: http.MethodGet, path: "/api/v1/logs/startups/startup-20200101-000000.log", wantStatus: http.StatusNotFound},

		{name: "usage-metrics-get", method: http.MethodGet, path: "/api/v1/ui/usage-metrics", wantStatus: http.StatusOK},

		{name: "settings-get", method: http.MethodGet, path: "/api/v1/settings", wantStatus: http.StatusOK},

		{name: "doctor", method: http.MethodGet, path: "/api/v1/doctor", wantStatus: http.StatusOK},

		{name: "integrations-list", method: http.MethodGet, path: "/api/v1/integrations", wantStatus: http.StatusOK},
		{name: "integrations-get", method: http.MethodGet, path: "/api/v1/integrations/codex", wantStatus: http.StatusOK},
		{name: "integrations-get-missing", method: http.MethodGet, path: "/api/v1/integrations/missing", wantStatus: http.StatusNotFound},
		{name: "integrations-models", method: http.MethodGet, path: "/api/v1/integrations/codex/models", wantStatus: http.StatusNotFound},

		{name: "updates", method: http.MethodGet, path: "/api/v1/updates", wantStatus: http.StatusOK},

		{name: "auth-session-anonymous", method: http.MethodGet, path: "/api/v1/auth/session", wantStatus: http.StatusOK},
		{name: "oauth-get-unknown", method: http.MethodGet, path: "/api/v1/oauth/unknown/start", wantStatus: http.StatusNotFound},
		{name: "oauth-post-unknown", method: http.MethodPost, path: "/api/v1/oauth/unknown/start", wantStatus: http.StatusServiceUnavailable},
		{name: "route-missing", method: http.MethodGet, path: "/api/v1/missing", wantStatus: http.StatusNotFound},

		{name: "daemon-stop", method: http.MethodPost, path: "/api/v1/daemon/stop", body: `{"instance_id":"other"}`, wantStatus: http.StatusNotImplemented},
		{name: "daemon-restart", method: http.MethodPost, path: "/api/v1/daemon/restart", wantStatus: http.StatusNotImplemented},
		{name: "daemon-force-restart", method: http.MethodPost, path: "/api/v1/daemon/force-restart", wantStatus: http.StatusNotImplemented},
		{name: "daemon-shutdown", method: http.MethodPost, path: "/api/v1/daemon/shutdown", wantStatus: http.StatusNotImplemented},

		{name: "auth-login-refused", method: http.MethodPost, path: "/api/v1/auth/login", body: `{"admin_token":"wrong"}`, wantStatus: http.StatusForbidden},
		{name: "auth-logout-anonymous", method: http.MethodPost, path: "/api/v1/auth/logout", wantStatus: http.StatusNoContent},

		{name: "accounts-create", method: http.MethodPost, path: "/api/v1/accounts", body: `{"provider_id":"openai","label":"contract","secret":"sk-contract-new"}`, wantStatus: http.StatusCreated, capture: "id"},
		{name: "accounts-create-duplicate", method: http.MethodPost, path: "/api/v1/accounts", body: `{"provider_id":"openai","label":"contract","secret":"sk-contract-new"}`, wantStatus: http.StatusConflict},
		{name: "accounts-patch", method: http.MethodPatch, path: "/api/v1/accounts/{id}", body: `{"status":"paused"}`, wantStatus: http.StatusOK},
		{name: "accounts-set-context", method: http.MethodPost, path: "/api/v1/accounts/{id}/models/context", body: `{"model_ids":["gpt-4o"],"context_window":400000}`, wantStatus: http.StatusOK},
		{name: "accounts-delete", method: http.MethodDelete, path: "/api/v1/accounts/{id}", wantStatus: http.StatusNoContent},

		{name: "keys-create", method: http.MethodPost, path: "/api/v1/clients/keys", body: `{"name":"contract-key","kind":"agent","client":"codex"}`, wantStatus: http.StatusCreated, capture: "key.id"},
		{name: "keys-update", method: http.MethodPatch, path: "/api/v1/clients/keys/{id}", body: `{"name":"contract-key-renamed"}`, wantStatus: http.StatusOK},
		{name: "keys-rotate", method: http.MethodPost, path: "/api/v1/clients/keys/{id}/rotate", wantStatus: http.StatusOK},
		{name: "keys-deletions", method: http.MethodPost, path: "/api/v1/clients/keys/deletions", body: `{}`, wantStatus: http.StatusOK},
		{name: "keys-delete", method: http.MethodDelete, path: "/api/v1/clients/keys/{id}", wantStatus: http.StatusNoContent},

		{name: "connections-patch", method: http.MethodPatch, path: "/api/v1/connections/openai", body: `{"switch_on_4xx":false}`, wantStatus: http.StatusOK},
		{name: "connections-patch-template-settings", method: http.MethodPatch, path: "/api/v1/connections/openai/template-settings", body: `{}`, wantStatus: http.StatusNotFound},
		{name: "connections-add-model", method: http.MethodPost, path: "/api/v1/connections/openai/models", body: `{"model_id":"contract-model"}`, wantStatus: http.StatusCreated},
		{name: "connections-clone-model", method: http.MethodPost, path: "/api/v1/connections/openai/models/clone", body: `{"source_model_id":"gpt-4o","model_id":"contract-clone"}`, wantStatus: http.StatusCreated},
		{name: "connections-set-context", method: http.MethodPost, path: "/api/v1/connections/openai/models/context", body: `{"model_ids":["gpt-4o"],"context_window":2000}`, wantStatus: http.StatusOK},
		{name: "connections-set-capabilities", method: http.MethodPost, path: "/api/v1/connections/openai/models/capabilities", body: `{"model_ids":["gpt-4o"],"reasoning":false}`, wantStatus: http.StatusOK},
		{name: "connections-toggle-models", method: http.MethodPost, path: "/api/v1/connections/openai/models/enabled", body: `{"model_ids":["contract-model"],"enabled":false}`, wantStatus: http.StatusOK},
		{name: "connections-refresh-models", method: http.MethodPost, path: "/api/v1/connections/openai/models/refresh", wantStatus: http.StatusOK},
		{name: "connections-refresh-quota", method: http.MethodPost, path: "/api/v1/connections/openai/quota/refresh", wantStatus: http.StatusOK},
		{name: "connections-probe", method: http.MethodPost, path: "/api/v1/connections/probe", body: `{"provider_id":"openai","credential":{"kind":"api_key","secret":"sk-contract-new"}}`, wantStatus: http.StatusOK, capture: "probe_id"},
		{name: "connections-probe-commit", method: http.MethodPost, path: "/api/v1/connections/probes/{probe_id}/commit", body: `{"label":"contract-probed"}`, wantStatus: http.StatusCreated},
		{name: "connections-probe-discard", method: http.MethodDelete, path: "/api/v1/connections/probes/{probe_id}", wantStatus: http.StatusNoContent},

		{name: "catalog-refresh", method: http.MethodPost, path: "/api/v1/catalog/refresh", wantStatus: http.StatusOK},
		{name: "models-patch", method: http.MethodPatch, path: "/api/v1/models/openai/gpt-4o", body: `{"enabled":true}`, wantStatus: http.StatusOK},
		{name: "models-delete", method: http.MethodDelete, path: "/api/v1/models/openai/contract-clone", wantStatus: http.StatusNoContent},

		{name: "routes-put", method: http.MethodPut, path: "/api/v1/routes/contract-fast", body: `{"label":"Contract","strategy":"priority","enabled":true,"listed":true,"members":[{"provider_id":"openai","model_id":"gpt-4o","weight":1,"enabled":true}]}`, wantStatus: http.StatusOK},
		{name: "routes-get", method: http.MethodGet, path: "/api/v1/routes/contract-fast", wantStatus: http.StatusOK},
		{name: "routes-delete", method: http.MethodDelete, path: "/api/v1/routes/contract-fast", wantStatus: http.StatusNoContent},

		{name: "settings-patch-language", method: http.MethodPatch, path: "/api/v1/settings/language", body: `{"language":"de"}`, wantStatus: http.StatusOK},
		{name: "settings-patch-appearance", method: http.MethodPatch, path: "/api/v1/settings/appearance", body: `{"accent":"blue","theme":"dark","quota_display":"remaining"}`, wantStatus: http.StatusOK},
		{name: "settings-patch-access", method: http.MethodPatch, path: "/api/v1/settings/access", body: `{"allow_external":false}`, wantStatus: http.StatusOK},
		{name: "settings-patch-network", method: http.MethodPatch, path: "/api/v1/settings/network", body: `{"bind":"127.0.0.1","port":12000,"openai_port":12001,"anthropic_port":12002,"gemini_port":0}`, wantStatus: http.StatusOK},
		{name: "settings-patch-providers", method: http.MethodPatch, path: "/api/v1/settings/providers", body: `{"catalog_url":"https://models.example.test/api.json","proxy_url":"","timeout_seconds":300,"retry_backoff":[[1,3],[3,5],[5,10]]}`, wantStatus: http.StatusOK},
		{name: "settings-patch-system", method: http.MethodPatch, path: "/api/v1/settings/system", body: `{"autostart":true,"log_level":"debug","updates_url":"","updates_download":""}`, wantStatus: http.StatusOK},
		{name: "settings-get-after", method: http.MethodGet, path: "/api/v1/settings", wantStatus: http.StatusOK},
		{name: "settings-put-retention", method: http.MethodPut, path: "/api/v1/settings/retention", body: `{"usage_days":30,"max_events":0,"max_bytes":0}`, wantStatus: http.StatusOK},
		{name: "settings-preview-retention", method: http.MethodPut, path: "/api/v1/settings/retention/preview", body: `{"usage_days":30,"max_events":0,"max_bytes":0}`, wantStatus: http.StatusOK},
		{name: "settings-run-retention", method: http.MethodPost, path: "/api/v1/settings/retention/run", wantStatus: http.StatusOK},
		{name: "settings-start-sweep", method: http.MethodPost, path: "/api/v1/settings/retention/sweep", wantStatus: http.StatusAccepted},
		{name: "settings-get-sweep", method: http.MethodGet, path: "/api/v1/settings/retention/sweep", wantStatus: http.StatusOK},

		{name: "usage-metrics-put", method: http.MethodPut, path: "/api/v1/ui/usage-metrics", body: `{"metrics":["requests","errors","spend","duration_avg","attempts","cache_read"]}`, wantStatus: http.StatusOK},

		{name: "modelsdev-refresh", method: http.MethodPost, path: "/api/v1/modelsdev/refresh", wantStatus: http.StatusOK},

		{name: "accounts-refresh-models", method: http.MethodPost, path: "/api/v1/accounts/one/models/refresh", wantStatus: http.StatusBadRequest},
		{name: "accounts-refresh-quota-missing", method: http.MethodPost, path: "/api/v1/accounts/missing/models/refresh", wantStatus: http.StatusNotFound},

		{name: "integrations-codex-context", method: http.MethodPost, path: "/api/v1/integrations/codex/context", body: `{"enabled":true}`, wantStatus: http.StatusOK},
		{name: "integrations-token", method: http.MethodGet, path: "/api/v1/integrations/codex/token", wantStatus: http.StatusNotFound},
		{name: "integrations-verify", method: http.MethodPost, path: "/api/v1/integrations/codex/verify", wantStatus: http.StatusNotFound},
		{name: "integrations-chat-turn", method: http.MethodPost, path: "/api/v1/integrations/codex/chat", body: `{"model":"gpt-4o","prompt":"hi"}`, wantStatus: http.StatusNotFound},
		{name: "integrations-chat", method: http.MethodPost, path: "/api/v1/integrations/chat", body: `{"mode":"direct","provider_id":"openai","model_id":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`, wantStatus: http.StatusOK},
		{name: "integrations-restart", method: http.MethodPost, path: "/api/v1/integrations/codex/restart", wantStatus: http.StatusOK},
		{name: "integrations-repair", method: http.MethodPost, path: "/api/v1/integrations/codex/repair", wantStatus: http.StatusOK},
		{name: "integrations-rotate", method: http.MethodPost, path: "/api/v1/integrations/codex/rotate", wantStatus: http.StatusOK},
		{name: "integrations-restore", method: http.MethodPost, path: "/api/v1/integrations/codex/restore", wantStatus: http.StatusOK},
		{name: "integrations-disable", method: http.MethodPost, path: "/api/v1/integrations/codex/disable", wantStatus: http.StatusOK},
		{name: "integrations-enable", method: http.MethodPost, path: "/api/v1/integrations/codex/enable", body: `{"overwrite":false}`, wantStatus: http.StatusOK},

		{name: "connections-delete-model", method: http.MethodDelete, path: "/api/v1/models/openai/contract-model", wantStatus: http.StatusNoContent},
	})
}
