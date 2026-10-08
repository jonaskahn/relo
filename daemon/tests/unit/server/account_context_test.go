package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestAccountContextEndpoints covers the two routes the account context action
// reads and writes: one account's effective windows, and a batch write that
// sizes every model of that account at once.
func TestAccountContextEndpoints(t *testing.T) {
	harness := newHarness(t)

	accounts := readJSON[struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}](t, harness.management(http.MethodGet, "/api/v1/accounts", adminToken, nil))
	if len(accounts.Items) == 0 {
		t.Fatal("the harness stored no account")
	}
	id := accounts.Items[0].ID

	type contextModel struct {
		ModelID       string `json:"model_id"`
		ContextWindow *int64 `json:"context_window"`
		Override      *int64 `json:"override"`
	}
	read := func() []contextModel {
		t.Helper()
		body := readJSON[struct {
			Models []contextModel `json:"models"`
		}](t, harness.management(http.MethodGet,
			"/api/v1/accounts/"+id+"/models/context", adminToken, nil))
		return body.Models
	}

	initial := read()
	if len(initial) == 0 {
		t.Fatal("the account reported no models")
	}
	for _, model := range initial {
		if model.Override != nil {
			t.Fatalf("model %s starts with an override, want none", model.ModelID)
		}
	}

	write := readJSON[struct {
		Applied []string `json:"applied"`
		Skipped []struct {
			ModelID string `json:"model_id"`
			Reason  string `json:"reason"`
		} `json:"skipped"`
	}](t, harness.management(http.MethodPost, "/api/v1/accounts/"+id+"/models/context", adminToken,
		strings.NewReader(`{"model_ids":["gpt-4o"],"context_window":400000}`)))
	if len(write.Applied) != 1 || write.Applied[0] != "gpt-4o" {
		t.Fatalf("write = %+v, want gpt-4o applied", write)
	}

	sized := read()
	var found bool
	for _, model := range sized {
		if model.ModelID != "gpt-4o" {
			continue
		}
		found = true
		if model.Override == nil || *model.Override != 400_000 {
			t.Fatalf("override = %v, want the written window", model.Override)
		}
		if model.ContextWindow == nil || *model.ContextWindow != 400_000 {
			t.Fatalf("effective = %v, want the account's own window", model.ContextWindow)
		}
	}
	if !found {
		t.Fatal("gpt-4o is missing from the account context")
	}

	// A clear puts the model back on the layers beneath.
	cleared := readJSON[struct {
		Applied []string `json:"applied"`
	}](t, harness.management(http.MethodPost, "/api/v1/accounts/"+id+"/models/context", adminToken,
		strings.NewReader(`{"model_ids":["gpt-4o"],"context_window":null}`)))
	if len(cleared.Applied) != 1 {
		t.Fatalf("clear applied = %+v, want the one model", cleared.Applied)
	}
	for _, model := range read() {
		if model.ModelID == "gpt-4o" && model.Override != nil {
			t.Fatalf("override after clear = %v, want none", model.Override)
		}
	}
}

// readJSON decodes one management response, failing the test on a bad status.
func readJSON[T any](t *testing.T, response interface {
	Result() *http.Response
}) T {
	t.Helper()
	res := response.Result()
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body T
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode the response: %v", err)
	}
	return body
}
