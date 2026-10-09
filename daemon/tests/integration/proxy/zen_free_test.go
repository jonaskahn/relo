package proxy_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
)

// The gateway answers the free lane only as a stream, only to a request whose
// conversation identity is the shape it accepts, and only under the public
// credential. A mock that enforces exactly that is what proves Relo's request
// satisfies the lane, rather than proving only that Relo can send a body.
const (
	zenSessionShape = "^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$"
	zenRequestShape = "^msg_[0-9a-f]{12}[0-9A-Za-z]{14}$"
)

// zenGateway is a mock of the free lane that refuses what the gateway refuses.
type zenGateway struct {
	server   *httptest.Server
	requests int
	//
	// last is everything one accepted request carried, so a test can assert on
	// the identity the relay built rather than only on the reply it got.
	last zenAttempt
}

type zenAttempt struct {
	path    string
	headers http.Header
	body    map[string]any
}

// zenLaneTools are the tools the lane inspects for on every model except the
// one it lets through without them. A coding client sends exactly these, which
// is why a native tool set satisfies the lane without Relo adding anything.
var zenLaneTools = []string{"bash", "glob", "grep", "read"}

// zenToolFreeModel is the one model the lane answers without tools declared,
// which is what makes it the lane's only tool-free answer.
const zenToolFreeModel = "space-bunny-free"

// zenToolsSatisfyLane reports whether the request declares the tools the lane
// expects. Every other model it measured refused a request without them, and an
// empty array is not a waiver, so the mock holds that rule.
func zenToolsSatisfyLane(body map[string]any) bool {
	if model, _ := body["model"].(string); model == zenToolFreeModel {
		return true
	}
	declared, _ := body["tools"].([]any)
	for _, want := range zenLaneTools {
		if !zenDeclares(declared, want) {
			return false
		}
	}
	return true
}

func newZenGateway(t *testing.T, answer func(w http.ResponseWriter, attempt zenAttempt)) *zenGateway {
	t.Helper()
	gateway := &zenGateway{}
	gateway.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gateway.requests++
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[{"id":"space-bunny-free"},`+
				`{"id":"muse-spark-1.3-contributor-free"}]}`)
			return
		}
		body := decodeZenBody(t, r)
		gateway.last = zenAttempt{path: r.URL.Path, headers: r.Header.Clone(), body: body}

		if refuse := zenRefusal(r, body); refuse != "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"FreeTierError","message":"`+refuse+`"}}`)
			return
		}
		answer(w, gateway.last)
	}))
	t.Cleanup(gateway.server.Close)
	return gateway
}

func (g *zenGateway) URL() string {
	return g.server.URL
}

// TestZenFreeServesAToolCallAndItsResult covers the turn a coding client makes:
// it declares its own tools, the model calls one, the client answers with the
// result, and the model replies. Relo forwards each of those; it never runs the
// tool itself, so the gateway sees the client's own declarations untouched.
func TestZenFreeServesAToolCallAndItsResult(t *testing.T) {
	gateway := newZenGateway(t, zenAnswerToolTurn)
	daemon := startZenFreeDaemon(t, gateway.URL(), zenFreeModel{ID: "mimo-v2.6-flash-free"})

	t.Run("the model calls a tool the client declared", func(t *testing.T) {
		response := send(t, daemon.dataPlane, "/v1/chat/completions", dataPlaneToken, zenToolRequest())
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(response.Body)
			t.Fatalf("status = %d, body = %s", response.StatusCode, body)
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !strings.Contains(string(body), `"tool_calls"`) || !strings.Contains(string(body), `"read"`) {
			t.Fatalf("body = %s, want the tool call the model made", body)
		}
		if !strings.Contains(string(body), `"finish_reason":"tool_calls"`) {
			t.Fatalf("body = %s, want the turn reported as a tool call", body)
		}
		if !zenDeclaresCall(gateway.last, "read") {
			t.Fatalf("upstream tools = %v, want the client's own four declarations", gateway.last.body["tools"])
		}
	})

	t.Run("the result travels back with the call it answers", func(t *testing.T) {
		response := send(t, daemon.dataPlane, "/v1/chat/completions", dataPlaneToken, zenToolResultRequest())
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(response.Body)
			t.Fatalf("status = %d, body = %s", response.StatusCode, body)
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !strings.Contains(string(body), "It says hello") {
			t.Fatalf("body = %s, want the answer built from the tool result", body)
		}
		if !zenCarriesResult(gateway.last, "call_zen_1", "hello") {
			t.Fatalf("upstream messages = %v, want the result addressed to its call",
				gateway.last.body["messages"])
		}
	})
}

