package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/upstream"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

const recoveredFailoverWait = 15 * time.Minute

// recoveredFailoverHarness builds the real server collaborators around an
// isolated pool on the harness clock, so the test moves the same time the
// breakers read without changing the shared harness.
func recoveredFailoverHarness(t *testing.T, entries ...account.PoolEntry) *harness {
	t.Helper()
	h := newHarness(t)
	h.server = h.newServerWithOptions(h.cfg, entries, func(options *server.Options) {
		pools := account.NewManagerWithOptions(
			sqlite.NewCredentialStore(h.db), testkit.SecretStore(h.secrets),
			account.ManagerOptions{Clock: h.clock, FailoverBackoff: []time.Duration{0, recoveredFailoverWait}},
		)
		if err := pools.LoadFromDB(context.Background()); err != nil {
			t.Fatal(err)
		}
		h.pools = pools
		options.Pools = pools
		options.Catalog = h.newCatalog(pools)
		h.service = h.newService(options.Catalog, pools)
		options.Router = routing.New(routing.Options{
			Catalog: options.Catalog, Pools: pools, FailoverBackoff: account.DefaultFailoverBackoff(),
		})
		options.Relay = platform.NewRelay(h.newCredentials(pools),
			upstream.NewExecutor(&http.Client{}, nil, h.logger))
		options.Accounts = h.accounts
		options.Status = h.status
		options.Routes = h.routes
		options.CatalogAPI = h.service
		options.Keys = h.keys
	})
	return h
}

// recoveredFailoverRequest sends one inference request with a named
// conversation, so the follow-up message stays on the same account pin.
func recoveredFailoverRequest(t *testing.T, h *harness, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	handler, found := h.server.DataPlaneHandler(inference.ProtocolOpenAI)
	if !found {
		t.Fatal("the server serves no openai port")
	}
	recorder := httptest.NewRecorder()
	req := request(http.MethodPost, path, dataPlaneToken, strings.NewReader(body))
	req.Header.Set("session-id", "conversation-recovery")
	handler.ServeHTTP(recorder, req)
	return recorder
}

func recoveredFailoverBody(stream bool) string {
	flag := "false"
	if stream {
		flag = "true"
	}
	return `{"model":"gpt-4o","max_tokens":16,"stream":` + flag +
		`,"messages":[{"role":"user","content":"hi"}]}`
}

// TestFailoverUsesTheExpiredTrialOfAnUnselectedAccount is the HTTP-level
// regression for the reported request: a healthy primary leaves the backup's
// recovered trial alone, and a long limit on the primary then reaches the
// backup within the same request, streaming or not.
func TestFailoverUsesTheExpiredTrialOfAnUnselectedAccount(t *testing.T) {
	for _, stream := range []bool{true, false} {
		name := "non-streaming"
		if stream {
			name = "streaming"
		}
		t.Run(name, func(t *testing.T) {
			h := recoveredFailoverHarness(t, defaultEntry(), spareEntry())
			h.secrets[spareCredentialRef] = "sk-second"
			if stream {
				chatStreamFixture(h, "openai/chat_streaming.txt", http.StatusOK)
			} else {
				chatFixture(h, "openai/chat_nonstreaming.json", http.StatusOK)
			}
			setRetryAttempts(t, h, "openai", 1)
			pool := h.pools.GetPool("openai")
			pool.SetFailoverBackoff([]time.Duration{0, recoveredFailoverWait})
			pool.MarkServed("one")
			if verdict := pool.RecordFailure("two", http.StatusTooManyRequests, recoveredFailoverWait); verdict != account.VerdictRetry {
				t.Fatalf("RecordFailure() = %v, want retry", verdict)
			}
			h.clock.Add(recoveredFailoverWait)
			if state := pool.Health("two").State; state != account.BreakerHalfOpen {
				t.Fatalf("backup state = %q, want half_open", state)
			}

			body := recoveredFailoverBody(stream)
			first := recoveredFailoverRequest(t, h, "/v1/chat/completions", body)
			if first.Code != http.StatusOK {
				t.Fatalf("first status = %d, body = %s", first.Code, first.Body.String())
			}
			if h.upstream.lastAuthorization() != "Bearer sk-test" {
				t.Fatalf("first authorization = %q, want the primary", h.upstream.lastAuthorization())
			}
			if state := pool.Health("two").State; state != account.BreakerHalfOpen {
				t.Fatalf("backup state after selection = %q, want the trial still available", state)
			}
			if trial := pool.Breaker("two").Allows(); !trial {
				t.Fatal("selecting the primary consumed the backup's recovery trial")
			}

			before := h.upstream.requestCount()
			started := time.Now()
			h.upstream.setHeaders(map[string]string{"Retry-After": "16452"})
			h.upstream.setAuthStatuses(map[string]int{
				"Bearer sk-test":   http.StatusTooManyRequests,
				"Bearer sk-second": http.StatusOK,
			})
			response := recoveredFailoverRequest(t, h, "/v1/chat/completions", body)
			h.upstream.setHeaders(nil)
			h.upstream.setAuthStatuses(nil)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s, want the backup's answer", response.Code, response.Body.String())
			}
			auths := authorizations(h)
			if len(auths) != before+2 {
				t.Fatalf("authorizations = %v, want one primary and one backup attempt", auths)
			}
			if !strings.Contains(auths[before], "sk-test") || !strings.Contains(auths[before+1], "sk-second") {
				t.Fatalf("attempts = %q then %q, want primary then backup", auths[before], auths[before+1])
			}
			if elapsed := time.Since(started); elapsed > 5*time.Second {
				t.Fatalf("elapsed = %v, want failover without the provider's wait", elapsed)
			}
			if strings.Contains(response.Body.String(), "account_exhausted") {
				t.Fatalf("body = %s, want the provider's answer rather than a local refusal", response.Body.String())
			}
			if !strings.Contains(response.Body.String(), "Hello") && !strings.Contains(response.Body.String(), "Hi there") {
				t.Fatalf("body = %s, want the backup's successful answer", response.Body.String())
			}

			h.settle()
			next := recoveredFailoverRequest(t, h, "/v1/chat/completions", body)
			if next.Code != http.StatusOK {
				t.Fatalf("follow-up status = %d, body = %s", next.Code, next.Body.String())
			}
			if auth := h.upstream.lastAuthorization(); !strings.Contains(auth, "sk-second") {
				t.Fatalf("follow-up authorization = %q, want the backup to keep serving", auth)
			}
			h.settle()

			rows := failoverAttemptRows(t, h)
			if len(rows) != 4 {
				t.Fatalf("attempt rows = %+v, want two attempts of each request", rows)
			}
			want := []failoverAttempt{
				{"one", http.StatusOK},
				{"one", http.StatusTooManyRequests},
				{"two", http.StatusOK},
				{"two", http.StatusOK},
			}
			for index, expected := range want {
				if rows[index] != expected {
					t.Fatalf("attempt row %d = %+v, want %+v (all: %+v)", index, rows[index], expected, rows)
				}
			}
		})
	}
}

type failoverAttempt struct {
	credentialID string
	status       int
}

func failoverAttemptRows(t *testing.T, h *harness) []failoverAttempt {
	t.Helper()
	rows, err := h.db.SQL().Query("SELECT credential_id, status FROM usage_attempts ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	var attempts []failoverAttempt
	for rows.Next() {
		var attempt failoverAttempt
		if err := rows.Scan(&attempt.credentialID, &attempt.status); err != nil {
			t.Fatal(err)
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return attempts
}
