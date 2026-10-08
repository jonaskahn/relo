package access_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/access"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/inference"
)

func TestClientBaseURL(t *testing.T) {
	t.Run("each protocol names the port it answers on", func(t *testing.T) {
		cfg := config.DefaultConfig()
		for protocol, want := range map[string]string{
			inference.ProtocolOpenAI:    "http://127.0.0.1:10201",
			inference.ProtocolAnthropic: "http://127.0.0.1:10202",
		} {
			got, err := access.ClientBaseURL(protocol, cfg.Server)
			if err != nil {
				t.Fatalf("ClientBaseURL(%s) error = %v", protocol, err)
			}
			if got != want {
				t.Fatalf("ClientBaseURL(%s) = %q, want %q", protocol, got, want)
			}
		}
	})

	t.Run("a protocol with no port is refused", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Server.DataPlane.OpenAI = 0
		if _, err := access.ClientBaseURL(inference.ProtocolOpenAI, cfg); !errors.Is(err, access.ErrProtocolDisabled) {
			t.Fatalf("ClientBaseURL() error = %v, want %v", err, access.ErrProtocolDisabled)
		}
	})
}

// TestProfilesNameTheirOwnPort guards the one thing a step must never do:
// send a client to a listener that does not speak its protocol.
func TestProfilesNameTheirOwnPort(t *testing.T) {
	cfg := config.DefaultConfig()
	for _, profile := range access.ClientProfiles() {
		if !inference.ProtocolKnown(profile.Protocol) {
			t.Fatalf("%s: protocol %q is not one Relo serves", profile.ID, profile.Protocol)
		}
		baseURL, err := access.ClientBaseURL(profile.Protocol, cfg.Server)
		if err != nil {
			t.Fatalf("%s: ClientBaseURL() error = %v", profile.ID, err)
		}
		fill := access.SetupFill{Token: "relo_test", BaseURL: baseURL, Model: access.DefaultModel}
		for _, step := range profile.Instructions(fill) {
			if strings.Contains(step, ":10101") {
				t.Fatalf("%s: step %q points at the management listener", profile.ID, step)
			}
			for other, port := range map[string]int{
				inference.ProtocolOpenAI:    config.DefaultOpenAIPort,
				inference.ProtocolAnthropic: config.DefaultAnthropicPort,
			} {
				if other == profile.Protocol {
					continue
				}
				if strings.Contains(step, ":"+strconv.Itoa(port)) {
					t.Fatalf("%s: step %q points at the %s port", profile.ID, step, other)
				}
			}
		}
	}
}

func TestCodexStepsUseTheManagedKey(t *testing.T) {
	profile, found := access.ClientProfileFor("codex")
	if !found {
		t.Fatal("ClientProfileFor(codex) not found")
	}
	fill := access.SetupFill{Token: "relo_test", BaseURL: "http://127.0.0.1:10201", Model: access.DefaultModel}
	joined := strings.Join(profile.Instructions(fill), "\n")
	for _, want := range []string{"RELO_CODEX_API_KEY", "http://127.0.0.1:10201/v1"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("codex steps = %q, want %s", joined, want)
		}
	}
	if strings.Contains(joined, "RELO_TOKEN") || strings.Contains(joined, "RELO_API_KEY") {
		t.Fatalf("codex steps = %q, want RELO_CODEX_API_KEY rather than a shared key", joined)
	}
}

func TestDesktopStepsDoNotInstallAnAuthSource(t *testing.T) {
	profile, found := access.ClientProfileFor("claude-desktop")
	if !found {
		t.Fatal("ClientProfileFor(claude-desktop) not found")
	}
	fill := access.SetupFill{Token: "relo_test", BaseURL: "http://127.0.0.1:10202", Model: access.DefaultModel}
	joined := strings.Join(append(profile.Instructions(fill), profile.Verification(fill)), "\n")
	for _, forbidden := range []string{"apiKey", "ANTHROPIC_AUTH_TOKEN", "relo_test"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("desktop steps = %q, want no %s", joined, forbidden)
		}
	}
	if !strings.Contains(joined, "http://127.0.0.1:10202") {
		t.Fatalf("desktop steps = %q, want the Anthropic surface", joined)
	}
}

// TestProfilesAreCopies covers the profile accessor with mutation isolation:
// changing a returned profile never moves the shipped roster.
func TestProfilesAreCopies(t *testing.T) {
	profiles := access.ClientProfiles()
	profiles[0].Steps[0] = "mutated"
	if again := access.ClientProfiles(); again[0].Steps[0] == "mutated" {
		t.Fatal("ClientProfiles() shares its storage with callers")
	}
}
