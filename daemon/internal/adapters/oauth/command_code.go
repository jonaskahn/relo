// Command-code flow: sign-in through a browser approval that hands back an
// API key.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	providerCommandCode = FlowCommandCode
	commandCodeStudio   = "https://commandcode.ai"
	commandCodeAuthPath = "/studio/auth/cli"
	commandCodeAPIBase  = "https://api.commandcode.ai"
	commandCodeWhoAmI   = "/alpha/whoami"
	commandCodePort     = 5959
	commandCodePath     = "/callback"
	commandCodeUserKey  = "userId"
)

// ErrCommandCodeKeyRejected reports a key the provider rejected.
var ErrCommandCodeKeyRejected = errors.New("command code rejected the API key")

// commandCodeIdentity is the shape the whoami endpoint answers with. Older
// responses used the flat fields, so both are accepted.
type commandCodeIdentity struct {
	UserID   string `json:"userId"`
	UserName string `json:"userName"`
	KeyName  string `json:"keyName"`
	User     struct {
		ID       string `json:"id"`
		UserName string `json:"userName"`
		Name     string `json:"name"`
	} `json:"user"`
}

func (i commandCodeIdentity) userID() string {
	return firstNonEmpty(i.UserID, i.User.ID)
}

func (i commandCodeIdentity) userName() string {
	return firstNonEmpty(i.UserName, i.User.UserName, i.User.Name)
}

// NewCommandCodeFlow returns the login for a Command Code account: a browser
// approval that hands the API key back as the authorization code.
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

// CallbackPort returns the port the browser fallback needs.
func (f *commandCodeFlow) CallbackPort() int {
	return f.browser.CallbackPort()
}

// Login opens the browser approval.
func (f *commandCodeFlow) Login(ctx context.Context, opts LoginOpts) (*OAuthCredential, error) {
	return f.browser.Login(ctx, opts)
}

// Refresh re-validates the key, which is long-lived and only ever rotated
// by a new login.
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
	credential.AccountID = firstNonEmpty(identity.userID(), credential.AccountID)
	credential.Email = firstNonEmpty(identity.userName(), credential.Email)
	addExtra(credential, commandCodeUserKey, identity.userID())
	addExtra(credential, "keyName", identity.KeyName)
	return nil
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
