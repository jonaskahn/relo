package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
)

// TestUsageFiltersByAccountAndSummarizes covers the two reads the usage page
// adds: filtering and grouping by the stored account identifier rather than
// the display label, and a summary that totals every matching event rather
// than the groups a limited rollup returned.
func TestUsageFiltersByAccountAndSummarizes(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	// Two accounts of one connection share a label, which is what the label
	// filter cannot tell apart and the identifier filter must.
	events := []sqlite.UsageEvent{
		{RequestID: "a", Timestamp: 1_700_000_000_000, Provider: "openai", Model: "gpt-4o", Status: 200,
			CredentialLabel: "work", CredentialID: "cred-1", InputTokens: 10, OutputTokens: 5},
		{RequestID: "b", Timestamp: 1_700_000_001_000, Provider: "openai", Model: "gpt-4o", Status: 200,
			CredentialLabel: "work", CredentialID: "cred-2", InputTokens: 20, OutputTokens: 7},
		{RequestID: "c", Timestamp: 1_700_000_002_000, Provider: "anthropic", Model: "claude", Status: 500,
			CredentialLabel: "work", CredentialID: "cred-1", InputTokens: 1, OutputTokens: 1},
	}
	for _, event := range events {
		if _, err := harness.usage.AppendEvent(ctx, event); err != nil {
			t.Fatalf("AppendEvent(%s) error = %v", event.RequestID, err)
		}
	}

	t.Run("filter by account identifier", func(t *testing.T) {
		rows := usageRows(t, harness, "account=cred-1")
		if len(rows) != 2 {
			t.Fatalf("rows = %+v, want the two requests cred-1 served", rows)
		}
		if rows := usageRows(t, harness, "account=cred-2"); len(rows) != 1 {
			t.Fatalf("rows = %+v, want the one request cred-2 served", rows)
		}
	})

	t.Run("group by account keeps two labels apart", func(t *testing.T) {
		rows := usageGroups(t, harness, "group_by=account")
		if len(rows) != 2 {
			t.Fatalf("groups = %+v, want one group per stored account", rows)
		}
		byKey := map[string]int64{}
		for _, row := range rows {
			byKey[row.Key] = row.Requests
		}
		if byKey["cred-1"] != 2 || byKey["cred-2"] != 1 {
			t.Fatalf("groups = %+v, want cred-1=2 and cred-2=1", rows)
		}
	})

	t.Run("summary totals every matching event", func(t *testing.T) {
		// One group is what a limited rollup answers, but the summary still
		// totals all three events.
		if groups := usageGroups(t, harness, "group_by=provider&limit=1"); len(groups) != 1 {
			t.Fatalf("groups = %+v, want the rollup limited to one", groups)
		}
		summary := usageSummary(t, harness, "")
		if summary.Requests != 3 {
			t.Fatalf("summary requests = %d, want 3", summary.Requests)
		}
		if summary.InputTokens != 31 || summary.OutputTokens != 13 {
			t.Fatalf("summary tokens = %d/%d, want 31/13", summary.InputTokens, summary.OutputTokens)
		}
		if summary.Errors != 1 {
			t.Fatalf("summary errors = %d, want 1", summary.Errors)
		}
		filtered := usageSummary(t, harness, "account=cred-1")
		if filtered.Requests != 2 || filtered.InputTokens != 11 {
			t.Fatalf("filtered summary = %+v, want the two cred-1 requests", filtered)
		}
	})
}

func usageGroups(t *testing.T, harness *harness, query string) []struct {
	Key      string
	Requests int64
} {
	t.Helper()
	response := harness.management(http.MethodGet, "/api/v1/activity/usage?"+query, adminToken, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", response.Code, response.Body.String())
	}
	body := struct {
		Items []struct {
			Key      string
			Requests int64
		}
	}{}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode the response: %v", err)
	}
	return body.Items
}

func usageRows(t *testing.T, harness *harness, query string) []struct {
	RequestID string `json:"request_id"`
} {
	t.Helper()
	response := harness.management(http.MethodGet, "/api/v1/activity/requests?"+query, adminToken, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", response.Code, response.Body.String())
	}
	body := struct {
		Items []struct {
			RequestID string `json:"request_id"`
		}
	}{}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode the response: %v", err)
	}
	return body.Items
}

func usageSummary(t *testing.T, harness *harness, query string) struct {
	Requests     int64 `json:"requests"`
	Errors       int64 `json:"errors"`
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
} {
	t.Helper()
	response := harness.management(http.MethodGet, "/api/v1/activity/usage/summary?"+query, adminToken, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", response.Code, response.Body.String())
	}
	body := struct {
		Requests     int64 `json:"requests"`
		Errors       int64 `json:"errors"`
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	}{}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode the response: %v", err)
	}
	return body
}
