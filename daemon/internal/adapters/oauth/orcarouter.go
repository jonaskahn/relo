// Orca Router sign-in flow: exchange, refresh, and scope checks.
package oauth

import (
	"context"
	"errors"
)

const (
	providerOrcaRouter  = FlowOrcaRouter
	orcaRouterHost      = "https://www.orcarouter.ai"
	orcaRouterAuthPath  = "/auth"
	orcaRouterKeyPath   = "/api/v1/auth/keys"
	orcaRouterPort      = 51733
	orcaRouterPath      = "/callback"
	orcaRouterScope     = "api"
	orcaRouterUserKey   = "userId"
	orcaRouterHistoryID = "quotaHistoryId"
)

// ErrOrcaRouterScope reports a key exchange that granted less than the
// api scope Relo needs to send requests.
var ErrOrcaRouterScope = errors.New("the key exchange did not grant the api scope")

// NewOrcaRouterFlow returns the browser PKCE login for an OrcaRouter
// account, whose credential is an API key rather than a token pair.
func NewOrcaRouterFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	host := option(settings.Endpoints.APIBaseURL, orcaRouterHost)
	return newAuthCodeFlow(authCodeConfig{
		providerID: providerOrcaRouter,
		authURL:    option(settings.Endpoints.AuthURL, host+orcaRouterAuthPath),
		tokenURL:   option(settings.Endpoints.TokenURL, host+orcaRouterKeyPath),
		scopes:     []string{orcaRouterScope},
		port:       orcaRouterPort,
		path:       orcaRouterPath,
		authParams: map[string]string{"app_name": "Relo"},
		exchange:   orcaRouterExchange,
		refresh:    orcaRouterRefresh,
		instructions: "Approve access in your browser. " +
			"If the browser cannot reach this machine, paste the final redirect URL when prompted.",
	}, settings)
}

func orcaRouterExchange(ctx context.Context, client tokenClient, request exchangeRequest) (tokenResponse, error) {
	payload := map[string]string{
		"code":                  request.Code,
		"code_verifier":         request.Verifier,
		"code_challenge_method": "S256",
	}
	body, status, err := client.postJSONRaw(ctx, request.TokenURL, payload, nil)
	if err != nil {
		return tokenResponse{}, err
	}
	return orcaRouterKeyResponse(body, status)
}

func orcaRouterRefresh(ctx context.Context, client tokenClient, request refreshRequest) (*OAuthCredential, error) {
	key := request.Credential.AccessToken
	if key == "" {
		return nil, ErrNoRefreshToken
	}
	refreshed := request.Credential.Clone()
	refreshed.AccessToken = key
	refreshed.RefreshToken = key
	addExtra(&refreshed, orcaRouterHistoryID, firstNonEmpty(request.Credential.Extra[orcaRouterHistoryID], key))
	if err := orcaRouterScopeCheck(refreshed.Extra); err != nil {
		return nil, err
	}
	return &refreshed, nil
}

func orcaRouterKeyResponse(body []byte, status int) (tokenResponse, error) {
	if status < 200 || status >= 300 {
		return tokenResponse{}, responseError{Status: status, Code: errorCode(body)}
	}
	payload := struct {
		Key   string `json:"key"`
		Scope string `json:"scope"`
		User  string `json:"user_id"`
	}{}
	if err := decodeJSONBody(body, &payload); err != nil {
		return tokenResponse{}, err
	}
	if payload.Key == "" {
		return tokenResponse{}, ErrTokenResponse
	}
	response := tokenResponse{AccessToken: payload.Key, RefreshToken: payload.Key, Scope: payload.Scope}
	response.body = body
	if err := orcaRouterScopeValid(payload.Scope); err != nil {
		return tokenResponse{}, err
	}
	return response, nil
}

func orcaRouterScopeValid(scope string) error {
	if scope != "" && scope != orcaRouterScope {
		return ErrOrcaRouterScope
	}
	return nil
}

func orcaRouterScopeCheck(extra map[string]string) error {
	return orcaRouterScopeValid(extra["scope"])
}
