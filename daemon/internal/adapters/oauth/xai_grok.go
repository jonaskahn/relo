// xAI device flow: Grok sign-in through a user code.
package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const (
	providerXAI     = FlowGrok
	xaiClientID     = "b1a00492-073a-47ea-816f-4c329264a828"
	xaiDiscoveryURL = "https://auth.x.ai/.well-known/openid-configuration"
	xaiScope        = "openid profile email offline_access grok-cli:access api:access"
	// xaiVerifyPage is where a code is typed when the authorization start
	// response names no address of its own.
	xaiVerifyPage = "https://accounts.x.ai/oauth2/device"
)

// Discovery errors name why a Grok endpoint was refused: the response named
// somewhere Relo will not send a credential.
var (
	xaiTrustedHosts      = map[string]bool{"auth.x.ai": true, "accounts.x.ai": true}
	ErrUntrustedEndpoint = errors.New("the discovery response named an endpoint Relo does not trust")
)

type xaiFlow struct {
	settings Options
	client   tokenClient
}

// NewXAIDeviceFlow returns the device-code login for a Grok account.
func NewXAIDeviceFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	return &xaiFlow{settings: settings, client: settings.tokenClient(nil)}
}

// ProviderID returns the provider this flow logs into.
func (f *xaiFlow) ProviderID() string {
	return providerXAI
}

// CallbackPort returns zero: the Grok login runs a device grant, which
// never opens a loopback listener.
func (f *xaiFlow) CallbackPort() int {
	return 0
}

// Login runs the device grant against the discovered endpoints.
func (f *xaiFlow) Login(ctx context.Context, opts LoginOpts) (*OAuthCredential, error) {
	flow, err := f.deviceFlow(ctx)
	if err != nil {
		return nil, err
	}
	return flow.Login(ctx, opts)
}

// Refresh exchanges the stored refresh token, retrying the transient refusals
// the vendor asks for with a Retry-After delay.
func (f *xaiFlow) Refresh(ctx context.Context, cred *OAuthCredential) (*OAuthCredential, error) {
	flow, err := f.deviceFlow(ctx)
	if err != nil {
		return nil, err
	}
	return flow.Refresh(ctx, cred)
}

// Validate reports whether the credential can still serve a request.
func (f *xaiFlow) Validate(_ context.Context, cred *OAuthCredential) error {
	return validateCredential(f.settings.clockOrReal().Now(), cred)
}

func (f *xaiFlow) deviceFlow(ctx context.Context) (*deviceFlow, error) {
	deviceURL, tokenURL, err := f.endpoints(ctx)
	if err != nil {
		return nil, err
	}
	return newDeviceFlow(deviceConfig{
		providerID:      providerXAI,
		clientID:        xaiClientID,
		startURL:        deviceURL,
		tokenURL:        tokenURL,
		verificationURL: option(f.settings.Endpoints.VerificationURL, xaiVerifyPage),
		scopes:          []string{xaiScope},
		finish:          xaiIdentity,
	}, f.settings), nil
}

func xaiIdentity(_ context.Context, _ tokenClient, response tokenResponse, credential *OAuthCredential) error {
	credential.AccountID, credential.Email = IdentityFromTokens(response.IDToken, response.AccessToken)
	return nil
}

func (f *xaiFlow) endpoints(ctx context.Context) (string, string, error) {
	if f.settings.Endpoints.DeviceURL != "" && f.settings.Endpoints.TokenURL != "" {
		return f.settings.Endpoints.DeviceURL, f.settings.Endpoints.TokenURL, nil
	}
	payload, err := discoverXAI(ctx, f.client, option(f.settings.Endpoints.DiscoveryURL, xaiDiscoveryURL))
	if err != nil {
		return "", "", err
	}
	deviceURL, err := trustedEndpoint(payload.DeviceAuthorizationEndpoint)
	if err != nil {
		return "", "", err
	}
	tokenURL, err := trustedEndpoint(payload.TokenEndpoint)
	if err != nil {
		return "", "", err
	}
	return option(f.settings.Endpoints.DeviceURL, deviceURL), option(f.settings.Endpoints.TokenURL, tokenURL), nil
}

type xaiDiscovery struct {
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
}

func discoverXAI(ctx context.Context, client tokenClient, endpoint string) (xaiDiscovery, error) {
	body, status, err := client.get(ctx, endpoint, nil)
	if err != nil {
		return xaiDiscovery{}, fmt.Errorf("discover the xAI endpoints: %w", err)
	}
	if status < 200 || status >= 300 {
		return xaiDiscovery{}, fmt.Errorf("discover the xAI endpoints: %w", responseError{Status: status})
	}
	payload := xaiDiscovery{}
	if err := decodeJSONBody(body, &payload); err != nil {
		return xaiDiscovery{}, err
	}
	if payload.DeviceAuthorizationEndpoint == "" || payload.TokenEndpoint == "" {
		return xaiDiscovery{}, fmt.Errorf("discovery response carries no endpoints: %w", ErrUntrustedEndpoint)
	}
	return payload, nil
}

func trustedEndpoint(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("discovery named an unparsable endpoint: %w", ErrUntrustedEndpoint)
	}
	host := strings.ToLower(parsed.Hostname())
	if parsed.Scheme != "https" || parsed.Port() != "" || parsed.User != nil || !xaiTrustedHosts[host] {
		return "", fmt.Errorf("discovery named %s: %w", host, ErrUntrustedEndpoint)
	}
	return parsed.String(), nil
}
