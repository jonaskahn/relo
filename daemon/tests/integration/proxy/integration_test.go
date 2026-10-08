package proxy_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegrationWiresAnAgentAndProvesItsKey is the end-to-end promise of an
// integration: the key it mints authenticates against the running data plane,
// and removing the integration puts the operator's own file back as it was.
func TestIntegrationWiresAnAgentAndProvesItsKey(t *testing.T) {
	origin := newUpstream(t, "openai/chat_streaming.txt", true)
	daemon := startDaemon(t, origin.URL())
	codexHome := filepath.Join(t.TempDir(), ".codex")
	t.Setenv("CODEX_HOME", codexHome)
	configPath := filepath.Join(codexHome, "config.toml")
	original := "# my codex config\napproval_policy = \"never\"\n"
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("create the codex home: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatalf("seed the codex config: %v", err)
	}

	ctx := context.Background()
	result, err := daemon.integrations.EnableIntegration(ctx, "codex")
	if err != nil {
		t.Fatalf("EnableIntegration() error = %v", err)
	}
	if !strings.HasPrefix(result.Token, "rlo_ak_") {
		t.Fatalf("token = %q, want a client key", result.Token)
	}
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read the codex config: %v", err)
	}
	if !strings.Contains(string(config), "model_providers.relo") {
		t.Fatalf("codex config = %q, want Relo's provider", config)
	}
	if !strings.Contains(string(config), "approval_policy") {
		t.Fatalf("codex config = %q, want the operator's own row kept", config)
	}

	// The key the integration minted reaches the data plane, which is the
	// difference between a file Relo wrote and an agent that works.
	verify, err := daemon.integrations.VerifyIntegration(ctx, "codex")
	if err != nil {
		t.Fatalf("VerifyIntegration() error = %v", err)
	}
	if !verify.OK || verify.Status != http.StatusOK {
		t.Fatalf("verify = %+v, want the data plane to accept the key", verify)
	}
	if !strings.Contains(verify.URL, "/v1/models") {
		t.Fatalf("verify URL = %q, want the data plane's model list", verify.URL)
	}

	if err := daemon.integrations.DisableIntegration(ctx, "codex"); err != nil {
		t.Fatalf("DisableIntegration() error = %v", err)
	}
	restored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read the codex config after disable: %v", err)
	}
	if string(restored) != original {
		t.Fatalf("restored config = %q, want %q", restored, original)
	}
	view, err := daemon.integrations.Integration(ctx, "codex")
	if err != nil {
		t.Fatalf("Integration() error = %v", err)
	}
	if view.Enabled || view.State != "off" {
		t.Fatalf("integration after disable = %+v, want it off", view)
	}
	if _, err := daemon.integrations.VerifyIntegration(ctx, "codex"); err == nil {
		t.Fatal("VerifyIntegration() after disable error = nil, want a missing key")
	}
}
