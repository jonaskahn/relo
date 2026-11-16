package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/application/claudebindings"
	"github.com/jonaskahn/relo/internal/application/docstore"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/server"
)

// fakeIntercept stands in for the proxy runtime, so a test can put Claude Code
// in proxy mode without opening a listener.
type fakeIntercept struct{}

func (fakeIntercept) Ensure(context.Context, string, appintegration.ProxyTarget) (appintegration.ProxyEnv, error) {
	return appintegration.ProxyEnv{URL: "http://relo:secret@127.0.0.1:51999", CAPath: "/state/ca.pem"}, nil
}
func (fakeIntercept) Status(string) (string, bool) { return "127.0.0.1:51999", true }
func (fakeIntercept) Retire(string) error          { return nil }

type documentRows struct{ repo *sqlite.Documents }

func (d documentRows) List(ctx context.Context, namespace string) ([]docstore.Row, error) {
	rows, err := d.repo.List(ctx, namespace)
	listed := make([]docstore.Row, 0, len(rows))
	for _, row := range rows {
		listed = append(listed, docstore.Row{ID: row.ID, Version: row.Version, Body: row.Body})
	}
	return listed, err
}

func (d documentRows) Put(ctx context.Context, namespace string, row docstore.Row, nowMs int64) error {
	return d.repo.Put(ctx, namespace, sqlite.DocumentRow{ID: row.ID, Version: row.Version, Body: row.Body}, nowMs)
}

func (d documentRows) Delete(ctx context.Context, namespace, id string) error {
	return d.repo.Delete(ctx, namespace, id)
}

// bindingsServer builds a server whose Claude Code integration can run in proxy
// mode and whose bindings are stored in the harness database.
func bindingsServer(t *testing.T) (*harness, *server.Server) {
	t.Helper()
	return bindingsServerWith(t, nil)
}

// bindingsServerWith is bindingsServer over a Desktop picker.
func bindingsServerWith(t *testing.T, picker appintegration.DesktopPicker) (*harness, *server.Server) {
	t.Helper()
	t.Setenv("CODEX_HOME", t.TempDir())
	h := newHarness(t)
	paths := h.integrationPaths()
	if err := os.MkdirAll(os.Getenv("CLAUDE_CONFIG_DIR"), 0o755); err != nil {
		t.Fatalf("create the Claude directory: %v", err)
	}
	served := h.newServerWithOptions(h.cfg, []account.PoolEntry{defaultEntry()}, func(options *server.Options) {
		options.Integrations = appintegration.NewService(appintegration.ServiceOptions{
			Paths: platform.NewPathResolver(paths), Agents: platform.NewAgentRegistry(),
			Files: platform.NewClientFiles(paths), Processes: platform.NewAgentProcesses(),
			Wire: platform.NewWireCodec(), Store: platform.NewIntegrationStore(h.db),
			Catalog: h.catalog, Keys: h.keys, Now: h.clock.Now, Proxy: fakeIntercept{}, Picker: picker,
			ClaudeAuth: func() appintegration.ClaudeAuth { return appintegration.ClaudeAuthLogin },
		})
		options.ClaudeBindings = claudebindings.New(claudebindings.Options{
			Documents: docstore.NewCollection(documentRows{repo: sqlite.NewDocuments(h.db)}, claudebindings.Namespace,
				docstore.Codec[claudebindings.Document]{Decode: claudebindings.Decode, Encode: claudebindings.Encode},
				docstore.Options{}),
			Catalog: h.catalog,
		})
	})
	return h, served
}

func call(t *testing.T, h *harness, served *server.Server, method, path, payload string) (int, string) {
	t.Helper()
	recorder := h.managementOn(served, method, path, adminToken, strings.NewReader(payload))
	return recorder.Code, recorder.Body.String()
}

func TestBindingsAreListedWithTheOfficialIDsAnOperatorCanBind(t *testing.T) {
	h, served := bindingsServer(t)
	status, body := call(t, h, served, http.MethodGet, "/api/v1/integrations/claude-code/bindings", "")
	if status != http.StatusOK {
		t.Fatalf("GET bindings = %d %s", status, body)
	}
	var listed struct {
		Bindings []json.RawMessage `json:"bindings"`
		Official []struct {
			ID, Family, Source string
		} `json:"official"`
	}
	if err := json.Unmarshal([]byte(body), &listed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(listed.Bindings) != 0 || len(listed.Official) == 0 || listed.Official[0].Source != "seed" {
		t.Fatalf("listed = %+v, want no bindings and the seed ids", listed)
	}
}

func TestBindingsBelongToTheTwoProxyClientsOnly(t *testing.T) {
	h, served := bindingsServer(t)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		status, _ := call(t, h, served, method, "/api/v1/integrations/codex/bindings", `{}`)
		if status != http.StatusNotFound {
			t.Fatalf("%s bindings of codex = %d, want 404", method, status)
		}
	}
}

