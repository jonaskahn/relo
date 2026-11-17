package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/config"
	appproxy "github.com/jonaskahn/relo/internal/proxy"
)

// TestProxyContextStateNeedsAConnection covers the per-connection state: a
// context without one reads as no sticky proxy, and pinning or unpinning it is
// a no-op rather than a panic.
func TestProxyContextStateNeedsAConnection(t *testing.T) {
	server := New(Options{})
	plain := context.Background()
	if state := proxyConnFrom(plain); state != nil {
		t.Fatalf("proxyConnFrom() = %+v, want nil", state)
	}
	if server.stickyProxy(plain) != "" {
		t.Fatal("stickyProxy() without a connection = non-empty")
	}
	server.pinProxy(plain, "one")
	server.unpinProxy(plain, "one")
	if server.stickyProxy(plain) != "" {
		t.Fatal("a pin on a plain context survived")
	}

	// With a connection the state is shared, and an unpin of another id keeps
	// the pin this connection holds.
	ctx := withProxyConn(plain, nil)
	server.pinProxy(ctx, "one")
	if got := server.stickyProxy(ctx); got != "one" {
		t.Fatalf("stickyProxy() = %q, want the pinned id", got)
	}
	server.unpinProxy(ctx, "two")
	if got := server.stickyProxy(ctx); got != "one" {
		t.Fatalf("an unpin of another id dropped the pin: %q", got)
	}
	server.unpinProxy(ctx, "one")
	if got := server.stickyProxy(ctx); got != "" {
		t.Fatalf("stickyProxy() after its own unpin = %q", got)
	}
}

// TestSelectProxyReportsAnUnreadablePool covers the error the pool's own read
// can return, which the caller treats as a failure rather than a direct
// connection.
func TestSelectProxyReportsAnUnreadablePool(t *testing.T) {
	broken := appproxy.New(appproxy.Options{
		Store:   &brokenProxyStore{},
		Secrets: &proxyTestSecrets{values: map[string]string{}},
	})
	server := New(Options{Proxies: broken})
	if _, err := server.selectProxy(context.Background(), true, nil); err == nil {
		t.Fatal("selectProxy() over an unreadable pool error = nil, want the failure")
	}
}

type brokenProxyStore struct{}

var errBrokenStore = errors.New("the database is closed")

func (s *brokenProxyStore) List(context.Context) ([]appproxy.Entry, error) {
	return nil, errBrokenStore
}
func (s *brokenProxyStore) Get(context.Context, string) (appproxy.Entry, bool, error) {
	return appproxy.Entry{}, false, errBrokenStore
}
func (s *brokenProxyStore) Create(context.Context, appproxy.Entry) (appproxy.Entry, error) {
	return appproxy.Entry{}, errBrokenStore
}
func (s *brokenProxyStore) Update(context.Context, appproxy.Entry) error { return errBrokenStore }
func (s *brokenProxyStore) Delete(context.Context, string) error         { return errBrokenStore }
func (s *brokenProxyStore) Count(context.Context) (int, error)           { return 0, errBrokenStore }

// TestSelectProxyGoesDirectWhenEveryEntryIsDisabled covers the refusal the
// pool reports for a pool with no enabled proxy: a connection that asks for
// one goes direct rather than failing.
func TestSelectProxyGoesDirectWhenEveryEntryIsDisabled(t *testing.T) {
	service, _ := testProxyService(t)
	if _, err := service.Create(context.Background(), appproxy.Draft{
		Scheme: "http", Host: "127.0.0.1", Port: 7890, Enabled: false,
	}); err != nil {
		t.Fatal(err)
	}
	server := New(Options{Proxies: service})
	selection, err := server.selectProxy(context.Background(), true, nil)
	if err != nil || selection.ID != "" {
		t.Fatalf("selectProxy() with a disabled-only pool = %#v, %v", selection, err)
	}
}

// TestCoolProxyNeedsASelectionAndAPool covers the two guards: an empty
// selection and a server with no pool both make cooling a no-op.
func TestCoolProxyNeedsASelectionAndAPool(t *testing.T) {
	ctx := withProxyConn(context.Background(), nil)
	New(Options{}).coolProxy(ctx, appproxy.Selection{}, time.Minute)
	server := New(Options{})
	server.coolProxy(ctx, appproxy.Selection{ID: "one"}, time.Minute)
	if server.stickyProxy(ctx) != "" {
		t.Fatal("cooling without a pool pinned something")
	}
}

// TestProxyContextCarriesTheSelection covers the context a selected proxy
// adds: an empty selection leaves it untouched, and a URL is what the relay
// reads back.
func TestProxyContextCarriesTheSelection(t *testing.T) {
	server := New(Options{})
	plain := context.Background()
	if got := server.proxyContext(plain, appproxy.Selection{}); got != plain {
		t.Fatal("proxyContext() with no URL replaced the context")
	}
	carried := server.proxyContext(plain, appproxy.Selection{ID: "one", URL: "http://127.0.0.1:7890"})
	if raw := config.OutboundProxyFrom(carried); raw != "http://127.0.0.1:7890" {
		t.Fatalf("outbound proxy = %q, want the selection's URL", raw)
	}
}

// TestProxyCoolingRefusalAnswersBothSurfaces covers the two shapes the cooling
// refusal takes: Relo's own error document and Anthropic's, which carries a
// Retry-After the client honours.
func TestProxyCoolingRefusalAnswersBothSurfaces(t *testing.T) {
	service, _ := testProxyService(t)
	created, err := service.Create(context.Background(), appproxy.Draft{
		Scheme: "http", Host: "127.0.0.1", Port: 7890, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	service.Cool(created.ID, 90*time.Second)
	server := New(Options{Proxies: service})
	_, cause := server.selectProxy(context.Background(), true, nil)
	if !errors.Is(cause, appproxy.ErrAllCooling) {
		t.Fatalf("selectProxy() = %v, want all cooling", cause)
	}

	// A cause that is not the pool's own error waits the default second.
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	server.proxyCoolingRefusal(recorder, request, false, errors.New("something else"))
	if recorder.Code != http.StatusServiceUnavailable || recorder.Header().Get("Retry-After") != "1" {
		t.Fatalf("default wait = %d, Retry-After %q", recorder.Code, recorder.Header().Get("Retry-After"))
	}

	anthropic := httptest.NewRecorder()
	server.proxyCoolingRefusal(anthropic, request, true, cause)
	if anthropic.Code != http.StatusServiceUnavailable {
		t.Fatalf("anthropic status = %d, want 503", anthropic.Code)
	}
	// The document itself is the formats codec's, which a server without one
	// leaves empty; what this refusal owns is the status and the wait.
	if wait := anthropic.Header().Get("Retry-After"); wait != "90" {
		t.Fatalf("Retry-After = %q, want the pool's 90 seconds", wait)
	}
}

// TestProxyContextStateSurvivesARequest covers the state the server stores on
// a connection: a listener's accept loop makes one per connection.
func TestProxyContextStateSurvivesARequest(t *testing.T) {
	left, right := net.Pipe()
	defer func() { _ = left.Close() }()
	defer func() { _ = right.Close() }()
	ctx := withProxyConn(context.Background(), left)
	state := proxyConnFrom(ctx)
	if state == nil {
		t.Fatal("withProxyConn() stored no state")
	}
	server := New(Options{})
	server.pinProxy(ctx, "one")
	if got := server.stickyProxy(ctx); got != "one" {
		t.Fatalf("stickyProxy() = %q", got)
	}
}
