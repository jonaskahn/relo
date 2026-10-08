// DeepSeek and Teamo balance probers.
package quota

import (
	"context"

	"github.com/jonaskahn/relo/internal/activity"
)

const (
	deepSeekOrigin = "https://api.deepseek.com"
	teamoOrigin    = "https://api.teamorouter.com"
)

// DeepSeekProber reads the balances available to a DeepSeek API key.
type DeepSeekProber struct{ balanceHTTP }

// NewDeepSeekProber returns a DeepSeek API-key balance prober.
func NewDeepSeekProber(options ProberOptions) *DeepSeekProber {
	return &DeepSeekProber{newBalanceHTTP(options)}
}

// Probe returns one money reading for each currency DeepSeek reports.
func (p *DeepSeekProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	endpoint := probeEndpoint(p.endpoint, credential.BaseURL, deepSeekOrigin, "/user/balance")
	body, err := p.get(ctx, endpoint, credential)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Balances []struct {
			Currency string `json:"currency"`
			Total    string `json:"total_balance"`
		} `json:"balance_infos"`
	}
	if err := decodeBalance(body, &payload); err != nil {
		return nil, err
	}
	samples := make([]activity.WindowSample, 0, len(payload.Balances))
	for _, balance := range payload.Balances {
		window := balanceWindow
		if len(payload.Balances) > 1 {
			window += "-" + balance.Currency
		}
		if sample, ok := moneyString(window, balance.Total, balance.Currency); ok {
			samples = append(samples, sample)
		}
	}
	return requireBalance(credential.ID, samples)
}

// TeamoRouterProber reads the available balance of a TeamoRouter API key.
type TeamoRouterProber struct{ balanceHTTP }

// NewTeamoRouterProber returns a TeamoRouter API-key balance prober.
func NewTeamoRouterProber(options ProberOptions) *TeamoRouterProber {
	return &TeamoRouterProber{newBalanceHTTP(options)}
}

// Probe returns the account balance TeamoRouter reports.
func (p *TeamoRouterProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	endpoint := probeEndpoint(p.endpoint, credential.BaseURL, teamoOrigin, "/v1/billing/balance")
	body, err := p.get(ctx, endpoint, credential)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Balance struct {
			Value    string `json:"value"`
			Currency string `json:"currency"`
		} `json:"balance"`
	}
	if err := decodeBalance(body, &payload); err != nil {
		return nil, err
	}
	sample, ok := moneyString(balanceWindow, payload.Balance.Value, payload.Balance.Currency)
	if !ok {
		return nil, activity.ErrProbeResponse
	}
	return []activity.WindowSample{sample}, nil
}
