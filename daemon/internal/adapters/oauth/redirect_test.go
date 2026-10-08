package oauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestRedirectModes covers the two ways a provider can hand the browser back:
// straight to the callback page, and through the loopback address a provider
// pins, which forwards to that page.
func TestRedirectModes(t *testing.T) {
	tokens := tokenStub(t)

	t.Run("direct sends the browser to the page", func(t *testing.T) {
		broker := NewCallbackBroker(CallbackBrokerOptions{ManagementPort: 10101})
		flow := newAuthCodeFlow(probeConfig(tokens.URL, redirectDirect), Options{Callbacks: broker})
		prompt := loginInBackground(t, flow, 5*time.Second)

		redirect, state := authorizeParts(t, prompt)
		if want := broker.PageURL("probe"); redirect != want {
			t.Fatalf("redirect_uri = %q, want the callback page %q", redirect, want)
		}
		if prompt.Ticket == "" {
			t.Fatal("the prompt carries no ticket for the page")
		}
		ticket, err := broker.Deliver("probe", url.Values{"code": {"code-1"}, "state": {state}})
		if err != nil {
			t.Fatalf("Deliver() error = %v", err)
		}
		if ticket != prompt.Ticket {
			t.Fatalf("ticket = %q, want the one the prompt published", ticket)
		}
	})

	t.Run("bridge binds the pinned address and forwards to the page", func(t *testing.T) {
		broker := NewCallbackBroker(CallbackBrokerOptions{ManagementPort: 10101})
		flow := newAuthCodeFlow(probeConfig(tokens.URL, redirectBridge), Options{Callbacks: broker})
		prompt := loginInBackground(t, flow, 5*time.Second)

		redirect, state := authorizeParts(t, prompt)
		if !strings.HasPrefix(redirect, "http://127.0.0.1:") || !strings.HasSuffix(redirect, "/oauth/callback") {
			t.Fatalf("redirect_uri = %q, want the pinned loopback address", redirect)
		}
		answer := get(t, redirect+"?"+url.Values{"code": {"code-1"}, "state": {state}}.Encode())
		if answer.status != http.StatusSeeOther {
			t.Fatalf("bridge status = %d, want the redirect to the page", answer.status)
		}
		location, err := url.Parse(answer.location)
		if err != nil {
			t.Fatalf("parse the redirect %q: %v", answer.location, err)
		}
		if got := location.Scheme + "://" + location.Host + location.Path; got != broker.PageURL("probe") {
			t.Fatalf("bridge sent the browser to %q, want the callback page", got)
		}
		if ticket := location.Query().Get("ticket"); ticket != prompt.Ticket {
			t.Fatalf("ticket = %q, want the one the prompt published", ticket)
		}
	})

	t.Run("a bridge without a broker answers the browser itself", func(t *testing.T) {
		flow := newAuthCodeFlow(probeConfig(tokens.URL, redirectBridge), Options{})
		prompt := loginInBackground(t, flow, 5*time.Second)
		if prompt.Ticket != "" {
			t.Fatalf("ticket = %q, want none without a broker", prompt.Ticket)
		}
		redirect, state := authorizeParts(t, prompt)
		answer := get(t, redirect+"?"+url.Values{"code": {"code-1"}, "state": {state}}.Encode())
		if answer.status != http.StatusOK || !strings.Contains(answer.body, "Authorization received") {
			t.Fatalf("answer = %d %q, want the page the flow renders itself", answer.status, answer.body)
		}
	})

	t.Run("a login nobody answers times out", func(t *testing.T) {
		broker := NewCallbackBroker(CallbackBrokerOptions{ManagementPort: 10101})
		flow := newAuthCodeFlow(probeConfig(tokens.URL, redirectDirect), Options{Callbacks: broker})
		_, err := flow.Login(context.Background(), LoginOpts{
			NoBrowser: true,
			Prompt:    func(AuthPrompt) {},
			Timeout:   50 * time.Millisecond,
		})
		if !errors.Is(err, ErrCallbackTimeout) {
			t.Fatalf("Login() error = %v, want %v", err, ErrCallbackTimeout)
		}
	})
}

