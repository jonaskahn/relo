// Package credential turns one stored account into the authentication one
// upstream attempt carries: an API key read from the secret store, or a
// subscription token refreshed before it expires.
package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/awsauth"
	"github.com/jonaskahn/relo/internal/adapters/gcpauth"
	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/catalog"
	"net/http"
)

var (
	// ErrNoCredential reports a provider with nothing left to try.
	ErrNoCredential = account.ErrNoCredentials
	// ErrReauthRequired reports a credential the upstream refused, which only
	// a new login can fix.
	ErrReauthRequired = errors.New("the account must be linked again")
	// ErrNotSignedIn reports a sign-in provider whose credential Relo cannot
	// read back as a token.
	ErrNotSignedIn = errors.New("the stored credential is not an oauth token")
)

const refreshMargin = 2 * time.Minute

// Pools is what the source needs from the credential pools: an account that
// can take a request, its secret, and the feedback that moves it in or out of
// rotation.
type Pools interface {
	Select(providerID string, selection account.Selection) (account.PoolEntry, error)
	// Entry returns one named credential, which is what reading a single
	// account's own model roster requires.
	Entry(providerID, credentialID string) (account.PoolEntry, bool)
	ResolveSecret(entry account.PoolEntry) (string, error)
	RecordSuccess(providerID, id string)
	RecordFailure(providerID, id string, status int, retryAfter time.Duration)
	MarkNeedsReauth(ctx context.Context, providerID, id string) error
}

// Secrets reads and writes the values a credential points at.
type Secrets interface {
	Get(ref string) (string, error)
	Set(ref, value string) error
}

// Flows resolves the login flow that refreshes one provider's token.
type Flows interface {
	Flow(providerID string) (oauth.OAuthFlow, error)
}

// Authorizer turns a stored subscription credential into the authentication a
// request to that sign-in carries.
type Authorizer func(oauth.OAuthCredential) oauth.Authorization

// Adapters resolves the authorizer of one sign-in provider.
type Adapters interface {
	Authorizer(providerID string) (Authorizer, bool)
}

// RefreshPolicy reports whether one provider may renew an access token on
// its own. A nil policy allows every provider to refresh.
type RefreshPolicy interface {
	AutoRefresh(ctx context.Context, providerID string) bool
}

// Options tunes a source.
type Options struct {
	Pools    Pools
	Secrets  Secrets
	Flows    Flows
	Adapters Adapters
	Policy   RefreshPolicy
	Now      func() time.Time
}

// Source resolves the credential one attempt runs with.
type Source struct {
	pools     Pools
	secrets   Secrets
	flows     Flows
	adapters  Adapters
	policy    RefreshPolicy
	now       func() time.Time
	refreshes *refreshGuard
}

// New returns a credential source over the given pools and stores.
func New(options Options) *Source {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Source{
		pools: options.Pools, secrets: options.Secrets, flows: options.Flows,
		adapters: options.Adapters, policy: options.Policy, now: now, refreshes: newRefreshGuard(),
	}
}

// Request names the provider one attempt is for.
type Request = catalog.AuthRequest

// Authorization is one resolved credential: what to send, where, and what to
// call it in a log.
type Authorization = catalog.Authorization

// Authorize resolves the credential one attempt runs with: no credential at
// all for a keyless endpoint, a stored key for an API-key provider, and a
// live token for a sign-in.
func (s *Source) Authorize(ctx context.Context, request Request) (Authorization, error) {
	switch request.Auth {
	case catalog.AuthNone:
		return Authorization{Label: "keyless", Auth: catalog.AuthNone}, nil
	}
	entry, err := s.pools.Select(request.ProviderID, request.Selection)
	if err != nil {
		return Authorization{}, err
	}
	return s.authorizeEntry(ctx, request, entry)
}

// AuthorizeCredential resolves one named account rather than the pool's own
// choice, so a per-account model listing reads the roster of the account the
// caller asked about.
func (s *Source) AuthorizeCredential(ctx context.Context, request Request, credentialID string) (Authorization, error) {
	if request.Auth == catalog.AuthNone {
		return Authorization{Label: "keyless", Auth: catalog.AuthNone}, nil
	}
	entry, found := s.pools.Entry(request.ProviderID, credentialID)
	if !found {
		return Authorization{}, fmt.Errorf("%s: %w", credentialID, account.ErrNotFound)
	}
	return s.authorizeEntry(ctx, request, entry)
}

