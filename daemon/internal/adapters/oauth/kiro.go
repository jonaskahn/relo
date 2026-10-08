// Kiro sign-in flow and its session reader.
package oauth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

const (
	providerKiro         = FlowKiro
	kiroRefreshPathFmt   = "https://prod.%s.auth.tray.kiro.dev/refreshToken"
	kiroOIDCPathFmt      = "https://oidc.%s.amazonaws.com/token"
	kiroDefaultRegion    = "us-east-1"
	kiroProfileExtraKey  = "profileArn"
	kiroRegionExtraKey   = "region"
	kiroClientExtraKey   = "clientId"
	kiroSecretExtraKey   = "clientSecret"
	kiroAuthTypeExtraKey = "authType"
	kiroDesktopAuthType  = "kiro_desktop"
	kiroInstallHint      = "install the Kiro CLI and run 'kiro auth login', or import its session with relo account import kiro"
)

// Kiro errors name why its CLI session failed: none installed, none usable,
// or one restored from import that callers must treat as fragile.
var (
	ErrKiroCLIMissing      = errors.New("no Kiro CLI session is available; " + kiroInstallHint)
	ErrKiroSessionEmpty    = errors.New("the Kiro CLI session store holds no usable credential")
	ErrKiroSessionRecovery = errors.New("the Kiro session was restored from the import snapshot after the grant was rejected")
)

// KiroSession is one credential an installed Kiro CLI left behind.
type KiroSession struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	ProfileARN   string
	Region       string
	APIRegion    string
	ClientID     string
	ClientSecret string
	AccountID    string
	Email        string
}

// KiroSessionReader reads the session store of an installed Kiro CLI.
// The composition root decides which store that is, so this package
// never opens a database.
type KiroSessionReader interface {
	Sessions(ctx context.Context) ([]KiroSession, error)
}

type kiroFlow struct {
	settings Options
	client   tokenClient
	clock    clock.Clock
	snapshot []KiroSession
}

// NewKiroFlow returns the import-first login for a Kiro account.
func NewKiroFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	return &kiroFlow{settings: settings, client: settings.tokenClient(nil), clock: settings.clockOrReal()}
}

// ProviderID returns the provider this flow logs into.
func (f *kiroFlow) ProviderID() string {
	return providerKiro
}

// CallbackPort returns zero: the Kiro login imports a session an installed
// CLI already holds, so it never binds a loopback listener.
func (f *kiroFlow) CallbackPort() int {
	return 0
}

// Login imports the installed Kiro CLI session and remembers it as the
// snapshot a rejected refresh can be recovered from.
func (f *kiroFlow) Login(ctx context.Context, opts LoginOpts) (*OAuthCredential, error) {
	sessions, err := f.sessions(ctx)
	if err != nil {
		return nil, err
	}
	f.snapshot = sessions
	session := newestSession(sessions, f.clock.Now())
	if session.RefreshToken == "" && session.AccessToken == "" {
		return nil, ErrKiroSessionEmpty
	}
	announce(opts, AuthPrompt{Instructions: "Imported the installed Kiro CLI session."})
	return session.credential(), nil
}

// Refresh renews the session against Kiro, and falls back to the AWS
// OIDC token endpoint when the account has no Kiro-hosted session.
func (f *kiroFlow) Refresh(ctx context.Context, cred *OAuthCredential) (*OAuthCredential, error) {
	if cred == nil || cred.RefreshToken == "" {
		return nil, ErrNoRefreshToken
	}
	refreshed, err := f.refreshKiro(ctx, cred)
	if err == nil {
		return refreshed, nil
	}
	if !errors.Is(err, ErrRefreshRejected) {
		return nil, err
	}
	return f.recover(ctx, cred, err)
}

// Validate reports whether the credential can still serve a request.
func (f *kiroFlow) Validate(_ context.Context, cred *OAuthCredential) error {
	return validateCredential(f.clock.Now(), cred)
}

func (f *kiroFlow) sessions(ctx context.Context) ([]KiroSession, error) {
	if f.settings.KiroSessions == nil {
		return nil, ErrKiroCLIMissing
	}
	sessions, err := f.settings.KiroSessions.Sessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("import the Kiro CLI session: %w", err)
	}
	return sessions, nil
}

