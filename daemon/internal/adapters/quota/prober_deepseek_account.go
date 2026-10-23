// DeepSeek account quota prober.
package quota

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/activity"
)

const (
	// deepSeekPlatformOrigin answers the account summary. It is the platform
	// host, not the inference host the connection sends its requests to.
	deepSeekPlatformOrigin = "https://platform.deepseek.com"
	deepSeekSummaryPath    = "/api/v0/users/get_user_summary"
	// deepSeekAccountAuthHeader carries the imported harness grant.
	deepSeekAccountAuthHeader = "x-dsh-auth-token"
)

// DeepSeekAccountProber reports whether a DeepSeek account grant is still
// usable. The platform publishes wallet amounts rather than rate-limit
// windows, so a successful probe confirms the credential and reports no
// window: a made-up percentage would rank an empty wallet as healthy.
type DeepSeekAccountProber struct {
	client   *http.Client
	endpoint string
}

// NewDeepSeekAccountProber returns the prober for an imported DeepSeek
// account grant.
func NewDeepSeekAccountProber(options ProberOptions) *DeepSeekAccountProber {
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: probeTimeout}
	}
	endpoint := strings.TrimSpace(options.Endpoint)
	if endpoint == "" {
		endpoint = deepSeekPlatformOrigin + deepSeekSummaryPath
	}
	return &DeepSeekAccountProber{client: client, endpoint: endpoint}
}

// Probe asks the platform about one account. A refused grant is reported as
// rejected so the account is marked for a new import; a wallet reading
// returns no window.
func (p *DeepSeekAccountProber) Probe(ctx context.Context, credential activity.Credential) ([]activity.WindowSample, error) {
	if credential.AccessToken == "" {
		return nil, activity.ErrNoCredential
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build the DeepSeek account probe: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set(deepSeekAccountAuthHeader, credential.AccessToken)
	if _, err := readProbeBody(p.client, request, credential.ID); err != nil {
		return nil, err
	}
	return nil, nil
}
