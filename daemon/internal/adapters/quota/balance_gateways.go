// Gateway balance probers: Vercel and router endpoints.
package quota

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"github.com/jonaskahn/relo/internal/activity"
)

const (
	vercelCreditsEndpoint = "https://ai-gateway.vercel.sh/v1/credits"
	orcaOrigin            = "https://api.orcarouter.ai"
	orcaUsagePath         = "/v1/dashboard/billing/usage"
	orcaSubscriptionPath  = "/v1/dashboard/billing/subscription"
	orcaUnlimitedCents    = int64(100000000)
)

// VercelGatewayProber reads the balance and lifetime spend of a Vercel AI Gateway key.
type VercelGatewayProber struct{ balanceHTTP }

// NewVercelGatewayProber returns a Vercel AI Gateway balance prober.
func NewVercelGatewayProber(options ProberOptions) *VercelGatewayProber {
	return &VercelGatewayProber{newBalanceHTTP(options)}
}

// Probe returns the balance and lifetime spend Vercel reports.
func (p *VercelGatewayProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	endpoint := probeEndpoint(p.endpoint, credential.BaseURL, endpointOrigin(vercelCreditsEndpoint), "/v1/credits")
	body, err := p.get(ctx, endpoint, credential)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Balance   string `json:"balance"`
		TotalUsed string `json:"total_used"`
	}
	if err := decodeBalance(body, &payload); err != nil {
		return nil, err
	}
	samples := make([]activity.WindowSample, 0, 2)
	if sample, ok := moneyString(balanceWindow, payload.Balance, "USD"); ok {
		samples = append(samples, sample)
	}
	if sample, ok := moneyString(spentWindow, payload.TotalUsed, "USD"); ok {
		samples = append(samples, sample)
	}
	return requireBalance(credential.ID, samples)
}

// OrcaRouterProber reads the spend and optional remaining key quota of an OrcaRouter key.
type OrcaRouterProber struct{ balanceHTTP }

// NewOrcaRouterProber returns an OrcaRouter API-key balance prober.
func NewOrcaRouterProber(options ProberOptions) *OrcaRouterProber {
	return &OrcaRouterProber{newBalanceHTTP(options)}
}

// Probe returns lifetime spend and, for capped keys, the remaining balance.
func (p *OrcaRouterProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	root := p.root(credential.BaseURL)
	usage, err := p.get(ctx, root+orcaUsagePath, credential)
	if err != nil {
		return nil, err
	}
	spent, err := orcaSpent(usage)
	if err != nil {
		return nil, err
	}
	samples := []activity.WindowSample{spent}
	if subscription, fetchErr := p.get(ctx, root+orcaSubscriptionPath, credential); fetchErr == nil {
		if balance, ok := orcaBalance(subscription, *spent.Amount); ok {
			samples = append(samples, balance)
		}
	}
	return samples, nil
}

func (p *OrcaRouterProber) root(baseURL string) string {
	if p.endpoint != "" {
		return strings.TrimSuffix(p.endpoint, "/")
	}
	if origin := endpointOrigin(baseURL); origin != "" {
		return origin
	}
	return orcaOrigin
}

func orcaSpent(body []byte) (activity.WindowSample, error) {
	var payload struct {
		TotalUsage json.Number `json:"total_usage"`
	}
	if err := decodeBalance(body, &payload); err != nil {
		return activity.WindowSample{}, err
	}
	cents, err := payload.TotalUsage.Int64()
	if err != nil {
		return activity.WindowSample{}, activity.ErrProbeResponse
	}
	sample, _ := money(spentWindow, float64(cents)/100, "USD")
	return sample, nil
}

func orcaBalance(body []byte, spent float64) (activity.WindowSample, bool) {
	var payload struct {
		Soft   *int64 `json:"soft_limit_usd"`
		Hard   *int64 `json:"hard_limit_usd"`
		System *int64 `json:"system_hard_limit_usd"`
	}
	if decodeBalance(body, &payload) != nil {
		return activity.WindowSample{}, false
	}
	limit := firstInt64(payload.Hard, payload.Soft, payload.System)
	if limit == nil || *limit == orcaUnlimitedCents {
		return activity.WindowSample{}, false
	}
	remaining := math.Max(0, float64(*limit)/100-spent)
	return money(balanceWindow, remaining, "USD")
}

func firstInt64(values ...*int64) *int64 {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
