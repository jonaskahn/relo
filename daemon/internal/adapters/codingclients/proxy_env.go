// Proxy mode settings: the environment a client needs to reach the vendor
// through Relo's local intercept instead of a rewritten base address.
package codingclients

import (
	"fmt"
	"strings"
)

// The keys proxy mode owns. HTTPS_PROXY is the intercept itself, and
// NODE_EXTRA_CA_CERTS is how a Node client trusts the certificate Relo
// presents for the vendor's host. Neither is installed in an OS trust store.
const (
	ProxyEnvHTTPS    = "HTTPS_PROXY"
	ProxyEnvCA       = "NODE_EXTRA_CA_CERTS"
	ProxyEnvNoProxy  = "NO_PROXY"
	proxyNoProxyBase = "127.0.0.1,localhost,::1,0.0.0.0"
)

// ProxyEnv is what one client's settings carry in proxy mode.
type ProxyEnv struct {
	// URL is the authenticated loopback address of the intercept.
	URL string
	// CAPath is the certificate the client must trust for the vendor's host.
	CAPath string
}

// MergeClaudeProxySettings returns Claude Code's settings pointed at the
// intercept, leaving every key Relo does not own exactly as it was. A proxy
// the operator set is refused rather than replaced: it may be the one that
// reaches their network at all.
func MergeClaudeProxySettings(existing string, env ProxyEnv) (string, error) {
	if strings.TrimSpace(env.URL) == "" || strings.TrimSpace(env.CAPath) == "" {
		return "", fmt.Errorf("claude-code: %w (proxy mode has no intercept to point at)", ErrUnrecognisedFile)
	}
	document, err := decodeSettings(existing)
	if err != nil {
		return "", err
	}
	settings, err := settingsEnv(document)
	if err != nil {
		return "", err
	}
	if current, ok := settings[ProxyEnvHTTPS].(string); ok && current != "" && current != env.URL {
		return "", fmt.Errorf("claude-code: %w (HTTPS_PROXY is already set to another proxy)", ErrUnrecognisedFile)
	}
	settings[ProxyEnvHTTPS] = env.URL
	settings[ProxyEnvCA] = env.CAPath
	settings[ProxyEnvNoProxy] = proxyNoProxy(settings[ProxyEnvNoProxy])
	document["env"] = settings
	return encodeSettings(document)
}

// StripClaudeProxySettings removes the proxy settings Relo wrote and restores
// a NO_PROXY value it joined, leaving a value the operator set alone.
func StripClaudeProxySettings(existing string, env ProxyEnv) (string, error) {
	document, err := decodeSettings(existing)
	if err != nil {
		return "", err
	}
	settings, err := settingsEnv(document)
	if err != nil {
		return "", err
	}
	if value, ok := settings[ProxyEnvHTTPS].(string); ok && value == env.URL {
		delete(settings, ProxyEnvHTTPS)
	}
	if value, ok := settings[ProxyEnvCA].(string); ok && value == env.CAPath {
		delete(settings, ProxyEnvCA)
	}
	if value, ok := settings[ProxyEnvNoProxy].(string); ok {
		if restored := stripProxyNoProxy(value); restored != value {
			if restored == "" {
				delete(settings, ProxyEnvNoProxy)
			} else {
				settings[ProxyEnvNoProxy] = restored
			}
		}
	}
	if len(settings) == 0 {
		delete(document, "env")
	} else {
		document["env"] = settings
	}
	return encodeSettings(document)
}

// ClaudeProxySettingsPointAt reports whether a settings document already
// carries the intercept this proxy mode would write.
func ClaudeProxySettingsPointAt(content string, env ProxyEnv) bool {
	document, err := decodeSettings(content)
	if err != nil {
		return false
	}
	settings, err := settingsEnv(document)
	if err != nil {
		return false
	}
	proxy, _ := settings[ProxyEnvHTTPS].(string)
	ca, _ := settings[ProxyEnvCA].(string)
	return proxy == env.URL && ca == env.CAPath
}

// proxyNoProxy joins Relo's loopback bypass onto whatever the operator
// already bypasses, preserving their order and their spelling.
func proxyNoProxy(current any) string {
	value, _ := current.(string)
	kept := make([]string, 0, 8)
	seen := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		kept = append(kept, part)
	}
	for _, part := range strings.Split(proxyNoProxyBase, ",") {
		if !seen[part] {
			seen[part] = true
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, ",")
}

// stripProxyNoProxy removes the loopback bypass Relo joined, and reports an
// empty string when nothing the operator wrote is left.
func stripProxyNoProxy(value string) string {
	ours := map[string]bool{}
	for _, part := range strings.Split(proxyNoProxyBase, ",") {
		ours[part] = true
	}
	kept := make([]string, 0, 8)
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" || ours[part] {
			continue
		}
		kept = append(kept, part)
	}
	return strings.Join(kept, ",")
}

// ProxySettingsState is how a client's settings stand against proxy mode.
// The console shows it so an operator reads why the wiring is not in place.
type ProxySettingsState string

// The states a settings document can be in.
const (
	ProxyStateAbsent     ProxySettingsState = "absent"
	ProxyStateApplied    ProxySettingsState = "applied"
	ProxyStateStale      ProxySettingsState = "stale"
	ProxyStateForeign    ProxySettingsState = "foreign"
	ProxyStateUnreadable ProxySettingsState = "unreadable"
)

// InspectClaudeProxySettings classifies a settings document against the
// intercept this build would write.
func InspectClaudeProxySettings(content string, env ProxyEnv) ProxySettingsState {
	if strings.TrimSpace(content) == "" {
		return ProxyStateAbsent
	}
	document, err := decodeSettings(content)
	if err != nil {
		return ProxyStateUnreadable
	}
	settings, err := settingsEnv(document)
	if err != nil {
		return ProxyStateUnreadable
	}
	proxy, _ := settings[ProxyEnvHTTPS].(string)
	switch {
	case proxy == "":
		return ProxyStateAbsent
	case proxy != env.URL:
		return ProxyStateForeign
	case ClaudeProxySettingsPointAt(content, env):
		return ProxyStateApplied
	default:
		return ProxyStateStale
	}
}
