// Meta Muse sign-in flow with keychain-backed identity.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	providerMetaMuse        = FlowMetaMuse
	musePointerFile         = ".config/muse/auth.json"
	museKeychainService     = "ai.meta.dev.credentials"
	museKeychainAccount     = "meta"
	museDeviceClientID      = "1031625952748946"
	museDeviceStartURL      = "https://auth.meta.com/oidc/device/authorization/"
	museDeviceTokenURL      = "https://auth.meta.com/oidc/device/token/"
	museVerificationURL     = "https://auth.meta.com/oauth/device/"
	museKeyURL              = "https://api.meta.ai/muse-code/key"
	museOAuthAccessExtraKey = "museOAuthAccessToken"
	museUserExtraKey        = "museUserId"
	museTierExtraKey        = "museTierName"
)

// MuseConsentWarning is the terms warning Relo shows before it reads a
// Meta credential. Reusing the CLI's key is a weaker claim than a device
// login, so the operator decides knowingly.
const MuseConsentWarning = "Meta Muse access relies on a personal-account credential. " +
	"Using it with Relo may conflict with Meta's terms of service for the Muse Code client."

// Muse errors name why its credential pointer failed: nothing installed,
// a locked keychain, or a pointer without a key.
var (
	ErrMusePointerMissing = errors.New("no Meta Muse credential pointer is installed; run the Muse Code CLI login first")
	ErrMuseKeychainLocked = errors.New("the Meta Muse secret is in the OS keychain, which Relo cannot read on this host")
	ErrMuseKeyMissing     = errors.New("the Meta Muse credential holds no API key")
)

// KeychainReader reads a secret another vendor stored in the OS
// keychain. Only the secret package touches the keychain itself.
type KeychainReader interface {
	Get(service, account string) (string, error)
}

type metaMuseFlow struct {
	settings Options
	client   tokenClient
}

// NewMetaMuseFlow returns the import-first login for a Meta Muse account.
func NewMetaMuseFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	return &metaMuseFlow{settings: settings, client: settings.tokenClient(nil)}
}

// ProviderID returns the provider this flow logs into.
func (f *metaMuseFlow) ProviderID() string {
	return providerMetaMuse
}

// CallbackPort returns zero: the Meta Muse login imports a key an installed
// CLI already holds, so it never binds a loopback listener.
func (f *metaMuseFlow) CallbackPort() int {
	return 0
}

// Login warns about the terms, imports the installed Muse credential,
// and starts a device approval when there is none.
func (f *metaMuseFlow) Login(ctx context.Context, opts LoginOpts) (*OAuthCredential, error) {
	warn(f.settings, MuseConsentWarning)
	imported, err := f.readPointer()
	if err == nil {
		announce(opts, AuthPrompt{Instructions: "Imported the installed Meta Muse credential."})
		return imported, nil
	}
	if !errors.Is(err, ErrMusePointerMissing) && !errors.Is(err, ErrMuseKeychainLocked) {
		return nil, err
	}
	return f.deviceLogin(ctx, opts)
}

// Refresh re-reads the pointer, and re-mints the model key when the
// device access token is the only thing Relo holds.
func (f *metaMuseFlow) Refresh(ctx context.Context, cred *OAuthCredential) (*OAuthCredential, error) {
	if cred == nil {
		return nil, ErrNoRefreshToken
	}
	if cred.Extra[museOAuthAccessExtraKey] != "" {
		refreshed := cred.Clone()
		if err := f.mintKey(ctx, &refreshed); err != nil {
			return nil, err
		}
		return &refreshed, nil
	}
	imported, err := f.readPointer()
	if err != nil {
		return nil, fmt.Errorf("refresh the Meta Muse credential: %w", err)
	}
	merged := mergeCredential(*cred, *imported)
	return &merged, nil
}

// Validate reports whether the credential can still serve a request.
func (f *metaMuseFlow) Validate(_ context.Context, cred *OAuthCredential) error {
	return validateCredential(f.settings.clockOrReal().Now(), cred)
}

