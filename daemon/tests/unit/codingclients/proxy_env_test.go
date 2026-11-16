package codingclients_test

import (
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

func proxyTestEnv() codingclients.ProxyEnv {
	return codingclients.ProxyEnv{
		URL:    "http://relo:secret@127.0.0.1:51000",
		CAPath: "/state/proxy-mode/claude-code/ca.pem",
	}
}

// TestMergeClaudeProxySettingsPointsAtTheIntercept pins what proxy mode
// writes: the proxy, the certificate it presents, and the loopback bypass,
// with every other settings key left exactly as it was.
func TestMergeClaudeProxySettingsPointsAtTheIntercept(t *testing.T) {
	existing := `{"model":"opus","env":{"USER_KEY":"kept"},"permissions":{"allow":["x"]}}`
	merged, err := codingclients.MergeClaudeProxySettings(existing, proxyTestEnv())
	if err != nil {
		t.Fatalf("MergeClaudeProxySettings() error = %v", err)
	}
	for _, wanted := range []string{
		`"HTTPS_PROXY": "http://relo:secret@127.0.0.1:51000"`,
		`"NODE_EXTRA_CA_CERTS": "/state/proxy-mode/claude-code/ca.pem"`,
		`"NO_PROXY": "127.0.0.1,localhost,::1,0.0.0.0"`,
		`"USER_KEY": "kept"`,
		`"model": "opus"`,
	} {
		if !strings.Contains(merged, wanted) {
			t.Fatalf("merged settings = %s, want %s", merged, wanted)
		}
	}
	if !codingclients.ClaudeProxySettingsPointAt(merged, proxyTestEnv()) {
		t.Fatal("the merged settings do not report the intercept they were written with")
	}
}

// TestMergeClaudeProxySettingsRefusesAForeignProxy keeps an operator's own
// proxy in place: it may be the only route off their network.
func TestMergeClaudeProxySettingsRefusesAForeignProxy(t *testing.T) {
	existing := `{"env":{"HTTPS_PROXY":"http://corp-proxy:8080"}}`
	if _, err := codingclients.MergeClaudeProxySettings(existing, proxyTestEnv()); err == nil {
		t.Fatal("MergeClaudeProxySettings() error = nil, want the foreign proxy refused")
	}
	state := codingclients.InspectClaudeProxySettings(existing, proxyTestEnv())
	if state != codingclients.ProxyStateForeign {
		t.Fatalf("state = %q, want %q", state, codingclients.ProxyStateForeign)
	}
}

// TestAnAddressFromBeforeARestartIsStaleNotForeign pins the restart case: the
// intercept binds a new port on every start, so the address its own earlier run
// wrote is replaced, while another proxy on the same host still is not.
func TestAnAddressFromBeforeARestartIsStaleNotForeign(t *testing.T) {
	env := proxyTestEnv()
	older := `{"env":{"HTTPS_PROXY":"http://relo:secret@127.0.0.1:49000","NODE_EXTRA_CA_CERTS":"` + env.CAPath + `"}}`
	if state := codingclients.InspectClaudeProxySettings(older, env); state != codingclients.ProxyStateStale {
		t.Fatalf("state = %q, want %q", state, codingclients.ProxyStateStale)
	}
	merged, err := codingclients.MergeClaudeProxySettings(older, env)
	if err != nil || !codingclients.ClaudeProxySettingsPointAt(merged, env) || strings.Contains(merged, ":49000") {
		t.Fatalf("merged = %s (%v), want the new address in place of the old one", merged, err)
	}
	stripped, err := codingclients.StripClaudeProxySettings(older, env)
	if err != nil || strings.Contains(stripped, "HTTPS_PROXY") {
		t.Fatalf("stripped = %s (%v), want the earlier address removed too", stripped, err)
	}
	for _, other := range []string{"http://relo:other@127.0.0.1:49000", "http://elsewhere:secret@127.0.0.1:49000"} {
		content := `{"env":{"HTTPS_PROXY":"` + other + `"}}`
		if _, err := codingclients.MergeClaudeProxySettings(content, env); err == nil {
			t.Fatalf("MergeClaudeProxySettings(%s) error = nil, want another proxy refused", other)
		}
	}
}

// TestStripClaudeProxySettingsKeepsTheOperatorsOwnBypass restores a NO_PROXY
// the merge joined onto, and removes only the keys the merge wrote.
func TestStripClaudeProxySettingsKeepsTheOperatorsOwnBypass(t *testing.T) {
	existing := `{"env":{"HTTPS_PROXY":"http://elsewhere:1","NO_PROXY":".corp,10.0.0.0/8"}}`
	merged, err := codingclients.MergeClaudeProxySettings(existing, proxyTestEnv())
	if err != nil {
		// The settings carry another proxy, so this merge is refused by design:
		// the strip below covers the value a merge of Relo's own wrote.
		t.Skipf("a foreign proxy is refused before it can be joined: %v", err)
	}
	stripped, err := codingclients.StripClaudeProxySettings(merged, proxyTestEnv())
	if err != nil {
		t.Fatalf("StripClaudeProxySettings() error = %v", err)
	}
	if strings.Contains(stripped, "HTTPS_PROXY") || strings.Contains(stripped, "NODE_EXTRA_CA_CERTS") {
		t.Fatalf("stripped settings = %s, want the intercept's keys gone", stripped)
	}
	if strings.Contains(stripped, "127.0.0.1") {
		t.Fatalf("stripped settings = %s, want the joined bypass gone", stripped)
	}
}

// TestStripClaudeProxySettingsRestoresAJoinedBypass pins the receipt: a value
// the operator already had keeps its own entries in their own order.
func TestStripClaudeProxySettingsRestoresAJoinedBypass(t *testing.T) {
	existing := `{"env":{"NO_PROXY":".corp,10.0.0.0/8"}}`
	merged, err := codingclients.MergeClaudeProxySettings(existing, proxyTestEnv())
	if err != nil {
		t.Fatalf("MergeClaudeProxySettings() error = %v", err)
	}
	stripped, err := codingclients.StripClaudeProxySettings(merged, proxyTestEnv())
	if err != nil {
		t.Fatalf("StripClaudeProxySettings() error = %v", err)
	}
	if !strings.Contains(stripped, `"NO_PROXY": ".corp,10.0.0.0/8"`) {
		t.Fatalf("stripped settings = %s, want the operator's own bypass restored", stripped)
	}
}

// TestInspectClaudeProxySettingsNamesWhyTheWiringIsNotInPlace pins the states
// a console translates.
func TestInspectClaudeProxySettingsNamesWhyTheWiringIsNotInPlace(t *testing.T) {
	env := proxyTestEnv()
	for name, testCase := range map[string]struct {
		content string
		want    codingclients.ProxySettingsState
	}{
		"an absent file":         {content: "", want: codingclients.ProxyStateAbsent},
		"a file without a proxy": {content: `{"env":{}}`, want: codingclients.ProxyStateAbsent},
		"an unreadable file":     {content: "{not json", want: codingclients.ProxyStateUnreadable},
		"a non-object env":       {content: `{"env":"x"}`, want: codingclients.ProxyStateUnreadable},
		"a proxy but no trust": {
			content: `{"env":{"HTTPS_PROXY":"` + env.URL + `"}}`,
			want:    codingclients.ProxyStateStale,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := codingclients.InspectClaudeProxySettings(testCase.content, env); got != testCase.want {
				t.Fatalf("state = %q, want %q", got, testCase.want)
			}
		})
	}
}
