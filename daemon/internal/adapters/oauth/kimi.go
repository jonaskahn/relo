// Kimi sign-in flow.
package oauth

import "time"

const (
	providerKimi  = FlowKimi
	kimiClientID  = "17e5f671-d194-4dfb-9706-5516cb48c098"
	kimiOAuthHost = "https://auth.kimi.com"
	kimiStartPath = "/api/oauth/device_authorization"
	kimiTokenPath = "/api/oauth/token"
	kimiTokenTTL  = time.Hour
)

// NewKimiFlow returns the device-code login for a Kimi account, which
// needs no loopback redirect.
func NewKimiFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	host := option(settings.Endpoints.APIBaseURL, kimiOAuthHost)
	return newDeviceFlow(deviceConfig{
		providerID:      providerKimi,
		clientID:        kimiClientID,
		startURL:        option(settings.Endpoints.DeviceURL, host+kimiStartPath),
		tokenURL:        option(settings.Endpoints.TokenURL, host+kimiTokenPath),
		verificationURL: option(settings.Endpoints.VerificationURL, host+"/device"),
		tokenHeaders:    map[string]string{"User-Agent": defaultUserAgent},
	}, settings)
}
