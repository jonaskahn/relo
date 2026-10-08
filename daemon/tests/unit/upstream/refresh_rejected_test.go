package dispatch_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/adapters/templates"
	"github.com/jonaskahn/relo/internal/adapters/upstream"
	"github.com/jonaskahn/relo/internal/catalog"
)

func TestExecuteRetriesClaudeUnauthorizedOnce(t *testing.T) {
	var calls atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") == "Bearer stale" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	t.Cleanup(origin.Close)

	exchange := completeExchange(origin.URL)
	exchange.ProviderID = oauth.FlowClaude
	exchange.Final = false
	exchange.Options.CredentialRef = "stale"
	exchange.OnUnauthorized = func(context.Context) (catalog.Authorization, error) {
		return catalog.Authorization{Token: "fresh"}, nil
	}
	result, err := newExecutor(t).Execute(context.Background(), exchange, httptest.NewRecorder())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != http.StatusOK || result.RefreshSettled {
		t.Fatalf("result status = %d settled = %v, want a served retry", result.Status, result.RefreshSettled)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}

func TestExecuteSettlesAFailedRefresh(t *testing.T) {
	var calls atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(origin.Close)
	exchange := completeExchange(origin.URL)
	exchange.ProviderID = oauth.FlowClaude
	exchange.OnUnauthorized = func(context.Context) (catalog.Authorization, error) {
		return catalog.Authorization{}, errors.New("token endpoint is down")
	}
	result, err := newExecutor(t).Execute(context.Background(), exchange, httptest.NewRecorder())
	if err == nil {
		t.Fatal("Execute() error = nil, want the original refusal")
	}
	if result.Status != http.StatusUnauthorized || !result.RefreshSettled {
		t.Fatalf("result status = %d settled = %v, want a settled 401", result.Status, result.RefreshSettled)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want the refused request only", calls.Load())
	}
}

func TestRefreshRejected(t *testing.T) {
	ctx := context.Background()
	entry := account.PoolEntry{ID: "cred-1", ProviderID: oauth.FlowClaude, Label: "Claude", SecretRef: "secret"}
	pools := &memoryPools{entries: map[string]account.PoolEntry{entry.ID: entry}}
	secrets := &memorySecrets{values: map[string]string{
		"secret": `{"access_token":"stale","refresh_token":"refresh"}`,
	}}
	flows := &memoryFlows{next: &oauth.OAuthCredential{AccessToken: "fresh", RefreshToken: "rotated"}}
	source := upstream.New(upstream.Options{
		Pools: pools, Secrets: secrets, Flows: flows, Adapters: templateAdapters{},
	})

	t.Run("rotates a refused token", func(t *testing.T) {
		auth, err := source.RefreshRejected(ctx, oauth.FlowClaude, entry.ID, "stale")
		if err != nil {
			t.Fatalf("RefreshRejected() error = %v", err)
		}
		if auth.Token != "fresh" || flows.calls != 1 {
			t.Fatalf("token = %q calls = %d, want the rotated token from one refresh", auth.Token, flows.calls)
		}
		if got := secrets.values["secret"]; !strings.Contains(got, "rotated") {
			t.Fatalf("stored = %s, want the rotated refresh token", got)
		}
	})

	t.Run("reuses a token another request already stored", func(t *testing.T) {
		before := flows.calls
		auth, err := source.RefreshRejected(ctx, oauth.FlowClaude, entry.ID, "stale")
		if err != nil {
			t.Fatalf("RefreshRejected() error = %v", err)
		}
		if auth.Token != "fresh" || flows.calls != before {
			t.Fatalf("token = %q calls = %d, want the stored token without a refresh", auth.Token, flows.calls)
		}
	})

	t.Run("a rejected refresh asks for a new login", func(t *testing.T) {
		secrets.values["secret"] = `{"access_token":"stale","refresh_token":"refresh"}`
		flows.err = oauth.ErrRefreshRejected
		_, err := source.RefreshRejected(ctx, oauth.FlowClaude, entry.ID, "stale")
		if !errors.Is(err, upstream.ErrReauthRequired) || len(pools.reauth) != 1 {
			t.Fatalf("error = %v marked = %v, want a reauth", err, pools.reauth)
		}
	})

	t.Run("a transport failure leaves the account active", func(t *testing.T) {
		pools.reauth = nil
		flows.err = errors.New("token endpoint is down")
		_, err := source.RefreshRejected(ctx, oauth.FlowClaude, entry.ID, "stale")
		if err == nil || errors.Is(err, upstream.ErrReauthRequired) || len(pools.reauth) != 0 {
			t.Fatalf("error = %v marked = %v, want the transport failure", err, pools.reauth)
		}
	})

	t.Run("another sign-in provider rotates a refused token", func(t *testing.T) {
		codex := account.PoolEntry{ID: "cred-2", ProviderID: "openai-codex", Label: "ChatGPT", SecretRef: "codex-secret"}
		pools.entries[codex.ID] = codex
		secrets.values["codex-secret"] = `{"access_token":"stale","refresh_token":"refresh"}`
		flows.err = nil
		flows.next = &oauth.OAuthCredential{AccessToken: "fresh", RefreshToken: "rotated"}
		auth, err := source.RefreshRejected(ctx, "openai-codex", codex.ID, "stale")
		if err != nil {
			t.Fatalf("RefreshRejected() error = %v", err)
		}
		if auth.Token != "fresh" {
			t.Fatalf("token = %q, want the rotated token", auth.Token)
		}
		if got := secrets.values["codex-secret"]; !strings.Contains(got, "rotated") {
			t.Fatalf("stored = %s, want the rotated refresh token", got)
		}
	})

	t.Run("a provider with no login flow keeps the refusal", func(t *testing.T) {
		key := account.PoolEntry{ID: "key-1", ProviderID: "no-flow", SecretRef: "key-secret"}
		pools.entries[key.ID] = key
		secrets.values["key-secret"] = `{"access_token":"stale","refresh_token":"refresh"}`
		flows.noFlow = map[string]bool{"no-flow": true}
		defer func() { flows.noFlow = nil }()
		_, err := source.RefreshRejected(ctx, "no-flow", key.ID, "stale")
		if !errors.Is(err, oauth.ErrFlowNotFound) {
			t.Fatalf("RefreshRejected() error = %v, want %v", err, oauth.ErrFlowNotFound)
		}
	})

	t.Run("a rotation a peer stored is adopted", func(t *testing.T) {
		pools.reauth = nil
		secrets.values["secret"] = `{"access_token":"stale","refresh_token":"refresh"}`
		flows.err = oauth.ErrRefreshRejected
		flows.onRefresh = func() {
			secrets.values["secret"] = `{"access_token":"peer-fresh","refresh_token":"peer-rotated"}`
		}
		defer func() { flows.onRefresh = nil; flows.err = nil }()
		auth, err := source.RefreshRejected(ctx, oauth.FlowClaude, entry.ID, "stale")
		if err != nil {
			t.Fatalf("RefreshRejected() error = %v", err)
		}
		if auth.Token != "peer-fresh" {
			t.Fatalf("token = %q, want the peer's token", auth.Token)
		}
		if len(pools.reauth) != 0 {
			t.Fatalf("marked = %v, want no reauth for a peer rotation", pools.reauth)
		}
	})

	t.Run("concurrent refusals refresh once", func(t *testing.T) {
		secrets.values["secret"] = `{"access_token":"stale","refresh_token":"refresh"}`
		flows.err = nil
		flows.calls = 0
		flows.started = make(chan struct{}, 1)
		flows.block = make(chan struct{})
		var group sync.WaitGroup
		group.Add(2)
		errs := make(chan error, 2)
		for range 2 {
			go func() {
				defer group.Done()
				_, err := source.RefreshRejected(ctx, oauth.FlowClaude, entry.ID, "stale")
				errs <- err
			}()
		}
		<-flows.started
		time.Sleep(20 * time.Millisecond)
		close(flows.block)
		group.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("RefreshRejected() error = %v", err)
			}
		}
		if flows.calls != 1 {
			t.Fatalf("calls = %d, want one refresh", flows.calls)
		}
	})
}

