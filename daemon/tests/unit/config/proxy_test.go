package config_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/config"
)

// TestValidateProxyURLAcceptsOnlyHTTP pins the one rule the whole proxy
// feature rests on: an outbound proxy is an http or https address with a host,
// anything else is refused rather than silently connecting direct.
func TestValidateProxyURLAcceptsOnlyHTTP(t *testing.T) {
	for _, valid := range []string{
		"", "   ", "http://proxy.example:8080", "https://proxy.example",
		"http://127.0.0.1:1080", "HTTP://proxy.example",
	} {
		if err := config.ValidateProxyURL(valid); err != nil {
			t.Errorf("ValidateProxyURL(%q) = %v, want accepted", valid, err)
		}
	}
	for _, invalid := range []string{
		"socks5://127.0.0.1:1080", "ftp://proxy.example", "proxy.example:8080",
		"http://", "://nohost", "http://[::1",
	} {
		if err := config.ValidateProxyURL(invalid); !errors.Is(err, config.ErrInvalidProxy) {
			t.Errorf("ValidateProxyURL(%q) = %v, want ErrInvalidProxy", invalid, err)
		}
	}
}

// TestProxyURLRoundTripThroughTheStartupFile covers the write and read the
// console performs, including the three shapes the file can be in: a file with
// no proxy table, one whose table has no url, and one that already stores a
// url. Every other setting in the file has to survive.
func TestProxyURLRoundTripThroughTheStartupFile(t *testing.T) {
	for name, original := range map[string]string{
		"no proxy table":  "# keep this comment\n[server]\nport = 12345\n\n[ui]\nlanguage = \"de\"\n",
		"empty table":     "[server]\nport = 12345\n\n[proxy]\n\n[logging]\nlevel = \"debug\"\n",
		"existing url":    "[server]\nport = 12345\n\n[proxy]\nurl = \"http://old.example\"\n\n[logging]\nlevel = \"debug\"\n",
		"absent file":     "",
		"comment on head": "# keep this comment\n[server]\nport = 12345\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if original != "" {
				if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			if err := config.UpdateProxyURL(path, "https://proxy.example:8443"); err != nil {
				t.Fatalf("UpdateProxyURL() error = %v", err)
			}
			raw, err := config.ReadProxyURL(path)
			if err != nil {
				t.Fatalf("ReadProxyURL() error = %v", err)
			}
			if raw != "https://proxy.example:8443" {
				t.Fatalf("ReadProxyURL() = %q, want the stored proxy", raw)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// The rest of the file is the operator's, not ours to rewrite.
			for _, text := range []string{"port = 12345", "language = \"de\"", "level = \"debug\"", "# keep this comment"} {
				if strings.Contains(original, text) && !strings.Contains(string(data), text) {
					t.Fatalf("lost %q in %s", text, data)
				}
			}

			// Writing over the stored proxy replaces it rather than adding a
			// second url the file would then read as invalid.
			if err := config.UpdateProxyURL(path, "http://127.0.0.1:1080"); err != nil {
				t.Fatal(err)
			}
			data, err = os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Count(string(data), "url = "); got != 1 {
				t.Fatalf("the file names a proxy %d times, want once:\n%s", got, data)
			}
			if raw, err = config.ReadProxyURL(path); err != nil || raw != "http://127.0.0.1:1080" {
				t.Fatalf("ReadProxyURL() = %q, %v, want the replacement", raw, err)
			}
		})
	}
}

// TestReadProxyURLAcceptsAnAbsentFileAndRefusesABrokenOne covers the two ways
// reading the stored proxy can go wrong before an operator has set one: the
// startup file is not there yet, or it is not valid TOML.
func TestReadProxyURLAcceptsAnAbsentFileAndRefusesABrokenOne(t *testing.T) {
	dir := t.TempDir()

	raw, err := config.ReadProxyURL(filepath.Join(dir, "absent.toml"))
	if err != nil || raw != "" {
		t.Fatalf("ReadProxyURL(absent) = %q, %v, want an empty proxy", raw, err)
	}

	broken := filepath.Join(dir, "broken.toml")
	if err := os.WriteFile(broken, []byte("this is not = = toml\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.ReadProxyURL(broken); err == nil {
		t.Fatal("ReadProxyURL() accepted a file that is not TOML")
	}

	// A stored proxy the rules reject is refused rather than ignored: an
	// operator who wrote socks5:// asked for something Relo cannot do, and
	// quietly connecting direct would hide that.
	invalid := filepath.Join(dir, "invalid.toml")
	if err := os.WriteFile(invalid, []byte("[proxy]\nurl = \"socks5://127.0.0.1:1080\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.ReadProxyURL(invalid); !errors.Is(err, config.ErrInvalidProxy) {
		t.Fatalf("ReadProxyURL() = %v, want ErrInvalidProxy", err)
	}
}

// TestUpdateProxyURLRefusesAProxyItCannotUse keeps an invalid address from
// ever reaching the startup file.
func TestUpdateProxyURLRefusesAProxyItCannotUse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := "[server]\nport = 12345\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateProxyURL(path, "socks5://127.0.0.1:1080"); !errors.Is(err, config.ErrInvalidProxy) {
		t.Fatalf("UpdateProxyURL() = %v, want ErrInvalidProxy", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("a refused write changed the file to %s", data)
	}
}

// TestWithOutboundProxyCarriesTheProxyOnTheContext covers the hop between the
// settings the relay reads and the transport that sends the request: the proxy
// is on the context, and an unset one leaves the context as it was.
func TestWithOutboundProxyCarriesTheProxyOnTheContext(t *testing.T) {
	ctx := context.Background()
	if got := config.OutboundProxyFrom(ctx); got != "" {
		t.Fatalf("OutboundProxyFrom(plain) = %q, want empty", got)
	}
	if config.WithOutboundProxy(ctx, "  ") != ctx {
		t.Fatal("an empty proxy changed the context")
	}

	carrying := config.WithOutboundProxy(ctx, "  https://proxy.example:8443  ")
	if got := config.OutboundProxyFrom(carrying); got != "https://proxy.example:8443" {
		t.Fatalf("OutboundProxyFrom() = %q, want the trimmed proxy", got)
	}
}

// TestOutboundTransportRoutesThroughTheProxyOnTheRequest covers the decision
// the transport makes per request: a context carrying a proxy sends the
// request through it, and one without falls back to the process proxy.
func TestOutboundTransportRoutesThroughTheProxyOnTheRequest(t *testing.T) {
	transport := config.OutboundTransport()

	proxied := &http.Request{URL: mustURL(t, "https://upstream.example/v1")}
	proxied = proxied.WithContext(config.WithOutboundProxy(proxied.Context(), "http://proxy.example:8080"))
	target, err := transport.Proxy(proxied)
	if err != nil {
		t.Fatalf("Proxy() error = %v", err)
	}
	if target == nil || target.Host != "proxy.example:8080" {
		t.Fatalf("Proxy() = %v, want the proxy on the request context", target)
	}

	direct := &http.Request{URL: mustURL(t, "https://upstream.example/v1")}
	target, err = transport.Proxy(direct)
	if err != nil {
		t.Fatalf("Proxy() error = %v", err)
	}
	// The fallback is the process proxy, which is nil unless HTTP_PROXY is set
	// in the environment; either way it must not be the request's own URL.
	if target != nil && target.Host == "upstream.example" {
		t.Fatalf("Proxy() = %v, want the request sent to its own host", target)
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