func (s *Source) authorizeEntry(ctx context.Context, request Request, entry account.PoolEntry) (Authorization, error) {
	switch request.Auth {
	case catalog.AuthOAuth:
		return s.authorizeSignIn(ctx, request, entry)
	case catalog.AuthAWS:
		return s.authorizeAWS(ctx, request, entry)
	case catalog.AuthGCP:
		return s.authorizeGCP(ctx, request, entry)
	default:
		return s.authorizeKey(ctx, request, entry)
	}
}

// Direct resolves a credential that Relo has not stored yet, which is what a
// connection test authorizes with. It signs, exchanges and places a key in
// headers exactly as a routed request does, so a test that passes says the
// credential works rather than only that some request went out.
func Direct(ctx context.Context, request Request, secret string) (Authorization, error) {
	switch request.Auth {
	case catalog.AuthNone:
		return Authorization{Label: "keyless", Auth: catalog.AuthNone}, nil
	case catalog.AuthOAuth:
		return Authorization{}, fmt.Errorf("%s: %w", request.ProviderID, ErrNotSignedIn)
	case catalog.AuthAWS:
		return directAWS(request, secret)
	case catalog.AuthGCP:
		return directGCP(ctx, secret)
	default:
		keyHeader := request.KeyHeader
		if keyHeader == "" {
			keyHeader = catalog.KeyHeaderBearer
		}
		return Authorization{
			Auth: catalog.AuthAPIKey, KeyHeader: keyHeader, Token: secret,
		}, nil
	}
}
func (s *Source) authorizeKey(_ context.Context, request Request, entry account.PoolEntry) (Authorization, error) {
	secret, err := s.pools.ResolveSecret(entry)
	if err != nil {
		return Authorization{}, err
	}
	keyHeader := request.KeyHeader
	if keyHeader == "" {
		keyHeader = catalog.KeyHeaderBearer
	}
	return Authorization{
		CredentialID: entry.ID,
		Label:        entry.Label,
		Auth:         catalog.AuthAPIKey,
		KeyHeader:    keyHeader,
		Token:        secret,
	}, nil
}

func (s *Source) authorizeAWS(_ context.Context, request Request, entry account.PoolEntry) (Authorization, error) {
	secret, err := s.pools.ResolveSecret(entry)
	if err != nil {
		return Authorization{}, err
	}
	auth, err := directAWS(request, secret)
	if err != nil {
		return Authorization{}, err
	}
	auth.CredentialID, auth.Label = entry.ID, entry.Label
	return auth, nil
}

type awsKeySet struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token"`
	Region          string `json:"region"`
}

func directAWS(request Request, secret string) (Authorization, error) {
	var keys awsKeySet
	if err := json.Unmarshal([]byte(secret), &keys); err != nil || keys.AccessKeyID == "" {
		return Authorization{
			Auth:      catalog.AuthAWS,
			KeyHeader: catalog.KeyHeaderBearer,
			Token:     secret,
		}, nil
	}
	region := keys.Region
	if region == "" {
		region = request.Region
	}
	if region == "" {
		region = "us-east-1"
	}
	service := request.Service
	if service == "" {
		service = "bedrock"
	}
	return signedAWSAuthorization(keys, region, service), nil
}

func signedAWSAuthorization(keys awsKeySet, region, service string) Authorization {
	return Authorization{
		Auth:      catalog.AuthAWS,
		KeyHeader: catalog.KeyHeaderBearer,
		Signer: func(req *http.Request) error {
			body, err := awsauth.ReadAndReplaceBody(req)
			if err != nil {
				return err
			}
			return awsauth.Sign(awsauth.SignOptions{
				Request: req, Body: body,
				AccessKey: keys.AccessKeyID, SecretKey: keys.SecretAccessKey,
				SessionToken: keys.SessionToken, Region: region, Service: service,
			})
		},
	}
}

