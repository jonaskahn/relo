package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
)

// TestRequestLogFiltersByNameAndClient covers the two filters the logs page
// offers: one access key by name, and the coding client a key was issued for.
// Both travel to the same read, so a filter the handler forgets to parse
// would quietly return every row.
func TestRequestLogFiltersByNameAndClient(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	events := []sqlite.UsageEvent{
		{RequestID: "a", Timestamp: 1_000, Provider: "openai", Model: "gpt-4o", Status: 200, ClientKeyID: "key-codex", ClientKeyName: "codex-key", ClientApp: "codex"},
		{RequestID: "b", Timestamp: 2_000, Provider: "openai", Model: "gpt-4o", Status: 200, ClientKeyID: "key-cursor", ClientKeyName: "cursor-key", ClientApp: "cursor"},
	}
	for _, event := range events {
		if _, err := harness.usage.AppendEvent(ctx, event); err != nil {
			t.Fatalf("AppendEvent(%s) error = %v", event.RequestID, err)
		}
	}

	list := func(t *testing.T, query string) []struct {
		RequestID     string
		ClientKeyName string
	} {
		t.Helper()
		response := harness.management(http.MethodGet, "/api/v1/activity/requests?"+query, adminToken, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", response.Code, response.Body.String())
		}
		body := struct {
			Items []struct {
				RequestID     string
				ClientKeyName string
			}
		}{}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode the response: %v", err)
		}
		return body.Items
	}

	t.Run("by access key", func(t *testing.T) {
		rows := list(t, "client=key-cursor")
		if len(rows) != 1 || rows[0].RequestID != "b" {
			t.Fatalf("rows = %+v, want the request made with key-cursor", rows)
		}
	})

	t.Run("by coding client", func(t *testing.T) {
		rows := list(t, "client_app=codex")
		if len(rows) != 1 || rows[0].RequestID != "a" {
			t.Fatalf("rows = %+v, want the request made with a codex key", rows)
		}
	})

	t.Run("both filters name one request", func(t *testing.T) {
		if rows := list(t, "client=key-codex&client_app=codex"); len(rows) != 1 {
			t.Fatalf("rows = %+v, want the one request both filters name", rows)
		}
		if rows := list(t, "client=key-cursor&client_app=codex"); len(rows) != 0 {
			t.Fatalf("rows = %+v, want no request to match two different keys", rows)
		}
	})

	t.Run("an oversized client is refused", func(t *testing.T) {
		response := harness.management(http.MethodGet,
			"/api/v1/activity/requests?client_app="+strings.Repeat("x", 300), adminToken, nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (%s)", response.Code, response.Body.String())
		}
	})
}

// TestRequestLogFiltersByStatusClass covers the filter the logs page offers:
// its Error option names the two classes a refusal arrives in, and its OK
// option names the answers. A page that sent one exact code left a 429 out of
// the list an operator asked for.
func TestRequestLogFiltersByStatusClass(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	events := []sqlite.UsageEvent{
		{RequestID: "ok", Timestamp: 1_000, Provider: "openai", Model: "gpt-4o", Status: 200},
		{RequestID: "missing", Timestamp: 2_000, Provider: "openai", Model: "gpt-4o", Status: 404},
		{RequestID: "limited", Timestamp: 3_000, Provider: "openai", Model: "gpt-4o", Status: 429},
		{RequestID: "broken", Timestamp: 4_000, Provider: "openai", Model: "gpt-4o", Status: 500},
	}
	for _, event := range events {
		if _, err := harness.usage.AppendEvent(ctx, event); err != nil {
			t.Fatalf("AppendEvent(%s) error = %v", event.RequestID, err)
		}
	}

	ids := func(t *testing.T, query string) []string {
		t.Helper()
		response := harness.management(http.MethodGet, "/api/v1/activity/requests?"+query, adminToken, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", response.Code, response.Body.String())
		}
		body := struct {
			Items []struct{ RequestID string }
		}{}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode the response: %v", err)
		}
		out := make([]string, 0, len(body.Items))
		for _, item := range body.Items {
			out = append(out, item.RequestID)
		}
		return out
	}

	t.Run("both error classes", func(t *testing.T) {
		got := ids(t, "status=4xx,5xx")
		want := []string{"broken", "limited", "missing"}
		if len(got) != len(want) {
			t.Fatalf("rows = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("rows = %v, want %v", got, want)
			}
		}
	})

	t.Run("the answer class", func(t *testing.T) {
		got := ids(t, "status=2xx")
		if len(got) != 1 || got[0] != "ok" {
			t.Fatalf("rows = %v, want the answered request alone", got)
		}
	})

	t.Run("one exact code still works", func(t *testing.T) {
		got := ids(t, "status=429")
		if len(got) != 1 || got[0] != "limited" {
			t.Fatalf("rows = %v, want the rate-limited request alone", got)
		}
	})

	t.Run("an unknown class is refused", func(t *testing.T) {
		response := harness.management(http.MethodGet, "/api/v1/activity/requests?status=6xx", adminToken, nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (%s)", response.Code, response.Body.String())
		}
	})
}
