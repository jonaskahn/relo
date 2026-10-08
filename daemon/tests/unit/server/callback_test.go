package server_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/server"
)

// callbackBroker returns a broker for a server on the given management port.
func callbackBroker(port int) *oauth.CallbackBroker {
	return oauth.NewCallbackBroker(oauth.CallbackBrokerOptions{ManagementPort: port})
}

// callbackHarness returns a harness whose server reports logins through the
// given broker, so a test drives the callback page the daemon serves.
func callbackHarness(t *testing.T, broker *oauth.CallbackBroker) *harness {
	t.Helper()
	built := newHarness(t)
	built.server = built.newServerWithOptions(built.cfg, []account.PoolEntry{defaultEntry()},
		func(options *server.Options) { options.Callbacks = platform.NewCallbackBridge(broker) })
	return built
}

func TestCallbackPage(t *testing.T) {
	broker := callbackBroker(10101)
	harness := callbackHarness(t, broker)
	login := broker.Register("chatgpt", "state-1")
	ticket := login.Ticket()

	t.Run("the page names its provider and polls its own login", func(t *testing.T) {
		response := harness.management(http.MethodGet, "/callback/chatgpt?ticket="+ticket, "", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
		body := response.Body.String()
		for _, fragment := range []string{
			"chatgpt",
			"/callback/chatgpt/status?ticket=" + ticket,
			"Finishing the login",
			"Authorized by chatgpt",
			"Exchanging the authorization",
			"Account stored",
			// The page counts down in the open before it closes itself, with the
			// seconds left as the placeholder its script replaces each tick.
			`data-close-seconds="3"`,
			"Closing in __SECONDS__",
			`id="countdown"`,
		} {
			if !strings.Contains(body, fragment) {
				t.Fatalf("the page does not carry %q", fragment)
			}
		}
		for _, header := range []string{"Content-Security-Policy", "Referrer-Policy", "Cache-Control", "X-Frame-Options"} {
			if response.Header().Get(header) == "" {
				t.Fatalf("the page carries no %s header", header)
			}
		}
	})

	t.Run("the page needs no session", func(t *testing.T) {
		if response := harness.management(http.MethodGet, "/callback/chatgpt?ticket="+ticket, "", nil); response.Code != http.StatusOK {
			t.Fatalf("status = %d, want the page without a session", response.Code)
		}
	})

	t.Run("the page paints the current mark and carries its own icon", func(t *testing.T) {
		body := harness.management(http.MethodGet, "/callback/chatgpt?ticket="+ticket, "", nil).Body.String()
		// The brand mark is one node with two arcs around it. A page that
		// paints anything else is showing a mark Relo no longer uses.
		if !strings.Contains(body, "M256 91 A165 165 0 0 1 421 256") {
			t.Fatalf("the page paints no brand mark: %s", body)
		}
		// The icon comes with the document, so the browser never falls back to
		// a console icon or a cached one.
		icon := callbackIconOf(t, body)
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(icon, "data:image/svg+xml;base64,"))
		if err != nil {
			t.Fatalf("decode the page icon: %v", err)
		}
		// The page carries the plated logo, not a loose mark: the icon must
		// carry the squircle plate's gradient, the mark baked at 0.66 of it.
		if !strings.Contains(string(decoded), "<linearGradient") || !strings.Contains(string(decoded), "A108.90 108.90") {
			t.Fatalf("the page icon is not the brand mark: %s", decoded)
		}
	})

	t.Run("the status reports how far the login got", func(t *testing.T) {
		status := callbackStatus(t, harness, "chatgpt", ticket)
		if status.Phase != oauth.CallbackWaiting || status.Provider != "chatgpt" {
			t.Fatalf("status = %+v, want the waiting phase", status)
		}
		if status.Account != "" || status.Error != "" {
			t.Fatalf("status = %+v, want no account and no failure yet", status)
		}
	})

	t.Run("a delivered grant moves the login to the exchange", func(t *testing.T) {
		delivered, err := broker.Deliver("chatgpt", url.Values{"code": {"code-1"}, "state": {"state-1"}})
		if err != nil {
			t.Fatalf("Deliver() error = %v", err)
		}
		if delivered != ticket {
			t.Fatalf("ticket = %q, want the registered one", delivered)
		}
		if status := callbackStatus(t, harness, "chatgpt", ticket); status.Phase != oauth.CallbackExchanging {
			t.Fatalf("phase = %q, want the exchange", status.Phase)
		}
	})

	t.Run("a stored account is reported with its label", func(t *testing.T) {
		broker.Complete(ticket, "work@example.test", nil)
		status := callbackStatus(t, harness, "chatgpt", ticket)
		if status.Phase != oauth.CallbackConnected || status.Account != "work@example.test" {
			t.Fatalf("status = %+v, want the stored account", status)
		}
	})

	t.Run("the status of an unknown ticket is a JSON 404", func(t *testing.T) {
		response := harness.management(http.MethodGet, "/callback/chatgpt/status?ticket=nothing", "", nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", response.Code)
		}
		if !strings.Contains(response.Header().Get("Content-Type"), "json") {
			t.Fatalf("content type = %q, want JSON", response.Header().Get("Content-Type"))
		}
	})

	t.Run("an unknown ticket renders the expired state", func(t *testing.T) {
		response := harness.management(http.MethodGet, "/callback/chatgpt?ticket=nothing", "", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want the page that explains it", response.Code)
		}
		if !strings.Contains(response.Body.String(), "This login is no longer open") {
			t.Fatalf("body = %q, want the expired state", response.Body.String())
		}
	})
}

