package oauth_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/oauth"
)

// TestBrowserLoginThroughTheCallbackPage covers the login a daemon runs: the
// flow registers with the broker, the browser reaches the loopback address
// the provider pins, and the listener there forwards it to the page the
// daemon serves with the ticket that page reads.
func TestBrowserLoginThroughTheCallbackPage(t *testing.T) {
	server, recorder := newProvider(t)
	server.handle("/token", jsonHandler(`{"access_token":"access-1","refresh_token":"refresh-1","expires_in":3600}`))
	broker := oauth.NewCallbackBroker(oauth.CallbackBrokerOptions{ManagementPort: 10101})
	flow := oauth.NewAnthropicFlow(
		oauth.WithCallbacks(broker),
		oauth.WithCallbackPort(-1),
		oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.url("/authorize"), TokenURL: server.url("/token")}),
	)

	prompted := make(chan oauth.AuthPrompt, 1)
	done := make(chan error, 1)
	go func() {
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			NoBrowser: true,
			Prompt:    func(prompt oauth.AuthPrompt) { prompted <- prompt },
			Timeout:   10 * time.Second,
		})
		done <- err
	}()

	var prompt oauth.AuthPrompt
	select {
	case prompt = <-prompted:
	case <-time.After(10 * time.Second):
		t.Fatal("the login never published its address")
	}
	if prompt.Ticket == "" {
		t.Fatal("the prompt carries no ticket for the callback page")
	}

	parsed, err := url.Parse(prompt.URL)
	if err != nil {
		t.Fatalf("parse the authorize url: %v", err)
	}
	redirect, state := parsed.Query().Get("redirect_uri"), parsed.Query().Get("state")
	if redirect == "" || state == "" {
		t.Fatalf("authorize url = %q, want a redirect_uri and a state", prompt.URL)
	}
	callbackURL, err := url.Parse(redirect)
	if err != nil {
		t.Fatalf("parse redirect_uri %q: %v", redirect, err)
	}
	if callbackURL.Hostname() != "localhost" {
		t.Fatalf("redirect_uri host = %q, want localhost", callbackURL.Hostname())
	}
	callbackPort, err := strconv.Atoi(callbackURL.Port())
	if err != nil {
		t.Fatalf("parse redirect_uri port: %v", err)
	}
	if want := broker.PageURL("claude"); want == redirect {
		t.Fatalf("redirect_uri = %q, want the loopback address the provider pins", redirect)
	}

	answer := deliverRedirect(t, redirect, url.Values{"code": {"code-1"}, "state": {state}})
	if answer.status != http.StatusSeeOther {
		t.Fatalf("bridge status = %d, want the redirect to the page", answer.status)
	}
	want := broker.TicketURL("claude", prompt.Ticket)
	if answer.location != want {
		t.Fatalf("bridge sent the browser to %q, want %q", answer.location, want)
	}
	status, found := broker.Status(prompt.Ticket)
	if !found || status.Phase != oauth.CallbackExchanging {
		t.Fatalf("status = %+v, want the exchange", status)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Login() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the login never finished")
	}
	rebind(t, callbackPort)
	assertJSONFields(t, recorder.last("/token").body, map[string]string{"code": "code-1"})
}

func TestClaudeCallbackPortClosesOnTimeout(t *testing.T) {
	broker := oauth.NewCallbackBroker(oauth.CallbackBrokerOptions{ManagementPort: 10101})
	flow := oauth.NewAnthropicFlow(
		oauth.WithCallbacks(broker),
		oauth.WithCallbackPort(-1),
		oauth.WithEndpoints(oauth.Endpoints{AuthURL: "https://auth.test/authorize"}),
	)
	prompted := make(chan oauth.AuthPrompt, 1)
	done := make(chan error, 1)
	go func() {
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			NoBrowser: true,
			Prompt:    func(prompt oauth.AuthPrompt) { prompted <- prompt },
			Timeout:   50 * time.Millisecond,
		})
		done <- err
	}()

	var prompt oauth.AuthPrompt
	select {
	case prompt = <-prompted:
	case <-time.After(10 * time.Second):
		t.Fatal("the login never published its address")
	}
	parsed, err := url.Parse(prompt.URL)
	if err != nil {
		t.Fatalf("parse authorize url: %v", err)
	}
	callbackURL, err := url.Parse(parsed.Query().Get("redirect_uri"))
	if err != nil {
		t.Fatalf("parse redirect_uri: %v", err)
	}
	callbackPort, err := strconv.Atoi(callbackURL.Port())
	if err != nil {
		t.Fatalf("parse redirect_uri port: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, oauth.ErrCallbackTimeout) {
			t.Fatalf("Login() error = %v, want %v", err, oauth.ErrCallbackTimeout)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the login never timed out")
	}
	rebind(t, callbackPort)
}

