package server_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/server"
)

// chatGPTProvider is the catalog row a ChatGPT login is offered for, which
// the management API reads before it starts one.
// chatGPTFlowID is the provider a ChatGPT login is registered as: the flow
// the Codex sign-in runs, which is also the catalog row the console offers.
// The callback path names it, so the page and the login stay one identity.
const chatGPTFlowID = oauth.FlowChatGPT

func chatGPTProvider() sqlite.ProviderRow {
	return sqlite.ProviderRow{
		ID: chatGPTFlowID, Origin: string(catalog.OriginSignIn), Label: "ChatGPT",
		Auth: string(catalog.AuthOAuth), APIFormat: string(catalog.FormatOpenAIResp),
		BaseURL: "https://chatgpt.com/backend-api/codex", LoginFlows: []string{chatGPTFlowID},
		ModelsFormat: string(catalog.ModelsNone),
		Enabled:      true, Rank: 100, PoolStrategy: "least-loaded",
	}
}

// chatGPTLoginRunner performs one ChatGPT login through the real flow, which
// is what the daemon hands a login the console started.
type chatGPTLoginRunner struct {
	tokenURL string
	broker   *oauth.CallbackBroker
	// seen records the broker the server handed the login, which is what
	// keeps the page and the login one conversation.
	seen chan server.CallbackBroker
}

func (r chatGPTLoginRunner) Login(ctx context.Context, request server.LoginRequest) (server.LoginResult, error) {
	if request.Callbacks == nil {
		return server.LoginResult{}, errors.New("the server handed this login no callback broker")
	}
	if r.seen != nil {
		select {
		case r.seen <- request.Callbacks:
		default:
		}
	}
	flow := oauth.NewChatGPTFlow(
		oauth.WithCallbacks(r.broker),
		oauth.WithCallbackPort(-1),
		oauth.WithEndpoints(oauth.Endpoints{TokenURL: r.tokenURL}),
	)
	credential, err := flow.Login(ctx, oauth.LoginOpts{
		NoBrowser: true,
		Prompt: func(prompt oauth.AuthPrompt) {
			if request.Prompt != nil {
				_ = request.Prompt(server.LoginPrompt{
					URL: prompt.URL, Instructions: prompt.Instructions, Ticket: prompt.Ticket,
				})
			}
		},
	})
	if err != nil {
		return server.LoginResult{}, err
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		return server.LoginResult{}, err
	}
	label := firstNonEmpty(request.Label, credential.Email, credential.AccountID, "default")
	return server.LoginResult{Label: label, Kind: string(catalog.AuthOAuth), Secret: string(encoded)}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// TestOpenAILoginEndsOnTheCallbackPage drives one ChatGPT login the way a
// browser does: it starts at the management API, follows the provider
// redirect through the loopback bridge the flow binds, and arrives at the
// page the daemon serves, which is where it reads the result.
func TestOpenAILoginEndsOnTheCallbackPage(t *testing.T) {
	tokens := fakeTokenEndpoint(t, chatGPTTokenResponse(t))

	t.Run("the happy path lands on the page and stores the account", func(t *testing.T) {
		harness, broker, seen := openAILoginHarness(t, tokens.url())
		operation := startOpenAILogin(t, harness, "work")
		select {
		case handed := <-seen:
			if handed != platform.NewCallbackBridge(broker) {
				t.Fatal("the login registered with a broker other than the one the server serves its callback page from")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the server never handed the login its callback broker")
		}
		authorize := waitForAuthorizeURL(t, harness, operation)

		location := followBridge(t, authorize, "code-1", "")
		if !strings.HasPrefix(location, "http://localhost:") || !strings.Contains(location, "/callback/"+chatGPTFlowID+"?ticket=") {
			t.Fatalf("bridge location = %q, want the daemon callback page", location)
		}
		ticket := ticketOf(t, location)

		state := waitForCallbackStatus(t, harness, chatGPTFlowID, ticket, oauth.CallbackConnected)
		if state.Account != "work" {
			t.Fatalf("account = %q, want the label the login was started with", state.Account)
		}
		page := fetchCallbackPage(t, harness, chatGPTFlowID, ticket)
		if !strings.Contains(page.Body.String(), "Connected") {
			t.Fatalf("page = %q, want the connected state", page.Body.String())
		}
		if !strings.Contains(page.Body.String(), "work") {
			t.Fatalf("page = %q, want the stored account", page.Body.String())
		}
		accounts, err := harness.accounts.Accounts(context.Background(), chatGPTFlowID)
		if err != nil {
			t.Fatalf("Accounts() error = %v", err)
		}
		if len(accounts) != 1 || accounts[0].Label != "work" {
			t.Fatalf("accounts = %+v, want the stored login", accounts)
		}
		if _, found := broker.Status(ticket); !found {
			t.Fatal("the broker forgot the login before the page could read it")
		}
	})

	t.Run("a provider that refused the login is reported on the page", func(t *testing.T) {
		harness, _, _ := openAILoginHarness(t, tokens.url())
		operation := startOpenAILogin(t, harness, "")
		authorize := waitForAuthorizeURL(t, harness, operation)

		response := bridgeRedirect(t, authorize, "", "access_denied")
		if response.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want the redirect to the page that explains it", response.Code)
		}
		page := fetchCallbackPage(t, harness, chatGPTFlowID, ticketOf(t, response.Header().Get("Location")))
		assertCallbackFailure(t, page.Body.String(), "The provider refused the authorization.")
		waitForLoginState(t, harness, operation, server.LoginFailed)
	})

	t.Run("a token endpoint that fails is reported on the page", func(t *testing.T) {
		broken := fakeTokenEndpoint(t, `{"error":"server_error"}`, http.StatusInternalServerError)
		harness, _, _ := openAILoginHarness(t, broken.url())
		operation := startOpenAILogin(t, harness, "")
		authorize := waitForAuthorizeURL(t, harness, operation)

		location := followBridge(t, authorize, "code-1", "")
		ticket := ticketOf(t, location)
		state := waitForCallbackStatus(t, harness, chatGPTFlowID, ticket, oauth.CallbackFailed)
		if state.Error != oauth.CallbackExchange {
			t.Fatalf("failure = %q, want the exchange", state.Error)
		}
		page := fetchCallbackPage(t, harness, chatGPTFlowID, ticket)
		assertCallbackFailure(t, page.Body.String(), "The provider would not exchange the authorization for a token.")
		waitForLoginState(t, harness, operation, server.LoginFailed)
	})
}

// TestOpenAILoginWithoutACallbackPage covers a flow with no broker to report
// through: it binds its own loopback listener and answers the browser itself,
// which is what a command line run does.
func TestOpenAILoginWithoutACallbackPage(t *testing.T) {
	tokens := fakeTokenEndpoint(t, chatGPTTokenResponse(t))
	prompted := make(chan oauth.AuthPrompt, 1)
	flow := oauth.NewChatGPTFlow(oauth.WithCallbackPort(-1), oauth.WithEndpoints(oauth.Endpoints{TokenURL: tokens.url()}))

	done := make(chan error, 1)
	go func() {
		_, err := flow.Login(context.Background(), oauth.LoginOpts{
			NoBrowser: true,
			Prompt:    func(prompt oauth.AuthPrompt) { prompted <- prompt },
			Timeout:   10 * time.Second,
		})
		done <- err
	}()
	prompt := <-prompted
	if prompt.Ticket != "" {
		t.Fatalf("ticket = %q, want none without a broker", prompt.Ticket)
	}
	response := bridgeRedirectFrom(t, prompt.URL, "code-1", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want the page the flow renders itself", response.Code)
	}
	if !strings.Contains(response.Body.String(), "Authorization received") {
		t.Fatalf("body = %q, want the flow own page", response.Body.String())
	}
	if err := <-done; err != nil {
		t.Fatalf("Login() error = %v", err)
	}
}

func chatGPTTokenResponse(t *testing.T) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"https://api.openai.com/auth":    map[string]any{"chatgpt_account_id": "account-1"},
		"https://api.openai.com/profile": map[string]any{"email": "user@example.test"},
	})
	if err != nil {
		t.Fatalf("marshal token claims: %v", err)
	}
	token := "e30." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
	return `{"access_token":"` + token + `","refresh_token":"refresh-1","expires_in":3600}`
}

