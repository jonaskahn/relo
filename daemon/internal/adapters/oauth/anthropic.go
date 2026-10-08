// Anthropic OAuth flow: token exchange, refresh, and account identity.
package oauth

import (
	"context"
	"fmt"
	"strings"
)

const (
	providerClaude    = FlowClaude
	anthropicClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	anthropicAuthURL  = "https://claude.ai/oauth/authorize"
	anthropicTokenURL = "https://api.anthropic.com/v1/oauth/token"
	anthropicScope    = "org:create_api_key user:profile user:inference"
	anthropicPort     = 54545
	anthropicPath     = "/callback"
)

// NewAnthropicFlow returns the browser PKCE login for a Claude account.
// The vendor stores its public client identifier base64-encoded; the
// decoded value is what the authorize endpoint expects.
func NewAnthropicFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	return newAuthCodeFlow(authCodeConfig{
		providerID:   providerClaude,
		clientID:     anthropicClientID,
		authURL:      option(settings.Endpoints.AuthURL, anthropicAuthURL),
		tokenURL:     option(settings.Endpoints.TokenURL, anthropicTokenURL),
		scopes:       []string{anthropicScope},
		port:         anthropicPort,
		path:         anthropicPath,
		redirectHost: "localhost",
		authParams:   map[string]string{"code": "true"},
		exchange:     anthropicExchange,
		decode:       anthropicAccount,
		refresh:      anthropicRefresh,
		instructions: "Complete the Claude login in your browser. " +
			"If the browser cannot reach this machine, paste the final redirect URL when prompted.",
	}, settings)
}

func anthropicExchange(ctx context.Context, client tokenClient, request exchangeRequest) (tokenResponse, error) {
	body := map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     request.ClientID,
		"code":          codeBeforeFragment(request.Code),
		"state":         stateAfterFragment(request.Code, request.State),
		"redirect_uri":  request.RedirectURI,
		"code_verifier": request.Verifier,
	}
	return client.postJSON(ctx, request.TokenURL, body, nil)
}

func anthropicRefresh(ctx context.Context, client tokenClient, request refreshRequest) (*OAuthCredential, error) {
	if request.Credential.RefreshToken == "" {
		return nil, ErrNoRefreshToken
	}
	body := map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     request.ClientID,
		"refresh_token": request.Credential.RefreshToken,
	}
	response, err := client.postJSON(ctx, request.Endpoint, body, nil)
	if err != nil {
		return nil, fmt.Errorf("refresh the access token: %w", refreshFailure(err))
	}
	return refreshedCredential(client, request, response)
}

type anthropicAccountBlock struct {
	Account struct {
		UUID  string `json:"uuid"`
		Email string `json:"email_address"`
	} `json:"account"`
}

func anthropicAccount(response tokenResponse, credential *OAuthCredential) error {
	block := anthropicAccountBlock{}
	if err := response.decodeInto(&block); err != nil {
		return nil
	}
	credential.AccountID = firstNonEmpty(block.Account.UUID, credential.AccountID)
	credential.Email = firstNonEmpty(strings.ToLower(block.Account.Email), credential.Email)
	return nil
}

func codeBeforeFragment(code string) string {
	value, _, _ := strings.Cut(code, "#")
	return strings.TrimSpace(value)
}

func stateAfterFragment(code, state string) string {
	_, fragment, found := strings.Cut(code, "#")
	if !found || strings.TrimSpace(fragment) == "" {
		return state
	}
	return strings.TrimSpace(fragment)
}

func refreshedCredential(client tokenClient, request refreshRequest, response tokenResponse) (*OAuthCredential, error) {
	refreshed, err := response.credential(client.clock.Now(), request.Scopes)
	if err != nil {
		return nil, err
	}
	merged := mergeCredential(request.Credential, refreshed)
	return &merged, nil
}