// zenAnswerToolTurn answers the first turn with a tool call and the second with
// text, which is the shape a tool-using conversation takes.
func zenAnswerToolTurn(w http.ResponseWriter, attempt zenAttempt) {
	w.Header().Set("Content-Type", "text/event-stream")
	if zenCarriesResult(attempt, "call_zen_1", "hello") {
		_, _ = io.WriteString(w, strings.Join([]string{
			`data: {"choices":[{"index":0,"delta":{"content":"It says hello"}}]}`,
			``,
			`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],` +
				`"usage":{"prompt_tokens":20,"completion_tokens":4}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n"))
		return
	}
	_, _ = io.WriteString(w, strings.Join([]string{
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_zen_1",` +
			`"type":"function","function":{"name":"read","arguments":"{\"filePath\":\"notes.md\"}"}}]}}]}`,
		``,
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],` +
			`"usage":{"prompt_tokens":18,"completion_tokens":9}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n"))
}

// zenDeclaresCall reports whether the upstream request still carries the client's
// own tool declarations rather than a set Relo supplied.
func zenDeclaresCall(attempt zenAttempt, name string) bool {
	declared, _ := attempt.body["tools"].([]any)
	return len(declared) == len(zenLaneTools) && zenDeclares(declared, name)
}

// zenCarriesResult reports whether a request carries the result of one call,
// which is how a tool-using turn tells Relo what actually happened.
func zenCarriesResult(attempt zenAttempt, callID, content string) bool {
	messages, _ := attempt.body["messages"].([]any)
	for _, entry := range messages {
		message, _ := entry.(map[string]any)
		if message["role"] != "tool" || message["tool_call_id"] != callID {
			continue
		}
		return strings.Contains(textOf(message["content"]), content)
	}
	return false
}

// textOf reads content a wire may carry as a string or as a part list.
func textOf(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		var text strings.Builder
		for _, entry := range value {
			if part, ok := entry.(map[string]any); ok {
				if line, ok := part["text"].(string); ok {
					text.WriteString(line)
				}
			}
		}
		return text.String()
	default:
		return ""
	}
}

// zenToolRequest is a client declaring the tools a coding session sends and
// asking for one of them.
func zenToolRequest() string {
	return `{"model":"mimo-v2.6-flash-free","stream":false,"messages":[{"role":"user",` +
		`"content":"Read notes.md"}],"tools":[` + zenToolDeclarations() + `]}`
}

// zenToolResultRequest carries the result of the call the model made.
func zenToolResultRequest() string {
	return `{"model":"mimo-v2.6-flash-free","stream":false,"messages":[` +
		`{"role":"user","content":"Read notes.md"},` +
		`{"role":"assistant","content":null,"tool_calls":[{"id":"call_zen_1","type":"function",` +
		`"function":{"name":"read","arguments":"{\"filePath\":\"notes.md\"}"}}]},` +
		`{"role":"tool","tool_call_id":"call_zen_1","content":"hello"}],` +
		`"tools":[` + zenToolDeclarations() + `]}`
}

func zenToolDeclarations() string {
	declarations := make([]string, 0, len(zenLaneTools))
	for _, name := range zenLaneTools {
		declarations = append(declarations,
			`{"type":"function","function":{"name":"`+name+`","description":"`+name+` tool",`+
				`"parameters":{"type":"object","properties":{"input":{"type":"string"}},"required":["input"]}}}`)
	}
	return strings.Join(declarations, ",")
}

// zenRefusal names the first lane rule the request breaks, which is what the
// gateway answers 403 for.
// zenDeclares reports whether one declaration list names this tool, on the
// Chat wire's nested shape or the Responses wire's flat one.
func zenDeclares(declared []any, name string) bool {
	for _, entry := range declared {
		tool, _ := entry.(map[string]any)
		if tool["name"] == name {
			return true
		}
		if nested, ok := tool["function"].(map[string]any); ok && nested["name"] == name {
			return true
		}
	}
	return false
}

func zenRefusal(r *http.Request, body map[string]any) string {
	if !zenToolsSatisfyLane(body) {
		return "the free lane requires the client to declare its file tools"
	}
	if r.Header.Get("Authorization") != "Bearer public" {
		return "the free lane is used without an account"
	}
	if !strings.Contains(r.Header.Get("User-Agent"), "opencode/") {
		return "the request does not come from the OpenCode client"
	}
	for _, name := range []string{"x-opencode-client", "x-opencode-project", "x-opencode-session", "x-opencode-request"} {
		if r.Header.Get(name) == "" {
			return "the request is missing " + name
		}
	}
	if !zenIDMatches(zenSession, r.Header.Get("x-opencode-session")) {
		return "the session identifier is not the shape the gateway accepts"
	}
	if !zenIDMatches(zenRequest, r.Header.Get("x-opencode-request")) {
		return "the request identifier is not the shape the gateway accepts"
	}
	if streamed, _ := body["stream"].(bool); !streamed {
		return "the free tier can only be used from within OpenCode"
	}
	return ""
}

var zenSession = regexp.MustCompile(zenSessionShape)
var zenRequest = regexp.MustCompile(zenRequestShape)

func zenIDMatches(shape *regexp.Regexp, value string) bool {
	return shape.MatchString(value)
}

func decodeZenBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read the upstream request: %v", err)
	}
	body := map[string]any{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode the upstream request %q: %v", raw, err)
	}
	return body
}

