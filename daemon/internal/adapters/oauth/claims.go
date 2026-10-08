// Identity claims: reading account and email out of vendor tokens.
package oauth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

const chatGPTAuthNamespace = "https://api.openai.com/auth"

// AccountIDFromTokens returns the account identifier an ID or access
// token carries, following the ChatGPT claim precedence: the top-level
// claim, then the OpenAI auth namespace, then the first organization.
func AccountIDFromTokens(idToken, accessToken string) string {
	for _, token := range []string{idToken, accessToken} {
		if claims, ok := decodeClaims(token); ok {
			if accountID := claims.accountID(); accountID != "" {
				return accountID
			}
		}
	}
	return ""
}

// EmailFromTokens returns the address an ID or access token names, and
// the empty string when neither carries one.
func EmailFromTokens(idToken, accessToken string) string {
	for _, token := range []string{idToken, accessToken} {
		if claims, ok := decodeClaims(token); ok {
			if email := claims.text("email"); email != "" {
				return strings.ToLower(email)
			}
		}
	}
	return ""
}

// SubjectFromToken returns the subject claim of a token, which the
// providers that mint opaque per-user identifiers key their accounts on.
func SubjectFromToken(token string) string {
	claims, ok := decodeClaims(token)
	if !ok {
		return ""
	}
	return claims.text("sub")
}

// StringClaim returns one string claim of a token, or the empty string.
func StringClaim(token, key string) string {
	claims, ok := decodeClaims(token)
	if !ok {
		return ""
	}
	return claims.text(key)
}

type claims struct {
	values map[string]any
}

func decodeClaims(token string) (claims, bool) {
	segments := strings.Split(token, ".")
	if len(segments) != 3 || segments[1] == "" {
		return claims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(segments[1], "="))
	if err != nil {
		return claims{}, false
	}
	values := map[string]any{}
	if err := json.Unmarshal(payload, &values); err != nil {
		return claims{}, false
	}
	return claims{values: values}, true
}

func (c claims) text(key string) string {
	value, found := c.values[key].(string)
	if !found {
		return ""
	}
	return strings.TrimSpace(value)
}

func (c claims) number(key string) (float64, bool) {
	value, found := c.values[key].(float64)
	return value, found
}

func (c claims) accountID() string {
	if accountID := c.text("chatgpt_account_id"); accountID != "" {
		return accountID
	}
	namespace, found := c.values[chatGPTAuthNamespace].(map[string]any)
	if found {
		if accountID, ok := namespace["chatgpt_account_id"].(string); ok && strings.TrimSpace(accountID) != "" {
			return strings.TrimSpace(accountID)
		}
	}
	organizations, found := c.values["organizations"].([]any)
	if !found || len(organizations) == 0 {
		return ""
	}
	first, found := organizations[0].(map[string]any)
	if !found {
		return ""
	}
	identifier, _ := first["id"].(string)
	return strings.TrimSpace(identifier)
}
