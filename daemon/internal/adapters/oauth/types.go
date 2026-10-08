// Package oauth implements the OAuth logins Relo offers its providers
// and the machinery they all share: PKCE, the loopback callback server,
// token exchange, the refresh guardian, and the flow registry.
package oauth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

// OAuthCredential is one logged-in account as a flow produces and
// refreshes it. The token material never reaches a log or a fixture.
type OAuthCredential struct {
	AccessToken  string            `json:"access_token"`
	RefreshToken string            `json:"refresh_token"`
	ExpiresAt    time.Time         `json:"expires_at"`
	AccountID    string            `json:"account_id,omitempty"`
	Email        string            `json:"email,omitempty"`
	Scope        string            `json:"scope,omitempty"`
	Extra        map[string]string `json:"extra,omitempty"`
}

// ExpiresWithin reports whether the access token expires inside the
// margin the caller wants to keep in hand.
func (c OAuthCredential) ExpiresWithin(now time.Time, margin time.Duration) bool {
	return !c.ExpiresAt.After(now.Add(margin))
}

// Clone returns a copy that shares no map with the receiver.
func (c OAuthCredential) Clone() OAuthCredential {
	clone := c
	if c.Extra == nil {
		return clone
	}
	clone.Extra = make(map[string]string, len(c.Extra))
	for key, value := range c.Extra {
		clone.Extra[key] = value
	}
	return clone
}

// AuthPrompt is the instruction a login shows a human: the URL to visit
// and, for device flows, the code to type into it. Ticket names the login on
// the callback page the browser lands on, which is how the surface that
// started the login reports its outcome; a flow that renders its own page
// leaves it empty.
type AuthPrompt struct {
	URL          string
	DeviceCode   string
	Instructions string
	Ticket       string
}

// LoginOpts tunes one login: where the prompt goes, whether a browser
// opens, and how long the flow waits for the human.
type LoginOpts struct {
	Prompt     func(AuthPrompt)
	ManualCode func(AuthPrompt) (string, error)
	NoBrowser  bool
	Timeout    time.Duration
}

// OAuthFlow is one provider's login, refresh, and validation.
type OAuthFlow interface {
	ProviderID() string
	// CallbackPort is the loopback port this flow's browser login needs, or
	// zero when the login waits on the callback page instead. A caller that
	// has to clear a port before a login can run reads it here.
	CallbackPort() int
	Login(ctx context.Context, opts LoginOpts) (*OAuthCredential, error)
	Refresh(ctx context.Context, cred *OAuthCredential) (*OAuthCredential, error)
	Validate(ctx context.Context, cred *OAuthCredential) error
}

// FlowFactory builds one provider's flow on demand.
type FlowFactory func() OAuthFlow

const (
	defaultLoginTimeout  = 5 * time.Minute
	defaultHTTPTimeout   = 30 * time.Second
	defaultRefreshMargin = 5 * time.Minute
	defaultUserAgent     = "relo"
)

// OAuth errors name every way a sign-in can fail, so every flow answers
// with the same vocabulary whatever vendor it speaks to.
var (
	ErrPortBusy           = errors.New("oauth callback port is already in use")
	ErrCallbackTimeout    = errors.New("oauth callback did not arrive in time")
	ErrInvalidState       = errors.New("oauth callback state does not match")
	ErrMissingCode        = errors.New("oauth answer carried no authorization code")
	ErrLoginCancelled     = errors.New("oauth login was cancelled")
	ErrTokenExchange      = errors.New("oauth token exchange failed")
	ErrTokenResponse      = errors.New("oauth token response carried no access token")
	ErrRefreshRejected    = errors.New("the provider rejected the refresh token; log in again")
	ErrNoRefreshToken     = errors.New("the credential carries no refresh token")
	ErrCredentialExpired  = errors.New("the credential access token has expired")
	ErrFlowNotFound       = errors.New("no oauth flow is registered for this provider")
	ErrBrowserUnavailable = errors.New("no browser is available to open")
	ErrDevicePending      = errors.New("device authorization is still pending")
	ErrDeviceDenied       = errors.New("device authorization was denied")
	ErrDeviceExpired      = errors.New("device authorization expired")
	ErrEndpointRequired   = errors.New("the provider endpoint is not configured")
)