// TestDesktopHasItsOwnBindingsList pins that the two clients do not share a
// list: Desktop's is readable, refused outside proxy mode, and empty while
// Claude Code's holds its own.
func TestDesktopHasItsOwnBindingsList(t *testing.T) {
	h, served := bindingsServer(t)
	if status, body := call(t, h, served, http.MethodGet, "/api/v1/integrations/claude-desktop/bindings", ""); status != http.StatusOK {
		t.Fatalf("GET Desktop bindings = %d %s", status, body)
	}
	status, body := call(t, h, served, http.MethodPost, "/api/v1/integrations/claude-desktop/bindings",
		`{"set":[{"native_id":"claude-haiku-4-5","target":{"kind":"model","provider_id":"openai","model_id":"gpt-4o"},"enabled":true}]}`)
	if status != http.StatusNotFound && status != http.StatusConflict {
		t.Fatalf("POST Desktop bindings while it is off = %d %s, want a refusal", status, body)
	}
}

func TestBindingsCannotBeWrittenOutsideProxyMode(t *testing.T) {
	h, served := bindingsServer(t)
	if status, body := call(t, h, served, http.MethodPost, "/api/v1/integrations/claude-code/enable", `{}`); status != http.StatusOK {
		t.Fatalf("enable = %d %s", status, body)
	}
	status, body := call(t, h, served, http.MethodPost, "/api/v1/integrations/claude-code/bindings",
		`{"set":[{"native_id":"claude-haiku-4-5","target":{"kind":"model","provider_id":"openai","model_id":"gpt-4o"},"enabled":true}]}`)
	if status != http.StatusConflict || !strings.Contains(body, "proxy mode") {
		t.Fatalf("POST in settings mode = %d %s, want a conflict naming proxy mode", status, body)
	}
}

func TestBindingsAreWrittenReadBackAndRefusedWhenInvalid(t *testing.T) {
	h, served := bindingsServer(t)
	if status, body := call(t, h, served, http.MethodPost, "/api/v1/integrations/claude-code/enable", `{"mode":"proxy"}`); status != http.StatusOK {
		t.Fatalf("enable in proxy mode = %d %s", status, body)
	}
	valid := `{"set":[{"native_id":"claude-haiku-4-5-20251001","target":{"kind":"model","provider_id":"openai","model_id":"gpt-4o"},` +
		`"direction":"fallback","on_failure":"native","enabled":true}]}`
	status, body := call(t, h, served, http.MethodPost, "/api/v1/integrations/claude-code/bindings", valid)
	if status != http.StatusOK || !strings.Contains(body, `"native_id":"claude-haiku-4-5"`) ||
		!strings.Contains(body, `"state":"bound"`) || !strings.Contains(body, `"direction":"fallback"`) {
		t.Fatalf("POST bindings = %d %s, want the binding stored under its undated id and bound", status, body)
	}
	for name, payload := range map[string]string{
		"an unknown model": `{"set":[{"native_id":"claude-opus-5","target":{"kind":"model","provider_id":"openai","model_id":"nope"},"enabled":true}]}`,
		"a non-Claude id":  `{"set":[{"native_id":"gpt-5","target":{"kind":"model","provider_id":"openai","model_id":"gpt-4o"},"enabled":true}]}`,
		"an unknown field": `{"set":[],"surprise":1}`,
	} {
		if status, body := call(t, h, served, http.MethodPost, "/api/v1/integrations/claude-code/bindings", payload); status != http.StatusBadRequest {
			t.Fatalf("%s = %d %s, want 400", name, status, body)
		}
	}
	status, body = call(t, h, served, http.MethodPost, "/api/v1/integrations/claude-code/bindings", `{"remove":["claude-haiku-4-5"]}`)
	if status != http.StatusOK || !strings.Contains(body, `"bindings":[]`) {
		t.Fatalf("remove = %d %s, want the binding gone", status, body)
	}
}
