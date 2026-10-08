package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestOAuthStartWithoutFlow(t *testing.T) {
	keyring.MockInitWithError(errors.New("no keychain in tests"))
	t.Cleanup(keyring.MockInit)

	home := testkit.TempHome(t)
	port := freePort(t)
	useFreePorts(t, home)
	logger, _ := testkit.TestLogger(t)

	// groq exists but publishes no login flow, which is what the operation has
	// to report rather than start. The daemon opens this database itself, so the
	// row is written before it starts.
	session, err := platform.OpenManagement(context.Background(), home, logger)
	if err != nil {
		t.Fatalf("OpenManagement() error = %v", err)
	}
	if err := sqlite.NewCatalogRepo(session.DB()).SaveProvider(context.Background(), sqlite.ProviderRow{
		ID: "groq", Origin: "custom", Label: "Groq", Auth: "api_key",
		APIFormat: "openai-chat", BaseURL: "https://groq.test/v1",
		ModelsFormat: "none", Enabled: true, Rank: 100, PoolStrategy: "least-loaded",
	}); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serving := make(chan error, 1)
	go func() {
		serving <- platform.Serve(ctx, platform.Options{Home: home, Port: port, Logger: logger})
	}()
	waitForHealthz(t, port)

	token := readToken(t, home, server.AdminTokenFile)
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	// groq is a provider without a login flow, so the runner fails the
	// operation instead of starting one.
	started := struct {
		OperationID string `json:"operation_id"`
		State       string `json:"state"`
	}{}
	start := managementCall(t, http.MethodPost, base+"/api/v1/oauth/groq/start", token, &started)
	if start != http.StatusAccepted {
		t.Fatalf("oauth start status = %d, want 202", start)
	}
	if started.OperationID == "" || started.State != "running" {
		t.Fatalf("oauth start = %+v, want a running operation", started)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		operation := struct {
			State string `json:"state"`
			Error string `json:"error"`
		}{}
		managementCall(t, http.MethodGet, base+"/api/v1/oauth/operations/"+started.OperationID, token, &operation)
		if operation.State == "failed" {
			if operation.Error == "" {
				t.Fatal("the failed operation carries no error")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the operation stayed %q, want failed", operation.State)
		}
		time.Sleep(25 * time.Millisecond)
	}

	cancel()
	if err := <-serving; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
}

// managementCall performs one authenticated management request and decodes
// its JSON body into into, which may be nil.
func managementCall(t *testing.T, method, url, token string, into any) int {
	t.Helper()
	request, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s error = %v", method, url, err)
	}
	defer func() { _ = response.Body.Close() }()
	if into != nil {
		if err := json.NewDecoder(response.Body).Decode(into); err != nil {
			t.Fatalf("decode the response: %v", err)
		}
	}
	return response.StatusCode
}
