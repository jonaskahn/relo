// OpenRouter quota prober for sign-in credentials.
package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/activity"
)

const (
	openRouterAPIRoot     = "https://openrouter.ai/api/v1"
	openRouterCreditsPath = "/credits"
	openRouterKeyPath     = "/key"
	openRouterCreditsWin  = "credits"
	openRouterKeyWin      = "key"
)

// OpenRouterProber reads the credit balance and optional key cap of an
// OpenRouter API key.
type OpenRouterProber struct {
	client     *http.Client
	creditsURL string
	keyURL     string
}

// NewOpenRouterProber returns the quota prober for an OpenRouter API key.
// Endpoint, when set, is the API root a test points at itself.
func NewOpenRouterProber(options SignInProberOptions) *OpenRouterProber {
	settings := options
	if settings.HTTPClient == nil {
		settings.HTTPClient = &http.Client{Timeout: probeTimeout}
	}
	root := strings.TrimSuffix(firstNonEmpty(settings.Endpoint, openRouterAPIRoot), "/")
	return &OpenRouterProber{
		client: settings.HTTPClient, creditsURL: root + openRouterCreditsPath,
		keyURL: root + openRouterKeyPath,
	}
}

// Probe returns the credit window, and the key-cap window when the key has
// one. A credits call that cannot be read is a failed probe; a key call that
// fails is dropped so the balance still stores.
func (p *OpenRouterProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	credits, err := p.get(ctx, p.creditsURL, credential)
	if err != nil {
		return nil, err
	}
	windows := openRouterCredits(credits)
	if key, keyErr := p.get(ctx, p.keyURL, credential); keyErr == nil {
		windows = append(windows, openRouterKey(key)...)
	}
	if len(windows) == 0 {
		return nil, fmt.Errorf("%s: %w", credential.ID, activity.ErrProbeResponse)
	}
	return windows, nil
}

func (p *OpenRouterProber) get(ctx context.Context, endpoint string, credential activity.Credential) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build the quota probe: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	return readProbeBody(p.client, request, credential.ID)
}

func openRouterCredits(body []byte) []activity.WindowSample {
	var payload struct {
		Data struct {
			TotalCredits *float64 `json:"total_credits"`
			TotalUsage   *float64 `json:"total_usage"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.Data.TotalCredits == nil || payload.Data.TotalUsage == nil {
		return nil
	}
	if *payload.Data.TotalCredits <= 0 {
		return nil
	}
	used := (*payload.Data.TotalUsage / *payload.Data.TotalCredits) * 100
	return []activity.WindowSample{{
		Window: openRouterCreditsWin, UsedPercent: clampPercent(used),
	}}
}

func openRouterKey(body []byte) []activity.WindowSample {
	var payload struct {
		Data struct {
			Limit          *float64 `json:"limit"`
			LimitRemaining *float64 `json:"limit_remaining"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.Data.Limit == nil || *payload.Data.Limit <= 0 {
		return nil
	}
	remaining := 0.0
	if payload.Data.LimitRemaining != nil {
		remaining = *payload.Data.LimitRemaining
	}
	used := ((*payload.Data.Limit - remaining) / *payload.Data.Limit) * 100
	return []activity.WindowSample{{
		Window: openRouterKeyWin, UsedPercent: clampPercent(used),
	}}
}
