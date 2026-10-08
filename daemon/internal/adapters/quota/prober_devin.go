// Devin quota prober for sign-in credentials.
package quota

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/activity"
)

const (
	devinDefaultServer = "https://server.codeium.com"
	devinStatusPath    = "/exa.seat_management_pb.SeatManagementService/GetUserStatus"
	devinCompatVersion = "1.108.2"
	devinDailyWindow   = "daily"
)

// DevinProber reads the daily and weekly windows of a Devin seat.
type DevinProber struct {
	client   *http.Client
	endpoint string
}

// NewDevinProber returns the quota prober for a signed-in Devin account.
func NewDevinProber(options SignInProberOptions) *DevinProber {
	settings := options
	if settings.HTTPClient == nil {
		settings.HTTPClient = &http.Client{Timeout: probeTimeout}
	}
	return &DevinProber{client: settings.HTTPClient, endpoint: strings.TrimSpace(settings.Endpoint)}
}

// Probe returns the windows one Devin seat has used. Remaining percentages
// are flipped to used, which is the shape every other prober stores.
func (p *DevinProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	request, err := devinProbeRequest(ctx, p.statusURL(credential), credential.AccessToken)
	if err != nil {
		return nil, err
	}
	body, err := readProbeBody(p.client, request, credential.ID)
	if err != nil {
		return nil, err
	}
	windows, err := devinWindows(body)
	if err != nil {
		return nil, err
	}
	if len(windows) == 0 {
		return nil, fmt.Errorf("%s: %w", credential.ID, activity.ErrProbeResponse)
	}
	return windows, nil
}

func devinProbeRequest(ctx context.Context, statusURL, accessToken string) (*http.Request, error) {
	payload, _ := json.Marshal(map[string]any{
		"metadata": map[string]string{
			"apiKey": accessToken, "ideName": "devin",
			"ideVersion": devinCompatVersion, "extensionName": "devin",
			"extensionVersion": devinCompatVersion, "locale": "en",
		},
	})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, statusURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build the quota probe: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")
	return request, nil
}

func (p *DevinProber) statusURL(credential activity.Credential) string {
	if p.endpoint != "" {
		return p.endpoint
	}
	base := firstNonEmpty(credential.Extra["api_server_url"], credential.BaseURL, devinDefaultServer)
	return strings.TrimSuffix(base, "/") + devinStatusPath
}

type devinStatus struct {
	UserStatus struct {
		PlanStatus struct {
			PlanInfo struct {
				HideDailyQuota bool `json:"hideDailyQuota"`
			} `json:"planInfo"`
			DailyRemaining  *float64 `json:"dailyQuotaRemainingPercent"`
			WeeklyRemaining *float64 `json:"weeklyQuotaRemainingPercent"`
			DailyReset      *int64   `json:"dailyQuotaResetAtUnix"`
			WeeklyReset     *int64   `json:"weeklyQuotaResetAtUnix"`
		} `json:"planStatus"`
	} `json:"userStatus"`
}

func devinWindows(body []byte) ([]activity.WindowSample, error) {
	status := devinStatus{}
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("decode the quota probe response: %w (%v)", activity.ErrProbeResponse, err)
	}
	plan := status.UserStatus.PlanStatus
	windows := make([]activity.WindowSample, 0, 2)
	if !plan.PlanInfo.HideDailyQuota && plan.DailyRemaining != nil {
		windows = append(windows, activity.WindowSample{
			Window: devinDailyWindow, UsedPercent: clampPercent(100 - *plan.DailyRemaining),
			ResetAt: unixOrZero(plan.DailyReset), Seconds: dailySeconds,
		})
	}
	weekly := plan.WeeklyRemaining
	if weekly == nil && plan.WeeklyReset != nil {
		exhausted := 0.0
		weekly = &exhausted
	}
	if weekly != nil {
		windows = append(windows, activity.WindowSample{
			Window: activity.WindowSevenDays, UsedPercent: clampPercent(100 - *weekly),
			ResetAt: unixOrZero(plan.WeeklyReset), Seconds: weeklySeconds,
		})
	}
	return windows, nil
}

func unixOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
