package proxy_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/proxy"
)

const cliAgent = "claude-cli/2.1.300 (external, cli)"

const ccConfig = `{"surface":"cc","config":{"id":"cc","models":[` +
	`{"id":"claude-opus-5-5","name":"Opus 5.5","short_name":"Opus","description":"For complex work",` +
	`"section":"main","fast_mode":{"type":"toggle"},"min_claude_code_version":"2.1.280",` +
	`"runtime":{"max_input_tokens":1000000,"family":"opus"}}]},"state":{"id":"cc"}}`

var reloRow = proxy.CatalogRow{ID: "claude-relo-openai--alpha", Name: "Alpha", ContextWindow: 400000}

// catalogVendor answers one fixed body for any path, gzip-encoded when the
// client allows it, as the vendor does.
type catalogVendor struct {
	server *httptest.Server
	status int
	body   string
	seen   *http.Request
}

func newCatalogVendor(t *testing.T, body string) *catalogVendor {
	t.Helper()
	v := &catalogVendor{status: http.StatusOK, body: body}
	v.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.seen = r
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Etag", `"upstream"`)
		payload := []byte(v.body)
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			var packed bytes.Buffer
			writer := gzip.NewWriter(&packed)
			_, _ = writer.Write(payload)
			_ = writer.Close()
			payload = packed.Bytes()
			w.Header().Set("Content-Encoding", "gzip")
		}
		w.WriteHeader(v.status)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(v.server.Close)
	return v
}

func (v *catalogVendor) get(t *testing.T, rows func() []proxy.CatalogRow, path, agent string) (*http.Response, []byte) {
	t.Helper()
	authority, err := proxy.LoadOrCreateAuthority(t.TempDir(), "api.anthropic.com")
	if err != nil {
		t.Fatalf("LoadOrCreateAuthority() error = %v", err)
	}
	pair, err := proxy.New(proxy.Options{
		Host: "api.anthropic.com", Upstream: v.server.URL, DataPlaneURL: v.server.URL,
		Token: "token", Authority: authority, Catalog: rows,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := pair.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { cancel(); pair.Wait() })
	request, err := http.NewRequest(http.MethodGet, "https://api.anthropic.com"+path, nil)
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}
	request.Header.Set("User-Agent", agent)
	request.Header.Set("Authorization", "Bearer user-oauth")
	response, err := interceptedClient(t, authority, pair.Addr()).Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = response.Body.Close() }()
	answer, _ := io.ReadAll(response.Body)
	return response, answer
}

func oneRow() []proxy.CatalogRow { return []proxy.CatalogRow{reloRow} }

func modelsOf(t *testing.T, answer []byte) []map[string]any {
	t.Helper()
	var parsed struct {
		Config struct {
			Models []map[string]any `json:"models"`
		} `json:"config"`
	}
	if err := json.Unmarshal(answer, &parsed); err != nil {
		t.Fatalf("the answer is not JSON: %v\n%s", err, answer)
	}
	return parsed.Config.Models
}

func TestTheCatalogGainsAClonedReloRowAndKeepsTheVendorsOwn(t *testing.T) {
	vendor := newCatalogVendor(t, ccConfig)
	response, answer := vendor.get(t, oneRow, "/api/organizations/org-1/model_selector/cc", cliAgent)
	models := modelsOf(t, answer)
	if len(models) != 2 || models[0]["id"] != "claude-opus-5-5" || models[0]["description"] != "For complex work" {
		t.Fatalf("models = %v, want the vendor's row untouched then Relo's", models)
	}
	added := models[1]
	if added["id"] != reloRow.ID || added["name"] != "Alpha" || added["section"] != "main" || added["short_name"] != "Alpha" {
		t.Fatalf("added row = %v, want Relo's id and name", added)
	}
	for _, key := range []string{"description", "fast_mode", "min_claude_code_version"} {
		if _, kept := added[key]; kept {
			t.Fatalf("added row kept %q, which describes the model it was copied from: %v", key, added)
		}
	}
	runtime := added["runtime"].(map[string]any)
	if runtime["max_input_tokens"] != float64(400000) || runtime["family"] != "opus" {
		t.Fatalf("runtime = %v, want Relo's window on the copied shape", runtime)
	}
	if vendor.seen.Header.Get("Authorization") != "Bearer user-oauth" {
		t.Fatalf("the catalog request lost the caller's credential")
	}
	if response.Header.Get("Content-Encoding") != "" || response.Header.Get("Etag") != "" ||
		response.Header.Get("Content-Length") != strconv.Itoa(len(answer)) {
		t.Fatalf("headers = %v, want an identity body with no stale validator", response.Header)
	}
	if !strings.Contains(string(answer), `"max_input_tokens":1000000`) {
		t.Fatalf("a number of the vendor's was reformatted: %s", answer)
	}
}