// TestLateRedirectIsRefused covers the redirect a provider sends after the
// login it names stopped waiting, which is what a browser closed for too long
// and reopened from its history leaves behind. Handing the code over would
// wake the page into an exchange no flow is running, so the redirect is
// refused instead and the page keeps reading the login it was left with.
func TestLateRedirectIsRefused(t *testing.T) {
	broker := NewCallbackBroker(CallbackBrokerOptions{ManagementPort: 10101})
	flow := newAuthCodeFlow(probeConfig(tokenStub(t).URL, redirectDirect), Options{Callbacks: broker})

	prompted := make(chan AuthPrompt, 1)
	done := make(chan error, 1)
	go func() {
		_, err := flow.Login(context.Background(), LoginOpts{
			NoBrowser: true,
			Prompt:    func(prompt AuthPrompt) { prompted <- prompt },
			Timeout:   50 * time.Millisecond,
		})
		done <- err
	}()

	var prompt AuthPrompt
	select {
	case prompt = <-prompted:
	case <-time.After(10 * time.Second):
		t.Fatal("the login never published its address")
	}
	if err := <-done; !errors.Is(err, ErrCallbackTimeout) {
		t.Fatalf("Login() error = %v, want the login to have timed out", err)
	}

	_, state := authorizeParts(t, prompt)
	if _, err := broker.Deliver("probe", url.Values{"code": {"code-1"}, "state": {state}}); !errors.Is(err, ErrCallbackExpired) {
		t.Fatalf("Deliver() error = %v, want the late redirect refused", err)
	}
	// The process that ran the login reports how it ended, and a redirect that
	// arrives after that leaves the page reading that outcome rather than
	// waking it into an exchange nothing is finishing.
	broker.Complete(prompt.Ticket, "", ErrCallbackTimeout)
	if _, err := broker.Deliver("probe", url.Values{"code": {"code-1"}, "state": {state}}); !errors.Is(err, ErrCallbackExpired) {
		t.Fatalf("Deliver() error = %v, want the late redirect refused", err)
	}
	status, found := broker.Status(prompt.Ticket)
	if !found || status.Phase != CallbackFailed || status.Error != CallbackTimeout {
		t.Fatalf("status = %+v, want the timed out login the page still reads", status)
	}
}

// probeConfig is one authorization-code flow whose provider address a test
// stands in for.
func probeConfig(tokenURL string, mode redirectMode) authCodeConfig {
	return authCodeConfig{
		providerID: "probe", clientID: "client-1",
		authURL: "https://auth.test/authorize", tokenURL: tokenURL,
		scopes: []string{"scope"}, path: "/oauth/callback",
		redirect: mode, instructions: "Finish the login in your browser.",
	}
}

// loginInBackground runs one login and waits for the prompt it publishes,
// which is the address a browser would be sent to. The delivery is the
// caller's, so the flow stays in flight.
func loginInBackground(t *testing.T, flow *authCodeFlow, timeout time.Duration) AuthPrompt {
	t.Helper()
	prompted := make(chan AuthPrompt, 1)
	done := make(chan error, 1)
	go func() {
		_, err := flow.Login(context.Background(), LoginOpts{
			NoBrowser: true,
			Prompt:    func(prompt AuthPrompt) { prompted <- prompt },
			Timeout:   timeout,
		})
		done <- err
	}()
	select {
	case prompt := <-prompted:
		t.Cleanup(func() {
			select {
			case err := <-done:
				_ = err
			case <-time.After(10 * time.Second):
				t.Error("the login did not finish")
			}
		})
		return prompt
	case <-time.After(10 * time.Second):
		t.Fatal("the login never published its address")
		return AuthPrompt{}
	}
}

// authorizeParts reads the loopback address and the state a prompt published.
func authorizeParts(t *testing.T, prompt AuthPrompt) (string, string) {
	t.Helper()
	parsed, err := url.Parse(prompt.URL)
	if err != nil {
		t.Fatalf("parse the authorize url %q: %v", prompt.URL, err)
	}
	redirect, state := parsed.Query().Get("redirect_uri"), parsed.Query().Get("state")
	if redirect == "" || state == "" {
		t.Fatalf("authorize url = %q, want a redirect_uri and a state", prompt.URL)
	}
	return redirect, state
}

// answer is what one delivered callback answered with.
type answer struct {
	status   int
	location string
	body     string
}

// get delivers one callback to a loopback listener without following the
// redirect it may answer with.
func get(t *testing.T, target string) answer {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	response, err := client.Get(target)
	if err != nil {
		t.Fatalf("deliver the callback to %s: %v", target, err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read the answer: %v", err)
	}
	return answer{status: response.StatusCode, location: response.Header.Get("Location"), body: string(body)}
}

// tokenStub answers the token exchange of one login.
func tokenStub(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"access-1","refresh_token":"refresh-1","expires_in":3600}`)
	}))
	t.Cleanup(server.Close)
	return server
}

// TestCallbackPortNamesTheListener covers what a login binds before it runs:
// nothing for a login that waits on the callback page, and the pinned port
// for one that listens on loopback.
func TestCallbackPortNamesTheListener(t *testing.T) {
	direct := newAuthCodeFlow(probeConfig("https://auth.test", redirectDirect), Options{})
	if got := direct.CallbackPort(); got != 0 {
		t.Fatalf("CallbackPort() = %d, want zero for a login that waits on the page", got)
	}
	bridged := newAuthCodeFlow(probeConfig("https://auth.test", redirectBridge), Options{CallbackPort: 4567})
	if got := bridged.CallbackPort(); got != 4567 {
		t.Fatalf("CallbackPort() = %d, want the pinned port", got)
	}
}
