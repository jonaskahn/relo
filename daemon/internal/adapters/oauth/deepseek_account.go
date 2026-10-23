// DeepSeek account flow: import the grant an installed DeepSeek Harness
// Desktop already holds.
package oauth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	providerDeepSeekAccount = FlowDeepSeekAccount

	// deepSeekPlatformOrigin is the platform host that mints and validates the
	// account grant. The inference host is a different one, so a credential
	// never travels to the address that validated it.
	deepSeekPlatformOrigin = "https://platform.deepseek.com"
	deepSeekProfilePath    = "/auth-api/v0/users/current"
	deepSeekAuthHeader     = "x-dsh-auth-token"

	// DeepSeekAccountHeader is where an imported account grant travels. The
	// grant is not a bearer token, so the connection declares this header and
	// the relay writes the token there instead of the Authorization slot.
	DeepSeekAccountHeader = deepSeekAuthHeader
	// DeepSeekAccountBaseURL is the only inference host an account grant may
	// reach, kept beside the header so a connection cannot name another.
	DeepSeekAccountBaseURL = "https://api.deepseek.com/anthropic/v1"
	// deepSeekCredentialsFile is the harness store read here. It belongs to
	// the harness: Relo reads it and writes nothing back, ever.
	deepSeekCredentialsFile = "credentials.yaml"
	deepSeekHarnessHome     = ".dsh"
	// deepSeekGrantRecord is the record a signed-in harness writes its grant
	// under. Any other key names an account this flow does not import.
	deepSeekGrantRecord = "deepseek-account-platform/default"
	deepSeekGrantKind   = "grant"

	deepSeekImportHint = "sign in to DeepSeek Harness Desktop, then import the account again"
)

// DeepSeek account errors name why the import failed: nothing signed in, no
// grant in the store, or a grant the platform refused.
var (
	ErrDeepSeekHarnessMissing = errors.New("no DeepSeek Harness credential is installed; " + deepSeekImportHint)
	ErrDeepSeekGrantMissing   = errors.New("the DeepSeek Harness credential holds no account grant; " + deepSeekImportHint)
	ErrDeepSeekGrantRejected  = errors.New("the DeepSeek account grant was refused; " + deepSeekImportHint)
	ErrDeepSeekIdentity       = errors.New("the DeepSeek account profile carries no account id")
)

// deepSeekCredentials is the shape of the harness store this flow reads, kept
// to the fields the import needs so an unrelated record is never decoded.
type deepSeekCredentials struct {
	Records map[string]struct {
		Kind    string `yaml:"kind"`
		Payload struct {
			Token string `yaml:"token"`
		} `yaml:"payload"`
	} `yaml:"records"`
}

type deepSeekAccountFlow struct {
	settings Options
	client   tokenClient
}

// NewDeepSeekAccountFlow returns the import-first login for a DeepSeek
// account.
func NewDeepSeekAccountFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	return &deepSeekAccountFlow{settings: settings, client: settings.tokenClient(nil)}
}

// ProviderID returns the provider this flow logs into.
func (f *deepSeekAccountFlow) ProviderID() string {
	return providerDeepSeekAccount
}

// CallbackPort returns zero: the login imports a grant the harness already
// holds, so it never binds a loopback listener.
func (f *deepSeekAccountFlow) CallbackPort() int {
	return 0
}

// Login imports the harness grant and verifies it against the platform before
// storing it. A grant the platform refuses is never stored.
func (f *deepSeekAccountFlow) Login(ctx context.Context, opts LoginOpts) (*OAuthCredential, error) {
	token, err := f.readGrant()
	if err != nil {
		return nil, err
	}
	identity, err := f.profile(ctx, token)
	if err != nil {
		return nil, err
	}
	announce(opts, AuthPrompt{Instructions: "Imported the installed DeepSeek Harness account."})
	return &OAuthCredential{
		AccessToken:  token,
		RefreshToken: token,
		AccountID:    identity.AccountID,
		Email:        identity.Email,
		Scope:        providerDeepSeekAccount,
	}, nil
}

