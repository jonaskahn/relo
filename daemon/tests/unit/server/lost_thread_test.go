package server_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/upstream"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

const threadFailure = `{"type":"error","error":{"type":"not_found_error","message":"No thread state was found for the requested previous_message_id.","details":{"error_code":"thread_not_found"}}}`

func persistentThreadHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.secrets[spareCredentialRef] = "sk-spare"
	h.server = h.newServerWithOptions(h.cfg, []account.PoolEntry{defaultEntry(), spareEntry()}, func(opts *server.Options) {
		h.pools = account.NewManagerWithOptions(sqlite.NewCredentialStore(h.db), testkit.SecretStore(h.secrets), account.ManagerOptions{ModelRepo: sqlite.NewCredentialModelStore(h.db)})
		if err := h.pools.LoadFromDB(context.Background()); err != nil {
			t.Fatal(err)
		}
		opts.Pools = h.pools
		opts.Catalog = h.newCatalog(h.pools)
		opts.Router = routing.New(routing.Options{Catalog: opts.Catalog, Pools: h.pools, FailoverBackoff: account.DefaultFailoverBackoff()})
		opts.Relay = platform.NewRelay(h.newCredentials(h.pools), upstream.NewExecutor(&http.Client{}, nil, h.logger))
	})
	return h
}

func assertThreadAccountsAvailable(t *testing.T, h *harness) {
	t.Helper()
	if err := h.server.SettleBookkeeping(context.Background()); err != nil {
		t.Fatal(err)
	}
	observations, _, err := sqlite.NewCredentialModelRepo(h.db).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range observations {
		if !row.Available {
			t.Fatalf("unexpected denial: %+v", row)
		}
	}
	for _, id := range []string{"one", "two"} {
		if !h.pools.ModelIndex().CanServe(id, []string{"gpt-4o"}) {
			t.Fatalf("%s excluded", id)
		}
		entry, ok := h.pools.GetPool("openai").Entry(id)
		if !ok || entry.Status != account.StatusActive {
			t.Fatalf("inactive account: %+v", entry)
		}
	}
}

func TestLostThreadLeavesAccountsAvailableAfterReload(t *testing.T) {
	h := persistentThreadHarness(t)
	h.upstream.status = 404
	h.upstream.sse = false
	h.upstream.rawBody = threadFailure
	response := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"continue"}],"previous_message_id":"missing"}`))
	if response.Code != 404 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if got := h.upstream.requestCount(); got != 1 {
		t.Fatalf("upstream attempts=%d, want 1", got)
	}
	assertThreadAccountsAvailable(t, h)
	h.upstream.status = 200
	h.upstream.sse = true
	h.upstream.rawBody = ""
	fresh := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if fresh.Code != 200 {
		t.Fatalf("fresh status=%d body=%s", fresh.Code, fresh.Body.String())
	}
	assertThreadAccountsAvailable(t, h)
	if err := h.pools.LoadFromDB(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertThreadAccountsAvailable(t, h)
}

func TestGenuineModelDenialStillPersistsAndFailsOver(t *testing.T) {
	h := persistentThreadHarness(t)
	h.upstream.status = 404
	h.upstream.sse = false
	h.upstream.rawBody = `{"error":{"type":"not_found_error","message":"model gpt-4o not found"}}`
	response := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	if response.Code != 404 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if got := h.upstream.requestCount(); got != 2 {
		t.Fatalf("upstream attempts=%d, want 2", got)
	}
	if err := h.server.SettleBookkeeping(context.Background()); err != nil {
		t.Fatal(err)
	}
	rows, _, err := sqlite.NewCredentialModelRepo(h.db).List(context.Background())
	if err != nil || len(rows) != 2 {
		t.Fatalf("observations=%+v err=%v", rows, err)
	}
	for _, row := range rows {
		if row.Available {
			t.Fatalf("denial lost: %+v", row)
		}
	}
	if err := h.pools.LoadFromDB(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two"} {
		if h.pools.ModelIndex().CanServe(id, []string{"gpt-4o"}) {
			t.Fatalf("%s denial lost on reload", id)
		}
	}
}
