// ChatGPT device flow: the code a user enters on another screen.
package oauth

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

const (
	providerChatGPTDevice = FlowChatGPTDevice
	chatGPTDeviceStartURL = "https://auth.openai.com/api/accounts/deviceauth/usercode"
	chatGPTDeviceTokenURL = "https://auth.openai.com/api/accounts/deviceauth/token"
	chatGPTDeviceRedirect = "https://auth.openai.com/deviceauth/callback"
	chatGPTDevicePage     = "https://auth.openai.com/codex/device"
	chatGPTDevicePending  = 403
	chatGPTDevicePolls    = 120
)

type chatGPTDeviceStart struct {
	DeviceAuthID string  `json:"device_auth_id"`
	UserCode     string  `json:"user_code"`
	UserCodeAlt  string  `json:"usercode"`
	Interval     seconds `json:"interval"`
}

type chatGPTDeviceGrant struct {
	AuthorizationCode string `json:"authorization_code"`
	CodeVerifier      string `json:"code_verifier"`
}

type chatGPTDeviceFlow struct {
	client         tokenClient
	clock          clock.Clock
	startURL       string
	deviceTokenURL string
	exchangeURL    string
	pageURL        string
	interval       time.Duration
}

// CallbackPort returns zero: a device grant never opens a loopback
// listener, so no port can stand in its way.
func (f *chatGPTDeviceFlow) CallbackPort() int {
	return 0
}

// NewChatGPTDeviceFlow returns the device-code login for a ChatGPT
// account, which needs no loopback redirect.
func NewChatGPTDeviceFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	withChatGPTTimeout(&settings)
	return &chatGPTDeviceFlow{
		client:         settings.tokenClient(nil),
		clock:          settings.clockOrReal(),
		startURL:       option(settings.Endpoints.DeviceURL, chatGPTDeviceStartURL),
		deviceTokenURL: option(settings.Endpoints.DeviceTokenURL, chatGPTDeviceTokenURL),
		exchangeURL:    option(settings.Endpoints.TokenURL, chatGPTTokenURL),
		pageURL:        option(settings.Endpoints.VerificationURL, chatGPTDevicePage),
		interval:       defaultDeviceInterval,
	}
}

// ProviderID returns the provider this flow logs into.
func (f *chatGPTDeviceFlow) ProviderID() string {
	return providerChatGPTDevice
}

// Login asks for a user code, waits for the human to approve it, and
// exchanges the approved grant for tokens.
func (f *chatGPTDeviceFlow) Login(ctx context.Context, opts LoginOpts) (*OAuthCredential, error) {
	start, err := f.start(ctx)
	if err != nil {
		return nil, err
	}
	announce(opts, AuthPrompt{
		URL:          f.pageURL,
		DeviceCode:   start.UserCode,
		Instructions: fmt.Sprintf("Enter the code %s at %s", start.UserCode, f.pageURL),
	})
	ctx, cancel := context.WithTimeout(ctx, loginTimeout(opts))
	defer cancel()
	grant, err := f.poll(ctx, start)
	if err != nil {
		return nil, err
	}
	return f.exchange(ctx, grant)
}

// Refresh exchanges the stored refresh token for a fresh access token.
func (f *chatGPTDeviceFlow) Refresh(ctx context.Context, cred *OAuthCredential) (*OAuthCredential, error) {
	request := refreshRequest{Endpoint: f.exchangeURL, ClientID: chatGPTClientID, Scopes: []string{chatGPTScope}}
	if cred != nil {
		request.Credential = *cred
	}
	return chatGPTRefresh(ctx, f.client, request)
}

// Validate reports whether the credential can still serve a request.
func (f *chatGPTDeviceFlow) Validate(_ context.Context, cred *OAuthCredential) error {
	return validateCredential(f.clock.Now(), cred)
}

func (f *chatGPTDeviceFlow) start(ctx context.Context) (chatGPTDeviceStart, error) {
	body, status, err := f.client.postJSONRaw(ctx, f.startURL, map[string]string{"client_id": chatGPTClientID}, nil)
	if err != nil {
		return chatGPTDeviceStart{}, fmt.Errorf("start the device authorization: %w", err)
	}
	if status < 200 || status >= 300 {
		return chatGPTDeviceStart{}, responseError{Status: status, Code: errorCode(body)}
	}
	start := chatGPTDeviceStart{}
	if err := decodeJSONBody(body, &start); err != nil {
		return chatGPTDeviceStart{}, err
	}
	if start.UserCode == "" {
		start.UserCode = start.UserCodeAlt
	}
	if start.DeviceAuthID == "" || start.UserCode == "" {
		return chatGPTDeviceStart{}, fmt.Errorf("device authorization start response carries no code: %w", ErrDeviceDenied)
	}
	return start, nil
}

func (f *chatGPTDeviceFlow) poll(ctx context.Context, start chatGPTDeviceStart) (chatGPTDeviceGrant, error) {
	interval := firstDuration(time.Duration(start.Interval)*time.Second, f.interval)
	for range chatGPTDevicePolls {
		grant, pending, err := f.pollOnce(ctx, start)
		if err != nil {
			return chatGPTDeviceGrant{}, err
		}
		if !pending {
			return grant, nil
		}
		if err := waitFor(ctx, f.clock, interval); err != nil {
			return chatGPTDeviceGrant{}, err
		}
	}
	return chatGPTDeviceGrant{}, ErrDeviceExpired
}

func (f *chatGPTDeviceFlow) pollOnce(ctx context.Context, start chatGPTDeviceStart) (chatGPTDeviceGrant, bool, error) {
	payload := map[string]string{"device_auth_id": start.DeviceAuthID, "user_code": start.UserCode}
	body, status, err := f.client.postJSONRaw(ctx, f.deviceTokenURL, payload, nil)
	if err != nil {
		return chatGPTDeviceGrant{}, false, fmt.Errorf("poll the device authorization: %w", err)
	}
	if status == chatGPTDevicePending || status == 404 {
		return chatGPTDeviceGrant{}, true, nil
	}
	if status < 200 || status >= 300 {
		return chatGPTDeviceGrant{}, false, responseError{Status: status, Code: errorCode(body)}
	}
	grant := chatGPTDeviceGrant{}
	if err := decodeJSONBody(body, &grant); err != nil {
		return chatGPTDeviceGrant{}, false, err
	}
	if grant.AuthorizationCode == "" || grant.CodeVerifier == "" {
		return chatGPTDeviceGrant{}, false, fmt.Errorf("device authorization response carries no grant: %w", ErrDeviceDenied)
	}
	return grant, false, nil
}

func (f *chatGPTDeviceFlow) exchange(ctx context.Context, grant chatGPTDeviceGrant) (*OAuthCredential, error) {
	response, err := f.client.postForm(ctx, f.exchangeURL, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {chatGPTClientID},
		"code":          {grant.AuthorizationCode},
		"code_verifier": {grant.CodeVerifier},
		"redirect_uri":  {chatGPTDeviceRedirect},
	})
	if err != nil {
		return nil, fmt.Errorf("exchange the device grant: %w", err)
	}
	credential, err := response.credential(f.clock.Now(), []string{chatGPTScope})
	if err != nil {
		return nil, err
	}
	if err := chatGPTIdentity(response, &credential); err != nil {
		return nil, err
	}
	return &credential, nil
}