func (s *Source) authorizeGCP(ctx context.Context, request Request, entry account.PoolEntry) (Authorization, error) {
	secret, err := s.pools.ResolveSecret(entry)
	if err != nil {
		return Authorization{}, err
	}
	auth, err := directGCP(ctx, secret)
	if err != nil {
		return Authorization{}, err
	}
	auth.CredentialID, auth.Label = entry.ID, entry.Label
	return auth, nil
}

func directGCP(ctx context.Context, secret string) (Authorization, error) {
	ts, err := gcpauth.NewTokenSource([]byte(secret), nil)
	if err == nil {
		token, err := ts.Token(ctx)
		if err != nil {
			return Authorization{}, err
		}
		return Authorization{
			Auth:      catalog.AuthGCP,
			KeyHeader: catalog.KeyHeaderBearer,
			Token:     token,
			Project:   ts.ProjectID(),
		}, nil
	}
	return Authorization{
		Auth:      catalog.AuthGCP,
		KeyHeader: catalog.KeyHeaderBearer,
		Token:     secret,
	}, nil
}

func (s *Source) authorizeSignIn(ctx context.Context, request Request, entry account.PoolEntry) (Authorization, error) {
	credential, err := s.refreshed(ctx, request.ProviderID, entry)
	if err != nil {
		return Authorization{}, err
	}
	return s.signInAuthorization(request.ProviderID, entry, credential)
}

func (s *Source) signInAuthorization(providerID string, entry account.PoolEntry, credential oauth.OAuthCredential) (Authorization, error) {
	adapter, found := s.adapters.Authorizer(providerID)
	if !found {
		return Authorization{}, fmt.Errorf("%s: %w", providerID, ErrNotSignedIn)
	}
	resolved := adapter(credential)
	return Authorization{
		CredentialID: entry.ID, Label: entry.Label, Auth: catalog.AuthOAuth,
		Token: resolved.Token, Headers: resolved.Headers,
		BaseURL: resolved.BaseURL, Project: resolved.Project,
	}, nil
}

// Outcome is what one attempt's status says about the credential behind it.
type Outcome struct {
	ProviderID   string
	CredentialID string
	Status       int
	RetryAfter   time.Duration
	// RefreshSettled reports that a refused token was already handled: the
	// refresh was rejected and the account now asks for a new login, or the
	// refresh failed in transit and the account stays active. The caller
	// must not treat that status as a fresh refusal.
	RefreshSettled bool
}

// Report records what an attempt's status means for its credential, so a
// paused account stays out of rotation and a refused token asks for a new
// login instead of failing every request after it.
func (s *Source) Report(ctx context.Context, outcome Outcome) error {
	if outcome.CredentialID == "" {
		return nil
	}
	switch {
	case outcome.Status >= 200 && outcome.Status < 300:
		s.pools.RecordSuccess(outcome.ProviderID, outcome.CredentialID)
		return nil
	case outcome.Status == 401 || outcome.Status == 403:
		if outcome.RefreshSettled {
			return nil
		}
		return s.pools.MarkNeedsReauth(ctx, outcome.ProviderID, outcome.CredentialID)
	default:
		s.pools.RecordFailure(outcome.ProviderID, outcome.CredentialID, outcome.Status, outcome.RetryAfter)
		return nil
	}
}

func (s *Source) refreshed(ctx context.Context, providerID string, entry account.PoolEntry) (oauth.OAuthCredential, error) {
	stored, err := s.read(entry)
	if err != nil {
		return oauth.OAuthCredential{}, err
	}
	if !stored.ExpiresWithin(s.now(), refreshMargin) {
		return stored, nil
	}
	if !s.autoRefresh(ctx, providerID) {
		return oauth.OAuthCredential{}, s.requireReauth(ctx, providerID, entry)
	}
	refreshed, err := s.refresh(ctx, providerID, entry, stored)
	if err != nil {
		return oauth.OAuthCredential{}, err
	}
	return refreshed, nil
}

