package codec

import (
	"maps"
	"net/http"
	"reflect"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

func TestApplyCredentialSelectsTheConfiguredHeader(t *testing.T) {
	for _, entry := range []struct{ name, key, auth, token, header, value string }{
		{name: "default bearer", token: "secret", header: "Authorization", value: "Bearer secret"},
		{name: "explicit bearer", key: "bearer", token: "secret", header: "Authorization", value: "Bearer secret"},
		{name: "anthropic", key: "x-api-key", token: "secret", header: "X-Api-Key", value: "secret"},
		{name: "google", key: "x-goog-api-key", token: "secret", header: "X-Goog-Api-Key", value: "secret"},
		{name: "azure", key: "api-key", token: "secret", header: "Api-Key", value: "secret"},
		{name: "custom", key: "X-Grant", token: "secret", header: "X-Grant", value: "secret"},
		{name: "missing credential"},
		{name: "keyless authentication", auth: wire.AuthNone, token: "secret"},
		{name: "disabled header", key: "none", token: "secret"},
	} {
		t.Run(entry.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "https://upstream.example", nil)
			if err != nil {
				t.Fatal(err)
			}
			wire.ApplyCredential(req, wire.CodecOpts{KeyHeader: entry.key, AuthMethod: entry.auth, CredentialRef: entry.token})
			want := http.Header{}
			if entry.header != "" {
				want.Set(entry.header, entry.value)
			}
			if !reflect.DeepEqual(req.Header, want) {
				t.Fatalf("headers = %v, want %v", req.Header, want)
			}
		})
	}
}

func TestMergeHeadersPreservesTransportAndCredentialOwnership(t *testing.T) {
	static := map[string]string{"Authorization": "Bearer stored", "X-Trace": "static", "X-Provider": "provider"}
	caller := map[string]string{"authorization": "Bearer attacker", "x-trace": "caller", "X-New": "new"}
	for _, name := range []string{"API-KEY", "X-API-KEY", "Host", "Content-Length", "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding", "Upgrade"} {
		caller[name] = "blocked"
	}
	staticBefore, callerBefore := maps.Clone(static), maps.Clone(caller)
	merged := wire.MergeHeaders(static, caller)
	want := map[string]string{"Authorization": "Bearer stored", "x-trace": "caller", "X-Provider": "provider", "X-New": "new"}
	if !maps.Equal(merged, want) {
		t.Fatalf("merged = %v, want %v", merged, want)
	}
	if !maps.Equal(static, staticBefore) || !maps.Equal(caller, callerBefore) {
		t.Fatal("merging mutated an input")
	}
	merged["X-Provider"] = "changed"
	if static["X-Provider"] != "provider" {
		t.Fatal("result aliases static headers")
	}
}

func TestApplyHeadersReplacesExistingValues(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://upstream.example", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Trace", "old")
	wire.ApplyHeaders(req, map[string]string{"x-trace": "new", "X-Provider": "provider"})
	if req.Header.Get("X-Trace") != "new" || req.Header.Get("X-Provider") != "provider" || len(req.Header.Values("X-Trace")) != 1 {
		t.Fatalf("headers = %v", req.Header)
	}
	wire.ApplyHeaders(req, nil)
	if got := wire.MergeHeaders(nil, nil); len(got) != 0 {
		t.Fatalf("empty merge = %v", got)
	}
}
