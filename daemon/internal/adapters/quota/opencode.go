// OpenCode quota prober.
package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/jonaskahn/relo/internal/activity"
)

const (
	openCodeGoStatusEndpoint = "https://opencode.ai/console/api/go/status"
	openCodeMonthSeconds     = int64(30 * 24 * 60 * 60)
)

// OpenCodeGoProber reads the session, weekly, and monthly windows OpenCode Go
// publishes on the console status API.
type OpenCodeGoProber struct {
	client   *http.Client
	endpoint string
}

// NewOpenCodeGoProber returns the quota prober for an OpenCode Go API key.
// Endpoint overrides the status URL, which a test points at itself.
func NewOpenCodeGoProber(options ProberOptions) *OpenCodeGoProber {
	settings := options
	if settings.HTTPClient == nil {
		settings.HTTPClient = &http.Client{Timeout: probeTimeout}
	}
	return &OpenCodeGoProber{
		client:   settings.HTTPClient,
		endpoint: strings.TrimSpace(settings.Endpoint),
	}
}

// Probe returns the three windows one OpenCode Go key has used. A missing
// subscription is not a refused login: the key is still the one the operator
// stored.
func (p *OpenCodeGoProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.statusURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("build the quota probe: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	body, status, err := readProbeStatus(p.client, request, credential.ID)
	if err != nil {
		return nil, err
	}
	if err := openCodeRefusal(credential.ID, status); err != nil {
		return nil, err
	}
	return openCodeWindows(body)
}

func (p *OpenCodeGoProber) statusURL() string {
	if p.endpoint != "" {
		return p.endpoint
	}
	return openCodeGoStatusEndpoint
}

func openCodeRefusal(credentialID string, status int) error {
	if status == http.StatusUnauthorized {
		return fmt.Errorf("probe the quota of %s: %w", credentialID, activity.ErrProbeRejected)
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("probe the quota of %s: %w (%d)", credentialID, activity.ErrProbeResponse, status)
	}
	return nil
}

// ErrEmptyAmount names why a reported balance cannot be read: an amount
// the endpoint left empty.
var ErrEmptyAmount = errors.New("empty amount")

type openCodeAmount float64

// UnmarshalJSON reads an amount the quota endpoint states as a number or a
// quoted string, so both spellings settle the same balance.
func (amount *openCodeAmount) UnmarshalJSON(raw []byte) error {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if text == "" || text == "null" {
		return ErrEmptyAmount
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return err
	}
	*amount = openCodeAmount(value)
	return nil
}

type openCodeAccess struct {
	EndsAt string                   `json:"endsAt"`
	Meters map[string]openCodeMeter `json:"meters"`
}

type openCodeMeter struct {
	Used     *openCodeAmount `json:"usedMicroCents"`
	Limit    *openCodeAmount `json:"limitMicroCents"`
	ResetsAt string          `json:"resetsAt"`
}

type openCodeWindowOrder struct {
	key       string
	window    string
	seconds   int64
	periodEnd bool
}

func openCodeWindows(body []byte) ([]activity.WindowSample, error) {
	var payload struct {
		Access *openCodeAccess `json:"access"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode the quota probe response: %w (%v)", activity.ErrProbeResponse, err)
	}
	if payload.Access == nil || payload.Access.Meters == nil {
		return nil, fmt.Errorf("access.meters: %w", activity.ErrProbeResponse)
	}
	return openCodeMeterWindows(payload.Access)
}

func openCodeMeterWindows(access *openCodeAccess) ([]activity.WindowSample, error) {
	order := []openCodeWindowOrder{
		{"fiveHour", activity.WindowFiveHours, 5 * 60 * 60, false},
		{"week", activity.WindowSevenDays, weeklySeconds, false},
		{"month", activity.WindowThirtyDays, openCodeMonthSeconds, true},
	}
	windows := make([]activity.WindowSample, 0, len(order))
	for _, item := range order {
		window, err := openCodeMeterWindow(access, item)
		if err != nil {
			return nil, err
		}
		windows = append(windows, window)
	}
	return windows, nil
}

func openCodeMeterWindow(access *openCodeAccess, item openCodeWindowOrder) (activity.WindowSample, error) {
	meter, found := access.Meters[item.key]
	if !found || meter.Used == nil || meter.Limit == nil {
		return activity.WindowSample{}, fmt.Errorf("%s: %w", item.key, activity.ErrProbeResponse)
	}
	resetAt := meter.ResetsAt
	if item.periodEnd && parseProbeTime(resetAt).IsZero() {
		resetAt = access.EndsAt
	}
	return activity.WindowSample{
		Window: item.window, UsedPercent: openCodePercent(*meter.Used, *meter.Limit),
		ResetAt: probeReset(resetAt), Seconds: item.seconds,
	}, nil
}

func openCodePercent(used, limit openCodeAmount) float64 {
	if limit <= 0 {
		return 0
	}
	return clampPercent(float64(used) / float64(limit) * 100)
}

func readProbeStatus(client *http.Client, request *http.Request, credentialID string) ([]byte, int, error) {
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("probe the quota of %s: %w", credentialID, err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, probeBodyLimit))
	if err != nil {
		return nil, response.StatusCode, fmt.Errorf("read the quota probe response: %w", err)
	}
	return raw, response.StatusCode, nil
}