// RefreshRejected renews the access token a request just refused, unless
// another request already stored a newer one. A provider with no login flow
// is refused. When automatic refresh is off, the account asks for a new
// login instead of calling the token endpoint.
func (s *Source) RefreshRejected(ctx context.Context, providerID, credentialID, rejectedToken string) (Authorization, error) {
	entry, found := s.pools.Entry(providerID, credentialID)
	if !found {
		return Authorization{}, fmt.Errorf("%s: %w", credentialID, account.ErrNotFound)
	}
	if s.flows == nil {
		return Authorization{}, fmt.Errorf("%s: %w", providerID, oauth.ErrFlowNotFound)
	}
	if _, err := s.flows.Flow(providerID); err != nil {
		return Authorization{}, err
	}
	stored, err := s.read(entry)
	if err != nil {
		return Authorization{}, err
	}
	if !s.autoRefresh(ctx, providerID) {
		return Authorization{}, s.requireReauth(ctx, providerID, entry)
	}
	next := stored
	if stored.AccessToken == rejectedToken {
		next, err = s.refresh(ctx, providerID, entry, stored)
		if err != nil {
			return Authorization{}, err
		}
	}
	return s.signInAuthorization(providerID, entry, next)
}

func (s *Source) autoRefresh(ctx context.Context, providerID string) bool {
	if s.policy == nil {
		return true
	}
	return s.policy.AutoRefresh(ctx, providerID)
}

func (s *Source) requireReauth(ctx context.Context, providerID string, entry account.PoolEntry) error {
	if err := s.pools.MarkNeedsReauth(ctx, providerID, entry.ID); err != nil {
		return err
	}
	return fmt.Errorf("%s: %w", entry.ID, ErrReauthRequired)
}

func (s *Source) read(entry account.PoolEntry) (oauth.OAuthCredential, error) {
	if s.secrets == nil || entry.SecretRef == "" {
		return oauth.OAuthCredential{}, fmt.Errorf("%s: %w", entry.ID, ErrNotSignedIn)
	}
	raw, err := s.secrets.Get(entry.SecretRef)
	if err != nil {
		return oauth.OAuthCredential{}, fmt.Errorf("read the credential of %s: %w", entry.ID, err)
	}
	var credential oauth.OAuthCredential
	if err := json.Unmarshal([]byte(raw), &credential); err != nil {
		return oauth.OAuthCredential{}, fmt.Errorf("%s: %w", entry.ID, ErrNotSignedIn)
	}
	return credential, nil
}

func (s *Source) refresh(ctx context.Context, providerID string, entry account.PoolEntry, stored oauth.OAuthCredential) (oauth.OAuthCredential, error) {
	if s.flows == nil {
		return oauth.OAuthCredential{}, fmt.Errorf("%s: %w", providerID, oauth.ErrFlowNotFound)
	}
	flow, err := s.flows.Flow(providerID)
	if err != nil {
		return oauth.OAuthCredential{}, err
	}
	return s.refreshes.run(entry.ID, func() (oauth.OAuthCredential, error) {
		next, refreshErr := flow.Refresh(ctx, &stored)
		if refreshErr != nil {
			if peer, found := s.peerRotated(entry, stored); found {
				return peer, nil
			}
			return oauth.OAuthCredential{}, s.rejected(ctx, providerID, entry, refreshErr)
		}
		if writeErr := s.write(entry, *next); writeErr != nil {
			return oauth.OAuthCredential{}, writeErr
		}
		return *next, nil
	})
}

func (s *Source) peerRotated(entry account.PoolEntry, attempted oauth.OAuthCredential) (oauth.OAuthCredential, bool) {
	fresh, err := s.read(entry)
	if err != nil {
		return oauth.OAuthCredential{}, false
	}
	if fresh.RefreshToken == attempted.RefreshToken {
		return oauth.OAuthCredential{}, false
	}
	return fresh, true
}

func (s *Source) rejected(ctx context.Context, providerID string, entry account.PoolEntry, cause error) error {
	if !errors.Is(cause, oauth.ErrRefreshRejected) {
		return cause
	}
	if err := s.pools.MarkNeedsReauth(ctx, providerID, entry.ID); err != nil {
		return err
	}
	return fmt.Errorf("%s: %w", entry.ID, ErrReauthRequired)
}

func (s *Source) write(entry account.PoolEntry, credential oauth.OAuthCredential) error {
	if s.secrets == nil {
		return nil
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		return fmt.Errorf("encode the refreshed credential of %s: %w", entry.ID, err)
	}
	if err := s.secrets.Set(entry.SecretRef, string(encoded)); err != nil {
		return fmt.Errorf("store the refreshed credential of %s: %w", entry.ID, err)
	}
	return nil
}
