// Qwen sign-in flow.
package oauth

import (
	"context"
)

const (
	providerQwen     = FlowQwen
	qwenClientID     = "relo-client"
	qwenScope        = "openid profile"
	qwenHost         = "https://auth.dashscope.aliyun.com"
	qwenDevicePath   = "/oauth2/device/code"
	qwenTokenPath    = "/oauth2/token"
	qwenVerification = "https://dashscope.aliyun.com/activate"
)

// NewQwenFlow returns the device-code login for an Alibaba Qwen account.
func NewQwenFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	host := option(settings.Endpoints.APIBaseURL, qwenHost)
	return newDeviceFlow(deviceConfig{
		providerID:      providerQwen,
		clientID:        qwenClientID,
		startURL:        option(settings.Endpoints.DeviceURL, host+qwenDevicePath),
		tokenURL:        option(settings.Endpoints.TokenURL, host+qwenTokenPath),
		verificationURL: option(settings.Endpoints.VerificationURL, qwenVerification),
		scopes:          []string{qwenScope},
		finish: func(_ context.Context, _ tokenClient, response tokenResponse, credential *OAuthCredential) error {
			credential.AccountID, credential.Email = IdentityFromTokens(response.IDToken, response.AccessToken)
			return nil
		},
	}, settings)
}
