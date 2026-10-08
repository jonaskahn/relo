// Shared OAuth helpers: body decoding and credential validation.
package oauth

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const projectExtraKey = "projectId"

func decodeJSONBody(body []byte, target any) error {
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode the provider response: %w", err)
	}
	return nil
}

func validateCredential(now time.Time, cred *OAuthCredential) error {
	switch {
	case cred == nil || cred.AccessToken == "":
		return ErrTokenResponse
	case expires(cred, now, 0):
		return ErrCredentialExpired
	default:
		return nil
	}
}

func expires(cred *OAuthCredential, now time.Time, margin time.Duration) bool {
	return !cred.ExpiresAt.IsZero() && cred.ExpiresWithin(now, margin)
}

func newUUID() string {
	raw := randomBytes(uuidBytes)
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
}

// IdentityFromTokens returns the account identifier and address an ID or
// access token names, preferring the ChatGPT claims and falling back to
// the standard subject claim every other provider uses.
func IdentityFromTokens(idToken, accessToken string) (accountID, email string) {
	accountID = firstNonEmpty(AccountIDFromTokens(idToken, accessToken), SubjectFromToken(accessToken), SubjectFromToken(idToken))
	email = firstNonEmpty(EmailFromTokens(idToken, accessToken))
	return accountID, email
}

func hasScope(token, scope string) bool {
	claims, ok := decodeClaims(token)
	if !ok {
		return false
	}
	for _, granted := range splitScopes(claims.text("scope")) {
		if granted == scope {
			return true
		}
	}
	return false
}

func splitScopes(raw string) []string {
	return strings.Fields(raw)
}