func (f *metaMuseFlow) readPointer() (*OAuthCredential, error) {
	raw, err := os.ReadFile(f.pointerPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrMusePointerMissing
		}
		return nil, fmt.Errorf("read the Meta Muse pointer: %w", err)
	}
	pointer := struct {
		Providers map[string]struct {
			Storage string `json:"storage"`
		} `json:"providers"`
	}{}
	if err := decodeJSONBody(raw, &pointer); err != nil {
		return nil, err
	}
	if pointer.Providers["meta"].Storage != "keychain" {
		return nil, ErrMusePointerMissing
	}
	return f.readKeychain()
}

func (f *metaMuseFlow) readKeychain() (*OAuthCredential, error) {
	if f.settings.Keychain == nil {
		return nil, ErrMuseKeychainLocked
	}
	secret, err := f.settings.Keychain.Get(museKeychainService, museKeychainAccount)
	if err != nil {
		return nil, fmt.Errorf("read the Meta Muse secret: %w", err)
	}
	key := apiKeyFromSecret(secret)
	if key == "" {
		return nil, ErrMuseKeyMissing
	}
	return &OAuthCredential{AccessToken: key, RefreshToken: key, Scope: providerMetaMuse}, nil
}

func (f *metaMuseFlow) deviceLogin(ctx context.Context, opts LoginOpts) (*OAuthCredential, error) {
	flow := newDeviceFlow(deviceConfig{
		providerID:      providerMetaMuse,
		clientID:        museDeviceClientID,
		startURL:        option(f.settings.Endpoints.DeviceURL, museDeviceStartURL),
		tokenURL:        option(f.settings.Endpoints.DeviceTokenURL, museDeviceTokenURL),
		verificationURL: option(f.settings.Endpoints.VerificationURL, museVerificationURL),
		finish:          f.finishDevice,
	}, f.settings)
	return flow.Login(ctx, opts)
}

func (f *metaMuseFlow) finishDevice(_ context.Context, _ tokenClient, response tokenResponse, credential *OAuthCredential) error {
	accountAccess := firstNonEmpty(response.AccessToken, response.RefreshToken)
	if accountAccess == "" {
		return ErrTokenResponse
	}
	credential.RefreshToken = accountAccess
	addExtra(credential, museOAuthAccessExtraKey, accountAccess)
	credential.AccountID, credential.Email = IdentityFromTokens("", accountAccess)
	return f.mintKey(context.Background(), credential)
}

func (f *metaMuseFlow) mintKey(ctx context.Context, credential *OAuthCredential) error {
	access := credential.Extra[museOAuthAccessExtraKey]
	if access == "" {
		return ErrNoRefreshToken
	}
	body, status, err := f.client.postJSONRaw(ctx, option(f.settings.Endpoints.APIBaseURL, museKeyURL), map[string]string{}, bearerHeader(access))
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return museKeyFailure(status, body)
	}
	return applyMuseKey(credential, body)
}

func museKeyFailure(status int, body []byte) error {
	if status == 401 || status == 403 {
		return ErrRefreshRejected
	}
	return responseError{Status: status, Code: errorCode(body)}
}

func applyMuseKey(credential *OAuthCredential, body []byte) error {
	payload := struct {
		Key      string `json:"key"`
		UserID   string `json:"user_id"`
		TierName string `json:"tier_name"`
	}{}
	if err := decodeJSONBody(body, &payload); err != nil {
		return err
	}
	if payload.Key == "" {
		return ErrMuseKeyMissing
	}
	credential.AccessToken = payload.Key
	addExtra(credential, museUserExtraKey, payload.UserID)
	addExtra(credential, museTierExtraKey, payload.TierName)
	return nil
}

func (f *metaMuseFlow) pointerPath() string {
	if f.settings.CredentialsPath != "" {
		return f.settings.CredentialsPath
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return musePointerFile
	}
	return filepath.Join(home, musePointerFile)
}

func apiKeyFromSecret(secret string) string {
	key := struct {
		APIKey string `json:"api_key"`
		Key    string `json:"key"`
	}{}
	if err := json.Unmarshal([]byte(secret), &key); err == nil {
		return firstNonEmpty(key.APIKey, key.Key)
	}
	return secret
}

func warn(settings Options, message string) {
	if settings.Warn != nil {
		settings.Warn(message)
	}
}