// zenChatStream answers one streamed Chat Completions turn.
func zenChatStream(w http.ResponseWriter, _ zenAttempt) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = io.WriteString(w, strings.Join([]string{
		`data: {"choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`,
		``,
		`data: {"choices":[{"index":0,"delta":{"reasoning_content":"thinking"}}]}`,
		``,
		`data: {"choices":[{"index":0,"delta":{"content":"OK"}}]}`,
		``,
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],` +
			`"usage":{"prompt_tokens":12,"completion_tokens":3}}`,
		``,
		`data: [DONE]`,
		``,
		``, //nolint
	}, "\n"))
}

// zenResponses answers one streamed Responses turn.
func zenResponsesStream(w http.ResponseWriter, _ zenAttempt) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = io.WriteString(w, strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"OK"}`,
		``,
		`data: {"type":"response.completed","response":{"status":"completed",` +
			`"usage":{"input_tokens":12,"output_tokens":3}}}`,
		``,
	}, "\n"))
}

// TestZenFreeReachesEachModelOnItsOwnEndpoint covers the gateway publishing one
// model list across several protocols: a turn only works because the model
// reached the endpoint that serves it, and the mock refuses the wrong one.
func TestZenFreeReachesEachModelOnItsOwnEndpoint(t *testing.T) {
	t.Run("a chat model is served on the chat wire", func(t *testing.T) {
		gateway := newZenGateway(t, zenChatStream)
		daemon := startZenFreeDaemon(t, gateway.URL(), zenFreeModel{ID: "space-bunny-free"})
		response := send(t, daemon.dataPlane, "/v1/chat/completions", dataPlaneToken, zenCompleteRequest())
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(response.Body)
			t.Fatalf("status = %d, body = %s", response.StatusCode, body)
		}
		if gateway.last.path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want the chat wire", gateway.last.path)
		}
	})

	t.Run("a responses model is served on the responses wire", func(t *testing.T) {
		gateway := newZenGateway(t, zenResponsesStream)
		daemon := startZenFreeDaemon(t, gateway.URL(), zenFreeModel{
			ID: "muse-spark-1.3-contributor-free", Format: catalog.FormatOpenAIResp,
		})
		response := send(t, daemon.dataPlane, "/v1/chat/completions", dataPlaneToken, zenResponsesRequest())
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(response.Body)
			t.Fatalf("status = %d, body = %s", response.StatusCode, body)
		}
		if gateway.last.path != responsesWire {
			t.Fatalf("path = %q, want the responses wire this model serves", gateway.last.path)
		}
		if gateway.last.body["store"] != false {
			t.Fatalf("body = %v, want the store flag the gateway is given", gateway.last.body)
		}
	})
}