// TestBrowserLoginWithoutACallbackPage covers a flow with no broker: it binds
// its own loopback listener and answers the browser itself.
func TestBrowserLoginWithoutACallbackPage(t *testing.T) {
	server, _ := newProvider(t)
	server.handle("/token", jsonHandler(`{"access_token":"access-1","refresh_token":"refresh-1","expires_in":3600}`))
	flow := oauth.NewAnthropicFlow(
		oauth.WithCallbackPort(-1),
		oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.url("/authorize"), TokenURL: server.url("/token")}),
	)

	prompted := make(chan oauth.AuthPrompt, 1)
	done := make(chan error, 1)
	go func() {
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			NoBrowser: true,
			Prompt:    func(prompt oauth.AuthPrompt) { prompted <- prompt },
			Timeout:   10 * time.Second,
		})
		done <- err
	}()

	var prompt oauth.AuthPrompt
	select {
	case prompt = <-prompted:
	case <-time.After(10 * time.Second):
		t.Fatal("the login never published its address")
	}
	if prompt.Ticket != "" {
		t.Fatalf("ticket = %q, want none without a broker", prompt.Ticket)
	}
	parsed, err := url.Parse(prompt.URL)
	if err != nil {
		t.Fatalf("parse the authorize url: %v", err)
	}
	redirect := parsed.Query().Get("redirect_uri")
	answer := deliverRedirect(t, redirect, url.Values{
		"code": {"code-1"}, "state": {parsed.Query().Get("state")},
	})
	if answer.status != http.StatusOK || !strings.Contains(answer.body, "Authorization received") {
		t.Fatalf("answer = %d %q, want the page the flow renders itself", answer.status, answer.body)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Login() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the login never finished")
	}
}

// TestBrowserLoginRefusesARefusedLogin covers the provider saying no: the
// waiting login is released with the refusal instead of waiting out its
// timeout.
func TestBrowserLoginRefusesARefusedLogin(t *testing.T) {
	server, _ := newProvider(t)
	server.handle("/token", jsonHandler(`{"access_token":"access-1"}`))
	broker := oauth.NewCallbackBroker(oauth.CallbackBrokerOptions{ManagementPort: 10101})
	flow := oauth.NewAnthropicFlow(
		oauth.WithCallbacks(broker),
		oauth.WithCallbackPort(-1),
		oauth.WithEndpoints(oauth.Endpoints{AuthURL: server.url("/authorize"), TokenURL: server.url("/token")}),
	)

	prompted := make(chan oauth.AuthPrompt, 1)
	done := make(chan error, 1)
	go func() {
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			NoBrowser: true,
			Prompt:    func(prompt oauth.AuthPrompt) { prompted <- prompt },
			Timeout:   10 * time.Second,
		})
		done <- err
	}()

	var prompt oauth.AuthPrompt
	select {
	case prompt = <-prompted:
	case <-time.After(10 * time.Second):
		t.Fatal("the login never published its address")
	}
	parsed, err := url.Parse(prompt.URL)
	if err != nil {
		t.Fatalf("parse the authorize url: %v", err)
	}
	answer := deliverRedirect(t, parsed.Query().Get("redirect_uri"), url.Values{
		"error": {"access_denied"}, "state": {parsed.Query().Get("state")},
	})
	if answer.status != http.StatusSeeOther {
		t.Fatalf("bridge status = %d, want the page that explains it", answer.status)
	}
	select {
	case err := <-done:
		if !errors.Is(err, oauth.ErrLoginCancelled) {
			t.Fatalf("Login() error = %v, want the refusal", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the refused login never returned")
	}
}

// delivered is what one callback answered with.
type delivered struct {
	status   int
	location string
	body     string
}

// deliverRedirect sends one provider redirect to a loopback callback address and
// reports the answer the browser would have read. The redirect is not
// followed: where it points is what a test asserts.
func deliverRedirect(t *testing.T, redirect string, query url.Values) delivered {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	response, err := client.Get(redirect + "?" + query.Encode())
	if err != nil {
		t.Fatalf("deliver the callback to %s: %v", redirect, err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read the answer: %v", err)
	}
	return delivered{status: response.StatusCode, location: response.Header.Get("Location"), body: string(body)}
}
