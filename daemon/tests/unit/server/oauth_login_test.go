package server_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/server"
)

// errStub is the failure a login test hands the stub runner.
var errStub = errors.New("the provider refused the login")

// TestStartLoginAndStatus covers the two operations the dashboard's provider
// drawer drives: starting a login, and reading what it is doing.
func TestStartLoginAndStatus(t *testing.T) {
	harness := newHarness(t)

	t.Run("a started login reports its state and its prompt", func(t *testing.T) {
		id, err := harness.server.StartLogin(context.Background(), "openai", "work")
		if err != nil {
			t.Fatalf("StartLogin() error = %v", err)
		}
		if id == "" {
			t.Fatal("StartLogin() returned no operation id")
		}
		state := waitForLogin(t, harness, id)
		if state.ProviderID != "openai" {
			t.Fatalf("provider = %q, want openai", state.ProviderID)
		}
		if state.State == "" || state.State == server.LoginRunning {
			t.Fatalf("state = %q, want the finished login", state.State)
		}
		if state.URL == "" {
			t.Fatal("the finished login carried no prompt")
		}
		if harness.login.count() == 0 {
			t.Fatal("the login runner was never asked to log in")
		}
	})

	t.Run("a login that fails reports why", func(t *testing.T) {
		harness.login.setFailure(errStub)
		t.Cleanup(func() { harness.login.setFailure(nil) })
		id, err := harness.server.StartLogin(context.Background(), "openai", "")
		if err != nil {
			t.Fatalf("StartLogin() error = %v", err)
		}
		state := waitForLogin(t, harness, id)
		if state.State != server.LoginFailed || state.Error == "" {
			t.Fatalf("state = %+v, want the recorded failure", state)
		}
	})

	t.Run("an unknown provider is refused before a login starts", func(t *testing.T) {
		before := harness.login.count()
		if _, err := harness.server.StartLogin(context.Background(), "nope", ""); err == nil {
			t.Fatal("StartLogin() error = nil, want an unknown provider refused")
		}
		if harness.login.count() != before {
			t.Fatal("a refused provider still started a login")
		}
	})

	t.Run("an operation the process does not track is not found", func(t *testing.T) {
		if _, found := harness.server.LoginStatus("nothing"); found {
			t.Fatal("LoginStatus(unknown) found something")
		}
	})
}

// TestLoginStatusWithoutAProcessManager covers a server built with no OAuth
// operations at all.
func TestLoginStatusWithoutAProcessManager(t *testing.T) {
	built := serverWithoutService(newHarness(t))
	if _, found := built.LoginStatus("anything"); found {
		t.Fatal("LoginStatus() reported an operation on a bare server")
	}
}

// TestStartLoginStoresAConnectionOnlyWhenTheLoginFinishes covers the rule the
// console relies on: a sign-in has no connection of its own until it has a
// credential, so a browser an operator opens and then abandons leaves no
// empty connection behind in the list.
func TestStartLoginStoresAConnectionOnlyWhenTheLoginFinishes(t *testing.T) {
	ctx := context.Background()

	t.Run("a login that fails leaves no connection", func(t *testing.T) {
		harness := newHarness(t)
		harness.login.setFailure(errStub)
		id, err := harness.server.StartLogin(ctx, "claude", "work")
		if err != nil {
			t.Fatalf("StartLogin() error = %v", err)
		}
		state := waitForLogin(t, harness, id)
		if state.State != server.LoginFailed {
			t.Fatalf("state = %+v, want the recorded failure", state)
		}
		if _, err := harness.service.Provider(ctx, "claude"); err == nil {
			t.Fatal("a failed login stored a connection")
		}
	})

	t.Run("a finished login stores the connection and its account together", func(t *testing.T) {
		harness := newHarness(t)
		id, err := harness.server.StartLogin(ctx, "claude", "work")
		if err != nil {
			t.Fatalf("StartLogin() error = %v", err)
		}
		state := waitForLogin(t, harness, id)
		if state.State != server.LoginDone {
			t.Fatalf("state = %+v, want the finished login", state)
		}
		if _, err := harness.service.Provider(ctx, "claude"); err != nil {
			t.Fatalf("the finished login left no connection: %v", err)
		}
		accounts, err := harness.accounts.Accounts(ctx, "claude")
		if err != nil {
			t.Fatalf("Accounts() error = %v", err)
		}
		if len(accounts) != 1 {
			t.Fatalf("accounts = %d, want the one the login stored", len(accounts))
		}
	})
}

// waitForLogin polls one login until it leaves the running state, which the
// stub runner does as soon as it is asked.
func waitForLogin(t *testing.T, harness *harness, id string) server.LoginState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, found := harness.server.LoginStatus(id)
		if !found {
			t.Fatalf("the login %s is no longer tracked", id)
		}
		if state.State != server.LoginRunning {
			return state
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the login never finished")
	return server.LoginState{}
}