// Refresh re-validates the stored grant. A harness grant has no rotation
// endpoint, so nothing is renewed and the harness store is never re-read: a
// refused grant asks the operator to import again.
func (f *deepSeekAccountFlow) Refresh(ctx context.Context, cred *OAuthCredential) (*OAuthCredential, error) {
	if cred == nil || cred.AccessToken == "" {
		return nil, ErrNoRefreshToken
	}
	if _, err := f.profile(ctx, cred.AccessToken); err != nil {
		return nil, err
	}
	refreshed := cred.Clone()
	return &refreshed, nil
}

// Validate reports whether the credential can still serve a request.
func (f *deepSeekAccountFlow) Validate(_ context.Context, cred *OAuthCredential) error {
	return validateCredential(f.settings.clockOrReal().Now(), cred)
}

// readGrant reads the one record a signed-in harness keeps. The store is
// another application's file, so it is opened read-only and nothing about it
// is written back.
func (f *deepSeekAccountFlow) readGrant() (string, error) {
	raw, err := os.ReadFile(f.credentialsPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrDeepSeekHarnessMissing
		}
		return "", fmt.Errorf("read the DeepSeek Harness credential: %w", err)
	}
	store := deepSeekCredentials{}
	if err := yaml.Unmarshal(raw, &store); err != nil {
		return "", fmt.Errorf("decode the DeepSeek Harness credential: %w", err)
	}
	record, found := store.Records[deepSeekGrantRecord]
	if !found || record.Kind != deepSeekGrantKind || record.Payload.Token == "" {
		return "", ErrDeepSeekGrantMissing
	}
	return record.Payload.Token, nil
}

// deepSeekIdentity is the account the platform profile names.
type deepSeekIdentity struct {
	AccountID string
	Email     string
}

// deepSeekProfile is the shape the platform profile answers with. The nested
// envelope is the live one; the id lives on the business payload.
type deepSeekProfile struct {
	Code int `json:"code"`
	Data struct {
		BizCode int `json:"biz_code"`
		BizData struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"biz_data"`
	} `json:"data"`
}

// profile asks the platform who a grant belongs to. The stable account id is
// the identity: an address or a display name can be shared, so neither ever
// stands in for it.
func (f *deepSeekAccountFlow) profile(ctx context.Context, token string) (deepSeekIdentity, error) {
	endpoint := option(f.settings.Endpoints.UserURL, deepSeekPlatformOrigin+deepSeekProfilePath)
	body, status, err := f.client.get(ctx, endpoint, map[string]string{deepSeekAuthHeader: token})
	if err != nil {
		return deepSeekIdentity{}, fmt.Errorf("validate the DeepSeek account grant: %w", err)
	}
	if status == 401 || status == 403 {
		return deepSeekIdentity{}, ErrDeepSeekGrantRejected
	}
	if status < 200 || status >= 300 {
		return deepSeekIdentity{}, fmt.Errorf("validate the DeepSeek account grant: %w", responseError{Status: status, Code: errorCode(body)})
	}
	profile := deepSeekProfile{}
	if err := decodeJSONBody(body, &profile); err != nil {
		return deepSeekIdentity{}, err
	}
	if profile.Code != 0 || profile.Data.BizCode != 0 {
		return deepSeekIdentity{}, ErrDeepSeekGrantRejected
	}
	biz := profile.Data.BizData
	biz.ID = strings.TrimSpace(biz.ID)
	if biz.ID == "" {
		return deepSeekIdentity{}, ErrDeepSeekIdentity
	}
	return deepSeekIdentity{AccountID: biz.ID, Email: strings.ToLower(strings.TrimSpace(biz.Email))}, nil
}

func (f *deepSeekAccountFlow) credentialsPath() string {
	if f.settings.CredentialsPath != "" {
		return f.settings.CredentialsPath
	}
	root := filepath.Join(homeDir(), deepSeekHarnessHome)
	if raw := strings.TrimSpace(os.Getenv("DSH_HOME")); raw != "" {
		root = raw
	}
	return filepath.Join(root, deepSeekCredentialsFile)
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
