// GitHub Copilot sign-in flow: token minting and identity.
package oauth

import (
	"context"
	"strings"
	"time"
)

const (
	providerGitHubCopilot = FlowCopilot
	copilotClientID       = "Iv1.b507a08c87ecfe98"
	copilotScope          = "read:user"
	copilotDeviceURL      = "https://github.com/login/device/code"
	copilotAccessURL      = "https://github.com/login/oauth/access_token"
	copilotTokenURL       = "https://api.github.com/copilot_internal/v2/token"
	copilotUserURL        = "https://api.github.com/user"
	copilotVerifyPage     = "https://github.com/login/device"
	copilotAPIBase        = "https://api.githubcopilot.com"
	copilotRefreshSkew    = time.Minute
	apiBaseExtraKey       = "apiBaseUrl"
	editorVersion         = "vscode/1.99.0"
	editorPluginVersion   = "copilot-chat/0.26.0"
	copilotIntegrationID  = "vscode-chat"
)

// NewGitHubCopilotFlow returns the device-code login for a GitHub
// Copilot account, which trades a GitHub token for a Copilot API token.
func NewGitHubCopilotFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	host := option(settings.Endpoints.APIBaseURL, copilotAPIBase)
	return newDeviceFlow(deviceConfig{
		providerID:      providerGitHubCopilot,
		clientID:        copilotClientID,
		startURL:        option(settings.Endpoints.DeviceURL, copilotDeviceURL),
		tokenURL:        option(settings.Endpoints.TokenURL, copilotAccessURL),
		verificationURL: option(settings.Endpoints.VerificationURL, copilotVerifyPage),
		scopes:          []string{copilotScope},
		grantType:       "urn:ietf:params:oauth:grant-type:device_code",
		finish:          copilotFinish(host, settings.Endpoints),
		refresh:         copilotRefresh(host, settings.Endpoints),
	}, settings)
}

func copilotFinish(apiBase string, endpoints Endpoints) finishHook {
	mintURL := option(endpoints.MintURL, copilotTokenURL)
	userURL := option(endpoints.UserURL, copilotUserURL)
	return func(ctx context.Context, client tokenClient, response tokenResponse, credential *OAuthCredential) error {
		if credential.RefreshToken == "" {
			credential.RefreshToken = response.AccessToken
		}
		if err := mintCopilotToken(ctx, client, apiBase, mintURL, credential); err != nil {
			return err
		}
		return copilotIdentity(ctx, client, userURL, credential)
	}
}

func copilotRefresh(apiBase string, endpoints Endpoints) refreshHook {
	mintURL := option(endpoints.MintURL, copilotTokenURL)
	return func(ctx context.Context, client tokenClient, request refreshRequest) (*OAuthCredential, error) {
		if request.Credential.RefreshToken == "" {
			return nil, ErrNoRefreshToken
		}
		refreshed := request.Credential.Clone()
		if err := mintCopilotToken(ctx, client, apiBase, mintURL, &refreshed); err != nil {
			return nil, err
		}
		return &refreshed, nil
	}
}

func mintCopilotToken(ctx context.Context, client tokenClient, apiBase, mintURL string, credential *OAuthCredential) error {
	body, status, err := client.get(ctx, mintURL, copilotHeaders(credential.RefreshToken))
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		if status == 401 || status == 403 {
			return ErrRefreshRejected
		}
		return responseError{Status: status, Code: errorCode(body)}
	}
	payload := struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expires_at"`
		Endpoints struct {
			API string `json:"api"`
		} `json:"endpoints"`
	}{}
	if err := decodeJSONBody(body, &payload); err != nil {
		return err
	}
	if payload.Token == "" {
		return ErrTokenResponse
	}
	credential.AccessToken = payload.Token
	credential.ExpiresAt = time.Unix(payload.ExpiresAt, 0).Add(-copilotRefreshSkew)
	addExtra(credential, apiBaseExtraKey, firstNonEmpty(payload.Endpoints.API, apiBase))
	return nil
}

func copilotIdentity(ctx context.Context, client tokenClient, userURL string, credential *OAuthCredential) error {
	body, status, err := client.get(ctx, userURL, copilotHeaders(credential.RefreshToken))
	if err != nil || status < 200 || status >= 300 {
		return nil
	}
	user := struct {
		Login string `json:"login"`
		Email string `json:"email"`
	}{}
	if err := decodeJSONBody(body, &user); err != nil {
		return nil
	}
	credential.AccountID = firstNonEmpty(user.Login, credential.AccountID)
	credential.Email = firstNonEmpty(strings.ToLower(user.Email), credential.Email)
	return nil
}

func copilotHeaders(githubToken string) map[string]string {
	return map[string]string{
		"Authorization":          "token " + githubToken,
		"User-Agent":             defaultUserAgent,
		"Editor-Version":         editorVersion,
		"Editor-Plugin-Version":  editorPluginVersion,
		"Copilot-Integration-Id": copilotIntegrationID,
	}
}