// TestZenFreeRefreshGivesAnExistingConnectionItsProtocols covers the connection
// Relo already built before a model's protocol was known: the refresh is what
// teaches the stored rows which endpoint serves them, so the turn after it works
// rather than failing on the wire the model does not answer on.
func TestZenFreeRefreshGivesAnExistingConnectionItsProtocols(t *testing.T) {
	gateway := newZenGateway(t, zenServeByWire)
	// The models are stored the way a connection that predates the protocol
	// mapping stored them: named, with no wire of their own.
	daemon := startZenFreeDaemon(t, gateway.URL(),
		zenFreeModel{ID: "space-bunny-free"},
		zenFreeModel{ID: "muse-spark-1.3-contributor-free"},
	)

	if _, err := daemon.service.RefreshProviderModels(context.Background(), catalog.OpenCodeFreeTemplate); err != nil {
		t.Fatalf("RefreshProviderModels() error = %v", err)
	}

	response := send(t, daemon.dataPlane, "/v1/chat/completions", dataPlaneToken, zenResponsesRequest())
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, body = %s", response.StatusCode, body)
	}
	if gateway.last.path != responsesWire {
		t.Fatalf("path = %q, want the refresh to have given this model its own wire", gateway.last.path)
	}
}

// zenServeByWire answers on the endpoint the request arrived at, so a model
// that reached the wrong wire gets no answer rather than a plausible one.
func zenServeByWire(w http.ResponseWriter, attempt zenAttempt) {
	if attempt.path == responsesWire {
		zenResponsesStream(w, attempt)
		return
	}
	zenChatStream(w, attempt)
}

// responsesWire is the endpoint the gateway serves its Responses models on.
const responsesWire = "/v1/responses"

// zenResponsesRequest names a Responses-wire model on the Chat surface, which
// is what a client does when it asks for the model by name. It declares the
// tools a coding session sends, because the lane refuses this model without them.
func zenResponsesRequest() string {
	return `{"model":"muse-spark-1.3-contributor-free","stream":false,` +
		`"messages":[{"role":"user","content":"Say OK."}],"tools":[` + zenToolDeclarations() + `]}`
}