func TestTheModelSelectorListShapeIsRewrittenOnTheCCSurfaceOnly(t *testing.T) {
	vendor := newCatalogVendor(t, `{"model_selector_config":[`+
		`{"id":"ccd","models":[{"id":"claude-opus-5-5","name":"Opus"}]},`+
		`{"id":"cc","models":[{"id":"claude-opus-5-5","name":"Opus"}]}]}`)
	_, answer := vendor.get(t, oneRow, "/api/model_selector/cc", cliAgent)
	var parsed struct {
		Surfaces []struct {
			ID     string           `json:"id"`
			Models []map[string]any `json:"models"`
		} `json:"model_selector_config"`
	}
	if err := json.Unmarshal(answer, &parsed); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	if len(parsed.Surfaces[0].Models) != 1 || len(parsed.Surfaces[1].Models) != 2 {
		t.Fatalf("surfaces = %+v, want Relo's row on cc only", parsed.Surfaces)
	}
}

func TestTheBootstrapListGainsTheRowAsAnAdditionalOption(t *testing.T) {
	vendor := newCatalogVendor(t, `{"additional_model_options":[{"model":"claude-opus-5-5","name":"Opus","description":"d"}]}`)
	_, answer := vendor.get(t, oneRow, "/api/claude_cli/bootstrap", "claude-code/2.1.300")
	var parsed struct {
		Options []map[string]string `json:"additional_model_options"`
	}
	if err := json.Unmarshal(answer, &parsed); err != nil || len(parsed.Options) != 2 ||
		parsed.Options[1]["model"] != reloRow.ID || parsed.Options[1]["name"] != "Alpha" {
		t.Fatalf("options = %+v (%v), want the vendor's then Relo's", parsed.Options, err)
	}
}

func TestACatalogIsLeftAsTheVendorSentItWhenNothingApplies(t *testing.T) {
	cases := map[string]struct {
		body   string
		status int
		path   string
		agent  string
		rows   func() []proxy.CatalogRow
	}{
		"a Claude Desktop process": {ccConfig, 200, "/api/model_selector/cc", "claude-cli/2.1.300 (external, claude-desktop)", oneRow},
		"an error answer":          {`{"error":"no"}`, 503, "/api/model_selector/cc", cliAgent, oneRow},
		"not JSON":                 {`<html>`, 200, "/api/model_selector/cc", cliAgent, oneRow},
		"no rows to add":           {ccConfig, 200, "/api/model_selector/cc", cliAgent, func() []proxy.CatalogRow { return nil }},
		"a row already listed": {ccConfig, 200, "/api/model_selector/cc", cliAgent,
			func() []proxy.CatalogRow { return []proxy.CatalogRow{{ID: "claude-opus-5-5", Name: "x"}} }},
		"no usable template":    {`{"config":{"id":"cc","models":[{"id":"gpt-x"}]}}`, 200, "/api/model_selector/cc", cliAgent, oneRow},
		"another path":          {ccConfig, 200, "/v1/models", cliAgent, oneRow},
		"another surface":       {`{"config":{"id":"ccd","models":[{"id":"claude-a"}]}}`, 200, "/api/model_selector/cc", cliAgent, oneRow},
		"a body that is a list": {`[1,2]`, 200, "/api/model_selector/cc", cliAgent, oneRow},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			vendor := newCatalogVendor(t, tc.body)
			vendor.status = tc.status
			response, answer := vendor.get(t, tc.rows, tc.path, tc.agent)
			if response.StatusCode != tc.status || string(answer) != tc.body {
				t.Fatalf("answer = %d %s, want the vendor's %d %s", response.StatusCode, answer, tc.status, tc.body)
			}
		})
	}
}

func TestACatalogOverTheSizeCapPassesThroughWhole(t *testing.T) {
	filler := strings.Repeat("a", 17<<20)
	body := `{"config":{"id":"cc","models":[{"id":"claude-opus-5-5","note":"` + filler + `"}]}}`
	vendor := newCatalogVendor(t, body)
	_, answer := vendor.get(t, oneRow, "/api/model_selector/cc", cliAgent)
	if len(answer) != len(body) {
		t.Fatalf("answer is %d bytes, want the vendor's %d", len(answer), len(body))
	}
}

func TestARowThatCannotFitTheBudgetRefusesTheWholeRewrite(t *testing.T) {
	huge := proxy.CatalogRow{ID: "claude-relo-x--big", Name: strings.Repeat("n", 300<<10)}
	vendor := newCatalogVendor(t, ccConfig)
	_, answer := vendor.get(t, func() []proxy.CatalogRow { return []proxy.CatalogRow{reloRow, huge} }, "/api/model_selector/cc", cliAgent)
	if string(answer) != ccConfig {
		t.Fatalf("answer = %s, want the vendor's catalog with no partial list", answer)
	}
}
