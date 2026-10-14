package platform

import (
	"testing"

	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/server"
)

func TestBoundListenersNamesOnlyTheSingleListener(t *testing.T) {
	if got := boundListeners(server.Addrs{}); got != nil {
		t.Fatalf("boundListeners(empty) = %v, want nil", got)
	}
	got := boundListeners(server.Addrs{Management: "127.0.0.1:10101"})
	if len(got) != 1 || got[0] != "127.0.0.1:10101" {
		t.Fatalf("boundListeners() = %v, want the management address", got)
	}
}

func TestDataPlaneBaseURLPointsAtTheSharedListener(t *testing.T) {
	if got := dataPlaneBaseURL(nil, inference.ProtocolOpenAI); got != "" {
		t.Fatalf("dataPlaneBaseURL(nil) = %q, want empty", got)
	}
	cfg := &config.Config{Server: config.ServerConfig{Bind: "127.0.0.1", Port: 10101}}
	if got, want := dataPlaneBaseURL(cfg, inference.ProtocolOpenAI), "http://127.0.0.1:10101/inference"; got != want {
		t.Fatalf("dataPlaneBaseURL(openai) = %q, want %q", got, want)
	}
	if got, want := dataPlaneBaseURL(cfg, inference.ProtocolAnthropic), "http://127.0.0.1:10101/inference-claude"; got != want {
		t.Fatalf("dataPlaneBaseURL(anthropic) = %q, want %q", got, want)
	}
	if got := dataPlaneBaseURL(cfg, inference.ProtocolGemini); got != "" {
		t.Fatalf("dataPlaneBaseURL(gemini) = %q, want empty", got)
	}
}