// TestZenFreeServesACompleteAnswerFromAStreamOnlyGateway covers the free lane
// as a client meets it: the gateway refuses everything this lane does not
// accept, so a turn only works because Relo asks for the stream the gateway
// requires and hands the caller the single body it wanted.
func TestZenFreeServesACompleteAnswerFromAStreamOnlyGateway(t *testing.T) {
	gateway := newZenGateway(t, zenChatStream)
	daemon := startZenFreeDaemon(t, gateway.URL(), zenFreeModel{ID: "space-bunny-free"})
	token := dataPlaneToken

	t.Run("a client asking for one body still gets one body", func(t *testing.T) {
		response := send(t, daemon.dataPlane, "/v1/chat/completions", token, zenCompleteRequest())
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(response.Body)
			t.Fatalf("status = %d, body = %s", response.StatusCode, body)
		}
		if got := response.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want the single body the client asked for", got)
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !strings.Contains(string(body), "OK") {
			t.Fatalf("body = %s, want the answer read out of the stream", body)
		}
		if !strings.Contains(string(body), `"object":"chat.completion"`) {
			t.Fatalf("body = %s, want a complete completion", body)
		}
	})

	t.Run("the request carried the identity the lane requires", func(t *testing.T) {
		attempt := gateway.last
		if attempt.path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want the chat wire this model answers on", attempt.path)
		}
		if attempt.body["model"] != "space-bunny-free" {
			t.Fatalf("model = %v, want the routed model", attempt.body["model"])
		}
		if streamed, _ := attempt.body["stream"].(bool); !streamed {
			t.Fatalf("body = %v, want the streamed request the gateway accepts", attempt.body)
		}
		for _, name := range []string{"x-opencode-client", "x-opencode-project", "x-opencode-session", "x-opencode-request"} {
			if attempt.headers.Get(name) == "" {
				t.Errorf("header %s is missing from %v", name, attempt.headers)
			}
		}
		if !zenIDMatches(zenSession, attempt.headers.Get("x-opencode-session")) {
			t.Errorf("session = %q, want the shape the gateway accepts",
				attempt.headers.Get("x-opencode-session"))
		}
		if !zenIDMatches(zenRequest, attempt.headers.Get("x-opencode-request")) {
			t.Errorf("request = %q, want the shape the gateway accepts",
				attempt.headers.Get("x-opencode-request"))
		}
	})

	t.Run("a streaming client is answered as a stream", func(t *testing.T) {
		response := send(t, daemon.dataPlane, "/v1/chat/completions", token, zenStreamingRequest())
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.StatusCode)
		}
		if got := response.Header.Get("Content-Type"); got != "text/event-stream" {
			t.Fatalf("Content-Type = %q, want the client's own stream", got)
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatalf("read stream: %v", err)
		}
		if !strings.Contains(string(body), "OK") || !strings.Contains(string(body), "data: [DONE]") {
			t.Fatalf("body = %s, want the relayed frames", body)
		}
	})
}

