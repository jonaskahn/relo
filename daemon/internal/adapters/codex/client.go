// Package codex owns the client identity and token claims used by ChatGPT's
// subscription-backed Codex endpoints.
package codex

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// Codex client identity is what ChatGPT's subscription endpoints accept:
// the family, the release whose protocol Relo speaks, and the beta flag.
const (
	// Originator identifies the client family to ChatGPT.
	Originator = "omp"
	// Version is the Codex client release whose protocol Relo speaks.
	Version = "0.159.0"
	// UserAgent is the HTTP client identity sent to ChatGPT.
	UserAgent = Originator + "/" + Version

	OpenAIBeta = "responses=experimental"

	authNamespace    = "https://api.openai.com/auth"
	profileNamespace = "https://api.openai.com/profile"
)

// Identity is the workspace identity carried by a ChatGPT token.
type Identity struct {
	AccountID string
	Email     string
	PlanType  string
	Residency string
}

// IdentityFromTokens reads ChatGPT's namespaced claims, preferring the access
// token because it identifies the workspace authorized for inference.
func IdentityFromTokens(accessToken, idToken string) Identity {
	identity := Identity{}
	for _, token := range []string{accessToken, idToken} {
		values, ok := claims(token)
		if !ok {
			continue
		}
		auth := namespace(values, authNamespace)
		profile := namespace(values, profileNamespace)
		identity.AccountID = first(identity.AccountID, text(auth, "chatgpt_account_id"), text(values, "chatgpt_account_id"))
		identity.PlanType = first(identity.PlanType, text(auth, "chatgpt_plan_type"))
		identity.Email = first(identity.Email, text(profile, "email"), text(values, "email"))
		identity.Residency = first(identity.Residency,
			text(auth, "chatgpt_data_residency"), text(values, "chatgpt_data_residency"),
			text(auth, "chatgpt_compute_residency"), text(values, "chatgpt_compute_residency"))
	}
	identity.Email = strings.ToLower(identity.Email)
	return identity
}

// ResidencyFromToken returns the region a ChatGPT workspace is pinned to.
func ResidencyFromToken(token string) string {
	return IdentityFromTokens(token, "").Residency
}

func claims(token string) (map[string]any, bool) {
	segments := strings.Split(token, ".")
	if len(segments) != 3 || segments[1] == "" {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(segments[1], "="))
	if err != nil {
		return nil, false
	}
	values := map[string]any{}
	if err := json.Unmarshal(payload, &values); err != nil {
		return nil, false
	}
	return values, true
}

func namespace(values map[string]any, name string) map[string]any {
	block, _ := values[name].(map[string]any)
	return block
}

func text(values map[string]any, name string) string {
	value, _ := values[name].(string)
	return strings.TrimSpace(value)
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
