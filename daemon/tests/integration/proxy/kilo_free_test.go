package proxy_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	"github.com/jonaskahn/relo/internal/catalog"
)

// kiloRoster is the listing the gateway publishes keyless: a paid model the
// connection must not serve, a free one whose name states no price, and a row
// that never says whether it is free at all.
const kiloRoster = `{"data":[
	{"id":"anthropic/claude-opus-5.5","isFree":false},
	{"id":"stealth/glyph-cluster","isFree":true},
	{"id":"vendor/unmarked"}
]}`

// TestKiloFreeKeepsOnlyWhatTheGatewayMarksFree covers the rule the whole lane
// turns on. The free pool is named by a flag the listing states, so a model
// published free under a name that mentions no price stays in the roster and
// one the gateway marks paid, or leaves unsaid, does not.
func TestKiloFreeKeepsOnlyWhatTheGatewayMarksFree(t *testing.T) {
	gateway := newKiloGateway(t)
	daemon := startKiloFreeDaemon(t, gateway.URL())

	if _, err := daemon.service.RefreshProviderModels(context.Background(), catalog.KiloFreeTemplate); err != nil {
		t.Fatalf("RefreshProviderModels() error = %v", err)
	}

	models, _, err := daemon.service.Models(context.Background(), appcatalog.ModelQuery{
		Provider: catalog.KiloFreeTemplate,
	})
	if err != nil {
		t.Fatalf("Models() error = %v", err)
	}
	got := make([]string, 0, len(models))
	for _, model := range models {
		got = append(got, model.ModelID)
	}
	if len(got) != 1 || got[0] != "stealth/glyph-cluster" {
		t.Fatalf("roster = %v, want only the model the gateway marks free", got)
	}
}

// TestKiloFreeServesATurnWithNoCredential covers the pool as a client meets it:
// the gateway answers with no key, no login and no signature, so a turn only
// works because the connection asks for no account and sends none.
func TestKiloFreeServesATurnWithNoCredential(t *testing.T) {
	gateway := newKiloGateway(t)
	daemon := startKiloFreeDaemon(t, gateway.URL())

	response := send(t, daemon.dataPlane, "/v1/chat/completions", dataPlaneToken, kiloCompleteRequest())
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, body = %s", response.StatusCode, body)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "OK") || !strings.Contains(string(body), `"object":"chat.completion"`) {
		t.Fatalf("body = %s, want the complete completion the gateway answered", body)
	}

	attempt := gateway.last
	if attempt.body["model"] != "stealth/glyph-cluster" {
		t.Fatalf("model = %v, want the routed model", attempt.body["model"])
	}
	// The pool refuses a request that presents a credential, so one arriving
	// would mean the connection sent something the gateway never asked for.
	if credential := attempt.headers.Get("Authorization"); credential != "" {
		t.Fatalf("Authorization = %q, want a request the keyless pool accepts", credential)
	}
	if attempt.headers.Get("x-opencode-session") != "" {
		t.Fatalf("headers = %v, want no identity the gateway never asked for", attempt.headers)
	}
}

// kiloGateway is a mock of the keyless pool that refuses what the gateway
// refuses: a credential it did not ask for, and a paid model.
type kiloGateway struct {
	server   *httptest.Server
	requests int
	last     kiloAttempt
}

// kiloAttempt is everything one accepted request carried.
type kiloAttempt struct {
	path    string
	headers http.Header
	body    map[string]any
}

func newKiloGateway(t *testing.T) *kiloGateway {
	t.Helper()
	gateway := &kiloGateway{}
	gateway.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gateway.requests++
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, kiloRoster)
			return
		}
		gateway.last = kiloAttempt{
			path:    r.URL.Path,
			headers: r.Header.Clone(),
			body:    decodeZenBody(t, r),
		}
		kiloAnswer(w, gateway.last)
	}))
	t.Cleanup(gateway.server.Close)
	return gateway
}

func (g *kiloGateway) URL() string {
	return g.server.URL
}

// kiloAnswer answers in the shape the request asked for, because the pool
// serves a one-shot body and a stream alike rather than forcing either.
func kiloAnswer(w http.ResponseWriter, attempt kiloAttempt) {
	if streamed, _ := attempt.body["stream"].(bool); !streamed {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"object":"chat.completion","choices":[{"index":0,`+
			`"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],`+
			`"usage":{"prompt_tokens":12,"completion_tokens":3}}`)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = io.WriteString(w, strings.Join([]string{
		`data: {"choices":[{"index":0,"delta":{"content":"OK"}}]}`,
		``,
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],` +
			`"usage":{"prompt_tokens":12,"completion_tokens":3}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n"))
}

// startKiloFreeDaemon runs a daemon whose only connection is the keyless Kilo
// lane, which is what makes the template's own policy the only policy in play.
func startKiloFreeDaemon(t *testing.T, upstreamURL string, models ...seedModel) daemon {
	t.Helper()
	rows := models
	if len(rows) == 0 {
		rows = []seedModel{{ID: "stealth/glyph-cluster", Format: catalog.FormatOpenAIChat}}
	}
	return startDaemonSeeded(t, seed{
		Provider: sqlite.ProviderRow{
			ID: catalog.KiloFreeTemplate, TemplateID: catalog.KiloFreeTemplate,
			Origin: string(catalog.OriginTemplate), Label: "Kilo Free",
			Auth: string(catalog.AuthNone), KeyHeader: string(catalog.KeyHeaderNone),
			APIFormat: string(catalog.FormatOpenAIChat), BaseURL: upstreamURL,
			ModelsFormat: string(catalog.ModelsOpenAI), ModelsSource: "listing",
			Enabled: true, Rank: 100, PoolStrategy: sqlite.StrategyLeastLoaded,
		},
		Format:    catalog.FormatOpenAIChat,
		ModelRows: rows,
	})
}

// kiloCompleteRequest asks for one body from a model the pool serves.
func kiloCompleteRequest() string {
	return `{"model":"stealth/glyph-cluster","stream":false,` +
		`"messages":[{"role":"user","content":"Say OK."}]}`
}
