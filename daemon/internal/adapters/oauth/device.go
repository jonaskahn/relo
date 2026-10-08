// Device-grant flow: polling until the user approves.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

const (
	defaultDeviceInterval = 5 * time.Second
	defaultDeviceExpiry   = 15 * time.Minute
	deviceSlowDownStep    = 5 * time.Second
	maxDeviceInterval     = 60 * time.Second
)

type deviceGrant struct {
	DeviceCode      string
	UserCode        string
	VerificationURL string
	// VerificationComplete is the address that carries the code already, which
	// some providers publish so the human does not have to type it.
	VerificationComplete string
	Interval             time.Duration
	ExpiresIn            time.Duration
}

type deviceStart struct {
	DeviceCode          string  `json:"device_code"`
	UserCode            string  `json:"user_code"`
	VerificationURI     string  `json:"verification_uri"`
	VerificationURL     string  `json:"verification_url"`
	VerificationURIComp string  `json:"verification_uri_complete"`
	ExpiresIn           seconds `json:"expires_in"`
	Interval            seconds `json:"interval"`
}

func parseDeviceStart(body []byte) (deviceStart, error) {
	start := deviceStart{}
	if err := json.Unmarshal(body, &start); err != nil {
		return deviceStart{}, fmt.Errorf("decode the device authorization response: %w", err)
	}
	return start, nil
}

func (s deviceStart) grant() (*deviceGrant, error) {
	if s.DeviceCode == "" || s.UserCode == "" {
		return nil, fmt.Errorf("device authorization start response carries no code: %w", ErrDeviceDenied)
	}
	grant := &deviceGrant{
		DeviceCode:           s.DeviceCode,
		UserCode:             s.UserCode,
		VerificationURL:      firstNonEmpty(s.VerificationURL, s.VerificationURI),
		VerificationComplete: s.VerificationURIComp,
		Interval:             firstDuration(time.Duration(s.Interval)*time.Second, defaultDeviceInterval),
		ExpiresIn:            firstDuration(time.Duration(s.ExpiresIn)*time.Second, defaultDeviceExpiry),
	}
	return grant, nil
}

func (g *deviceGrant) slowDown() {
	g.Interval = min(g.Interval+deviceSlowDownStep, maxDeviceInterval)
}

func (g *deviceGrant) wait(ctx context.Context, clk clock.Clock) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-clk.After(g.Interval):
		return nil
	}
}

type deviceConfig struct {
	providerID      string
	clientID        string
	startURL        string
	tokenURL        string
	verificationURL string
	scopes          []string
	grantType       string
	startParams     map[string]string
	tokenHeaders    map[string]string
	finish          finishHook
	refresh         refreshHook
}

type finishHook func(context.Context, tokenClient, tokenResponse, *OAuthCredential) error

type deviceFlow struct {
	cfg    deviceConfig
	client tokenClient
	clock  clock.Clock
}

func newDeviceFlow(cfg deviceConfig, settings Options) *deviceFlow {
	return &deviceFlow{
		cfg:    cfg,
		client: settings.tokenClient(cfg.tokenHeaders),
		clock:  settings.clockOrReal(),
	}
}

// ProviderID returns the provider this flow logs into.
func (f *deviceFlow) ProviderID() string {
	return f.cfg.providerID
}

// CallbackPort returns zero: a device login never opens a loopback
// listener, so no port can stand in its way.
func (f *deviceFlow) CallbackPort() int {
	return 0
}

// Login runs the device grant: the human approves a code on the
// provider's page while Relo polls for the token.
func (f *deviceFlow) Login(ctx context.Context, opts LoginOpts) (*OAuthCredential, error) {
	grant, err := f.start(ctx)
	if err != nil {
		return nil, err
	}
	announce(opts, f.prompt(grant))
	ctx, cancel := context.WithTimeout(ctx, loginTimeout(opts))
	defer cancel()
	response, err := f.poll(ctx, grant)
	if err != nil {
		return nil, err
	}
	return f.finish(ctx, response)
}

// Refresh exchanges the stored refresh token for a fresh access token.
func (f *deviceFlow) Refresh(ctx context.Context, cred *OAuthCredential) (*OAuthCredential, error) {
	if f.cfg.refresh != nil {
		return f.cfg.refresh(ctx, f.client, f.refreshRequest(cred))
	}
	return refreshToken(ctx, refreshOptions{client: f.client,
		endpoint: f.cfg.tokenURL, clientID: f.cfg.clientID, now: f.clock.Now(), cred: cred, scopes: f.cfg.scopes})
}

