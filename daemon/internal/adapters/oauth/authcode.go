// Authorization-code flow: the redirect endpoint a browser returns to.
package oauth

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

type authCodeConfig struct {
	providerID   string
	clientID     string
	clientSecret string
	authURL      string
	tokenURL     string
	scopes       []string
	port         int
	path         string
	redirectHost string
	// redirect says how the provider hands the browser back. An empty value
	// is redirectBridge, the safe default for a provider that registers the
	// address it will call back on.
	redirect     redirectMode
	instructions string
	authParams   map[string]string
	tokenHeaders map[string]string
	decode       func(tokenResponse, *OAuthCredential) error
	validate     func(context.Context, *OAuthCredential) error
	refresh      refreshHook
	exchange     codeExchange
	afterLogin   func(context.Context, *OAuthCredential) error
}

type redirectMode string

const (
	// redirectBridge sends the browser to the loopback address the provider
	// has registered for this client. The listener there hands the answer to
	// the login and forwards the browser to the callback page, so a provider
	// that pins its redirect address still ends on the page Relo serves.
	redirectBridge redirectMode = "bridge"
	// redirectDirect sends the browser straight to the callback page. Only a
	// provider that accepts that address for this client can be set to it.
	redirectDirect redirectMode = "direct"
)

type callbackEndpoint struct {
	redirectURI string
	ticket      string
	server      *CallbackServer
	login       *CallbackRegistration
}

func (e *callbackEndpoint) wait(ctx context.Context) (string, error) {
	if e.login != nil {
		return e.login.Wait(ctx)
	}
	result, err := e.server.Wait(ctx)
	if err != nil {
		return "", err
	}
	return result.Code, nil
}

func (e *callbackEndpoint) close() {
	if e.server != nil {
		_ = e.server.Close()
	}
}

func (e *callbackEndpoint) abandon() {
	if e.login != nil {
		e.login.abandon()
	}
}

type exchangeRequest struct {
	Code         string
	State        string
	Verifier     string
	RedirectURI  string
	ClientID     string
	ClientSecret string
	TokenURL     string
	Scopes       []string
}

type refreshRequest struct {
	Endpoint   string
	ClientID   string
	Scopes     []string
	Credential OAuthCredential
}

type refreshHook func(context.Context, tokenClient, refreshRequest) (*OAuthCredential, error)

type codeExchange func(context.Context, tokenClient, exchangeRequest) (tokenResponse, error)

type authCodeFlow struct {
	cfg    authCodeConfig
	client tokenClient
	clock  clock.Clock
	opts   Options
}

func newAuthCodeFlow(cfg authCodeConfig, settings Options) *authCodeFlow {
	cfg.port = settings.port(cfg.port)
	cfg.redirectHost = settings.host(cfg.redirectHost)
	return &authCodeFlow{
		cfg:    cfg,
		client: settings.tokenClient(cfg.tokenHeaders),
		clock:  settings.clockOrReal(),
		opts:   settings,
	}
}

// ProviderID returns the provider this flow logs into.
func (f *authCodeFlow) ProviderID() string {
	return f.cfg.providerID
}

// CallbackPort returns the loopback port this flow's browser login binds, or
// zero for a login that waits on the callback page instead of a listener of
// its own. A provider that registered a redirect address with its vendor
// hands the browser to that exact port, so a listener already on it blocks
// the login and Relo has no other port to use.
func (f *authCodeFlow) CallbackPort() int {
	if f.cfg.redirect == redirectDirect {
		return 0
	}
	return f.cfg.port
}

// Login runs the browser login and returns the credential it produced.
func (f *authCodeFlow) Login(ctx context.Context, opts LoginOpts) (*OAuthCredential, error) {
	verifier, challenge := GeneratePKCE()
	state := NewState()
	endpoint, err := f.openCallback(state)
	if err != nil {
		return nil, err
	}
	defer endpoint.close()
	defer endpoint.abandon()
	prompt := AuthPrompt{
		URL:          f.authorizeURL(state, challenge, endpoint.redirectURI),
		Instructions: f.cfg.instructions,
		Ticket:       endpoint.ticket,
	}
	code, err := f.authorizationCode(ctx, opts, endpoint, state, prompt)
	if err != nil {
		return nil, err
	}
	return f.exchange(ctx, exchangeRequest{
		Code: code, State: state, Verifier: verifier, RedirectURI: endpoint.redirectURI,
	})
}

func (f *authCodeFlow) openCallback(state string) (*callbackEndpoint, error) {
	broker := f.opts.Callbacks
	if broker == nil {
		server, err := NewCallbackServer(f.cfg.port, f.cfg.path, state)
		if err != nil {
			return nil, err
		}
		return &callbackEndpoint{server: server, redirectURI: server.RedirectURI(f.cfg.redirectHost)}, nil
	}
	registration := broker.Register(f.cfg.providerID, state)
	if f.cfg.redirect == redirectDirect {
		return &callbackEndpoint{
			login: registration, ticket: registration.Ticket(),
			redirectURI: broker.PageURL(f.cfg.providerID),
		}, nil
	}
	server, err := NewCallbackServer(f.cfg.port, f.cfg.path, state)
	if err != nil {
		return nil, err
	}
	server.forwardTo(broker, f.cfg.providerID)
	return &callbackEndpoint{
		server: server, login: registration, ticket: registration.Ticket(),
		redirectURI: server.RedirectURI(f.cfg.redirectHost),
	}, nil
}

