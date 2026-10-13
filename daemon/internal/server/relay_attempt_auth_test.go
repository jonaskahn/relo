package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/routing"
)

// stubReader is a catalog holding exactly one connection, which is what the
// auth contract of an attempt is read from.
type stubReader struct {
	connection catalog.ConnectionRecord
}

func (r stubReader) ListConnections(context.Context) ([]catalog.ConnectionRecord, error) {
	return []catalog.ConnectionRecord{r.connection}, nil
}
func (stubReader) ListModels(context.Context) ([]catalog.ModelRecord, error) { return nil, nil }
func (stubReader) ListModelFacts(context.Context) ([]catalog.FactsRecord, error) {
	return nil, nil
}
func (stubReader) ListRoutes(context.Context) ([]catalog.RouteRecord, error) { return nil, nil }

type stubFormats struct{}

func (stubFormats) Supports(catalog.APIFormat) bool { return true }

// recordingRelay answers every authorization with a keyless authorization, and
// keeps what the attempt asked for so a test can read the contract back.
type recordingRelay struct {
	request catalog.AuthRequest
	err     error
}

func (r *recordingRelay) Authorize(_ context.Context, request catalog.AuthRequest) (catalog.Authorization, error) {
	r.request = request
	if r.err != nil {
		return catalog.Authorization{}, r.err
	}
	return catalog.Authorization{Label: "keyless", Auth: catalog.AuthNone}, nil
}
func (r *recordingRelay) RefreshRejected(context.Context, string, string, string) (catalog.Authorization, error) {
	return catalog.Authorization{}, nil
}
func (r *recordingRelay) Report(context.Context, Outcome) error { return nil }
func (r *recordingRelay) Execute(context.Context, Exchange, http.ResponseWriter) (Result, error) {
	return Result{}, nil
}

func keylessServer(t *testing.T, relay Relay, connection catalog.ConnectionRecord) *Server {
	t.Helper()
	models := catalog.New(stubReader{connection: connection}, nil, stubFormats{})
	if err := models.Reload(t.Context()); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	return &Server{opts: Options{Catalog: models, Relay: relay}}
}

func keylessSpec(providerID string) attemptSpec {
	return attemptSpec{
		request:   httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil),
		candidate: routing.Candidate{ProviderID: providerID, ModelID: "m", Model: catalog.Model{ID: "m"}},
		outcome:   &requestOutcome{request: &inference.Request{Model: "m"}, requestID: "req-1"},
	}
}

// TestResolveAttemptCandidateAuthorizesTheCatalogConnection pins that an attempt
// asks for its credential under the connection the catalog holds. The caller
// holds no connection of its own - it resolved this one - so an auth request
// built from anything else names an empty auth kind, and a keyless lane then
// asks the pool for a credential it never had.
func TestResolveAttemptCandidateAuthorizesTheCatalogConnection(t *testing.T) {
	connection := catalog.ConnectionRecord{
		ID: catalog.OpenCodeFreeTemplate, TemplateID: catalog.OpenCodeFreeTemplate,
		Origin: string(catalog.OriginTemplate), Label: "Opencode Free",
		Auth: string(catalog.AuthNone), KeyHeader: string(catalog.KeyHeaderNone),
		APIFormat: string(catalog.FormatOpenAIChat), BaseURL: "https://opencode.ai/zen/v1",
		ModelsFormat: string(catalog.ModelsOpenAI), Enabled: true, Rank: 100,
		PoolStrategy: "least-loaded",
	}

	t.Run("a keyless lane is served without a credential", func(t *testing.T) {
		relay := &recordingRelay{}
		host, authorized, err := keylessServer(t, relay, connection).
			resolveAttemptCandidate(keylessSpec(connection.ID), map[string][]string{})
		if err != nil {
			t.Fatalf("resolveAttemptCandidate() error = %v", err)
		}
		if relay.request.Auth != catalog.AuthNone {
			t.Fatalf("auth = %q, want the connection's own keyless kind", relay.request.Auth)
		}
		if relay.request.ProviderID != connection.ID {
			t.Fatalf("provider = %q, want %q", relay.request.ProviderID, connection.ID)
		}
		if host.ID != connection.ID || authorized.Label != "keyless" {
			t.Fatalf("host = %q authorization = %+v, want the connection served keylessly", host.ID, authorized)
		}
	})

	t.Run("the connection's key header travels with the request", func(t *testing.T) {
		relay := &recordingRelay{}
		apiKey := connection
		apiKey.Auth, apiKey.KeyHeader = string(catalog.AuthAPIKey), string(catalog.KeyHeaderXGoogApiKey)
		if _, _, err := keylessServer(t, relay, apiKey).
			resolveAttemptCandidate(keylessSpec(apiKey.ID), map[string][]string{}); err != nil {
			t.Fatalf("resolveAttemptCandidate() error = %v", err)
		}
		if relay.request.Auth != catalog.AuthAPIKey || relay.request.KeyHeader != catalog.KeyHeaderXGoogApiKey {
			t.Fatalf("auth = %q key header = %q, want the connection's own settings",
				relay.request.Auth, relay.request.KeyHeader)
		}
	})

	t.Run("a pool with nothing to give is reported", func(t *testing.T) {
		relay := &recordingRelay{err: account.ErrNoCredentials}
		_, _, err := keylessServer(t, relay, connection).
			resolveAttemptCandidate(keylessSpec(connection.ID), map[string][]string{})
		if !errors.Is(err, account.ErrNoCredentials) {
			t.Fatalf("error = %v, want ErrNoCredentials", err)
		}
	})

	t.Run("a candidate the catalog dropped is refused", func(t *testing.T) {
		relay := &recordingRelay{}
		if _, _, err := keylessServer(t, relay, connection).
			resolveAttemptCandidate(keylessSpec("no-such-connection"), map[string][]string{}); err == nil {
			t.Fatal("resolveAttemptCandidate() accepted a connection the catalog does not hold")
		}
	})
}
