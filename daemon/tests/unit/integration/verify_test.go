package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/application/integration"
)

func contributions(t *testing.T, h *harness, id string) []integration.VerifyCheck {
	t.Helper()
	checks, err := h.manager.VerifyContributions(context.Background(), id)
	if err != nil {
		t.Fatalf("VerifyContributions(%s) error = %v", id, err)
	}
	return checks
}

func checkNamed(t *testing.T, checks []integration.VerifyCheck, name string) integration.VerifyCheck {
	t.Helper()
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("no %q check in %v", name, checks)
	return integration.VerifyCheck{}
}

// TestVerifyContributionsRegeneratesWhatIsMissing is the promise a verify
// makes: a deleted settings file and a deleted shell block come back, and a
// file that is present but different is reported rather than rewritten.
func TestVerifyContributionsRegeneratesWhatIsMissing(t *testing.T) {
	h := newHarness(t, dataPlane)
	if _, err := h.manager.Enable(context.Background(), "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	for _, check := range contributions(t, h, "codex") {
		if !check.OK || check.Fixed {
			t.Fatalf("check %+v, want every check passing with nothing fixed", check)
		}
	}

	configPath := h.paths.CodexConfig()
	if err := os.Remove(configPath); err != nil {
		t.Fatalf("remove the codex config: %v", err)
	}
	settings := checkNamed(t, contributions(t, h, "codex"), integration.CheckSettings)
	if !settings.OK || !settings.Fixed {
		t.Fatalf("settings check = %+v, want passing and fixed", settings)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("the codex config was not written again: %v", err)
	}

	zshenv := filepath.Join(h.paths.Home, ".zshenv")
	content, err := os.ReadFile(zshenv)
	if err != nil {
		t.Fatalf("read .zshenv: %v", err)
	}
	stripped := codingclients.StripEnvFile(string(content), "codex")
	if err := os.WriteFile(zshenv, []byte(stripped), 0o644); err != nil {
		t.Fatalf("strip the .zshenv block: %v", err)
	}
	env := checkNamed(t, contributions(t, h, "codex"), integration.CheckEnv)
	if !env.OK || !env.Fixed {
		t.Fatalf("env check = %+v, want passing and fixed", env)
	}
	if !codingclients.EnvPublished(h.paths, codingclients.Agent{ID: "codex", EnvKey: codingclients.EnvKeyName("codex")}) {
		t.Fatal("EnvPublished() = false, want the block published again")
	}

	if err := os.WriteFile(configPath, []byte("approval_policy = \"never\"\n"), 0o644); err != nil {
		t.Fatalf("drift the codex config: %v", err)
	}
	config := checkNamed(t, contributions(t, h, "codex"), integration.CheckConfig)
	if config.OK {
		t.Fatal("config check passes, want it to report the changed file")
	}
	drifted := readFile(t, configPath)
	if drifted != "approval_policy = \"never\"\n" {
		t.Fatalf("drifted config = %q, want it left alone", drifted)
	}
}

// TestVerifyContributionsSkipsFilesForManualAgents is the clients Relo hands
// a key: the login environment is still checked, and there is no settings
// file or block to check.
func TestVerifyContributionsSkipsFilesForManualAgents(t *testing.T) {
	h := newHarness(t, dataPlane)
	if _, err := h.manager.Enable(context.Background(), "cursor", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	checks := contributions(t, h, "cursor")
	if len(checks) != 1 || checks[0].Name != integration.CheckEnv {
		t.Fatalf("checks = %v, want only the env check", checks)
	}
	if !checks[0].OK {
		t.Fatalf("env check = %+v, want passing", checks[0])
	}
}