func (f *kiroFlow) refreshKiro(ctx context.Context, cred *OAuthCredential) (*OAuthCredential, error) {
	region := firstNonEmpty(cred.Extra[kiroRegionExtraKey], kiroDefaultRegion)
	endpoint := option(f.settings.Endpoints.TokenURL, fmt.Sprintf(kiroRefreshPathFmt, region))
	payload := map[string]string{"refreshToken": cred.RefreshToken}
	body, status, err := f.client.postJSONRaw(ctx, endpoint, payload, nil)
	if err != nil {
		return nil, fmt.Errorf("refresh the Kiro session: %w", err)
	}
	if status < 200 || status >= 300 {
		return nil, kiroRefreshFailure(status, body)
	}
	return applyKiroRefresh(cred, body, f.clock.Now())
}

func kiroRefreshFailure(status int, body []byte) error {
	if status == 401 || status == 403 {
		return fmt.Errorf("kiro refused the refresh token: %w", ErrRefreshRejected)
	}
	return fmt.Errorf("refresh the Kiro session: %w", responseError{Status: status, Code: errorCode(body)})
}

func applyKiroRefresh(cred *OAuthCredential, body []byte, now time.Time) (*OAuthCredential, error) {
	refreshed := struct {
		AccessToken  string  `json:"accessToken"`
		RefreshToken string  `json:"refreshToken"`
		ExpiresIn    seconds `json:"expiresIn"`
	}{}
	if err := decodeJSONBody(body, &refreshed); err != nil {
		return nil, err
	}
	if refreshed.AccessToken == "" {
		return nil, ErrTokenResponse
	}
	updated := cred.Clone()
	updated.AccessToken = refreshed.AccessToken
	updated.RefreshToken = firstNonEmpty(refreshed.RefreshToken, cred.RefreshToken)
	updated.ExpiresAt = now.Add(firstDuration(time.Duration(refreshed.ExpiresIn)*time.Second, defaultTokenLifetime))
	return &updated, nil
}

func (f *kiroFlow) recover(_ context.Context, cred *OAuthCredential, cause error) (*OAuthCredential, error) {
	session, found := snapshotSession(f.snapshot, cred)
	if !found {
		return nil, cause
	}
	restored := session.credential()
	if restored.AccessToken == "" {
		return nil, cause
	}
	return restored, nil
}

func newestSession(sessions []KiroSession, now time.Time) KiroSession {
	if len(sessions) == 0 {
		return KiroSession{}
	}
	newest := sessions[0]
	for _, session := range sessions[1:] {
		if session.ExpiresAt.After(newest.ExpiresAt) {
			newest = session
		}
	}
	return newest
}

func snapshotSession(sessions []KiroSession, cred *OAuthCredential) (KiroSession, bool) {
	for _, session := range sessions {
		if session.RefreshToken != "" && session.RefreshToken == cred.RefreshToken {
			return session, true
		}
		if session.AccountID != "" && session.AccountID == cred.AccountID {
			return session, true
		}
	}
	return KiroSession{}, false
}

func (s KiroSession) credential() *OAuthCredential {
	region := firstNonEmpty(s.Region, regionFromProfileARN(s.ProfileARN), kiroDefaultRegion)
	credential := &OAuthCredential{
		AccessToken:  s.AccessToken,
		RefreshToken: s.RefreshToken,
		ExpiresAt:    s.ExpiresAt,
		AccountID:    s.AccountID,
		Email:        s.Email,
		Scope:        providerKiro,
	}
	addExtra(credential, kiroProfileExtraKey, s.ProfileARN)
	addExtra(credential, kiroRegionExtraKey, region)
	addExtra(credential, "apiRegion", firstNonEmpty(s.APIRegion, region))
	addExtra(credential, kiroClientExtraKey, s.ClientID)
	addExtra(credential, kiroSecretExtraKey, s.ClientSecret)
	addExtra(credential, kiroAuthTypeExtraKey, kiroDesktopAuthType)
	return credential
}

// RegionFromProfileARN returns the region an AWS CodeWhisperer profile
// ARN names, which is where the session has to be refreshed.
func RegionFromProfileARN(profileARN string) string {
	segments := strings.Split(profileARN, ":")
	if len(segments) < 4 {
		return ""
	}
	return strings.TrimSpace(segments[3])
}

func regionFromProfileARN(profileARN string) string {
	return RegionFromProfileARN(profileARN)
}
