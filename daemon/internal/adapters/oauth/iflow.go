// iFlow sign-in flow.
package oauth

const (
	providerIFlow  = FlowIFlow
	iflowHost      = "https://apis.iflow.cn"
	iflowAuthPath  = "/oauth/authorize"
	iflowTokenPath = "/oauth/token"
	iflowPort      = 51735
	iflowPath      = "/callback"
	iflowScope     = "api"
)

// NewIFlowFlow returns the browser PKCE login for an iFlow account.
func NewIFlowFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	host := option(settings.Endpoints.APIBaseURL, iflowHost)
	return newAuthCodeFlow(authCodeConfig{
		providerID: providerIFlow,
		authURL:    option(settings.Endpoints.AuthURL, host+iflowAuthPath),
		tokenURL:   option(settings.Endpoints.TokenURL, host+iflowTokenPath),
		scopes:     []string{iflowScope},
		port:       iflowPort,
		path:       iflowPath,
		instructions: "Approve access in your browser. " +
			"If the browser cannot reach this machine, paste the final redirect URL when prompted.",
		decode: func(response tokenResponse, credential *OAuthCredential) error {
			credential.AccountID, credential.Email = IdentityFromTokens(response.IDToken, response.AccessToken)
			return nil
		},
	}, settings)
}
