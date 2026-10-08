package server_test

import (
	"context"
	"github.com/jonaskahn/relo/internal/inference"

	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestDataPlaneListeners starts a server the way the daemon does and checks
// every listener it bound: each one answers for its own protocol behind a
// client key, and for nothing else.
func TestDataPlaneListeners(t *testing.T) {
	cfg := freeConfig(t)

	harness := newHarness(t)
	harness.server = harness.newServerWithConfig(cfg, defaultEntry())
	ctx, cancel := context.WithCancel(context.Background())
	serving := make(chan error, 1)
	go func() { serving <- harness.server.Start(ctx) }()
	management := waitForAddress(t, harness.server.Addr)
	openai := waitForDataPlaneAddress(t, harness.server, inference.ProtocolOpenAI)
	anthropic := waitForDataPlaneAddress(t, harness.server, inference.ProtocolAnthropic)

	t.Run("each protocol answers on the port the configuration gave it", func(t *testing.T) {
		for protocol, address := range map[string]string{
			inference.ProtocolOpenAI: openai, inference.ProtocolAnthropic: anthropic,
		} {
			if want := "127.0.0.1:" + strconv.Itoa(cfg.Server.DataPlane.Port(protocol)); address != want {
				t.Fatalf("%s bound %q, want %q", protocol, address, want)
			}
		}
		if openai == management || anthropic == management {
			t.Fatal("a data plane listener took the management address")
		}
	})

	t.Run("every port answers the health probe in the open", func(t *testing.T) {
		for _, address := range []string{management, openai, anthropic} {
			response := httpGetOn(t, "http://"+address+"/healthz", "")
			if response.StatusCode != http.StatusOK {
				t.Fatalf("%s healthz = %d, want 200", address, response.StatusCode)
			}
			_ = response.Body.Close()
		}
	})

	t.Run("every port answers the browser's own icon request in the open", func(t *testing.T) {
		for _, address := range []string{management, openai, anthropic} {
			response := httpGetOn(t, "http://"+address+"/favicon.ico", "")
			if response.StatusCode != http.StatusOK {
				t.Errorf("%s favicon.ico = %d, want 200 without a client key", address, response.StatusCode)
				_ = response.Body.Close()
				continue
			}
			// The stub console answers every GET with a page, so an icon that
			// is really the icon is what proves the default is served here
			// rather than the console falling through to it.
			icon, err := io.ReadAll(response.Body)
			if err != nil {
				t.Errorf("%s favicon.ico: read the icon: %v", address, err)
			} else if len(icon) < 4 || string(icon[:4]) != "\x00\x00\x01\x00" {
				t.Errorf("%s favicon.ico = %q, want the generated icon", address, icon[:min(len(icon), 8)])
			}
			_ = response.Body.Close()
		}
	})

	t.Run("each port relays its own shape behind a client key", func(t *testing.T) {
		response := httpPostTo(t, "http://"+openai+"/v1/chat/completions", dataPlaneToken, streamingBody())
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("the OpenAI-compatible port = %d, want 200", response.StatusCode)
		}
		body, err := io.ReadAll(response.Body)
		if err != nil || !strings.Contains(string(body), "[DONE]") {
			t.Fatalf("body = %q, want the relayed stream", body)
		}
	})

	t.Run("a port refuses the shape of another protocol", func(t *testing.T) {
		response := httpPostTo(t, "http://"+openai+"/v1/messages", dataPlaneToken, "{}")
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("the OpenAI-compatible port answered an Anthropic shape with %d", response.StatusCode)
		}
	})

	t.Run("a port refuses a request with no client key", func(t *testing.T) {
		for _, target := range []string{
			"http://" + openai + "/v1/chat/completions",
			"http://" + anthropic + "/v1/messages",
		} {
			response := httpPostTo(t, target, "", "{}")
			if response.StatusCode != http.StatusUnauthorized {
				t.Fatalf("%s = %d, want 401 without a key", target, response.StatusCode)
			}
			_ = response.Body.Close()
		}
	})

	cancel()
	select {
	case err := <-serving:
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the daemon did not stop")
	}
	for _, address := range []string{management, openai, anthropic} {
		if conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond); err == nil {
			_ = conn.Close()
			t.Fatalf("%s still accepts connections after shutdown", address)
		}
	}
}

// TestDataPlanePortBusy reports which listener would not bind, so an operator
// reading one line knows which port to change.
func TestDataPlanePortBusy(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("take a port: %v", err)
	}
	defer func() { _ = busy.Close() }()
	port := busy.Addr().(*net.TCPAddr).Port

	cfg := freeConfig(t)
	cfg.Server.DataPlane.OpenAI = port
	harness := newHarness(t)
	harness.server = harness.newServerWithConfig(cfg, defaultEntry())
	err = harness.server.Start(context.Background())
	if err == nil {
		t.Fatal("Start() error = nil, want the busy port refused")
	}
	if !strings.Contains(err.Error(), inference.ProtocolOpenAI) || !strings.Contains(err.Error(), strconv.Itoa(port)) {
		t.Fatalf("Start() error = %v, want it to name the %s port %d", err, inference.ProtocolOpenAI, port)
	}
}

// httpPostTo sends one inference request to a bound listener.
func httpPostTo(t *testing.T, target, token, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build the request to %s: %v", target, err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request to %s failed: %v", target, err)
	}
	return response
}

// httpGetOn performs one request against a bound listener.
func httpGetOn(t *testing.T, target, token string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("build the request to %s: %v", target, err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request to %s failed: %v", target, err)
	}
	return response
}
