package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/wire/formats"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

type rejectedLogin struct {
	requests chan string
}

func (runner rejectedLogin) Login(_ context.Context, request server.LoginRequest) (server.LoginResult, error) {
	runner.requests <- request.ProviderID
	return server.LoginResult{}, errors.New("stub login stopped")
}

// quietCallbacks stands in for the callback broker a test never polls: the
// login still starts and fails the way the API reports it.
type quietCallbacks struct{}

func (quietCallbacks) Deliver(provider string, query url.Values) (string, error) {
	return "", nil
}

func (quietCallbacks) Status(ticket string) (server.CallbackStatus, bool) {
	return server.CallbackStatus{}, false
}

func (quietCallbacks) Complete(ticket, account string, err error) {}

func TestOAuthStartUsesFlowPathParameter(t *testing.T) {
	const providerID = "openai-codex"
	const adminToken = "test-admin-token"

	db := testkit.OpenTestDB(t)
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: providerID, Label: "ChatGPT (Codex sign-in)",
		Auth: catalog.AuthOAuth, APIFormat: catalog.FormatOpenAIResp,
		BaseURL: "https://chatgpt.com/backend-api/codex",
	})
	activeCatalog := catalog.New(sqlite.NewCatalogReader(db), nil, formats.New())
	if err := activeCatalog.Reload(context.Background()); err != nil {
		t.Fatalf("reload catalog: %v", err)
	}
	manager := appcatalog.New(appcatalog.Options{
		Store: sqlite.NewCatalogRepo(db), Catalog: activeCatalog,
		Entries: sqlite.NewCredentialStore(db), Templates: platform.TemplateRegistry{},
	})
	accounts := appaccount.New(appaccount.Options{Entries: sqlite.NewCredentialStore(db)})
	runner := rejectedLogin{requests: make(chan string, 1)}
	cfg := config.DefaultConfig()
	cfg.Admin.Login = true
	served := server.New(server.Options{
		Config: &cfg, AdminToken: adminToken,
		CatalogAPI: manager, Accounts: accounts, Login: runner,
		Callbacks: quietCallbacks{},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/"+providerID+"/start?label=work", nil)
	request.Header.Set("Authorization", "Bearer "+adminToken)
	response := httptest.NewRecorder()
	served.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, body = %s", response.Code, response.Body.String())
	}
	var started struct {
		OperationID string `json:"operation_id"`
		ProviderID  string `json:"provider_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if started.OperationID == "" || started.ProviderID != providerID {
		t.Fatalf("start response = %+v, want a Codex operation", started)
	}
	select {
	case got := <-runner.requests:
		if got != providerID {
			t.Fatalf("login runner provider = %q, want %q", got, providerID)
		}
	case <-time.After(time.Second):
		t.Fatal("login runner was not called")
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/oauth/operations/"+started.OperationID, nil)
	request.Header.Set("Authorization", "Bearer "+adminToken)
	response = httptest.NewRecorder()
	served.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("poll status = %d, body = %s", response.Code, response.Body.String())
	}
	var polled struct {
		OperationID string `json:"operation_id"`
		ProviderID  string `json:"provider_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &polled); err != nil {
		t.Fatalf("decode poll response: %v", err)
	}
	if polled.OperationID != started.OperationID || polled.ProviderID != providerID {
		t.Fatalf("poll response = %+v, want the Codex operation", polled)
	}
}

func TestOAuthRoutesDoNotConflict(t *testing.T) {
	const adminToken = "test-admin-token"
	cfg := config.DefaultConfig()
	cfg.Admin.Login = true
	served := server.New(server.Options{Config: &cfg, AdminToken: adminToken})
	handler := served.Handler()

	port := httptest.NewRequest(http.MethodGet, "/api/v1/oauth/openai/port-status", nil)
	port.Header.Set("Authorization", "Bearer "+adminToken)
	portResponse := httptest.NewRecorder()
	handler.ServeHTTP(portResponse, port)
	if portResponse.Code != http.StatusOK {
		t.Fatalf("port status = %d, body = %s", portResponse.Code, portResponse.Body.String())
	}

	missing := httptest.NewRequest(http.MethodGet, "/api/v1/oauth/operations/missing", nil)
	missing.Header.Set("Authorization", "Bearer "+adminToken)
	missingResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingResponse, missing)
	if missingResponse.Code != http.StatusNotFound {
		t.Fatalf("missing operation = %d, body = %s", missingResponse.Code, missingResponse.Body.String())
	}
}