// openAILoginHarness returns a harness whose server performs real ChatGPT
// logins, the broker they report through, and the channel that reports which
// broker the server handed the login.
func openAILoginHarness(t *testing.T, tokenURL string) (*harness, *oauth.CallbackBroker, chan server.CallbackBroker) {
	t.Helper()
	harness := newHarness(t)
	harness.providers = []sqlite.ProviderRow{chatGPTProvider()}
	broker := callbackBroker(harness.cfg.Server.Port)
	runner := chatGPTLoginRunner{tokenURL: tokenURL, broker: broker, seen: make(chan server.CallbackBroker, 1)}
	harness.server = harness.newServerWithOptions(harness.cfg, []account.PoolEntry{defaultEntry()},
		func(options *server.Options) {
			options.Callbacks = platform.NewCallbackBridge(broker)
			options.Login = runner
		})
	return harness, broker, runner.seen
}

// startOpenAILogin starts one login through the management API and returns the
// operation the console polls.
// fetchCallbackPage reads the daemon page one login is watched on, through
// the handler the daemon serves it from.
func fetchCallbackPage(t *testing.T, harness *harness, provider, ticket string) *httptest.ResponseRecorder {
	t.Helper()
	response := harness.management(http.MethodGet, "/callback/"+provider+"?ticket="+ticket, "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("the callback page = %d, want 200", response.Code)
	}
	return response
}

