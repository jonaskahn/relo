// Quota prober options and the Antigravity prober.
package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/antigravity"
)

const (
	// antigravityBaseDefault is the host Cloud Code Assist answers the quota
	// and listing calls on when a connection names no base URL.
	antigravityBaseDefault = "https://daily-cloudcode-pa.googleapis.com"
	// antigravitySummaryPath reports the account's two pools with both their
	// windows, which is the only call that knows the weekly ones.
	antigravitySummaryPath = "/v1internal:retrieveUserQuotaSummary"
	// antigravityModelsPath reports the per-model quotas, whose session
	// windows collapse into the same two pools.
	antigravityModelsPath = "/v1internal:fetchAvailableModels"
	// antigravityProjectExtra is the credential extra the sign-in stores the
	// Cloud Code Assist billing project under.
	antigravityProjectExtra = "projectId"
	// The pool names and the window lengths Antigravity reports.
	antigravityGemWindow    = "Gem"
	antigravityClaWindow    = "Cla"
	antigravityWeeklySuffix = " (Weekly)"
	antigravitySession      = int64(5 * 60 * 60)
	antigravityWeekly       = int64(7 * 24 * 60 * 60)
	probeBodyLimit          = 1 << 21
	refusalBodyLimit        = 64 << 10
	probeTimeout            = 30 * time.Second
	probeUserAgent          = "relo"
)

// ProberOptions tunes a provider prober.
type ProberOptions struct {
	HTTPClient *http.Client
	// Endpoint overrides the one full URL a prober asks, which is the Codex
	// usage call.
	Endpoint string
	// BaseURL is the host a Cloud Code Assist account answers a quota probe
	// on, which is the connection's own base URL. An empty value uses the
	// vendor's default host.
	BaseURL string
	// Now stamps a reset time a provider reports as a delay rather than as
	// a moment.
	Now func() time.Time
}

// AntigravityProber reads the Cloud Code Assist quota windows of a Google
// Antigravity account.
type AntigravityProber struct {
	client *http.Client
	base   string
}

// NewAntigravityProber returns the quota prober for Google Antigravity.
func NewAntigravityProber(options ProberOptions) *AntigravityProber {
	settings := options
	if settings.HTTPClient == nil {
		settings.HTTPClient = &http.Client{Timeout: probeTimeout}
	}
	return &AntigravityProber{
		client: settings.HTTPClient,
		base:   firstNonEmpty(strings.TrimSuffix(strings.TrimSpace(settings.BaseURL), "/"), antigravityBaseDefault),
	}
}

// Probe returns the quota windows of one Antigravity account. The summary
// reports both pools with their session and weekly windows; the per-model
// listing still reports the session pools, so an account whose summary is not
// readable keeps a usable reading.
func (p *AntigravityProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	summary, err := p.summary(ctx, credential)
	switch {
	case errors.Is(err, activity.ErrProbeRejected):
		// The credential itself was refused, and the listing below would be
		// refused with it.
		return nil, err
	case err != nil:
		// The summary is a convenience rather than the only way in.
		summary = antigravityQuotaSummary{}
	}
	if windows := antigravitySummaryWindows(summary); len(windows) > 0 {
		return windows, nil
	}
	models, err := p.models(ctx, credential)
	if err != nil {
		return nil, err
	}
	windows := deriveAntigravityWindows(models, antigravitySession)
	if len(windows) == 0 {
		return nil, fmt.Errorf("%s: %w", credential.ID, activity.ErrProbeResponse)
	}
	return windows, nil
}

func (p *AntigravityProber) summary(ctx context.Context, credential activity.Credential) (antigravityQuotaSummary, error) {
	raw, err := p.post(ctx, credential, antigravitySummaryPath)
	if err != nil {
		return antigravityQuotaSummary{}, err
	}
	summary := antigravityQuotaSummary{}
	if err := json.Unmarshal(raw, &summary); err != nil {
		return antigravityQuotaSummary{}, fmt.Errorf("decode the quota summary of %s: %w", credential.ID, err)
	}
	return summary, nil
}

