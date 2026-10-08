// Sign-in quota probers: Grok and the shared options.
package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/contentencoding"
)

// The endpoints and headers the sign-in providers publish their quota on. A
// probe reads only what the provider exposes: a connection whose plan has no
// meter answers unavailable rather than a made-up figure.
const (
	grokBillingEndpoint     = "https://cli-chat-proxy.grok.com/v1/billing?format=credits"
	grokTokenAuthHeader     = "x-xai-token-auth"
	grokTokenAuthValue      = "xai-grok-cli"
	grokAuthenticateHeader  = "x-authenticateresponse"
	grokAuthenticateValue   = "authenticate-response"
	grokClientVersionHeader = "x-grok-client-version"
	grokClientVersionValue  = "1.0.13"
	grokUserHeader          = "x-userid"

	cursorUsageEndpoint  = "https://api2.cursor.sh/aiserver.v1.DashboardService/GetCurrentPeriodUsage"
	cursorConnectVersion = "1"

	copilotUserEndpoint  = "https://api.github.com/copilot_internal/user"
	copilotEditorVersion = "vscode/1.96.2"
	copilotPluginVersion = "copilot-chat/0.26.7"
	copilotUserAgent     = "GitHubCopilotChat/0.26.7"

	kimiUsagePath = "/usages"

	// The window lengths these providers publish, so a stored window carries
	// the span it applies to even when the answer names only a percentage.
	monthlySeconds = int64(30 * 24 * 60 * 60)
	weeklySeconds  = int64(7 * 24 * 60 * 60)
	dailySeconds   = int64(24 * 60 * 60)
)

// SignInProberOptions tunes the probers whose endpoint may be pointed at a
// test server.
type SignInProberOptions struct {
	HTTPClient *http.Client
	Endpoint   string
	Now        func() time.Time
}

// GrokProber reads the shared weekly pool of a Grok subscription.
type GrokProber struct {
	client   *http.Client
	endpoint string
}

// NewGrokProber returns the quota prober for a signed-in Grok account.
func NewGrokProber(options SignInProberOptions) *GrokProber {
	settings := options
	if settings.HTTPClient == nil {
		settings.HTTPClient = &http.Client{Timeout: probeTimeout}
	}
	return &GrokProber{client: settings.HTTPClient, endpoint: firstNonEmpty(settings.Endpoint, grokBillingEndpoint)}
}

// Probe returns the weekly window one Grok account has left.
func (p *GrokProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build the quota probe: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", probeUserAgent)
	request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	request.Header.Set(grokTokenAuthHeader, grokTokenAuthValue)
	request.Header.Set(grokAuthenticateHeader, grokAuthenticateValue)
	request.Header.Set(grokClientVersionHeader, grokClientVersionValue)
	if user := credential.Extra["account_id"]; user != "" {
		request.Header.Set(grokUserHeader, user)
	}
	body, err := readProbeBody(p.client, request, credential.ID)
	if err != nil {
		return nil, err
	}
	return grokWindows(body, credential.ID)
}

