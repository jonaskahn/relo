// Muse quota reads Meta subscription windows from its key endpoint.
package muse

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/contentencoding"
)

const (
	failureBackoff = 5 * time.Minute
	weeklySeconds  = int64(7 * 24 * 60 * 60)
	bodyLimit      = 1 << 20
	userAgent      = "relo"
)

// QuotaProber reads the rolling and weekly windows Meta publishes for a
// Muse subscription. It answers only from vendor-reported values.
type QuotaProber struct {
	client   *http.Client
	endpoint string
	now      func() time.Time
	mu       sync.Mutex
	failed   map[string]time.Time
}

// QuotaProberOptions tunes the prober for an endpoint or clock.
type QuotaProberOptions struct {
	HTTPClient *http.Client
	Endpoint   string
	Now        func() time.Time
}

// NewQuotaProber returns a Muse subscription quota prober.
func NewQuotaProber(options QuotaProberOptions) *QuotaProber {
	settings := options
	if settings.HTTPClient == nil {
		settings.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if settings.Now == nil {
		settings.Now = time.Now
	}
	return &QuotaProber{
		client: settings.HTTPClient, endpoint: firstNonEmpty(settings.Endpoint, KeyEndpoint),
		now: settings.Now, failed: map[string]time.Time{},
	}
}

// Probe returns the usage windows one Muse subscription reported. A failed
// probe leaves the last reading alone for five minutes; a refused token asks
// for a new login instead.
func (p *QuotaProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	account := strings.TrimSpace(credential.Extra[AccessTokenExtraKey])
	if account == "" {
		return nil, activity.ErrNoCredential
	}
	if p.backedOff(credential.ID) {
		return nil, fmt.Errorf("%s: %w", credential.ID, activity.ErrProbeResponse)
	}
	response, err := p.fetch(ctx, account)
	if err != nil {
		p.noteFailure(credential.ID, err)
		return nil, err
	}
	p.clearFailure(credential.ID)
	if err := validateResponse(response); err != nil {
		return nil, fmt.Errorf("%s: %w", credential.ID, err)
	}
	samples := windows(response)
	if len(samples) == 0 {
		return nil, fmt.Errorf("%s: %w", credential.ID, activity.ErrProbeResponse)
	}
	return samples, nil
}

func (p *QuotaProber) fetch(ctx context.Context, account string) (KeyResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		return KeyResponse{}, fmt.Errorf("build the Muse quota probe: %w", err)
	}
	setProbeHeaders(request, account)
	response, err := p.client.Do(request)
	if err != nil {
		return KeyResponse{}, fmt.Errorf("probe Muse quota: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return KeyResponse{}, fmt.Errorf("probe Muse quota: %w", activity.ErrProbeRejected)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return KeyResponse{}, fmt.Errorf("probe Muse quota: %w (%d)", activity.ErrProbeResponse, response.StatusCode)
	}
	if err := contentencoding.Decode(response); err != nil {
		return KeyResponse{}, fmt.Errorf("decode the Muse quota response: %w", err)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, bodyLimit))
	if err != nil {
		return KeyResponse{}, fmt.Errorf("read the Muse quota response: %w", err)
	}
	return Decode(raw)
}

func setProbeHeaders(request *http.Request, account string) {
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Authorization", "Bearer "+account)
	for name, value := range Headers() {
		request.Header.Set(name, value)
	}
}

func validateResponse(response KeyResponse) error {
	switch {
	case response.Inactive():
		return ErrSubscriptionInactive
	case response.RequirePayment || response.PaymentURL() != "":
		return ErrPaymentRequired
	case response.SubsUsage == nil:
		return fmt.Errorf("%w: no usage reported", activity.ErrProbeResponse)
	default:
		return nil
	}
}

func windows(response KeyResponse) []activity.WindowSample {
	var samples []activity.WindowSample
	if response.SubsUsage == nil {
		return nil
	}
	if response.SubsUsage.Window != nil {
		if sample, ok := window(response.SubsUsage.Window, response.SubsUsage.Window.Seconds()); ok {
			samples = append(samples, sample)
		}
	}
	if sample, ok := window(response.SubsUsage.Weekly, weeklySeconds); ok {
		samples = append(samples, sample)
	}
	return samples
}

func window(value *Window, seconds int64) (activity.WindowSample, bool) {
	if value == nil || value.UsedPercent == nil {
		return activity.WindowSample{}, false
	}
	return activity.WindowSample{
		Window: windowName(seconds), UsedPercent: clampPercent(*value.UsedPercent),
		ResetAt: value.ResetAt(), Seconds: seconds,
	}, true
}

func windowName(seconds int64) string {
	if seconds == weeklySeconds {
		return activity.WindowSevenDays
	}
	if seconds > 0 && seconds%(60*60) == 0 {
		return fmt.Sprintf("%dh", seconds/(60*60))
	}
	return fmt.Sprintf("%dm", seconds/60)
}

func clampPercent(value float64) float64 {
	return min(max(value, 0), 100)
}

func (p *QuotaProber) backedOff(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.now().Before(p.failed[id])
}

func (p *QuotaProber) noteFailure(id string, err error) {
	if !errors.Is(err, activity.ErrProbeRejected) {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failed[id] = p.now().Add(failureBackoff)
}

func (p *QuotaProber) clearFailure(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.failed, id)
}
