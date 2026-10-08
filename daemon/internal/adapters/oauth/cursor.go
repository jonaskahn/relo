// Cursor sign-in flow.
package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

const (
	providerCursor     = FlowCursor
	cursorLoginURL     = "https://cursor.com/loginDeepControl"
	cursorPollURL      = "https://api2.cursor.sh/auth/poll"
	cursorRefreshURL   = "https://api2.cursor.sh/auth/exchange_user_api_key"
	cursorPollAttempts = 150
	cursorBaseDelay    = time.Second
	cursorBackoff      = 1.2
	cursorMaxDelay     = 10 * time.Second
	cursorRefreshSkew  = 5 * time.Minute
	cursorFallbackTTL  = time.Hour
	cursorErrorBudget  = 3
)

// ErrCursorLoginRejected reports a login Cursor answered with a status
// that only a fresh attempt can fix.
var ErrCursorLoginRejected = errors.New("cursor rejected the login attempt")

type cursorFlow struct {
	settings   Options
	client     tokenClient
	clock      clock.Clock
	loginURL   string
	pollURL    string
	refreshURL string
}

// NewCursorFlow returns the browser login for a Cursor account.
func NewCursorFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	return &cursorFlow{
		settings:   settings,
		client:     settings.tokenClient(nil),
		clock:      settings.clockOrReal(),
		loginURL:   option(settings.Endpoints.AuthURL, cursorLoginURL),
		pollURL:    option(settings.Endpoints.APIBaseURL, cursorPollURL),
		refreshURL: option(settings.Endpoints.TokenURL, cursorRefreshURL),
	}
}

// ProviderID returns the provider this flow logs into.
func (f *cursorFlow) ProviderID() string {
	return providerCursor
}

// CallbackPort returns zero: the Cursor login polls its own page for the
// answer instead of receiving a redirect on a loopback listener.
func (f *cursorFlow) CallbackPort() int {
	return 0
}

// Login opens the approval URL that carries the PKCE challenge and polls
// until Cursor hands back the tokens.
func (f *cursorFlow) Login(ctx context.Context, opts LoginOpts) (*OAuthCredential, error) {
	verifier, challenge := GeneratePKCE()
	requestID := newUUID()
	approvalURL := withQuery(f.loginURL, url.Values{
		"challenge":      {challenge},
		"uuid":           {requestID},
		"mode":           {"login"},
		"redirectTarget": {"cli"},
	})
	prompt := AuthPrompt{URL: approvalURL, Instructions: "Approve the Cursor login in your browser, then return here."}
	announce(opts, prompt)
	if err := openBrowser(opts, f.settings.Browser, approvalURL); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, loginTimeout(opts))
	defer cancel()
	return f.poll(ctx, requestID, verifier)
}

// Refresh exchanges the stored refresh token for fresh tokens.
func (f *cursorFlow) Refresh(ctx context.Context, cred *OAuthCredential) (*OAuthCredential, error) {
	if cred == nil || cred.RefreshToken == "" {
		return nil, ErrNoRefreshToken
	}
	headers := map[string]string{"Authorization": "Bearer " + cred.RefreshToken}
	body, status, err := f.client.postJSONRaw(ctx, f.refreshURL, map[string]string{}, headers)
	if err != nil {
		return nil, fmt.Errorf("refresh the access token: %w", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("refresh the access token: %w", responseError{Status: status, Code: errorCode(body)})
	}
	tokens := cursorTokens{}
	if err := decodeJSONBody(body, &tokens); err != nil {
		return nil, err
	}
	return f.credential(tokens, cred)
}

// Validate reports whether the credential can still serve a request.
func (f *cursorFlow) Validate(_ context.Context, cred *OAuthCredential) error {
	return validateCredential(f.clock.Now(), cred)
}

type cursorTokens struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

func (f *cursorFlow) poll(ctx context.Context, uuid, verifier string) (*OAuthCredential, error) {
	delay := cursorBaseDelay
	failures := 0
	for attempt := 0; attempt < cursorPollAttempts; attempt++ {
		if err := waitFor(ctx, f.clock, delay); err != nil {
			return nil, err
		}
		tokens, pending, err := f.pollOnce(ctx, uuid, verifier)
		if err != nil {
			return nil, err
		}
		if !pending {
			return f.credential(tokens, nil)
		}
		failures = 0
		delay = min(time.Duration(float64(delay)*cursorBackoff), cursorMaxDelay)
		if failures >= cursorErrorBudget {
			return nil, fmt.Errorf("poll the cursor login: %w", ErrLoginCancelled)
		}
	}
	return nil, fmt.Errorf("poll the cursor login: %w", ErrCallbackTimeout)
}

func (f *cursorFlow) pollOnce(ctx context.Context, uuid, verifier string) (cursorTokens, bool, error) {
	endpoint := withQuery(f.pollURL, url.Values{"uuid": {uuid}, "verifier": {verifier}})
	body, status, err := f.client.get(ctx, endpoint, nil)
	if err != nil {
		return cursorTokens{}, false, fmt.Errorf("poll the cursor login: %w", err)
	}
	switch {
	case status == 404:
		return cursorTokens{}, true, nil
	case status >= 200 && status < 300:
		return decodeCursorTokens(body)
	case cursorTerminalStatus(status):
		return cursorTokens{}, false, fmt.Errorf("cursor answered %d: %w", status, ErrCursorLoginRejected)
	default:
		return cursorTokens{}, false, fmt.Errorf("poll the cursor login: %w", responseError{Status: status, Code: errorCode(body)})
	}
}

func (f *cursorFlow) credential(tokens cursorTokens, previous *OAuthCredential) (*OAuthCredential, error) {
	if tokens.AccessToken == "" {
		return nil, ErrTokenResponse
	}
	credential := OAuthCredential{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    cursorExpiry(tokens.AccessToken, f.clock.Now()),
	}
	credential.AccountID = SubjectFromToken(tokens.AccessToken)
	credential.Email = EmailFromTokens("", tokens.AccessToken)
	if previous != nil {
		merged := mergeCredential(*previous, credential)
		return &merged, nil
	}
	return &credential, nil
}

func cursorTerminalStatus(status int) bool {
	switch status {
	case 400, 401, 403, 410:
		return true
	default:
		return false
	}
}

func decodeCursorTokens(body []byte) (cursorTokens, bool, error) {
	tokens := cursorTokens{}
	if err := decodeJSONBody(body, &tokens); err != nil {
		return cursorTokens{}, false, err
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		return cursorTokens{}, false, ErrTokenResponse
	}
	return tokens, false, nil
}

func cursorExpiry(token string, now time.Time) time.Time {
	claims, ok := decodeClaims(token)
	if !ok {
		return now.Add(cursorFallbackTTL)
	}
	expiry, ok := claims.number("exp")
	if !ok || expiry <= 0 {
		return now.Add(cursorFallbackTTL)
	}
	return time.Unix(int64(expiry), 0).Add(-cursorRefreshSkew)
}
