package upstream

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// The gateway's fingerprint gate is matched before any application bytes, so
// the offline-verifiable behaviour of the zen transport is its dialing
// discipline: it refuses every host that is not opencode.ai. The client-hello
// shape itself is verified by the live replay, which needs a server that
// accepts the spec.

func TestDialOpenCodeRefusesAnotherHost(t *testing.T) {
	transport := OpenCodeTransport(http.ProxyFromEnvironment)
	request, err := http.NewRequest("GET", "https://example.com/", nil)
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}
	response, err := transport.RoundTrip(request)
	if response != nil {
		_ = response.Body.Close()
	}
	if err == nil {
		t.Fatal("RoundTrip() error = nil, want the host refusal")
	}
	if !errors.Is(err, errOpenCodeHost) && !strings.Contains(err.Error(), "refuse a host") {
		t.Fatalf("RoundTrip() error = %v, want the host refusal", err)
	}
}

func TestDialOpenCodeOnlyFingerprintsItsOwnHost(t *testing.T) {
	transport := OpenCodeTransport(http.ProxyFromEnvironment)
	if transport.DialTLSContext == nil {
		t.Fatal("OpenCodeTransport has no custom TLS dialer")
	}
	conn, err := transport.DialTLSContext(context.Background(), "tcp", "api.example.org:443")
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil {
		t.Fatal("dialed a host the zen transport must refuse")
	}
	if !errors.Is(err, errOpenCodeHost) {
		t.Fatalf("dial error = %v, want errOpenCodeHost", err)
	}
}

func TestExecutorRoutesOnlyTheFreeLaneToTheZenClient(t *testing.T) {
	plain := &http.Client{}
	zen := &http.Client{}
	executor := NewExecutor(plain, zen, nil)
	if executor.clientFor(&Exchange{}) != plain {
		t.Fatal("a plain exchange must travel over the plain client")
	}
	if executor.clientFor(&Exchange{}) == zen {
		t.Fatal("a plain exchange must not reach the zen client")
	}
	// The free lane is named by its template id; the test constructs the
	// option the relay builds, so the routing rule is what is asserted.
	free := &Exchange{}
	free.Options.TemplateID = "opencode-free"
	if executor.clientFor(free) != zen {
		t.Fatal("the free lane must travel over the zen client")
	}
	// A nil zen client falls back to the plain client, so tests and older
	// wire-ups that build an executor without one still send requests.
	executor = NewExecutor(plain, nil, nil)
	if executor.clientFor(free) != plain {
		t.Fatal("a nil zen client must fall back to the plain client")
	}
}