// Options tunes a flow. The zero value keeps production behavior, so a
// caller only sets the seams a test or a self-hosted provider needs.
type Options struct {
	HTTPClient   *http.Client
	Clock        clock.Clock
	Browser      Browser
	Headless     bool
	Endpoints    Endpoints
	Retries      int
	CallbackPort int
	RedirectHost string
	// Callbacks routes the browser back to the login that started it when the
	// process runs the callback page. A flow built without one binds its own
	// loopback listener and renders the page itself, which is what a command
	// line run with no daemon does.
	Callbacks       *CallbackBroker
	KiroSessions    KiroSessionReader
	Keychain        KeychainReader
	CredentialsPath string
	Warn            func(message string)
	Logger          *slog.Logger
}

// Endpoints overrides provider addresses, so a test points a flow at a
// local server instead of the vendor.
type Endpoints struct {
	AuthURL         string
	TokenURL        string
	DeviceURL       string
	DeviceTokenURL  string
	VerificationURL string
	DailyAPIBaseURL string
	MintURL         string
	UserURL         string
	APIBaseURL      string
	DiscoveryURL    string
	AuthorizeURL    string
}

// Option sets one field of the flow options.
type Option func(*Options)

// WithHTTPClient sends every provider call through client.
func WithHTTPClient(client *http.Client) Option {
	return func(options *Options) { options.HTTPClient = client }
}

// WithClock reads wall time through clk, which tests drive by hand.
func WithClock(clk clock.Clock) Option {
	return func(options *Options) { options.Clock = clk }
}

// WithBrowser opens authorization URLs with browser.
func WithBrowser(browser Browser) Option {
	return func(options *Options) { options.Browser = browser }
}

// WithHeadless assumes no browser can open, so a login prints its
// authorization URL instead. It only fills the default browser: a browser
// set through WithBrowser keeps its own behavior.
func WithHeadless(headless bool) Option {
	return func(options *Options) {
		options.Headless = headless
		if options.Browser == nil {
			options.Browser = platformBrowser{headless: headless}
		}
	}
}

// WithEndpoints points a flow at other provider addresses.
func WithEndpoints(endpoints Endpoints) Option {
	return func(options *Options) { options.Endpoints = endpoints }
}

// WithRetries sets how often a flow retries a transient token failure.
func WithRetries(retries int) Option {
	return func(options *Options) { options.Retries = retries }
}

// WithCallbackPort binds the loopback listener to port.
func WithCallbackPort(port int) Option {
	return func(options *Options) { options.CallbackPort = port }
}

// WithRedirectHost sets the host name a provider redirects back to.
func WithRedirectHost(host string) Option {
	return func(options *Options) { options.RedirectHost = host }
}

// WithCallbacks routes every browser login through broker, so the page on
// the management listener is where the browser ends up.
func WithCallbacks(broker *CallbackBroker) Option {
	return func(options *Options) { options.Callbacks = broker }
}

// WithKiroSessions reads installed Kiro sessions from reader.
func WithKiroSessions(reader KiroSessionReader) Option {
	return func(options *Options) { options.KiroSessions = reader }
}

// WithKeychain reads secrets a vendor CLI stored in the OS keychain.
func WithKeychain(reader KeychainReader) Option {
	return func(options *Options) { options.Keychain = reader }
}

// WithCredentialsPath points a flow at another local credential file.
func WithCredentialsPath(path string) Option {
	return func(options *Options) { options.CredentialsPath = path }
}

// WithWarn sends the warnings a flow must show to warn.
func WithWarn(warn func(message string)) Option {
	return func(options *Options) { options.Warn = warn }
}

// WithLogger reports flow progress through logger.
func WithLogger(logger *slog.Logger) Option {
	return func(options *Options) { options.Logger = logger }
}

func newOptions(options ...Option) Options {
	settings := Options{}
	for _, option := range options {
		option(&settings)
	}
	return settings
}

func (o Options) httpClient() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return &http.Client{Timeout: defaultHTTPTimeout}
}

func (o Options) clockOrReal() clock.Clock {
	if o.Clock != nil {
		return o.Clock
	}
	return clock.New()
}

func (o Options) tokenClient(headers map[string]string) tokenClient {
	return tokenClient{http: o.httpClient(), clock: o.clockOrReal(), retries: o.Retries, headers: headers}
}

func (o Options) port(fallback int) int {
	switch {
	case o.CallbackPort < 0:
		return 0
	case o.CallbackPort > 0:
		return o.CallbackPort
	default:
		return fallback
	}
}

func (o Options) host(fallback string) string {
	if o.RedirectHost != "" {
		return o.RedirectHost
	}
	return fallback
}

func option(override, fallback string) string {
	if override != "" {
		return override
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
