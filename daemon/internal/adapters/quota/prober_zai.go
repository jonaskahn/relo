// ZAI quota prober for sign-in credentials.
package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/activity"
)

const zaiQuotaEndpoint = "https://api.z.ai/api/monitor/usage/quota/limit"

// ZAIProber reads the session and weekly token windows of a GLM Coding Plan.
type ZAIProber struct {
	client   *http.Client
	endpoint string
}

// NewZAIProber returns the quota prober for a Z.ai API key.
func NewZAIProber(options SignInProberOptions) *ZAIProber {
	settings := options
	if settings.HTTPClient == nil {
		settings.HTTPClient = &http.Client{Timeout: probeTimeout}
	}
	return &ZAIProber{
		client:   settings.HTTPClient,
		endpoint: firstNonEmpty(settings.Endpoint, zaiQuotaEndpoint),
	}
}

// Probe returns the percentage windows one Z.ai key has used.
func (p *ZAIProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build the quota probe: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	body, err := readProbeBody(p.client, request, credential.ID)
	if err != nil {
		return nil, err
	}
	if zaiNoCodingPlan(body) {
		return nil, fmt.Errorf("%s: %w", credential.ID, activity.ErrProbeResponse)
	}
	windows, err := zaiWindows(body)
	if err != nil {
		return nil, err
	}
	if len(windows) == 0 {
		return nil, fmt.Errorf("%s: %w", credential.ID, activity.ErrProbeResponse)
	}
	return windows, nil
}

type zaiQuota struct {
	Success *bool  `json:"success"`
	Msg     string `json:"msg"`
	Data    *struct {
		Limits []zaiLimit `json:"limits"`
	} `json:"data"`
	Limits []zaiLimit `json:"limits"`
}

type zaiLimit struct {
	Type       string   `json:"type"`
	Name       string   `json:"name"`
	Unit       *float64 `json:"unit"`
	Number     *float64 `json:"number"`
	Percentage *float64 `json:"percentage"`
	NextReset  *float64 `json:"nextResetTime"`
}

func zaiNoCodingPlan(body []byte) bool {
	quota := zaiQuota{}
	if json.Unmarshal(body, &quota) != nil || quota.Success == nil || *quota.Success {
		return false
	}
	return strings.Contains(strings.ToLower(quota.Msg), "coding plan")
}

func zaiWindows(body []byte) ([]activity.WindowSample, error) {
	quota := zaiQuota{}
	if err := json.Unmarshal(body, &quota); err != nil {
		return nil, fmt.Errorf("decode the quota probe response: %w (%v)", activity.ErrProbeResponse, err)
	}
	limits := quota.Limits
	if quota.Data != nil {
		limits = quota.Data.Limits
	}
	windows := make([]activity.WindowSample, 0, 2)
	for _, limit := range limits {
		kind := firstNonEmpty(limit.Type, limit.Name)
		if kind != "CREDIT_LIMIT" && kind != "TOKENS_LIMIT" {
			continue
		}
		sample, ok := zaiSample(limit)
		if ok {
			windows = append(windows, sample)
		}
	}
	return windows, nil
}

func zaiSample(limit zaiLimit) (activity.WindowSample, bool) {
	if limit.Percentage == nil || limit.Unit == nil || limit.Number == nil || *limit.Number <= 0 {
		return activity.WindowSample{}, false
	}
	seconds := zaiWindowSeconds(*limit.Unit, *limit.Number)
	if seconds <= 0 {
		return activity.WindowSample{}, false
	}
	reset := int64(0)
	if limit.NextReset != nil {
		reset = int64(*limit.NextReset / 1000)
	}
	return activity.WindowSample{
		Window: windowName(seconds), UsedPercent: clampPercent(*limit.Percentage),
		ResetAt: reset, Seconds: seconds,
	}, true
}

func zaiWindowSeconds(unit, number float64) int64 {
	var unitSeconds int64
	switch unit {
	case 3:
		unitSeconds = 60 * 60
	case 4:
		unitSeconds = dailySeconds
	case 6:
		unitSeconds = weeklySeconds
	case 5:
		unitSeconds = monthlySeconds
	default:
		return 0
	}
	return int64(number * float64(unitSeconds))
}