func (p *AntigravityProber) models(ctx context.Context, credential activity.Credential) (antigravityModels, error) {
	raw, err := p.post(ctx, credential, antigravityModelsPath)
	if err != nil {
		return antigravityModels{}, err
	}
	models := antigravityModels{}
	if err := json.Unmarshal(raw, &models); err != nil {
		return antigravityModels{}, fmt.Errorf("decode the quota probe response: %w", err)
	}
	return models, nil
}

func (p *AntigravityProber) post(ctx context.Context, credential activity.Credential, path string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base+path, strings.NewReader(antigravityProbeBody(credential)))
	if err != nil {
		return nil, fmt.Errorf("build the quota probe: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	// The probe reads the account's own quota, so it carries the client
	// fingerprint the credential was minted for rather than Relo's own name.
	request.Header.Set("User-Agent", antigravity.UserAgent())
	request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("probe the quota of %s: %w", credential.ID, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("probe the quota of %s: %w", credential.ID, activity.ErrProbeRejected)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		refusal, _ := io.ReadAll(io.LimitReader(response.Body, refusalBodyLimit))
		code, detail := antigravity.Refusal(refusal)
		return nil, fmt.Errorf("probe the quota of %s: %w: %s", credential.ID, activity.ErrProbeResponse, antigravity.Reason(response.StatusCode, code, detail))
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, probeBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("read the quota probe response: %w", err)
	}
	return raw, nil
}

func antigravityProbeBody(credential activity.Credential) string {
	project := credential.Extra[antigravityProjectExtra]
	if project == "" {
		return "{}"
	}
	// A map of strings cannot fail to encode.
	encoded, _ := json.Marshal(map[string]string{"project": project})
	return string(encoded)
}

type antigravityQuotaSummary struct {
	Groups   []antigravityQuotaGroup `json:"groups"`
	Response *antigravityQuotaGroups `json:"response"`
}

type antigravityQuotaGroups struct {
	Groups []antigravityQuotaGroup `json:"groups"`
}

type antigravityQuotaGroup struct {
	Buckets []json.RawMessage `json:"buckets"`
}

type antigravityQuotaBucket struct {
	BucketID          string   `json:"bucketId"`
	RemainingFraction *float64 `json:"remainingFraction"`
	ResetTime         string   `json:"resetTime"`
}

var antigravitySummaryBuckets = []struct {
	bucketID string
	window   string
	seconds  int64
}{
	{"gemini-5h", antigravityGemWindow, antigravitySession},
	{"gemini-weekly", antigravityGemWindow + antigravityWeeklySuffix, antigravityWeekly},
	{"3p-5h", antigravityClaWindow, antigravitySession},
	{"3p-weekly", antigravityClaWindow + antigravityWeeklySuffix, antigravityWeekly},
}

func antigravitySummaryWindows(summary antigravityQuotaSummary) []activity.WindowSample {
	pooled := poolAntigravityBuckets(summary)
	windows := make([]activity.WindowSample, 0, len(antigravitySummaryBuckets))
	for _, spec := range antigravitySummaryBuckets {
		bucket, found := pooled[spec.bucketID]
		if !found || bucket.RemainingFraction == nil {
			continue
		}
		windows = append(windows, activity.WindowSample{
			Window:      spec.window,
			UsedPercent: clampPercent(100 - *bucket.RemainingFraction*100),
			ResetAt:     resetTime(bucket.ResetTime),
			Seconds:     spec.seconds,
		})
	}
	return windows
}

func poolAntigravityBuckets(summary antigravityQuotaSummary) map[string]antigravityQuotaBucket {
	groups := summary.Groups
	if len(groups) == 0 && summary.Response != nil {
		groups = summary.Response.Groups
	}
	pooled := map[string]antigravityQuotaBucket{}
	for _, group := range groups {
		for _, raw := range group.Buckets {
			bucket := antigravityQuotaBucket{}
			if err := json.Unmarshal(raw, &bucket); err != nil || bucket.BucketID == "" {
				continue
			}
			if _, seen := pooled[bucket.BucketID]; !seen {
				pooled[bucket.BucketID] = bucket
			}
		}
	}
	return pooled
}

type antigravityModels struct {
	Models map[string]antigravityModel `json:"models"`
}

type antigravityModel struct {
	DisplayName     string                     `json:"displayName"`
	QuotaInfo       json.RawMessage            `json:"quotaInfo"`
	QuotaInfos      []json.RawMessage          `json:"quotaInfos"`
	QuotaInfoByTier map[string]json.RawMessage `json:"quotaInfoByTier"`
}

type antigravityQuota struct {
	RemainingFraction   *float64        `json:"remainingFraction"`
	RemainingPercentage *float64        `json:"remainingPercentage"`
	Remaining           json.RawMessage `json:"remaining"`
	ResetTime           string          `json:"resetTime"`
	Tier                string          `json:"tier"`
}

func deriveAntigravityWindows(models antigravityModels, seconds int64) []activity.WindowSample {
	windows := make([]activity.WindowSample, 0, 2)
	// Both loops walk a map, so both walk it in a stable order: the first
	// entry to claim a window label wins, and a random order would let one
	// probe of the same response report different windows than the next.
	for _, name := range sortedKeys(models.Models) {
		model := models.Models[name]
		entries := quotaEntries(model)
		for _, tier := range sortedKeys(entries) {
			quota := entries[tier]
			label := antigravityWindow(name, model.DisplayName, tier)
			if label == "" || hasWindow(windows, label) {
				continue
			}
			sample, ok := antigravitySample(label, quota, seconds)
			if ok {
				windows = append(windows, sample)
			}
		}
	}
	sort.Slice(windows, func(i, j int) bool { return windows[i].Window < windows[j].Window })
	return windows
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func quotaEntries(model antigravityModel) map[string]antigravityQuota {
	entries := map[string]antigravityQuota{}
	add := func(raw json.RawMessage, tier string) {
		if len(raw) == 0 {
			return
		}
		quota := antigravityQuota{}
		if err := json.Unmarshal(raw, &quota); err != nil {
			return
		}
		entries[firstNonEmpty(quota.Tier, tier)] = quota
	}
	add(model.QuotaInfo, "")
	for _, raw := range model.QuotaInfos {
		add(raw, "")
	}
	for tier, raw := range model.QuotaInfoByTier {
		add(raw, tier)
	}
	return entries
}

func antigravityWindow(modelID, displayName, tier string) string {
	haystack := strings.ToLower(modelID + " " + displayName + " " + tier)
	switch {
	case strings.Contains(haystack, "gemini"):
		return antigravityGemWindow
	case strings.Contains(haystack, "claude"), strings.Contains(haystack, "opus"),
		strings.Contains(haystack, "sonnet"), strings.Contains(haystack, "gpt-oss"),
		strings.Contains(haystack, "gpt_oss"):
		return antigravityClaWindow
	default:
		return ""
	}
}

func antigravitySample(label string, quota antigravityQuota, seconds int64) (activity.WindowSample, bool) {
	remaining, ok := remainingPercent(quota)
	if !ok {
		return activity.WindowSample{}, false
	}
	return activity.WindowSample{Window: label, UsedPercent: 100 - remaining, ResetAt: resetTime(quota.ResetTime), Seconds: seconds}, true
}

func remainingPercent(quota antigravityQuota) (float64, bool) {
	if quota.RemainingFraction != nil {
		return *quota.RemainingFraction * 100, true
	}
	if quota.RemainingPercentage != nil {
		return *quota.RemainingPercentage, true
	}
	nested := antigravityQuota{}
	if err := json.Unmarshal(quota.Remaining, &nested); err != nil {
		return 0, false
	}
	return remainingPercent(nested)
}

func resetTime(raw string) int64 {
	when, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return 0
	}
	return when.Unix()
}

func hasWindow(windows []activity.WindowSample, label string) bool {
	for _, window := range windows {
		if window.Window == label {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

const codexUsageEndpoint = "https://chatgpt.com/backend-api/wham/usage"

const codexAccountHeader = "chatgpt-account-id"

// CodexProber reads the rate-limit windows of a Codex (ChatGPT) account.
type CodexProber struct {
	client   *http.Client
	endpoint string
	now      func() time.Time
}

// NewCodexProber returns the quota prober for Codex.
func NewCodexProber(options ProberOptions) *CodexProber {
	settings := options
	if settings.HTTPClient == nil {
		settings.HTTPClient = &http.Client{Timeout: probeTimeout}
	}
	if settings.Now == nil {
		settings.Now = time.Now
	}
	return &CodexProber{
		client:   settings.HTTPClient,
		endpoint: firstNonEmpty(settings.Endpoint, codexUsageEndpoint),
		now:      settings.Now,
	}
}

// Probe returns the windows one Codex account has left.
func (p *CodexProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	response, err := p.fetch(ctx, credential)
	if err != nil {
		return nil, err
	}
	windows := response.windows(p.now())
	if len(windows) == 0 {
		return nil, fmt.Errorf("%s: %w", credential.ID, activity.ErrProbeResponse)
	}
	return windows, nil
}

func (p *CodexProber) fetch(ctx context.Context, credential activity.Credential) (codexUsage, error) {
	request, err := codexProbeRequest(ctx, p.endpoint, credential)
	if err != nil {
		return codexUsage{}, err
	}
	response, err := p.client.Do(request)
	if err != nil {
		return codexUsage{}, fmt.Errorf("probe the quota of %s: %w", credential.ID, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return codexUsage{}, fmt.Errorf("probe the quota of %s: %w", credential.ID, activity.ErrProbeRejected)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return codexUsage{}, fmt.Errorf("probe the quota of %s: %w", credential.ID, activity.ErrProbeResponse)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, probeBodyLimit))
	if err != nil {
		return codexUsage{}, fmt.Errorf("read the quota probe response: %w", err)
	}
	return decodeCodexUsage(raw)
}

func codexProbeRequest(ctx context.Context, endpoint string, credential activity.Credential) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build the quota probe: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", probeUserAgent)
	request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	if account := credential.Extra["account_id"]; account != "" {
		request.Header.Set(codexAccountHeader, account)
	}
	return request, nil
}

func decodeCodexUsage(raw []byte) (codexUsage, error) {
	usage := codexUsage{}
	if err := json.Unmarshal(raw, &usage); err != nil {
		return codexUsage{}, fmt.Errorf("decode the quota probe response: %w (%v)", activity.ErrProbeResponse, err)
	}
	return usage, nil
}

type codexUsage struct {
	RateLimit struct {
		PrimaryWindow   *codexWindow `json:"primary_window"`
		SecondaryWindow *codexWindow `json:"secondary_window"`
	} `json:"rate_limit"`
}

type codexWindow struct {
	UsedPercent        *float64 `json:"used_percent"`
	LimitWindowSeconds *int64   `json:"limit_window_seconds"`
	ResetAfterSeconds  *int64   `json:"reset_after_seconds"`
}

func (u codexUsage) windows(now time.Time) []activity.WindowSample {
	reported := []*codexWindow{u.RateLimit.PrimaryWindow, u.RateLimit.SecondaryWindow}
	windows := make([]activity.WindowSample, 0, len(reported))
	for _, window := range reported {
		if window == nil || window.UsedPercent == nil {
			continue
		}
		seconds := int64(0)
		if window.LimitWindowSeconds != nil {
			seconds = *window.LimitWindowSeconds
		}
		reset := int64(0)
		if window.ResetAfterSeconds != nil {
			reset = now.Add(time.Duration(*window.ResetAfterSeconds) * time.Second).Unix()
		}
		windows = append(windows, activity.WindowSample{
			Window: windowName(seconds), UsedPercent: clampPercent(*window.UsedPercent),
			ResetAt: reset, Seconds: seconds,
		})
	}
	return windows
}
