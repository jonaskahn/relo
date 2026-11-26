// Package muse owns the Meta Muse key response shared by sign-in and quota.
package muse

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// KeyEndpoint is where Meta exchanges an account token for a Muse key.
	KeyEndpoint = "https://api.meta.ai/muse-code/key"
	// APIVersion is the header value Meta requires on key and device calls.
	APIVersion = "1.0.0"
	// AccessTokenExtraKey holds the Meta account token behind a minted key.
	AccessTokenExtraKey = "museOAuthAccessToken"
	// UserExtraKey holds the stable Meta account identity.
	UserExtraKey = "museUserId"
	// TierExtraKey holds the subscription tier Meta reported at sign-in.
	TierExtraKey = "museTierName"
)

var (
	// ErrSubscriptionInactive reports a Meta account with no Muse subscription.
	ErrSubscriptionInactive = errors.New("the Meta Muse subscription is inactive")
	// ErrPaymentRequired reports a Meta account that must pay before Muse works.
	ErrPaymentRequired = errors.New("the Meta Muse subscription requires payment")
)

// Headers are the vendor headers every Muse key and device call carries.
func Headers() map[string]string {
	return map[string]string{"x-api-version": APIVersion}
}

// KeyResponse is the subscription state Meta returns for an account token.
type KeyResponse struct {
	APIKey           string `json:"api_key"`
	UserID           string `json:"user_id"`
	UserEmail        string `json:"user_email"`
	IsSubsActive     *bool  `json:"is_subs_active"`
	RequirePayment   bool   `json:"require_payment"`
	ActionURL        string `json:"action_url"`
	PaymentActionURL string `json:"require_payment_action_url"`
	SubsTierID       string `json:"subs_tier_id"`
	SubsTierName     string `json:"subs_tier_name"`
	SubsUsage        *Usage `json:"subs_usage"`
}

// Usage is the pair of windows a Muse subscription publishes.
type Usage struct {
	Window *Window `json:"window"`
	Weekly *Window `json:"weekly"`
}

// Window is one vendor-reported usage period.
type Window struct {
	UsedPercent        *float64        `json:"used_percent"`
	ResetsAt           json.RawMessage `json:"resets_at"`
	WindowDurationMins *float64        `json:"window_duration_mins"`
}

// Decode reads one key response. It never fills in usage Meta did not send.
func Decode(body []byte) (KeyResponse, error) {
	response := KeyResponse{}
	if err := json.Unmarshal(body, &response); err != nil {
		return response, fmt.Errorf("decode the Muse key response: %w", err)
	}
	return response, nil
}

// Inactive reports whether Meta explicitly refused a Muse subscription.
func (r KeyResponse) Inactive() bool {
	return r.IsSubsActive != nil && !*r.IsSubsActive
}

// PaymentURL is the action address Meta sent when payment is required.
func (r KeyResponse) PaymentURL() string {
	return firstNonEmpty(r.ActionURL, r.PaymentActionURL)
}

// Tier is the subscription name, falling back to the vendor tier identifier.
func (r KeyResponse) Tier() string {
	return firstNonEmpty(r.SubsTierName, r.SubsTierID)
}

// Identity returns the stable account id and email the response names.
func (r KeyResponse) Identity() (string, string) {
	id := strings.TrimSpace(r.UserID)
	email := strings.ToLower(strings.TrimSpace(r.UserEmail))
	if id == "" {
		id = email
	}
	return id, email
}

// ResetAt returns the vendor reset as Unix seconds, or zero when unstated.
func (w Window) ResetAt() int64 {
	raw := strings.TrimSpace(string(w.ResetsAt))
	if raw == "" || raw == "null" {
		return 0
	}
	if value, err := strconv.ParseFloat(raw, 64); err == nil {
		return resetSeconds(value)
	}
	var text string
	if err := json.Unmarshal(w.ResetsAt, &text); err != nil {
		return 0
	}
	if value, err := strconv.ParseFloat(text, 64); err == nil {
		return resetSeconds(value)
	}
	if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
		return parsed.Unix()
	}
	return 0
}

// Seconds returns the rolling window length Meta reported, or its documented
// five-hour default when Meta names none.
func (w Window) Seconds() int64 {
	if w.WindowDurationMins == nil || *w.WindowDurationMins <= 0 {
		return 5 * 60
	}
	return int64(*w.WindowDurationMins * 60)
}

func resetSeconds(value float64) int64 {
	if value <= 0 {
		return 0
	}
	if value > 1e12 {
		return int64(value / 1000)
	}
	return int64(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
