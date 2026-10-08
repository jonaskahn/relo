// Balance HTTP: the shared GET behind balance probers.
package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jonaskahn/relo/internal/activity"
)

const (
	balanceWindow = "balance"
	spentWindow   = "spent"
)

type balanceHTTP struct {
	client   *http.Client
	endpoint string
}

func newBalanceHTTP(options ProberOptions) balanceHTTP {
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: probeTimeout}
	}
	return balanceHTTP{client: client, endpoint: strings.TrimSpace(options.Endpoint)}
}

func (h balanceHTTP) get(ctx context.Context, endpoint string, credential activity.Credential) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build the quota probe: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	return readProbeBody(h.client, request, credential.ID)
}

func probeEndpoint(override, baseURL, fallback, path string) string {
	if override != "" {
		return override
	}
	base := endpointOrigin(baseURL)
	if base == "" {
		base = fallback
	}
	return strings.TrimSuffix(base, "/") + path
}

func endpointOrigin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func money(window string, amount float64, currency string) (activity.WindowSample, bool) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return activity.WindowSample{}, false
	}
	return activity.WindowSample{
		Window: window, Amount: new(amount), Currency: strings.ToUpper(strings.TrimSpace(currency)),
	}, true
}

func moneyString(window, amount, currency string) (activity.WindowSample, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(amount), 64)
	if err != nil {
		return activity.WindowSample{}, false
	}
	return money(window, value, currency)
}

func decodeBalance(body []byte, target any) error {
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode the quota probe response: %w (%v)", activity.ErrProbeResponse, err)
	}
	return nil
}

func requireBalance(credentialID string, samples []activity.WindowSample) ([]activity.WindowSample, error) {
	if len(samples) == 0 {
		return nil, fmt.Errorf("%s: %w", credentialID, activity.ErrProbeResponse)
	}
	return samples, nil
}

func balanceCurrency(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err == nil && strings.HasSuffix(strings.ToLower(parsed.Hostname()), ".cn") {
		return "CNY"
	}
	return "USD"
}
