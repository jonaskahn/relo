// Devin sign-in flow.
package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	providerDevin       = FlowDevin
	devinLoginURL       = "https://windsurf.com/login"
	devinRegisterURL    = "https://register.windsurf.com/exa.seat_management_pb.SeatManagementService/RegisterUser"
	devinPort           = 51234
	devinPath           = "/callback"
	devinCredentialsDir = "devin"
	devinCredentialsFle = "credentials.toml"
	devinImportHint     = "run 'devin auth login' and try again"
)

// Devin errors name why its CLI sign-in failed: no installed credential,
// an unreadable token, or a registration without a key.
var (
	ErrDevinCLIMissing = errors.New("no Devin CLI credential is installed; " + devinImportHint)
	ErrDevinTokenShape = errors.New("the Devin CLI credential holds no session token")
	ErrDevinNoAPIKey   = errors.New("the Devin registration returned no API key")
)

type devinCredentials struct {
	APIKey       string `toml:"windsurf_api_key"`
	Email        string `toml:"email"`
	APIServerURL string `toml:"api_server_url"`
}

type devinFlow struct {
	settings Options
	client   tokenClient
}

// NewDevinFlow returns the import-first login for a Devin account.
func NewDevinFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	return &devinFlow{settings: settings, client: settings.tokenClient(nil)}
}

// ProviderID returns the provider this flow logs into.
func (f *devinFlow) ProviderID() string {
	return providerDevin
}

// CallbackPort returns zero: the Devin login imports a key an installed CLI
// already holds, so it never binds a loopback listener.
func (f *devinFlow) CallbackPort() int {
	return 0
}

// Login imports the Devin CLI session, and logs in through the browser
// when the operator has no CLI credential installed.
func (f *devinFlow) Login(ctx context.Context, opts LoginOpts) (*OAuthCredential, error) {
	imported, err := f.importCredential()
	if err == nil {
		announce(opts, AuthPrompt{Instructions: "Imported the installed Devin CLI credential."})
		return imported, nil
	}
	if !errors.Is(err, ErrDevinCLIMissing) {
		return nil, err
	}
	return f.browserLogin(ctx, opts)
}

// Refresh re-imports the CLI credential, which is where the Devin CLI
// keeps the rotating session token, and otherwise keeps the API key.
func (f *devinFlow) Refresh(ctx context.Context, cred *OAuthCredential) (*OAuthCredential, error) {
	if cred == nil || cred.AccessToken == "" {
		return nil, ErrNoRefreshToken
	}
	imported, err := f.importCredential()
	if err != nil {
		return nil, fmt.Errorf("refresh the Devin credential: %w", err)
	}
	merged := mergeCredential(*cred, *imported)
	return &merged, nil
}

// Validate reports whether the credential can still serve a request.
func (f *devinFlow) Validate(_ context.Context, cred *OAuthCredential) error {
	return validateCredential(f.settings.clockOrReal().Now(), cred)
}

func (f *devinFlow) importCredential() (*OAuthCredential, error) {
	document := devinCredentials{}
	if _, err := toml.DecodeFile(f.credentialsPath(), &document); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrDevinCLIMissing
		}
		return nil, fmt.Errorf("read the Devin CLI credential: %w", err)
	}
	if document.APIKey == "" {
		return nil, ErrDevinTokenShape
	}
	credential := &OAuthCredential{
		AccessToken:  document.APIKey,
		RefreshToken: document.APIKey,
		Email:        strings.ToLower(document.Email),
		Scope:        providerDevin,
	}
	if document.APIServerURL != "" {
		credential.Extra = map[string]string{ExtraAPIBaseURL: document.APIServerURL}
	}
	credential.AccountID = SessionAccountID(document.APIKey)
	return credential, nil
}

func (f *devinFlow) browserLogin(ctx context.Context, opts LoginOpts) (*OAuthCredential, error) {
	state := NewState()
	_, challenge := GeneratePKCE()
	server, err := NewCallbackServer(f.settings.port(devinPort), devinPath, state)
	if err != nil {
		return nil, err
	}
	defer func() { _ = server.Close() }()
	redirectURI := server.RedirectURI(f.settings.host(""))
	prompt := AuthPrompt{
		URL: withQuery(f.loginURL(), url.Values{
			"redirect_uri": {redirectURI}, "state": {state},
			"code_challenge": {challenge}, "code_challenge_method": {"S256"},
		}),
		Instructions: "Sign in to Devin in your browser.",
	}
	announce(opts, prompt)
	code, err := f.authorizationCode(ctx, opts, server, state, prompt)
	if err != nil {
		return nil, err
	}
	return f.register(ctx, code, challenge, redirectURI)
}

func (f *devinFlow) authorizationCode(ctx context.Context, opts LoginOpts, server *CallbackServer, state string, prompt AuthPrompt) (string, error) {
	if opts.ManualCode != nil {
		return manualCode(state, opts.ManualCode, prompt)
	}
	if err := openBrowser(opts, f.settings.Browser, prompt.URL); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, loginTimeout(opts))
	defer cancel()
	result, err := server.Wait(ctx)
	if err != nil {
		return "", err
	}
	return result.Code, nil
}

func (f *devinFlow) register(ctx context.Context, signInToken, challenge, redirectURI string) (*OAuthCredential, error) {
	payload := map[string]string{"firebase_id_token": signInToken, "code_challenge": challenge, "redirect_uri": redirectURI}
	body, status, err := f.client.postJSONRaw(ctx, f.registerURL(), payload, nil)
	if err != nil {
		return nil, fmt.Errorf("register the Devin account: %w", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("register the Devin account: %w", responseError{Status: status, Code: errorCode(body)})
	}
	response := struct {
		APIKey string `json:"api_key"`
	}{}
	if err := decodeJSONBody(body, &response); err != nil {
		return nil, err
	}
	if response.APIKey == "" {
		return nil, ErrDevinNoAPIKey
	}
	return &OAuthCredential{AccessToken: response.APIKey, RefreshToken: response.APIKey, Scope: providerDevin}, nil
}

func (f *devinFlow) credentialsPath() string {
	if f.settings.CredentialsPath != "" {
		return f.settings.CredentialsPath
	}
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		return filepath.Join(dataHome, devinCredentialsDir, devinCredentialsFle)
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(devinCredentialsDir, devinCredentialsFle)
	}
	return filepath.Join(configDir, devinCredentialsDir, devinCredentialsFle)
}

func (f *devinFlow) loginURL() string {
	return option(f.settings.Endpoints.AuthURL, devinLoginURL)
}

func (f *devinFlow) registerURL() string {
	return option(f.settings.Endpoints.APIBaseURL, devinRegisterURL)
}

// SessionAccountID returns the stable account identifier a provider
// session token carries, or the token's own subject when it has none.
func SessionAccountID(token string) string {
	if accountID := AccountIDFromTokens("", token); accountID != "" {
		return accountID
	}
	return SubjectFromToken(token)
}
