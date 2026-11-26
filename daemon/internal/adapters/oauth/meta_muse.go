// Meta Muse sign-in flow: device approval and subscription key minting.
package oauth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/muse"
)

const (
	providerMetaMuse    = FlowMetaMuse
	museDeviceClientID  = "1031625952748946"
	museDeviceStartURL  = "https://auth.meta.com/oidc/device/authorization/"
	museDeviceTokenURL  = "https://auth.meta.com/oidc/device/token/"
	museVerificationURL = "https://auth.meta.com/oauth/device/"
)

var (
	// ErrMuseKeyMissing reports a key response with no usable Model API key.
	ErrMuseKeyMissing = errors.New("the Meta Muse credential holds no API key")
	// ErrMuseIdentityMissing reports a key response with no stable account identity.
	ErrMuseIdentityMissing = errors.New("the Meta Muse credential has no stable account identity")
)

type metaMuseFlow struct {
	settings Options
	client   tokenClient
}

// NewMetaMuseFlow returns the device login for a Meta Muse subscription.
func NewMetaMuseFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	return &metaMuseFlow{settings: settings, client: settings.tokenClient(muse.Headers())}
}

// ProviderID returns the provider this flow logs into.
func (f *metaMuseFlow) ProviderID() string {
	return providerMetaMuse
}

// CallbackPort returns zero: a device login never opens a loopback listener.
func (f *metaMuseFlow) CallbackPort() int {
	return 0
}

// Login asks the human to approve a Meta device code, then exchanges the
// account grant for the subscription Model API key.
func (f *metaMuseFlow) Login(ctx context.Context, opts LoginOpts) (*OAuthCredential, error) {
	flow := newDeviceFlow(deviceConfig{
		providerID:      providerMetaMuse,
		clientID:        museDeviceClientID,
		startURL:        option(f.settings.Endpoints.DeviceURL, museDeviceStartURL),
		tokenURL:        option(f.settings.Endpoints.DeviceTokenURL, museDeviceTokenURL),
		verificationURL: option(f.settings.Endpoints.VerificationURL, museVerificationURL),
		tokenHeaders:    muse.Headers(),
		finish:          f.finishDevice,
	}, f.settings)
	return flow.Login(ctx, opts)
}

// Refresh keeps an installed credential usable without minting a new key,
// which Meta rate-limits. A device credential has no local expiry.
func (f *metaMuseFlow) Refresh(_ context.Context, cred *OAuthCredential) (*OAuthCredential, error) {
	if cred == nil || cred.AccessToken == "" {
		return nil, ErrNoRefreshToken
	}
	refreshed := cred.Clone()
	refreshed.ExpiresAt = time.Time{}
	return &refreshed, nil
}

// Validate reports whether the credential can still serve a request.
func (f *metaMuseFlow) Validate(_ context.Context, cred *OAuthCredential) error {
	if cred == nil || cred.AccessToken == "" {
		return ErrTokenResponse
	}
	return nil
}

func (f *metaMuseFlow) finishDevice(ctx context.Context, _ tokenClient, response tokenResponse, credential *OAuthCredential) error {
	accountAccess := strings.TrimSpace(firstNonEmpty(response.AccessToken, response.RefreshToken))
	if accountAccess == "" {
		return ErrTokenResponse
	}
	credential.RefreshToken = accountAccess
	credential.ExpiresAt = time.Time{}
	addExtra(credential, muse.AccessTokenExtraKey, accountAccess)
	body, status, err := f.requestKey(ctx, accountAccess, true)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return museKeyFailure(status, body)
	}
	return f.applyKey(response, credential, body)
}

func (f *metaMuseFlow) requestKey(ctx context.Context, access string, onboard bool) ([]byte, int, error) {
	payload := map[string]bool{}
	if onboard {
		payload["onboard"] = true
	}
	return f.client.postJSONRaw(ctx, f.keyURL(), payload, bearerHeader(access))
}

func (f *metaMuseFlow) keyURL() string {
	return option(f.settings.Endpoints.APIBaseURL, muse.KeyEndpoint)
}

func museKeyFailure(status int, body []byte) error {
	if status == 401 || status == 403 {
		return ErrRefreshRejected
	}
	return responseError{Status: status, Code: errorCode(body)}
}

func (f *metaMuseFlow) applyKey(response tokenResponse, credential *OAuthCredential, body []byte) error {
	payload, err := muse.Decode(body)
	if err != nil {
		return err
	}
	if payload.Inactive() {
		return muse.ErrSubscriptionInactive
	}
	key := strings.TrimSpace(payload.APIKey)
	if key == "" {
		if payload.PaymentURL() != "" || payload.RequirePayment {
			return fmt.Errorf("%s: %w", payload.PaymentURL(), muse.ErrPaymentRequired)
		}
		return ErrMuseKeyMissing
	}
	id, email := payload.Identity()
	if id == "" {
		return ErrMuseIdentityMissing
	}
	credential.AccessToken = key
	credential.RefreshToken = firstNonEmpty(response.RefreshToken, credential.RefreshToken)
	credential.AccountID = firstNonEmpty(id, credential.AccountID)
	credential.Email = firstNonEmpty(email, credential.Email)
	addExtra(credential, muse.UserExtraKey, id)
	addExtra(credential, muse.TierExtraKey, payload.Tier())
	return nil
}