// TestZenFreeDeclaresToolsForAToolFreeTurn covers the plain-chat symptom:
// the client sent no tools, so Relo declares the names the lane inspects
// rather than forwarding the refusal the gateway would answer.
func TestZenFreeDeclaresToolsForAToolFreeTurn(t *testing.T) {
	t.Run("a chat model answers a tool-free turn", func(t *testing.T) {
		gateway := newZenGateway(t, zenChatStream)
		daemon := startZenFreeDaemon(t, gateway.URL(), zenFreeModel{ID: "mimo-v2.6-flash-free"})
		response := send(t, daemon.dataPlane, "/v1/chat/completions", dataPlaneToken,
			`{"model":"mimo-v2.6-flash-free","stream":false,"messages":[{"role":"user","content":"Say OK."}]}`)
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(response.Body)
			t.Fatalf("status = %d, body = %s", response.StatusCode, body)
		}
		declared, _ := gateway.last.body["tools"].([]any)
		for _, want := range zenLaneTools {
			if !zenDeclares(declared, want) {
				t.Fatalf("upstream tools = %v, want the declaration the lane inspects", gateway.last.body["tools"])
			}
		}
		if choice, _ := gateway.last.body["tool_choice"].(string); choice != "none" {
			t.Fatalf("tool_choice = %v, want tool calls forbidden", gateway.last.body["tool_choice"])
		}
		body, _ := io.ReadAll(response.Body)
		if !strings.Contains(string(body), "OK") {
			t.Fatalf("body = %s, want the answer read out of the stream", body)
		}
	})

	t.Run("a responses model answers a tool-free turn", func(t *testing.T) {
		gateway := newZenGateway(t, zenResponsesStream)
		daemon := startZenFreeDaemon(t, gateway.URL(), zenFreeModel{
			ID: "muse-spark-1.3-contributor-free", Format: catalog.FormatOpenAIResp,
		})
		response := send(t, daemon.dataPlane, "/v1/chat/completions", dataPlaneToken,
			`{"model":"muse-spark-1.3-contributor-free","stream":false,"messages":[{"role":"user","content":"Say OK."}]}`)
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(response.Body)
			t.Fatalf("status = %d, body = %s", response.StatusCode, body)
		}
		if gateway.last.path != responsesWire {
			t.Fatalf("path = %q, want the responses wire this model serves", gateway.last.path)
		}
		if !zenToolsSatisfyLane(gateway.last.body) {
			t.Fatalf("upstream tools = %v, want the declarations the lane inspects", gateway.last.body["tools"])
		}
	})

	t.Run("capitalized client tools gain the lowercase declarations", func(t *testing.T) {
		gateway := newZenGateway(t, zenChatStream)
		daemon := startZenFreeDaemon(t, gateway.URL(), zenFreeModel{ID: "mimo-v2.6-flash-free"})
		declarations := []string{}
		for _, name := range []string{"Bash", "Glob", "Grep", "Read"} {
			declarations = append(declarations,
				`{"type":"function","function":{"name":"`+name+`","description":"`+name+` tool",`+
					`"parameters":{"type":"object","properties":{}}}}`)
		}
		response := send(t, daemon.dataPlane, "/v1/chat/completions", dataPlaneToken,
			`{"model":"mimo-v2.6-flash-free","stream":false,"messages":[{"role":"user","content":"Say OK."}],`+
				`"tools":[`+strings.Join(declarations, ",")+`]}`)
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(response.Body)
			t.Fatalf("status = %d, body = %s", response.StatusCode, body)
		}
		declared, _ := gateway.last.body["tools"].([]any)
		for _, want := range append([]string{"Bash", "Glob", "Grep", "Read"}, zenLaneTools...) {
			if !zenDeclares(declared, want) {
				t.Fatalf("upstream tools = %v, want originals kept and lowercase appended", gateway.last.body["tools"])
			}
		}
	})
}

// zenFreeModel is one model on the keyless lane, with the upstream wire it
// answers on. An empty format means the connection's own default applies.
type zenFreeModel struct {
	ID     string
	Format catalog.APIFormat
}

// startZenFreeDaemon runs a daemon whose only connection is the keyless Zen
// lane, which is what makes the template's own policy the only policy in play.
func startZenFreeDaemon(t *testing.T, upstreamURL string, models ...zenFreeModel) daemon {
	t.Helper()
	return startDaemonSeeded(t, seed{
		Provider: sqlite.ProviderRow{
			ID: catalog.OpenCodeFreeTemplate, TemplateID: catalog.OpenCodeFreeTemplate,
			Origin: string(catalog.OriginTemplate), Label: "Opencode Free",
			Auth: string(catalog.AuthNone), KeyHeader: string(catalog.KeyHeaderNone),
			APIFormat: string(catalog.FormatOpenAIChat), BaseURL: upstreamURL + "/v1",
			ModelsFormat: string(catalog.ModelsOpenAI), ModelsSource: "listing",
			Enabled: true, Rank: 100, PoolStrategy: sqlite.StrategyLeastLoaded,
		},
		Format:    catalog.FormatOpenAIChat,
		ModelRows: zenFreeModelRows(models),
	})
}

// zenFreeModelRows renders the stored rows a discovery writes, so a test can
// seed the exact state discovery and a refresh both produce.
func zenFreeModelRows(models []zenFreeModel) []seedModel {
	rows := make([]seedModel, 0, len(models))
	for _, model := range models {
		format := model.Format
		if format == "" {
			format = catalog.FormatOpenAIChat
		}
		rows = append(rows, seedModel{ID: model.ID, Format: format})
	}
	return rows
}