func TestReportSkipsASettledRefusal(t *testing.T) {
	pools := &memoryPools{}
	source := upstream.New(upstream.Options{Pools: pools})
	err := source.Report(context.Background(), upstream.Outcome{
		ProviderID: oauth.FlowClaude, CredentialID: "cred-1", Status: http.StatusUnauthorized, RefreshSettled: true,
	})
	if err != nil || len(pools.reauth) != 0 {
		t.Fatalf("Report() error = %v marked = %v, want no reauth", err, pools.reauth)
	}
	if err := source.Report(context.Background(), upstream.Outcome{
		ProviderID: oauth.FlowClaude, CredentialID: "cred-1", Status: http.StatusUnauthorized,
	}); err != nil || len(pools.reauth) != 1 {
		t.Fatalf("Report() error = %v marked = %v, want a reauth", err, pools.reauth)
	}
}

func TestAutoRefreshOffSkipsRenewal(t *testing.T) {
	entry := account.PoolEntry{ID: "cred-1", ProviderID: oauth.FlowClaude, SecretRef: "secret"}
	pools := &memoryPools{entries: map[string]account.PoolEntry{entry.ID: entry}}
	secrets := &memorySecrets{values: map[string]string{"secret": `{"access_token":"stale","refresh_token":"refresh"}`}}
	flows := &memoryFlows{}
	source := upstream.New(upstream.Options{
		Pools: pools, Secrets: secrets, Flows: flows, Adapters: templateAdapters{},
		Policy: refreshOff{},
	})
	_, err := source.RefreshRejected(context.Background(), oauth.FlowClaude, entry.ID, "stale")
	if !errors.Is(err, upstream.ErrReauthRequired) || flows.calls != 0 || len(pools.reauth) != 1 {
		t.Fatalf("error = %v calls = %d marked = %v, want a reauth without a refresh", err, flows.calls, pools.reauth)
	}
}

