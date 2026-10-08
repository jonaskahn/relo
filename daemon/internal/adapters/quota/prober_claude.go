// Claude quota prober for sign-in credentials.
package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/wire/anthropic"
)

const (
	claudeUsageEndpoint = "https://api.anthropic.com/api/oauth/usage"
	claudeUsageAccept   = "application/json, text/plain, */*"
	claudeUsageEncoding = "gzip, compress, deflate, br"
	claudeSonnetWindow  = "Sonnet"
)

// ClaudeProber reads the session and weekly windows of a Claude subscription.
type ClaudeProber struct {
	client   *http.Client
	endpoint string
}

// NewClaudeProber returns the quota prober for a signed-in Claude account.
func NewClaudeProber(options SignInProberOptions) *ClaudeProber {
	settings := options
	if settings.HTTPClient == nil {
		settings.HTTPClient = &http.Client{Timeout: probeTimeout}
	}
	return &ClaudeProber{
		client:   settings.HTTPClient,
		endpoint: firstNonEmpty(settings.Endpoint, claudeUsageEndpoint),
	}
}

// Probe returns the windows one Claude account has used.
func (p *ClaudeProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build the quota probe: %w", err)
	}
	request.Header.Set("Accept", claudeUsageAccept)
	request.Header.Set("Accept-Encoding", claudeUsageEncoding)
	request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	request.Header.Set(anthropic.BetaHeader, anthropic.ClaudeCodeBeta)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connection", "keep-alive")
	request.Header.Set("User-Agent", anthropic.CLIUserAgent)
	body, err := readProbeBody(p.client, request, credential.ID)
	if err != nil {
		return nil, err
	}
	windows, err := claudeWindows(body)
	if err != nil {
		return nil, err
	}
	if len(windows) == 0 {
		return nil, fmt.Errorf("%s: %w", credential.ID, activity.ErrProbeResponse)
	}
	return windows, nil
}

type claudeUsage struct {
	FiveHour       *claudeWindow  `json:"five_hour"`
	SevenDay       *claudeWindow  `json:"seven_day"`
	SevenDaySonnet *claudeWindow  `json:"seven_day_sonnet"`
	Limits         []claudeScoped `json:"limits"`
}

type claudeWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    string   `json:"resets_at"`
}

type claudeScoped struct {
	Kind     string   `json:"kind"`
	Percent  *float64 `json:"percent"`
	ResetsAt string   `json:"resets_at"`
	Scope    struct {
		Model struct {
			DisplayName string `json:"display_name"`
		} `json:"model"`
	} `json:"scope"`
}

func claudeWindows(body []byte) ([]activity.WindowSample, error) {
	usage := claudeUsage{}
	if err := json.Unmarshal(body, &usage); err != nil {
		return nil, fmt.Errorf("decode the quota probe response: %w (%v)", activity.ErrProbeResponse, err)
	}
	windows := make([]activity.WindowSample, 0, 4)
	windows = appendClaudeWindow(windows, usage.FiveHour, activity.WindowFiveHours, antigravitySession)
	windows = appendClaudeWindow(windows, usage.SevenDay, activity.WindowSevenDays, weeklySeconds)
	windows = appendClaudeWindow(windows, usage.SevenDaySonnet, claudeSonnetWindow, weeklySeconds)
	for _, limit := range usage.Limits {
		if limit.Kind != "weekly_scoped" || limit.Scope.Model.DisplayName == "" || limit.Percent == nil {
			continue
		}
		windows = append(windows, activity.WindowSample{
			Window: limit.Scope.Model.DisplayName, UsedPercent: clampPercent(*limit.Percent),
			ResetAt: probeReset(limit.ResetsAt), Seconds: weeklySeconds,
		})
	}
	return windows, nil
}

func appendClaudeWindow(windows []activity.WindowSample, window *claudeWindow, name string, seconds int64) []activity.WindowSample {
	if window == nil || window.Utilization == nil {
		return windows
	}
	return append(windows, activity.WindowSample{
		Window: name, UsedPercent: clampPercent(*window.Utilization),
		ResetAt: probeReset(window.ResetsAt), Seconds: seconds,
	})
}
