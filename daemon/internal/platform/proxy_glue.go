// Proxy pool glue: the settings summary adapter and the connectivity check.
package platform

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/secrets"
	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	appstatus "github.com/jonaskahn/relo/internal/application/status"
	"github.com/jonaskahn/relo/internal/clock"
	"github.com/jonaskahn/relo/internal/config"
	appproxy "github.com/jonaskahn/relo/internal/proxy"
)

// ProxyPoolSummary adapts the pool to the settings card contract.
type ProxyPoolSummary struct{ Pool *appproxy.Service }

// ProxySecrets adapts the vault to the proxy pool's string-mode contract.
type ProxySecrets struct{ secrets.SecretStore }

// Mode names the secret backend as a plain string.
func (p ProxySecrets) Mode() string { return string(p.SecretStore.Mode()) }

// NewProxySecrets wraps a secret store for the proxy pool.
func NewProxySecrets(store secrets.SecretStore) appproxy.SecretStore {
	if store == nil {
		return nil
	}
	return ProxySecrets{store}
}

// ProxyPoolSummary reports the pool counts the providers card shows.
func (p ProxyPoolSummary) ProxyPoolSummary(ctx context.Context) (appsettings.ProxyPoolSummary, error) {
	summary, err := p.Pool.Summary(ctx)
	if err != nil {
		return appsettings.ProxyPoolSummary{}, err
	}
	return appsettings.ProxyPoolSummary{
		Total: summary.Total, Enabled: summary.Enabled, Cooling: summary.Cooling, AllCooling: summary.AllCooling,
	}, nil
}

// ProxyPoolStatus adapts the pool to the doctor's read-only check.
type ProxyPoolStatus struct{ Pool *appproxy.Service }

// ProxyPoolSummary reports the pool counts the doctor reads.
func (p ProxyPoolStatus) ProxyPoolSummary(ctx context.Context) (appstatus.ProxyPoolSummary, error) {
	summary, err := p.Pool.Summary(ctx)
	if err != nil {
		return appstatus.ProxyPoolSummary{}, err
	}
	return appstatus.ProxyPoolSummary{
		Total: summary.Total, Enabled: summary.Enabled, Cooling: summary.Cooling, AllCooling: summary.AllCooling,
	}, nil
}

// CheckProxy opens one TLS request through rawURL and requires a non-5xx
// answer, which is enough to tell a reachable proxy from a dead one.
func CheckProxy(ctx context.Context, rawURL string) error {
	proxyURL, err := config.NormalizeProxyURL(rawURL)
	if err != nil {
		return err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(mustParseURL(proxyURL))
	transport.TLSHandshakeTimeout = 10 * time.Second
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://example.com/", nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("proxy test answered %d", response.StatusCode)
	}
	return nil
}

func newProxyPool(db proxyStore, home string, store secrets.SecretStore, clk clock.Clock) *appproxy.Service {
	return appproxy.New(appproxy.Options{
		Store: db, Secrets: NewProxySecrets(store), Clock: clk,
		Backoff: func() []time.Duration {
			seconds, err := config.ReadUpstreamFailoverCooldown(config.ConfigPath(home))
			if err != nil {
				return config.FailoverBackoff(0)
			}
			return config.FailoverBackoff(seconds)
		},
		Check: CheckProxy,
	})
}

func mustParseURL(raw string) *url.URL {
	parsed, _ := url.Parse(raw)
	return parsed
}

// proxyStore is the storage half a pool needs; sqlite.ProxyStore satisfies it.
type proxyStore interface {
	List(ctx context.Context) ([]appproxy.Entry, error)
	Get(ctx context.Context, id string) (appproxy.Entry, bool, error)
	Create(ctx context.Context, entry appproxy.Entry) (appproxy.Entry, error)
	Update(ctx context.Context, entry appproxy.Entry) error
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int, error)
}