// Validate reports whether the credential can still serve a request.
func (f *deviceFlow) Validate(_ context.Context, cred *OAuthCredential) error {
	return validateCredential(f.clock.Now(), cred)
}

func (f *deviceFlow) refreshRequest(cred *OAuthCredential) refreshRequest {
	request := refreshRequest{Endpoint: f.cfg.tokenURL, ClientID: f.cfg.clientID, Scopes: f.cfg.scopes}
	if cred != nil {
		request.Credential = *cred
	}
	return request
}

func (f *deviceFlow) prompt(grant *deviceGrant) AuthPrompt {
	page := firstNonEmpty(grant.VerificationURL, f.cfg.verificationURL)
	return AuthPrompt{
		// The complete address carries the code, so it is the one to open; the
		// page without it stays in the instructions for a human who has to type
		// the code somewhere else.
		URL:          firstNonEmpty(grant.VerificationComplete, page),
		DeviceCode:   grant.UserCode,
		Instructions: fmt.Sprintf("Enter the code %s at %s", grant.UserCode, page),
	}
}

func (f *deviceFlow) start(ctx context.Context) (*deviceGrant, error) {
	form := url.Values{"client_id": {f.cfg.clientID}}
	if len(f.cfg.scopes) > 0 {
		form.Set("scope", strings.Join(f.cfg.scopes, " "))
	}
	for key, value := range f.cfg.startParams {
		form.Set(key, value)
	}
	body, status, err := f.client.postFormRaw(ctx, f.cfg.startURL, form, nil)
	if err != nil {
		return nil, fmt.Errorf("start the device authorization: %w", err)
	}
	if status < 200 || status >= 300 {
		return nil, responseError{Status: status, Code: errorCode(body)}
	}
	start, err := parseDeviceStart(body)
	if err != nil {
		return nil, err
	}
	return start.grant()
}

func (f *deviceFlow) poll(ctx context.Context, grant *deviceGrant) (tokenResponse, error) {
	deadline := f.clock.Now().Add(grant.ExpiresIn)
	for {
		response, err := f.pollOnce(ctx, grant)
		if err == nil {
			return response, nil
		}
		if !errors.Is(err, ErrDevicePending) {
			return tokenResponse{}, err
		}
		if !f.clock.Now().Before(deadline) {
			return tokenResponse{}, ErrDeviceExpired
		}
		if err := grant.wait(ctx, f.clock); err != nil {
			return tokenResponse{}, err
		}
	}
}

func (f *deviceFlow) pollOnce(ctx context.Context, grant *deviceGrant) (tokenResponse, error) {
	form := url.Values{
		"client_id":   {f.cfg.clientID},
		"device_code": {grant.DeviceCode},
		"grant_type":  {f.grantType()},
	}
	response, err := f.client.postForm(ctx, f.cfg.tokenURL, form)
	if err == nil {
		return response, nil
	}
	var refusal responseError
	if !errors.As(err, &refusal) {
		return tokenResponse{}, err
	}
	switch refusal.Code {
	case "authorization_pending", "device_authorization_pending":
		return tokenResponse{}, ErrDevicePending
	case "slow_down":
		grant.slowDown()
		return tokenResponse{}, ErrDevicePending
	case "access_denied":
		return tokenResponse{}, ErrDeviceDenied
	case "expired_token":
		return tokenResponse{}, ErrDeviceExpired
	default:
		return tokenResponse{}, err
	}
}

func (f *deviceFlow) finish(ctx context.Context, response tokenResponse) (*OAuthCredential, error) {
	credential, err := response.credential(f.clock.Now(), f.cfg.scopes)
	if err != nil {
		return nil, err
	}
	if f.cfg.finish != nil {
		if err := f.cfg.finish(ctx, f.client, response, &credential); err != nil {
			return nil, err
		}
	}
	return &credential, nil
}

func (f *deviceFlow) grantType() string {
	return firstNonEmpty(f.cfg.grantType, "urn:ietf:params:oauth:grant-type:device_code")
}

func errorCode(body []byte) string {
	response, err := parseTokenResponse(body)
	if err != nil {
		return ""
	}
	return response.Error.Code
}

func firstDuration(values ...time.Duration) time.Duration {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
