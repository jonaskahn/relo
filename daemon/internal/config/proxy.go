// Proxy is the outbound HTTP proxy an operator configures for opted-in connections.
package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// ErrInvalidProxy reports a proxy URL that is not an HTTP, HTTPS, or SOCKS5
// address.
var ErrInvalidProxy = errors.New("proxy URL must be http, https, or socks5")

// ProxyConfig is the outbound proxy stored in the startup file.
type ProxyConfig struct {
	URL string `toml:"url"`
}

type outboundProxyKey struct{}

type proxyRequiredKey struct{}

var proxyHeader = regexp.MustCompile(`^\s*\[proxy\]\s*(?:#.*)?$`)
var proxyKey = regexp.MustCompile(`^(\s*)url(\s*=\s*)(?:"[^"]*"|'[^']*'|[^#\r\n]*)(\s*(?:#.*)?)(\r?\n?)$`)

// NormalizeProxyURL turns a proxy address into the canonical form the relay
// stores and reads. It accepts a full URL ("http://user:pass@host:port"), a
// "host:port" pair, a "host:port:user:pass" quartet, or a "user:pass@host:port"
// shorthand. SOCKS5 and socks5h are accepted, and socks5h is stored as socks5.
// An empty string is the "no proxy" value. Anything else is refused.
func NormalizeProxyURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	candidate, ok := proxyCandidate(raw)
	if !ok {
		return "", ErrInvalidProxy
	}
	parsed, err := url.Parse(candidate)
	if err != nil || parsed == nil {
		return "", ErrInvalidProxy
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "socks5h" {
		scheme = "socks5"
	}
	if (scheme != "http" && scheme != "https" && scheme != "socks5") || parsed.Host == "" {
		return "", ErrInvalidProxy
	}
	parsed.Scheme = scheme
	return parsed.String(), nil
}

// proxyCandidate applies the documented shorthands before the URL parse.
func proxyCandidate(raw string) (string, bool) {
	if strings.Contains(raw, "://") {
		return raw, true
	}
	if strings.Contains(raw, "@") {
		return "http://" + raw, true
	}
	parts := strings.Split(raw, ":")
	switch len(parts) {
	case 2:
		if net.ParseIP(parts[0]) == nil {
			return "", false
		}
		return "http://" + raw, true
	case 4:
		if net.ParseIP(parts[0]) == nil {
			return "", false
		}
		return "http://" + parts[2] + ":" + parts[3] + "@" + parts[0] + ":" + parts[1], true
	default:
		return "", false
	}
}

// ValidateProxyURL accepts an empty URL or any address NormalizeProxyURL accepts.
func ValidateProxyURL(raw string) error {
	_, err := NormalizeProxyURL(raw)
	return err
}

// ReadProxyURL reads the proxy URL from the startup file. A missing file is
// an empty URL.
func ReadProxyURL(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var file struct {
		Proxy ProxyConfig `toml:"proxy"`
	}
	if _, err := toml.Decode(string(data), &file); err != nil {
		return "", err
	}
	raw := strings.TrimSpace(file.Proxy.URL)
	normalized, err := NormalizeProxyURL(raw)
	if err != nil {
		return "", err
	}
	return normalized, nil
}

// UpdateProxyURL stores the proxy URL, keeping the rest of the startup file.
func UpdateProxyURL(path, raw string) error {
	raw = strings.TrimSpace(raw)
	normalized, err := NormalizeProxyURL(raw)
	if err != nil {
		return err
	}
	appearanceMu.Lock()
	defer appearanceMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	updated := replaceProxy(string(data), normalized)
	return writeFileAtomically(path, updated)
}

// OutboundProxyFrom returns the proxy URL stored on ctx, or empty.
func OutboundProxyFrom(ctx context.Context) string {
	raw, _ := ctx.Value(outboundProxyKey{}).(string)
	return raw
}

// WithOutboundProxy returns a context whose outbound calls use raw when it
// is a proxy URL. An empty URL leaves the context unchanged.
func WithOutboundProxy(ctx context.Context, raw string) context.Context {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ctx
	}
	return context.WithValue(ctx, outboundProxyKey{}, raw)
}

// RequireProxy marks a context as one whose request must not fall back to a
// direct connection when no pool entry is healthy.
func RequireProxy(ctx context.Context) context.Context {
	return context.WithValue(ctx, proxyRequiredKey{}, true)
}

// ProxyRequired reports whether a context was marked by RequireProxy.
func ProxyRequired(ctx context.Context) bool {
	required, _ := ctx.Value(proxyRequiredKey{}).(bool)
	return required
}

// OutboundProxy returns the proxy URL a request opts into on its context, or
// the process proxy when it opts into nothing.
func OutboundProxy(request *http.Request) (*url.URL, error) {
	raw, _ := request.Context().Value(outboundProxyKey{}).(string)
	if raw != "" {
		return url.Parse(raw)
	}
	return http.ProxyFromEnvironment(request)
}

// OutboundTransport is the HTTP transport that sends a request through the
// proxy on its context, and otherwise uses the process proxy.
func OutboundTransport() *http.Transport {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = OutboundProxy
	return base
}

func replaceProxy(source, raw string) string {
	quoted := fmt.Sprintf("%q", raw)
	lines := strings.SplitAfter(source, "\n")
	inside, found, wrote := false, false, false
	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r\n")
		if proxyHeader.MatchString(trimmed) {
			inside, found = true, true
			continue
		}
		if inside && tableHeader.MatchString(trimmed) {
			if !wrote {
				lines[i] = "url = " + quoted + "\n" + line
				wrote = true
			}
			inside = false
		}
		if !inside || !proxyKey.MatchString(line) {
			continue
		}
		ending := "\n"
		if strings.HasSuffix(line, "\r\n") {
			ending = "\r\n"
		}
		lines[i] = "url = " + quoted + ending
		wrote = true
	}
	return finishProxyWrite(lines, inside, found, wrote, quoted)
}

func finishProxyWrite(lines []string, inside, found, wrote bool, quoted string) string {
	body := strings.Join(lines, "")
	if found && wrote {
		return body
	}
	if inside && !wrote {
		if body != "" && !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		return body + "url = " + quoted + "\n"
	}
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return body + "\n[proxy]\nurl = " + quoted + "\n"
}
