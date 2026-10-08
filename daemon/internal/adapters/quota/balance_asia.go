// Package quota probes vendor quota endpoints over HTTP and parses the
// usage windows their response headers report.
package quota

import (
	"context"

	"github.com/jonaskahn/relo/internal/activity"
)

const (
	moonshotOrigin    = "https://api.moonshot.ai"
	siliconFlowOrigin = "https://api.siliconflow.com"
	novitaOrigin      = "https://api.novita.ai"
)

// MoonshotProber reads the available balance of a Moonshot API key.
type MoonshotProber struct{ balanceHTTP }

// NewMoonshotProber returns a Moonshot API-key balance prober.
func NewMoonshotProber(options ProberOptions) *MoonshotProber {
	return &MoonshotProber{newBalanceHTTP(options)}
}

// Probe returns the available cash and voucher balance Moonshot reports.
func (p *MoonshotProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	endpoint := probeEndpoint(p.endpoint, credential.BaseURL, moonshotOrigin, "/v1/users/me/balance")
	body, err := p.get(ctx, endpoint, credential)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data struct {
			Available *float64 `json:"available_balance"`
		} `json:"data"`
	}
	if err := decodeBalance(body, &payload); err != nil {
		return nil, err
	}
	if payload.Data.Available == nil {
		return nil, activity.ErrProbeResponse
	}
	sample, _ := money(balanceWindow, *payload.Data.Available, balanceCurrency(credential.BaseURL))
	return []activity.WindowSample{sample}, nil
}

// SiliconFlowProber reads the available balance of a SiliconFlow API key.
type SiliconFlowProber struct{ balanceHTTP }

// NewSiliconFlowProber returns a SiliconFlow API-key balance prober.
func NewSiliconFlowProber(options ProberOptions) *SiliconFlowProber {
	return &SiliconFlowProber{newBalanceHTTP(options)}
}

// Probe returns the balance SiliconFlow includes in its user information.
func (p *SiliconFlowProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	endpoint := probeEndpoint(p.endpoint, credential.BaseURL, siliconFlowOrigin, "/v1/user/info")
	body, err := p.get(ctx, endpoint, credential)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data struct {
			Balance string `json:"balance"`
		} `json:"data"`
	}
	if err := decodeBalance(body, &payload); err != nil {
		return nil, err
	}
	sample, ok := moneyString(balanceWindow, payload.Data.Balance, balanceCurrency(credential.BaseURL))
	if !ok {
		return nil, activity.ErrProbeResponse
	}
	return []activity.WindowSample{sample}, nil
}

// NovitaProber reads the available balance of a Novita API key.
type NovitaProber struct{ balanceHTTP }

// NewNovitaProber returns a Novita API-key balance prober.
func NewNovitaProber(options ProberOptions) *NovitaProber {
	return &NovitaProber{newBalanceHTTP(options)}
}

// Probe returns Novita's available balance converted from 1/10000 USD.
func (p *NovitaProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	endpoint := probeEndpoint(p.endpoint, credential.BaseURL, novitaOrigin, "/openapi/v1/billing/balance/detail")
	body, err := p.get(ctx, endpoint, credential)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Available string `json:"availableBalance"`
	}
	if err := decodeBalance(body, &payload); err != nil {
		return nil, err
	}
	sample, ok := moneyString(balanceWindow, payload.Available, "USD")
	if !ok {
		return nil, activity.ErrProbeResponse
	}
	*sample.Amount /= 10000
	return []activity.WindowSample{sample}, nil
}
