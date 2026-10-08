package codec

import (
	"regexp"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

var openCodeSession = regexp.MustCompile(`^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`)
var openCodeRequest = regexp.MustCompile(`^msg_[0-9a-f]{12}[0-9A-Za-z]{14}$`)

func TestOpenCodeGoSession(t *testing.T) {
	const goURL = "https://opencode.ai/zen/go/v1"
	headers := map[string]string{"X-Trace": "abc"}

	t.Run("a go destination gains a stable session", func(t *testing.T) {
		first := wire.WithOpenCodeGoSession(headers, goURL, "openai-chat", "lane-1")
		again := wire.WithOpenCodeGoSession(headers, goURL, "openai-chat", "lane-1")
		if first[wire.OpenCodeGoSessionHeader] == "" {
			t.Fatal("session header is missing")
		}
		if first[wire.OpenCodeGoSessionHeader] != again[wire.OpenCodeGoSessionHeader] {
			t.Fatalf("session = %q, again = %q", first[wire.OpenCodeGoSessionHeader], again[wire.OpenCodeGoSessionHeader])
		}
		if headers[wire.OpenCodeGoSessionHeader] != "" {
			t.Fatal("the caller's header map was changed")
		}
		if first["X-Trace"] != "abc" {
			t.Fatalf("trace = %q", first["X-Trace"])
		}
	})

	t.Run("a different conversation or wire gets a different session", func(t *testing.T) {
		one := wire.OpenCodeGoSessionID("lane-1", "openai-chat")
		otherLane := wire.OpenCodeGoSessionID("lane-2", "openai-chat")
		otherWire := wire.OpenCodeGoSessionID("lane-1", "anthropic")
		if one == otherLane || one == otherWire {
			t.Fatalf("sessions collided: %q %q %q", one, otherLane, otherWire)
		}
	})

	t.Run("an operator session is left alone", func(t *testing.T) {
		owned := map[string]string{"X-Opencode-Session": "kept"}
		got := wire.WithOpenCodeGoSession(owned, goURL, "openai-chat", "lane-1")
		if got["X-Opencode-Session"] != "kept" || len(got) != 1 {
			t.Fatalf("headers = %#v", got)
		}
	})

	t.Run("another host is left alone", func(t *testing.T) {
		for _, base := range []string{
			"https://api.openai.com/v1",
			"https://opencode.ai/zen/v1",
			"https://example.com/zen/go/v1",
			"",
		} {
			got := wire.WithOpenCodeGoSession(headers, base, "openai-chat", "lane-1")
			if _, ok := got[wire.OpenCodeGoSessionHeader]; ok {
				t.Fatalf("base %q gained a session", base)
			}
		}
	})
}

func TestOpenCodeFreeHeadersMatchTheSignedOutCLI(t *testing.T) {
	first, err := wire.OpenCodeFreeHeaders("lane-free", "not-a-session")
	if err != nil {
		t.Fatalf("OpenCodeFreeHeaders() error = %v", err)
	}
	again, err := wire.OpenCodeFreeHeaders("lane-free", "")
	if err != nil {
		t.Fatalf("OpenCodeFreeHeaders() error = %v", err)
	}
	if first["User-Agent"] != "opencode/1.18.33" || first["x-opencode-client"] != "cli" {
		t.Fatalf("headers = %#v", first)
	}
	if first["Authorization"] != "Bearer public" || first["x-opencode-project"] != "global" {
		t.Fatalf("headers = %#v", first)
	}
	if !openCodeSession.MatchString(first["x-opencode-session"]) || !openCodeRequest.MatchString(first["x-opencode-request"]) {
		t.Fatalf("headers = %#v", first)
	}
	if first["x-opencode-session"] != again["x-opencode-session"] {
		t.Fatalf("session changed from %q to %q", first["x-opencode-session"], again["x-opencode-session"])
	}
	if first["x-opencode-request"] == again["x-opencode-request"] {
		t.Fatal("message id was reused")
	}
	other, err := wire.OpenCodeFreeHeaders("lane-other", "")
	if err != nil {
		t.Fatalf("OpenCodeFreeHeaders() error = %v", err)
	}
	if other["x-opencode-session"] == first["x-opencode-session"] {
		t.Fatal("lanes share a session")
	}
	kept, err := wire.OpenCodeFreeHeaders("lane-free", first["x-opencode-session"])
	if err != nil {
		t.Fatalf("OpenCodeFreeHeaders() error = %v", err)
	}
	if kept["x-opencode-session"] != first["x-opencode-session"] {
		t.Fatalf("session = %q, want the inbound ses_ id", kept["x-opencode-session"])
	}
}