type templateAdapters struct{}

func (templateAdapters) Authorizer(providerID string) (upstream.Authorizer, bool) {
	adapter, found := templates.Authorizer(providerID)
	if !found {
		return nil, false
	}
	return adapter, true
}

type memoryPools struct {
	entries map[string]account.PoolEntry
	reauth  []string
}

func (p *memoryPools) Select(string, account.Selection) (account.PoolEntry, error) {
	return account.PoolEntry{}, account.ErrNoCredentials
}
func (p *memoryPools) Entry(_, id string) (account.PoolEntry, bool) {
	entry, found := p.entries[id]
	return entry, found
}
func (p *memoryPools) ResolveSecret(account.PoolEntry) (string, error)  { return "", nil }
func (p *memoryPools) RecordSuccess(string, string)                     {}
func (p *memoryPools) RecordFailure(string, string, int, time.Duration) {}
func (p *memoryPools) MarkNeedsReauth(_ context.Context, _, id string) error {
	p.reauth = append(p.reauth, id)
	return nil
}

type memorySecrets struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *memorySecrets) Get(ref string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[ref], nil
}

func (s *memorySecrets) Set(ref, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[ref] = value
	return nil
}

type memoryFlows struct {
	mu      sync.Mutex
	next    *oauth.OAuthCredential
	err     error
	calls   int
	block   chan struct{}
	started chan struct{}
	// noFlow names providers with no login flow, and onRefresh runs before
	// a refresh answers, so a test can store a peer rotation mid-refresh.
	noFlow    map[string]bool
	onRefresh func()
}

func (f *memoryFlows) Flow(id string) (oauth.OAuthFlow, error) {
	if f.noFlow[id] {
		return nil, oauth.ErrFlowNotFound
	}
	return f, nil
}
func (f *memoryFlows) ProviderID() string { return oauth.FlowClaude }

func (f *memoryFlows) CallbackPort() int { return 0 }
func (f *memoryFlows) Login(context.Context, oauth.LoginOpts) (*oauth.OAuthCredential, error) {
	return nil, errors.New("unused")
}
func (f *memoryFlows) Validate(context.Context, *oauth.OAuthCredential) error { return nil }
func (f *memoryFlows) Refresh(context.Context, *oauth.OAuthCredential) (*oauth.OAuthCredential, error) {
	f.mu.Lock()
	f.calls++
	block, err, next, started := f.block, f.err, f.next, f.started
	hook := f.onRefresh
	f.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if block != nil {
		<-block
	}
	if hook != nil {
		hook()
	}
	if err != nil {
		return nil, err
	}
	if next == nil {
		next = &oauth.OAuthCredential{AccessToken: "fresh", RefreshToken: "rotated"}
	}
	return next, nil
}

type refreshOff struct{}

func (refreshOff) AutoRefresh(context.Context, string) bool { return false }
