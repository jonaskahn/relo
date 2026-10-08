package server_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	appaccount "github.com/jonaskahn/relo/internal/application/account"
	approuting "github.com/jonaskahn/relo/internal/application/routing"
)

// TestAccountAPIRefusesDuplicate covers the answer a console reads when an
// operator adds a key the provider already stores.
func TestAccountAPIRefusesDuplicate(t *testing.T) {
	harness := newHarness(t)
	body := `{"provider_id":"openai","label":"work","secret":"sk-duplicate"}`

	created := harness.management(http.MethodPost, "/api/v1/accounts", adminToken, strings.NewReader(body))
	if created.Code != http.StatusCreated {
		t.Fatalf("first POST /accounts = %d, want 201 (body %s)", created.Code, created.Body.String())
	}
	again := harness.management(http.MethodPost, "/api/v1/accounts", adminToken, strings.NewReader(body))
	if again.Code != http.StatusConflict {
		t.Fatalf("second POST /accounts = %d, want 409 (body %s)", again.Code, again.Body.String())
	}
	if code := errorCode(t, again); code != "conflict" {
		t.Fatalf("error code = %q, want conflict", code)
	}
	// The refusal names the account the provider already holds, so a console
	// can say which one the key would have repeated.
	if !strings.Contains(again.Body.String(), "work") {
		t.Fatalf("refusal = %s, want it to name the stored account", again.Body.String())
	}
}

func TestDeleteAccountAPIAlsoDeletesFinalConnection(t *testing.T) {
	h := newHarness(t)
	deleted := h.management(http.MethodDelete, "/api/v1/accounts/one", adminToken, nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("DELETE /accounts/one = %d, want 204 (body %s)", deleted.Code, deleted.Body.String())
	}
	provider := h.management(http.MethodGet, "/api/v1/connections/openai", adminToken, nil)
	if provider.Code != http.StatusNotFound {
		t.Fatalf("GET removed connection = %d, want 404", provider.Code)
	}
}

func TestDeleteAccountAPIKeepsConnectionWithAnotherAccount(t *testing.T) {
	h := newHarness(t)
	second, err := h.accounts.AddAccount(context.Background(), appaccount.NewAccount{
		ProviderID: "openai", Label: "second", SecretValue: "sk-second",
	})
	if err != nil {
		t.Fatalf("AddAccount() error = %v", err)
	}
	deleted := h.management(http.MethodDelete, "/api/v1/accounts/one", adminToken, nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("DELETE /accounts/one = %d, want 204", deleted.Code)
	}
	if got := h.management(http.MethodGet, "/api/v1/connections/openai", adminToken, nil).Code; got != http.StatusOK {
		t.Fatalf("GET retained connection = %d, want 200", got)
	}
	if got := h.management(http.MethodGet, "/api/v1/accounts/"+second.ID, adminToken, nil).Code; got != http.StatusOK {
		t.Fatalf("GET second account = %d, want 200", got)
	}
}

func TestDeleteFinalAccountAPIRefusesConnectionUsedByRoute(t *testing.T) {
	h := newHarness(t)
	if err := h.routes.Save(context.Background(), "fast", approuting.Write{
		Label: "Fast", Strategy: "priority", Enabled: true, Listed: true,
		Members: []approuting.MemberWrite{{ProviderID: "openai", ModelID: "gpt-4o", Weight: 1, Enabled: true}},
	}); err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}
	deleted := h.management(http.MethodDelete, "/api/v1/accounts/one", adminToken, nil)
	if deleted.Code != http.StatusConflict {
		t.Fatalf("DELETE referenced final account = %d, want 409 (body %s)", deleted.Code, deleted.Body.String())
	}
	if got := h.management(http.MethodGet, "/api/v1/accounts/one", adminToken, nil).Code; got != http.StatusOK {
		t.Fatalf("GET retained account = %d, want 200", got)
	}
	if got := h.management(http.MethodGet, "/api/v1/connections/openai", adminToken, nil).Code; got != http.StatusOK {
		t.Fatalf("GET retained connection = %d, want 200", got)
	}
}