func TestCallbackRefusals(t *testing.T) {
	broker := callbackBroker(10101)
	harness := callbackHarness(t, broker)

	t.Run("a redirect for a provider nothing started is refused", func(t *testing.T) {
		response := callbackRedirect(harness, "/callback/cursor?code=abc&state=unknown")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", response.Code)
		}
		assertCallbackFailure(t, response.Body.String(), "This login is no longer open")
	})

	t.Run("a redirect with a spent state is refused", func(t *testing.T) {
		broker.Register("chatgpt", "state-spent")
		first := callbackRedirect(harness, "/callback/chatgpt?code=abc&state=state-spent")
		if first.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want the redirect to the page", first.Code)
		}
		second := callbackRedirect(harness, "/callback/chatgpt?code=abc&state=state-spent")
		if second.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want the spent state refused", second.Code)
		}
	})

	t.Run("a state another provider started is refused", func(t *testing.T) {
		broker.Register("claude", "state-claude")
		response := callbackRedirect(harness, "/callback/chatgpt?code=abc&state=state-claude")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", response.Code)
		}
	})

	t.Run("a provider that refused the login says so", func(t *testing.T) {
		login := broker.Register("chatgpt", "state-denied")
		response := callbackRedirect(harness, "/callback/chatgpt?error=access_denied&state=state-denied")
		if response.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want the redirect to the page", response.Code)
		}
		status := callbackStatus(t, harness, "chatgpt", login.Ticket())
		if status.Phase != oauth.CallbackFailed || status.Error != oauth.CallbackDenied {
			t.Fatalf("status = %+v, want the denied failure", status)
		}
		page := harness.management(http.MethodGet, "/callback/chatgpt?ticket="+ticketOf(t, response.Header().Get("Location")), "", nil)
		assertCallbackFailure(t, page.Body.String(), "The provider refused the authorization.")
		assertCallbackFailure(t, page.Body.String(), "Not connected")
	})

	t.Run("a redirect with no state is refused", func(t *testing.T) {
		response := callbackRedirect(harness, "/callback/chatgpt?code=abc")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", response.Code)
		}
	})
}

// TestCallbackPageNamesWhyALoginFailed covers the sentence each failure
// category renders, so a browser reads what happened without being shown
// anything about the grant.
func TestCallbackPageNamesWhyALoginFailed(t *testing.T) {
	cases := []struct {
		name     string
		cause    error
		sentence string
	}{
		{"a refused authorization", oauth.ErrLoginCancelled, "The provider refused the authorization."},
		{"a spent callback", oauth.ErrInvalidState, "The callback did not match the login Relo started, or it was already used."},
		{"a provider that never answered", oauth.ErrCallbackTimeout, "The provider did not answer in time."},
		{"a failed exchange", oauth.ErrTokenExchange, "The provider would not exchange the authorization for a token."},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			broker := callbackBroker(10101)
			harness := callbackHarness(t, broker)
			login := broker.Register("claude", "state-1")
			broker.Complete(login.Ticket(), "", test.cause)
			page := harness.management(http.MethodGet, "/callback/claude?ticket="+login.Ticket(), "", nil)
			assertCallbackFailure(t, page.Body.String(), test.sentence)
			assertCallbackFailure(t, page.Body.String(), "Not connected")
		})
	}
}

// TestCallbackNeverLogsTheGrant proves the grant and the state that guards it
// stay out of the request log the daemon writes.
func TestCallbackNeverLogsTheGrant(t *testing.T) {
	broker := callbackBroker(10101)
	harness := callbackHarness(t, broker)
	broker.Register("chatgpt", "state-secret")
	code := "secret-authorization-code"

	response := callbackRedirect(harness, "/callback/chatgpt?code="+code+"&state=state-secret")
	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want the redirect to the page", response.Code)
	}
	ticket := ticketOf(t, response.Header().Get("Location"))
	harness.management(http.MethodGet, "/callback/chatgpt?ticket="+ticket, "", nil)
	harness.management(http.MethodGet, "/callback/chatgpt/status?ticket="+ticket, "", nil)

	logged := harness.logBuffer.String()
	for _, secret := range []string{code, "state-secret", "code=", "state="} {
		if strings.Contains(logged, secret) {
			t.Fatalf("the log carries %q: %s", secret, logged)
		}
	}
}

// callbackStatus reads the status of one login the way the page polls it.
func callbackStatus(t *testing.T, harness *harness, provider, ticket string) oauth.CallbackStatus {
	t.Helper()
	response := harness.management(http.MethodGet, "/callback/"+provider+"/status?ticket="+ticket, "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status endpoint = %d, want 200", response.Code)
	}
	var status oauth.CallbackStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode the callback status: %v", err)
	}
	return status
}

// callbackRedirect delivers one provider redirect to the management listener.
func callbackRedirect(harness *harness, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	harness.server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func assertCallbackFailure(t *testing.T, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

// ticketOf reads the ticket the callback page is watched with.
// callbackIconOf reads the icon a callback page declares for its own tab.
func callbackIconOf(t *testing.T, body string) string {
	t.Helper()
	matched := callbackIconPattern.FindStringSubmatch(body)
	if len(matched) != 2 {
		t.Fatalf("the page declares no icon: %s", body)
	}
	return matched[1]
}

// callbackIconPattern reads the icon one callback document declares.
var callbackIconPattern = regexp.MustCompile(`rel="icon" href="([^"]+)"`)

func ticketOf(t *testing.T, location string) string {
	t.Helper()
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse the redirect location %q: %v", location, err)
	}
	ticket := parsed.Query().Get("ticket")
	if ticket == "" {
		t.Fatalf("location = %q, want a ticket", location)
	}
	return ticket
}
