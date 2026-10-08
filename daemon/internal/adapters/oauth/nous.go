// Nous sign-in flow.
package oauth

import (
	"context"
	"errors"
)

const (
	providerNous     = FlowNous
	nousClientID     = "hermes-cli"
	nousScope        = "inference:invoke"
	nousPortalBase   = "https://portal.nousresearch.com"
	nousStartPath    = "/api/oauth/device/code"
	nousTokenPath    = "/api/oauth/token"
	nousVerification = "https://portal.nousresearch.com/login"
)

// ErrMissingScope reports a token that does not grant what Relo asked
// for, which happens when the portal answers a device login with a
// different account than the one the operator expected.
var ErrMissingScope = errors.New("the token does not grant the requested scope")

// NewNousFlow returns the device-code login for a Nous Portal account.
func NewNousFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	host := option(settings.Endpoints.APIBaseURL, nousPortalBase)
	return newDeviceFlow(deviceConfig{
		providerID:      providerNous,
		clientID:        nousClientID,
		startURL:        option(settings.Endpoints.DeviceURL, host+nousStartPath),
		tokenURL:        option(settings.Endpoints.TokenURL, host+nousTokenPath),
		verificationURL: option(settings.Endpoints.VerificationURL, nousVerification),
		scopes:          []string{nousScope},
		finish:          nousFinish,
	}, settings)
}

func nousFinish(_ context.Context, _ tokenClient, response tokenResponse, credential *OAuthCredential) error {
	if !hasScope(response.AccessToken, nousScope) {
		return ErrMissingScope
	}
	credential.AccountID, credential.Email = IdentityFromTokens(response.IDToken, response.AccessToken)
	return nil
}