// Refresh exchanges the stored refresh token for a fresh access token.
func (f *authCodeFlow) Refresh(ctx context.Context, cred *OAuthCredential) (*OAuthCredential, error) {
	if f.cfg.refresh != nil {
		return f.cfg.refresh(ctx, f.client, f.refreshRequest(cred))
	}
	return refreshToken(ctx, refreshOptions{client: f.client,
		endpoint: f.cfg.tokenURL, clientID: f.cfg.clientID, now: f.clock.Now(), cred: cred, scopes: f.cfg.scopes})
}

// Validate reports whether the credential can still serve a request.
func (f *authCodeFlow) Validate(ctx context.Context, cred *OAuthCredential) error {
	switch {
	case cred == nil || cred.AccessToken == "":
		return ErrTokenResponse
	case expires(cred, f.clock.Now(), 0):
		return ErrCredentialExpired
	case f.cfg.validate != nil:
		return f.cfg.validate(ctx, cred)
	default:
		return nil
	}
}

func (f *authCodeFlow) refreshRequest(cred *OAuthCredential) refreshRequest {
	request := refreshRequest{Endpoint: f.cfg.tokenURL, ClientID: f.cfg.clientID, Scopes: f.cfg.scopes}
	if cred != nil {
		request.Credential = *cred
	}
	return request
}

func (f *authCodeFlow) authorizationCode(ctx context.Context, opts LoginOpts, endpoint *callbackEndpoint, state string, prompt AuthPrompt) (string, error) {
	announce(opts, prompt)
	if opts.ManualCode != nil {
		return manualCode(state, opts.ManualCode, prompt)
	}
	if err := openBrowser(opts, f.opts.Browser, prompt.URL); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, loginTimeout(opts))
	defer cancel()
	return endpoint.wait(ctx)
}

func (f *authCodeFlow) exchange(ctx context.Context, request exchangeRequest) (*OAuthCredential, error) {
	request.ClientID = f.cfg.clientID
	request.ClientSecret = f.cfg.clientSecret
	request.TokenURL = f.cfg.tokenURL
	request.Scopes = f.cfg.scopes
	response, err := f.tokenResponse(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("exchange the authorization code: %w", err)
	}
	credential, err := f.credential(response)
	if err != nil {
		return nil, err
	}
	if f.cfg.afterLogin != nil {
		if err := f.cfg.afterLogin(ctx, credential); err != nil {
			return nil, err
		}
	}
	return credential, nil
}

func (f *authCodeFlow) tokenResponse(ctx context.Context, request exchangeRequest) (tokenResponse, error) {
	if f.cfg.exchange != nil {
		return f.cfg.exchange(ctx, f.client, request)
	}
	return f.client.postForm(ctx, f.cfg.tokenURL, f.authorizationForm(request))
}

func (f *authCodeFlow) authorizationForm(request exchangeRequest) url.Values {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {request.ClientID},
		"code":          {request.Code},
		"code_verifier": {request.Verifier},
		"redirect_uri":  {request.RedirectURI},
	}
	if f.cfg.clientSecret != "" {
		form.Set("client_secret", f.cfg.clientSecret)
	}
	return form
}

func (f *authCodeFlow) credential(response tokenResponse) (*OAuthCredential, error) {
	credential, err := response.credential(f.clock.Now(), f.cfg.scopes)
	if err != nil {
		return nil, err
	}
	if f.cfg.decode != nil {
		if err := f.cfg.decode(response, &credential); err != nil {
			return nil, err
		}
	}
	return &credential, nil
}

func (f *authCodeFlow) authorizeURL(state, challenge, redirectURI string) string {
	query := url.Values{
		"client_id":             {f.cfg.clientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"scope":                 {strings.Join(f.cfg.scopes, " ")},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	for key, value := range f.cfg.authParams {
		query.Set(key, value)
	}
	return withQuery(f.cfg.authURL, query)
}

func withQuery(endpoint string, query url.Values) string {
	separator := "?"
	if strings.Contains(endpoint, "?") {
		separator = "&"
	}
	return endpoint + separator + query.Encode()
}

func announce(opts LoginOpts, prompt AuthPrompt) {
	if opts.Prompt == nil {
		return
	}
	opts.Prompt(prompt)
}

func loginTimeout(opts LoginOpts) time.Duration {
	if opts.Timeout <= 0 {
		return defaultLoginTimeout
	}
	return opts.Timeout
}

func manualCode(state string, read func(AuthPrompt) (string, error), prompt AuthPrompt) (string, error) {
	answer, err := read(prompt)
	if err != nil {
		return "", fmt.Errorf("read the authorization answer: %w", err)
	}
	result, err := parseCallbackAnswer(answer, state)
	if err != nil {
		return "", err
	}
	return result.Code, nil
}
