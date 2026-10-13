package proxy_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
)

// goLongToolName is a tool a coding session actually sends once it loads an
// MCP server, and it is longer than the paid gateway accepts.
const goLongToolName = "mcp__claude_ai_Cloudflare_Developer_Platform__complete_authentication"

// goToolNameLimit is the longest tool name the gateway accepts, which the
// mock holds the way the gateway does.
const goToolNameLimit = 64

// goGateway is a mock of the paid OpenCode Go lane: it takes an API key and
// refuses a tool name past the gateway's limit, which is the refusal a coding
// session meets rather than a text turn.
type goGateway struct {
	server *httptest.Server
	last   map[string]any
	auths  []string
}

func newGoGateway(t *testing.T, answer func(w http.ResponseWriter, body map[string]any)) *goGateway {
	t.Helper()
	gateway := &goGateway{}
	gateway.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := map[string]any{}
		_ = json.Unmarshal(raw, &body)
		gateway.last = body
		gateway.auths = append(gateway.auths, r.Header.Get("Authorization"))
		if refused := overlongToolName(body); refused != "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","param":"name",`+
				`"message":"`+"`name` must be at most 64 characters"+`"}}`)
			return
		}
		answer(w, body)
	}))
	t.Cleanup(gateway.server.Close)
	return gateway
}

func (g *goGateway) URL() string {
	return g.server.URL
}

// overlongToolName returns the first name past the limit the request sends, in
// a declaration or in a replayed call, which is the name the gateway refuses.
func overlongToolName(body map[string]any) string {
	for _, entry := range zenItems(body["tools"]) {
		tool, _ := entry.(map[string]any)
		if function, ok := tool["function"].(map[string]any); ok {
			if name, _ := function["name"].(string); len(name) > goToolNameLimit {
				return name
			}
		}
	}
	for _, entry := range zenItems(body["messages"]) {
		for _, call := range zenItems(zenField(entry, "tool_calls")) {
			if name, _ := zenField(zenField(call, "function"), "name").(string); len(name) > goToolNameLimit {
				return name
			}
		}
	}
	return ""
}

// offeredToolName is the name of the first tool a request declares, which is
// the name the model was offered.
func offeredToolName(body map[string]any) string {
	for _, entry := range zenItems(body["tools"]) {
		tool, _ := entry.(map[string]any)
		if function, ok := tool["function"].(map[string]any); ok {
			if name, _ := function["name"].(string); name != "" {
				return name
			}
		}
	}
	return ""
}

// TestOpenCodeGoShortensAndNamesBackALongToolName covers the paid Go lane: the
// gateway refuses the whole turn for one name past its limit, so the name goes
// out under an alias and the answer names the tool the client declared.
func TestOpenCodeGoShortensAndNamesBackALongToolName(t *testing.T) {
	if len(goLongToolName) <= goToolNameLimit {
		t.Fatalf("the sample name is %d characters, want one past the gateway limit",
			len(goLongToolName))
	}
	gateway := newGoGateway(t, goAnswersWithACallToTheOfferedTool)
	daemon := startOpenCodeGoDaemon(t, gateway.URL())

	response := send(t, daemon.dataPlane, "/v1/chat/completions", dataPlaneToken, goToolRequest())
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, body = %s", response.StatusCode, body)
	}

	sent := offeredToolName(gateway.last)
	if sent == "" || sent == goLongToolName || len(sent) > goToolNameLimit {
		t.Fatalf("upstream tool name = %q, want a name of at most %d characters",
			sent, goToolNameLimit)
	}
	if len(gateway.auths) == 0 || gateway.auths[len(gateway.auths)-1] == "" {
		t.Fatalf("authorizations = %v, want the connection's own account", gateway.auths)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), goLongToolName) {
		t.Fatalf("body = %s, want the call named as the client declared it", body)
	}
}

// goAnswersWithACallToTheOfferedTool answers with a call to the first tool the
// request declared, which is the name the model was offered.
func goAnswersWithACallToTheOfferedTool(w http.ResponseWriter, body map[string]any) {
	w.Header().Set("Content-Type", "text/event-stream")
	name := offeredToolName(body)
	_, _ = io.WriteString(w, strings.Join([]string{
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_go_1",` +
			`"type":"function","function":{"name":"` + name + `","arguments":"{}"}}]}}]}`,
		``,
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],` +
			`"usage":{"prompt_tokens":12,"completion_tokens":3}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n"))
}

// goToolRequest declares a tool past the gateway's limit and asks for one.
func goToolRequest() string {
	return `{"model":"deepseek-v4.1-flash","stream":true,"messages":[{"role":"user",` +
		`"content":"list the files"}],"tools":[{"type":"function","function":{"name":"` +
		goLongToolName + `","description":"list files","parameters":{"type":"object","properties":{}}}}]}`
}

// startOpenCodeGoDaemon runs a daemon whose only connection is the paid Go
// lane, which is what makes the connection's own identity the policy in play.
func startOpenCodeGoDaemon(t *testing.T, upstreamURL string) daemon {
	t.Helper()
	return startDaemonSeeded(t, seed{
		Provider: sqlite.ProviderRow{
			ID: catalog.OpenCodeGoTemplate, TemplateID: catalog.OpenCodeGoTemplate,
			Origin: string(catalog.OriginTemplate), Label: "OpenCode Go",
			Auth: string(catalog.AuthAPIKey), KeyHeader: string(catalog.KeyHeaderBearer),
			APIFormat: string(catalog.FormatOpenAIChat), BaseURL: upstreamURL + "/v1",
			ModelsFormat: string(catalog.ModelsOpenAI), ModelsSource: "listing",
			Enabled: true, Rank: 100, PoolStrategy: sqlite.StrategyLeastLoaded,
		},
		Format:    catalog.FormatOpenAIChat,
		ModelRows: []seedModel{{ID: "deepseek-v4.1-flash"}},
		Account:   "sk-test",
	})
}