type grokQuotaPayload struct {
	Config struct {
		CreditUsagePercent *float64 `json:"creditUsagePercent"`
		CurrentPeriod      *struct {
			Type  string `json:"type"`
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"currentPeriod"`
	} `json:"config"`
}

func grokWindows(body []byte, credentialID string) ([]activity.WindowSample, error) {
	var payload grokQuotaPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode the quota probe response: %w (%v)", activity.ErrProbeResponse, err)
	}
	period := payload.Config.CurrentPeriod
	if period == nil || period.Type == "" {
		return nil, fmt.Errorf("%s: %w", credentialID, activity.ErrProbeResponse)
	}
	seconds := weeklySeconds
	reset := int64(0)
	if start, end := parseProbeTime(period.Start), parseProbeTime(period.End); !start.IsZero() && end.After(start) {
		seconds = int64(end.Sub(start).Seconds())
		reset = end.Unix()
	}
	used := float64(0)
	if payload.Config.CreditUsagePercent != nil {
		used = *payload.Config.CreditUsagePercent
	}
	return []activity.WindowSample{{
		Window: windowName(seconds), UsedPercent: clampPercent(used), ResetAt: reset, Seconds: seconds,
	}}, nil
}

// CursorProber reads the monthly allowance of a Cursor account.
type CursorProber struct {
	client   *http.Client
	endpoint string
}

// NewCursorProber returns the quota prober for a signed-in Cursor account.
func NewCursorProber(options SignInProberOptions) *CursorProber {
	settings := options
	if settings.HTTPClient == nil {
		settings.HTTPClient = &http.Client{Timeout: probeTimeout}
	}
	return &CursorProber{client: settings.HTTPClient, endpoint: firstNonEmpty(settings.Endpoint, cursorUsageEndpoint)}
}

// Probe returns the allowance window one Cursor account has left.
func (p *CursorProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, strings.NewReader("{}"))
	if err != nil {
		return nil, fmt.Errorf("build the quota probe: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", probeUserAgent)
	request.Header.Set("Connect-Protocol-Version", cursorConnectVersion)
	request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	body, err := readProbeBody(p.client, request, credential.ID)
	if err != nil {
		return nil, err
	}
	return cursorWindows(body, credential.ID)
}

type cursorPlanUsage struct {
	Limit            *float64 `json:"limit"`
	LimitCents       *float64 `json:"limitCents"`
	Remaining        *float64 `json:"remaining"`
	RemainingCents   *float64 `json:"remainingCents"`
	IncludedSpend    *float64 `json:"includedSpend"`
	TotalPercentUsed *float64 `json:"totalPercentUsed"`
	AutoPercentUsed  *float64 `json:"autoPercentUsed"`
	APIPercentUsed   *float64 `json:"apiPercentUsed"`
}

type cursorQuotaPayload struct {
	PlanUsage       *cursorPlanUsage `json:"planUsage"`
	BillingCycleEnd string           `json:"billingCycleEnd"`
}

func cursorWindows(body []byte, credentialID string) ([]activity.WindowSample, error) {
	var payload cursorQuotaPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode the quota probe response: %w (%v)", activity.ErrProbeResponse, err)
	}
	if payload.PlanUsage == nil {
		return nil, fmt.Errorf("%s: %w", credentialID, activity.ErrProbeResponse)
	}
	used, known := cursorUsed(payload.PlanUsage)
	if !known {
		return nil, fmt.Errorf("%s: %w", credentialID, activity.ErrProbeResponse)
	}
	windows := []activity.WindowSample{{
		Window: "30d", UsedPercent: clampPercent(used),
		ResetAt: parseProbeTime(payload.BillingCycleEnd).Unix(), Seconds: monthlySeconds,
	}}
	// The two pools Cursor reports beside the total are the meters a plan
	// actually draws down, so they travel as extra windows when present.
	if payload.PlanUsage.AutoPercentUsed != nil {
		windows = append(windows, activity.WindowSample{Window: "auto", UsedPercent: clampPercent(*payload.PlanUsage.AutoPercentUsed), Seconds: monthlySeconds})
	}
	if payload.PlanUsage.APIPercentUsed != nil {
		windows = append(windows, activity.WindowSample{Window: "api", UsedPercent: clampPercent(*payload.PlanUsage.APIPercentUsed), Seconds: monthlySeconds})
	}
	return windows, nil
}

func cursorUsed(plan *cursorPlanUsage) (float64, bool) {
	if plan.TotalPercentUsed != nil {
		return *plan.TotalPercentUsed, true
	}
	limit := pickFloat(plan.Limit, plan.LimitCents)
	remaining := pickFloat(plan.Remaining, plan.RemainingCents)
	if limit != nil && *limit > 0 && remaining != nil {
		return clampPercent(((*limit - *remaining) / *limit) * 100), true
	}
	return 0, false
}

// CopilotProber reads the monthly credits of a GitHub Copilot seat.
type CopilotProber struct {
	client   *http.Client
	endpoint string
}

// NewCopilotProber returns the quota prober for a signed-in Copilot account.
func NewCopilotProber(options SignInProberOptions) *CopilotProber {
	settings := options
	if settings.HTTPClient == nil {
		settings.HTTPClient = &http.Client{Timeout: probeTimeout}
	}
	return &CopilotProber{client: settings.HTTPClient, endpoint: firstNonEmpty(settings.Endpoint, copilotUserEndpoint)}
}

// Probe returns the credit window one Copilot seat has left. The GitHub token
// behind the Copilot token is what this endpoint accepts, so an account with
// no refresh token answers unavailable rather than a wrong figure.
func (p *CopilotProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	token := credential.RefreshToken
	if token == "" {
		return nil, activity.ErrNoCredential
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build the quota probe: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", copilotUserAgent)
	request.Header.Set("Editor-Version", copilotEditorVersion)
	request.Header.Set("Editor-Plugin-Version", copilotPluginVersion)
	request.Header.Set("Authorization", "token "+token)
	body, err := readProbeBody(p.client, request, credential.ID)
	if err != nil {
		return nil, err
	}
	return copilotWindows(body, credential.ID)
}

type copilotQuotaSnapshot struct {
	Entitlement      *float64 `json:"entitlement"`
	Remaining        *float64 `json:"remaining"`
	PercentRemaining *float64 `json:"percent_remaining"`
	Unlimited        bool     `json:"unlimited"`
}

type copilotQuotaPayload struct {
	QuotaResetDate string                          `json:"quota_reset_date"`
	Snapshots      map[string]copilotQuotaSnapshot `json:"quota_snapshots"`
}

func copilotWindows(body []byte, credentialID string) ([]activity.WindowSample, error) {
	var payload copilotQuotaPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode the quota probe response: %w (%v)", activity.ErrProbeResponse, err)
	}
	snapshot, found := payload.Snapshots["premium_interactions"]
	if !found || snapshot.Unlimited {
		return nil, fmt.Errorf("%s: %w", credentialID, activity.ErrProbeResponse)
	}
	used, known := copilotUsed(snapshot)
	if !known {
		return nil, fmt.Errorf("%s: %w", credentialID, activity.ErrProbeResponse)
	}
	return []activity.WindowSample{{
		Window: "30d", UsedPercent: clampPercent(used),
		ResetAt: parseProbeTime(payload.QuotaResetDate).Unix(), Seconds: monthlySeconds,
	}}, nil
}

func copilotUsed(snapshot copilotQuotaSnapshot) (float64, bool) {
	switch {
	case snapshot.PercentRemaining != nil:
		return 100 - *snapshot.PercentRemaining, true
	case snapshot.Entitlement != nil && *snapshot.Entitlement > 0 && snapshot.Remaining != nil:
		return ((*snapshot.Entitlement - *snapshot.Remaining) / *snapshot.Entitlement) * 100, true
	default:
		return 0, false
	}
}

// KimiProber reads the weekly subscription window of a Kimi Code account.
type KimiProber struct {
	client *http.Client
}

// NewKimiProber returns the quota prober for a signed-in Kimi account.
func NewKimiProber(options SignInProberOptions) *KimiProber {
	settings := options
	if settings.HTTPClient == nil {
		settings.HTTPClient = &http.Client{Timeout: probeTimeout}
	}
	return &KimiProber{client: settings.HTTPClient}
}

// Probe returns the window one Kimi account has left, read from the
// connection's own base URL.
func (p *KimiProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	base := strings.TrimSuffix(strings.TrimSpace(credential.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("%s: %w", credential.ID, activity.ErrProbeResponse)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+kimiUsagePath, nil)
	if err != nil {
		return nil, fmt.Errorf("build the quota probe: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", probeUserAgent)
	request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	body, err := readProbeBody(p.client, request, credential.ID)
	if err != nil {
		return nil, err
	}
	return kimiWindows(body, credential.ID)
}

type kimiQuotaPayload struct {
	Data  json.RawMessage `json:"data"`
	Usage *kimiRow        `json:"usage"`
	Total *kimiRow        `json:"totalQuota"`
}

func kimiWindows(body []byte, credentialID string) ([]activity.WindowSample, error) {
	var payload kimiQuotaPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode the quota probe response: %w (%v)", activity.ErrProbeResponse, err)
	}
	usage := payload.Usage
	if usage == nil && len(payload.Data) > 0 {
		var nested struct {
			Usage *kimiRow `json:"usage"`
		}
		if err := json.Unmarshal(payload.Data, &nested); err == nil {
			usage = nested.Usage
		}
	}
	percent, found := usage.percent()
	if !found {
		return nil, fmt.Errorf("%s: %w", credentialID, activity.ErrProbeResponse)
	}
	return []activity.WindowSample{{
		Window: windowName(weeklySeconds), UsedPercent: clampPercent(percent),
		ResetAt: usage.resetAt(), Seconds: weeklySeconds,
	}}, nil
}

type kimiRow struct {
	Limit       *float64 `json:"limit"`
	Used        *float64 `json:"used"`
	Remaining   *float64 `json:"remaining"`
	Utilization *float64 `json:"utilization"`
	Percent     *float64 `json:"percent"`
	UsedPercent *float64 `json:"usedPercent"`
	ResetTime   string   `json:"resetTime"`
}

func (r *kimiRow) percent() (float64, bool) {
	if r == nil {
		return 0, false
	}
	if r.Limit != nil && *r.Limit > 0 {
		used := r.Used
		if used == nil && r.Remaining != nil {
			derived := *r.Limit - *r.Remaining
			used = &derived
		}
		if used != nil {
			return (*used / *r.Limit) * 100, true
		}
	}
	for _, direct := range []*float64{r.Utilization, r.Percent, r.UsedPercent} {
		if direct != nil {
			return *direct, true
		}
	}
	return 0, false
}

func (r *kimiRow) resetAt() int64 {
	if r == nil {
		return 0
	}
	return parseProbeTime(r.ResetTime).Unix()
}

func readProbeBody(client *http.Client, request *http.Request, credentialID string) ([]byte, error) {
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("probe the quota of %s: %w", credentialID, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("probe the quota of %s: %w", credentialID, activity.ErrProbeRejected)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("probe the quota of %s: %w (%d)", credentialID, activity.ErrProbeResponse, response.StatusCode)
	}
	if err := contentencoding.Decode(response); err != nil {
		return nil, fmt.Errorf("decode the quota probe response: %w", err)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, probeBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("read the quota probe response: %w", err)
	}
	return raw, nil
}

func probeReset(raw string) int64 {
	parsed := parseProbeTime(raw)
	if parsed.IsZero() {
		return 0
	}
	return parsed.Unix()
}

func parseProbeTime(raw string) time.Time {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func pickFloat(values ...*float64) *float64 {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