func startOpenAILogin(t *testing.T, harness *harness, label string) string {
	t.Helper()
	response := harness.management(http.MethodPost, "/api/v1/oauth/"+chatGPTFlowID+"/start?label="+label, adminToken, nil)
	if response.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, want 202 (body %s)", response.Code, response.Body.String())
	}
	var started struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode the start response: %v", err)
	}
	if started.OperationID == "" {
		t.Fatal("the start response carries no operation id")
	}
	return started.OperationID
}

// waitForAuthorizeURL polls one operation until the flow has published the
// address the browser has to visit.
func waitForAuthorizeURL(t *testing.T, harness *harness, operation string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response := harness.management(http.MethodGet, "/api/v1/oauth/operations/"+operation, adminToken, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("status of the operation = %d, want 200", response.Code)
		}
		var state struct {
			State string `json:"state"`
			URL   string `json:"url"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
			t.Fatalf("decode the operation: %v", err)
		}
		if state.URL != "" {
			return state.URL
		}
		if state.State == server.LoginFailed {
			t.Fatalf("the login failed before it published an address: %s", state.Error)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the login never published an authorize address")
	return ""
}

// waitForLoginState polls one operation until it reports the state a test
// waits for.
func waitForLoginState(t *testing.T, harness *harness, operation, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response := harness.management(http.MethodGet, "/api/v1/oauth/operations/"+operation, adminToken, nil)
		var state struct {
			State string `json:"state"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
			t.Fatalf("decode the operation: %v", err)
		}
		if state.State == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the login never reported %q", want)
}

// followBridge plays the browser: it delivers one authorization to the
// loopback address the provider redirects to, and returns where the browser
// was sent next.
func followBridge(t *testing.T, authorizeURL, code, failure string) string {
	t.Helper()
	response := bridgeRedirect(t, authorizeURL, code, failure)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("bridge status = %d, want the redirect to the page (body %s)", response.Code, response.Body.String())
	}
	return response.Header().Get("Location")
}

// bridgeRedirect delivers one provider redirect to the bridge the flow bound,
// which is a loopback listener rather than a handler.
func bridgeRedirect(t *testing.T, authorizeURL, code, failure string) *httptest.ResponseRecorder {
	t.Helper()
	redirect, state := authorizeParts(t, authorizeURL)
	return deliverCallback(t, redirect, code, failure, state)
}

func bridgeRedirectFrom(t *testing.T, authorizeURL, code, failure string) *httptest.ResponseRecorder {
	t.Helper()
	redirect, state := authorizeParts(t, authorizeURL)
	return deliverCallback(t, redirect, code, failure, state)
}

// deliverCallback sends one authorization to a loopback callback address and
// reports the answer the browser would have read. The redirect is not
// followed: where it points is what a test asserts.
func deliverCallback(t *testing.T, redirect, code, failure, state string) *httptest.ResponseRecorder {
	t.Helper()
	query := url.Values{}
	if failure != "" {
		query.Set("error", failure)
	}
	if code != "" {
		query.Set("code", code)
	}
	query.Set("state", state)
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
		t.Fatalf("read the bridge answer: %v", err)
	}
	recorder := httptest.NewRecorder()
	for name, values := range response.Header {
		for _, value := range values {
			recorder.Header().Add(name, value)
		}
	}
	recorder.Code = response.StatusCode
	_, _ = recorder.Body.Write(body)
	return recorder
}

// authorizeParts reads the loopback address and the state a login published.
func authorizeParts(t *testing.T, authorizeURL string) (string, string) {
	t.Helper()
	parsed, err := url.Parse(authorizeURL)
	if err != nil {
		t.Fatalf("parse the authorize url %q: %v", authorizeURL, err)
	}
	redirect := parsed.Query().Get("redirect_uri")
	state := parsed.Query().Get("state")
	if redirect == "" || state == "" {
		t.Fatalf("authorize url = %q, want a redirect_uri and a state", authorizeURL)
	}
	return redirect, state
}

// waitForCallbackStatus polls the callback status of one login until it
// reaches the phase a test waits for.
func waitForCallbackStatus(t *testing.T, harness *harness, provider, ticket, want string) oauth.CallbackStatus {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status := callbackStatus(t, harness, provider, ticket)
		if status.Phase == want {
			return status
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the login never reached %q", want)
	return oauth.CallbackStatus{}
}

// tokenEndpoint stands in for a provider token endpoint.
type tokenEndpoint struct {
	server *httptest.Server
	body   string
	status int
}

func (e *tokenEndpoint) url() string { return e.server.URL }

// fakeTokenEndpoint answers the token exchange of one login with the body a
// test hands it.
func fakeTokenEndpoint(t *testing.T, body string, status ...int) *tokenEndpoint {
	t.Helper()
	code := http.StatusOK
	if len(status) > 0 {
		code = status[0]
	}
	endpoint := &tokenEndpoint{body: body, status: code}
	endpoint.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(endpoint.status)
		_, _ = io.WriteString(w, endpoint.body)
	}))
	t.Cleanup(endpoint.server.Close)
	return endpoint
}
