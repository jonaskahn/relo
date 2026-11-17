package platform

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/secrets"
	"github.com/jonaskahn/relo/internal/clock"
	"github.com/jonaskahn/relo/internal/config"
	appproxy "github.com/jonaskahn/relo/internal/proxy"
)

// writeFailoverCooldown seeds the startup file with the failover ladder a
// cooling proxy walks, which is the only thing the pool's backoff reads.
func writeFailoverCooldown(t *testing.T, home string, seconds int) {
	t.Helper()
	path := config.ConfigPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create the config directory: %v", err)
	}
	body := "[upstream]\nfailover_cooldown_seconds = " + ladderItoa(seconds) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("seed the startup file: %v", err)
	}
}

func ladderItoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

// TestNewProxyPoolReadsTheConfiguredLadder covers both branches of the backoff
// the daemon gives the pool: a startup file naming a cooldown, and one that
// cannot be read, which falls back to the default ladder.
func TestNewProxyPoolReadsTheConfiguredLadder(t *testing.T) {
	ctx := context.Background()
	for _, testCase := range []struct {
		name    string
		seconds int
		seed    bool
		want    time.Duration
	}{
		{"a configured cooldown", 900, true, 900 * time.Second},
		// An unreadable file answers the default cooldown, whose first step is
		// no wait, so the very first failure is not held back.
		{"a missing startup file", 0, false, 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			home := t.TempDir()
			if testCase.seed {
				writeFailoverCooldown(t, home, testCase.seconds)
			}
			pool := newProxyPool(newPoolStore(), home, &modeSecrets{mode: secrets.ModeEncryptedFile}, clock.New())
			item, err := pool.Create(ctx, appproxy.Draft{
				Scheme: "http", Host: "127.0.0.1", Port: 7890, Enabled: true,
			})
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			wait := pool.Cool(item.ID, 0)
			if wait != testCase.want {
				t.Fatalf("Cool() = %v, want %v", wait, testCase.want)
			}
			// The second failure walks one step further up the same ladder.
			if again := pool.Cool(item.ID, 0); again <= wait && wait > 0 {
				t.Fatalf("second Cool() = %v, want a longer wait than %v", again, wait)
			}
		})
	}
}

// TestNormalizeProxyURLShorthands covers every accepted shape and the ones the
// normalizer refuses, including a host:port pair whose host is not an address.
func TestNormalizeProxyURLShorthands(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1:7890",
		"user:pass@proxy.internal:7890",
		"127.0.0.1:7890",
		"127.0.0.1:7890:user:pass",
	} {
		normalized, err := config.NormalizeProxyURL(raw)
		if err != nil || normalized == "" {
			t.Fatalf("NormalizeProxyURL(%q) = %q, %v", raw, normalized, err)
		}
	}
	// A pair or quartet whose first field is a name is not a documented
	// shorthand, so it is refused rather than guessed at.
	for _, raw := range []string{"proxy.internal:7890", "proxy.internal:7890:user:pass"} {
		if _, err := config.NormalizeProxyURL(raw); err == nil {
			t.Fatalf("NormalizeProxyURL(%q) error = nil, want a refusal", raw)
		}
	}
}

// TestRequireProxyMarksTheContext covers the marker a forced connection sets,
// which tells the relay not to fall back to a direct connection.
func TestRequireProxyMarksTheContext(t *testing.T) {
	ctx := context.Background()
	if config.ProxyRequired(ctx) {
		t.Fatal("ProxyRequired() on a plain context = true")
	}
	required := config.RequireProxy(ctx)
	if !config.ProxyRequired(required) {
		t.Fatal("ProxyRequired() on a marked context = false")
	}
	// The marker survives a further value, which the relay adds per attempt.
	withProxy := config.WithOutboundProxy(required, "http://127.0.0.1:7890")
	if !config.ProxyRequired(withProxy) || config.OutboundProxyFrom(withProxy) == "" {
		t.Fatal("the proxy and the marker do not both survive")
	}
}
