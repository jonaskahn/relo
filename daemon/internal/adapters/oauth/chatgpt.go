// ChatGPT sign-in flow: exchange, refresh, and identity.
package oauth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/codex"
)

const (
	providerChatGPT = FlowChatGPT
	chatGPTClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	chatGPTAuthURL  = "https://auth.openai.com/oauth/authorize"
	chatGPTTokenURL = "https://auth.openai.com/oauth/token"
	chatGPTPort     = 1455
	chatGPTPath     = "/auth/callback"
	chatGPTScope    = "openid profile email offline_access api.connectors.read api.connectors.invoke"
	chatGPTTimeout  = 15 * time.Second

	// ExtraChatGPTPlanType is the subscription plan named by the authorized
	// ChatGPT workspace.
	ExtraChatGPTPlanType = "chatgpt_plan_type"
)

// NewChatGPTFlow returns the browser PKCE login for a ChatGPT account,
// the credential the Codex client uses.
func NewChatGPTFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	withChatGPTTimeout(&settings)
	return newAuthCodeFlow(authCodeConfig{
		providerID:   providerChatGPT,
		clientID:     chatGPTClientID,
		authURL:      option(settings.Endpoints.AuthURL, chatGPTAuthURL),
		tokenURL:     option(settings.Endpoints.TokenURL, chatGPTTokenURL),
		scopes:       []string{chatGPTScope},
		port:         chatGPTPort,
		path:         chatGPTPath,
		redirectHost: "localhost",
		// The authorization extra the Codex client sends for this client
		// identifier: the simplified consent, and the organization claims on
		// the id token. They are what the ChatGPT authorization is exercised
		// with, so Relo asks for the same.
		authParams: map[string]string{
			"id_token_add_organizations": "true",
			"codex_cli_simplified_flow":  "true",
			"originator":                 codex.Originator,
		},
		decode:  chatGPTIdentity,
		refresh: chatGPTRefresh,
		instructions: "Complete the ChatGPT login in your browser. " +
			"If the browser cannot reach this machine, paste the final redirect URL when prompted.",
	}, settings)
}

func withChatGPTTimeout(settings *Options) {
	if settings.HTTPClient == nil {
		settings.HTTPClient = &http.Client{Timeout: chatGPTTimeout}
		return
	}
	client := *settings.HTTPClient
	client.Timeout = chatGPTTimeout
	settings.HTTPClient = &client
}

func chatGPTIdentity(response tokenResponse, credential *OAuthCredential) error {
	identity := codex.IdentityFromTokens(response.AccessToken, response.IDToken)
	if identity.AccountID == "" && identity.Email == "" {
		return fmt.Errorf("ChatGPT token carries no account id or email: %w", ErrTokenResponse)
	}
	credential.AccountID = identity.AccountID
	credential.Email = identity.Email
	addExtra(credential, ExtraChatGPTPlanType, identity.PlanType)
	return nil
}

func chatGPTRefresh(ctx context.Context, client tokenClient, request refreshRequest) (*OAuthCredential, error) {
	refreshed, err := refreshToken(ctx, refreshOptions{client: client,
		endpoint: request.Endpoint, clientID: request.ClientID,
		now: client.clock.Now(), cred: &request.Credential, scopes: request.Scopes})
	if err != nil {
		return nil, err
	}
	refreshed.AccountID = request.Credential.AccountID
	refreshed.Email = request.Credential.Email
	if refreshed.Extra == nil {
		refreshed.Extra = map[string]string{}
	}
	refreshed.Extra[ExtraChatGPTPlanType] = request.Credential.Extra[ExtraChatGPTPlanType]
	return refreshed, nil
}
