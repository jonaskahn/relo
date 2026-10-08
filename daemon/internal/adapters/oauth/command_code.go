// Command-code flow: sign-in through a pasted authorization code.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	providerCommandCode  = FlowCommandCode
	commandCodeStudio    = "https://commandcode.ai"
	commandCodeAuthPath  = "/studio/auth/cli"
	commandCodeAPIBase   = "https://api.commandcode.ai"
	commandCodeWhoAmI    = "/alpha/whoami"
	commandCodePort      = 5959
	commandCodePath      = "/callback"
	commandCodeAuthDir   = ".commandcode"
	commandCodeAuthFile  = "auth.json"
	commandCodeUserKey   = "userId"
	commandCodeImportKey = "source"
)

// Command-code errors name why a pasted-code sign-in failed: the key the
// CLI rejected and the CLI authentication that is absent.
var (
	ErrCommandCodeKeyRejected = errors.New("command code rejected the API key")
	ErrCommandCodeAuthMissing = errors.New("no Command Code CLI authentication is installed")
)

type commandCodeAuth struct {
	APIKey   string `json:"apiKey"`
	UserID   string `json:"userId"`
	UserName string `json:"userName"`
	KeyName  string `json:"keyName"`
}

type commandCodeIdentity struct {
	UserID   string `json:"userId"`
	UserName string `json:"userName"`
	KeyName  string `json:"keyName"`
}

// NewCommandCodeFlow returns the login for a Command Code account: the
// installed CLI key when there is one, a browser approval otherwise,
// which hands the API key back as the authorization code.
func NewCommandCodeFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	flow := &commandCodeFlow{settings: settings}
	browser := newAuthCodeFlow(authCodeConfig{
		providerID: providerCommandCode,
		authURL:    option(settings.Endpoints.AuthURL, commandCodeStudio+commandCodeAuthPath),
		scopes:     []string{"api"},
		port:       commandCodePort,
		path:       commandCodePath,
		authParams: map[string]string{"app_name": "Relo"},
		exchange:   flow.exchange,
		decode:     flow.decode,
		instructions: "Sign in with Command Code in your browser. " +
			"If the browser cannot reach this machine, paste the final redirect URL when prompted.",
	}, settings)
	flow.browser = browser
	return flow
}

type commandCodeFlow struct {
	settings Options
	browser  *authCodeFlow
}

// ProviderID returns the provider this flow logs into.
func (f *commandCodeFlow) ProviderID() string {
	return providerCommandCode
}

// CallbackPort returns the port the browser fallback needs. A login that
// imports an installed CLI's key never reaches it.
func (f *commandCodeFlow) CallbackPort() int {
	return f.browser.CallbackPort()
}

// Login imports the installed CLI key, and falls back to the browser.
func (f *commandCodeFlow) Login(ctx context.Context, opts LoginOpts) (*OAuthCredential, error) {
	imported, err := f.importCredential(ctx)
	if err == nil {
		announce(opts, AuthPrompt{Instructions: "Imported the installed Command Code CLI authentication."})
		return imported, nil
	}
	if !errors.Is(err, ErrCommandCodeAuthMissing) {
		return nil, err
	}
	return f.browser.Login(ctx, opts)
}

// Refresh re-validates the key, which is long-lived and only ever
// rotated by a new login.
func (f *commandCodeFlow) Refresh(ctx context.Context, cred *OAuthCredential) (*OAuthCredential, error) {
	if cred == nil || cred.AccessToken == "" {
		return nil, ErrNoRefreshToken
	}
	_, err := f.whoami(ctx, cred.AccessToken)
	if err != nil {
		return nil, err
	}
	refreshed := cred.Clone()
	return &refreshed, nil
}

// Validate reports whether the credential can still serve a request.
func (f *commandCodeFlow) Validate(_ context.Context, cred *OAuthCredential) error {
	return validateCredential(f.settings.clockOrReal().Now(), cred)
}

func (f *commandCodeFlow) exchange(ctx context.Context, _ tokenClient, request exchangeRequest) (tokenResponse, error) {
	identity, err := f.whoami(ctx, request.Code)
	if err != nil {
		return tokenResponse{}, err
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return tokenResponse{}, fmt.Errorf("encode the Command Code identity: %w", err)
	}
	response := tokenResponse{AccessToken: request.Code, RefreshToken: request.Code}
	response.body = encoded
	return response, nil
}

func (f *commandCodeFlow) decode(response tokenResponse, credential *OAuthCredential) error {
	identity := commandCodeIdentity{}
	if err := response.decodeInto(&identity); err != nil {
		return nil
	}
	credential.AccountID = firstNonEmpty(identity.UserID, credential.AccountID)
	credential.Email = firstNonEmpty(identity.UserName, credential.Email)
	addExtra(credential, commandCodeUserKey, identity.UserID)
	addExtra(credential, "keyName", identity.KeyName)
	return nil
}

func (f *commandCodeFlow) importCredential(ctx context.Context) (*OAuthCredential, error) {
	path := f.authPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrCommandCodeAuthMissing
		}
		return nil, fmt.Errorf("read the Command Code CLI authentication: %w", err)
	}
	auth := commandCodeAuth{}
	if err := decodeJSONBody(raw, &auth); err != nil {
		return nil, err
	}
	key := strings.TrimSpace(auth.APIKey)
	if key == "" {
		return nil, ErrCommandCodeAuthMissing
	}
	if _, err := f.whoami(ctx, key); err != nil {
		return nil, err
	}
	credential := &OAuthCredential{AccessToken: key, RefreshToken: key, AccountID: auth.UserID, Email: auth.UserName}
	addExtra(credential, commandCodeUserKey, auth.UserID)
	addExtra(credential, commandCodeImportKey, filepath.Base(path))
	return credential, nil
}

func (f *commandCodeFlow) whoami(ctx context.Context, key string) (commandCodeIdentity, error) {
	endpoint := option(f.settings.Endpoints.APIBaseURL, commandCodeAPIBase) + commandCodeWhoAmI
	body, status, err := f.settings.tokenClient(nil).get(ctx, endpoint, bearerHeader(key))
	if err != nil {
		return commandCodeIdentity{}, fmt.Errorf("validate the Command Code key: %w", err)
	}
	if status == 401 || status == 403 {
		return commandCodeIdentity{}, fmt.Errorf("validate the Command Code key: %w", ErrCommandCodeKeyRejected)
	}
	if status < 200 || status >= 300 {
		return commandCodeIdentity{}, fmt.Errorf("validate the Command Code key: %w", responseError{Status: status, Code: errorCode(body)})
	}
	identity := commandCodeIdentity{}
	if err := decodeJSONBody(body, &identity); err != nil {
		return commandCodeIdentity{}, err
	}
	return identity, nil
}

func (f *commandCodeFlow) authPath() string {
	if f.settings.CredentialsPath != "" {
		return f.settings.CredentialsPath
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(commandCodeAuthDir, commandCodeAuthFile)
	}
	return filepath.Join(home, commandCodeAuthDir, commandCodeAuthFile)
}
