package codingclients_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

func TestEnvKeyNameIsPerClient(t *testing.T) {
	if got := codingclients.EnvKeyName("opencode"); got != "RELO_OPENCODE_API_KEY" {
		t.Fatalf("EnvKeyName(opencode) = %q", got)
	}
	if got := codingclients.EnvKeyName("claude-code"); got != "RELO_CLAUDE_CODE_API_KEY" {
		t.Fatalf("EnvKeyName(claude-code) = %q", got)
	}
}

func TestEveryAgentPublishesAnEnvKey(t *testing.T) {
	for _, agent := range codingclients.Agents() {
		want := codingclients.EnvKeyName(agent.ID)
		if agent.EnvKey != want {
			t.Fatalf("%s EnvKey = %q, want %q", agent.ID, agent.EnvKey, want)
		}
	}
}

func TestEnvFileKeepsPerClientBlocks(t *testing.T) {
	opencode := codingclients.MergeEnvFile("# keep\n", "opencode", "/tmp/opencode/key")
	if !strings.Contains(opencode, "# keep") || !strings.Contains(opencode, "RELO_OPENCODE_API_KEY") {
		t.Fatalf("opencode block = %s, want the operator line and Relo's export", opencode)
	}
	both := codingclients.MergeEnvFile(opencode, "hermes", "/tmp/hermes/key")
	if !strings.Contains(both, "RELO_OPENCODE_API_KEY") || !strings.Contains(both, "RELO_HERMES_API_KEY") {
		t.Fatalf("merged = %s, want both variables", both)
	}
	again := codingclients.MergeEnvFile(both, "opencode", "/tmp/opencode/key")
	if strings.Count(again, "RELO_OPENCODE_API_KEY") != 1 {
		t.Fatalf("second merge = %s, want one OpenCode block", again)
	}
	stripped := codingclients.StripEnvFile(again, "opencode")
	if strings.Contains(stripped, "RELO_OPENCODE_API_KEY") || !strings.Contains(stripped, "RELO_HERMES_API_KEY") || !strings.Contains(stripped, "# keep") {
		t.Fatalf("stripped = %s, want Hermes and the operator line", stripped)
	}
	empty := codingclients.StripEnvFile(codingclients.EnvBlock("hermes", "/tmp/hermes/key"), "hermes")
	if empty != "" {
		t.Fatalf("empty strip = %q, want no leftover", empty)
	}
}

// TestEnvPublishedFollowsTheShellBlock is what a verification reads: the
// variable counts as published while the fenced block is in the shell files,
// and missing as soon as one of them loses it.
func TestEnvPublishedFollowsTheShellBlock(t *testing.T) {
	home := t.TempDir()
	paths := codingclients.Paths{Home: home, StateHome: t.TempDir()}
	agent, found := codingclients.AgentFor("codex")
	if !found {
		t.Fatal("AgentFor(codex) not found")
	}
	if codingclients.EnvPublished(paths, agent) {
		t.Fatal("EnvPublished() = true, want false before anything is published")
	}
	if err := codingclients.PublishEnv(paths, agent); err != nil {
		t.Fatalf("PublishEnv() error = %v", err)
	}
	if !codingclients.EnvPublished(paths, agent) {
		t.Fatal("EnvPublished() = false, want true after PublishEnv")
	}
	bashrc := filepath.Join(home, ".bashrc")
	content, err := os.ReadFile(bashrc)
	if err != nil {
		t.Fatalf("read .bashrc: %v", err)
	}
	stripped := codingclients.StripEnvFile(string(content), agent.ID)
	if err := os.WriteFile(bashrc, []byte(stripped), 0o644); err != nil {
		t.Fatalf("strip the .bashrc block: %v", err)
	}
	if codingclients.EnvPublished(paths, agent) {
		t.Fatal("EnvPublished() = true, want false after one shell file loses the block")
	}
	if err := codingclients.PublishEnv(paths, agent); err != nil {
		t.Fatalf("PublishEnv() error = %v", err)
	}
	if !codingclients.EnvPublished(paths, agent) {
		t.Fatal("EnvPublished() = false, want true after publishing again")
	}
}

// TestEnvPublishedWithoutAVariable is the agents that read no variable: there
// is nothing to lose, so the check passes.
func TestEnvPublishedWithoutAVariable(t *testing.T) {
	paths := codingclients.Paths{Home: t.TempDir()}
	if !codingclients.EnvPublished(paths, codingclients.Agent{ID: "none"}) {
		t.Fatal("EnvPublished() = false, want true for an agent with no variable")
	}
}

func TestEnvCoversLoginShellsAndFish(t *testing.T) {
	home := t.TempDir()
	paths := codingclients.Paths{Home: home, StateHome: t.TempDir()}
	agent, found := codingclients.AgentFor("codex")
	if !found {
		t.Fatal("AgentFor(codex) not found")
	}
	if err := codingclients.PublishEnv(paths, agent); err != nil {
		t.Fatalf("PublishEnv() error = %v", err)
	}
	for _, name := range []string{".zshenv", ".zprofile", ".bashrc", ".bash_profile", ".profile"} {
		content, err := os.ReadFile(filepath.Join(home, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(content), "RELO_CODEX_API_KEY") {
			t.Fatalf("%s = %s, want the codex variable", name, content)
		}
	}
	fish, err := os.ReadFile(filepath.Join(home, ".config", "fish", "conf.d", "relo.fish"))
	if err != nil {
		t.Fatalf("read relo.fish: %v", err)
	}
	if !strings.Contains(string(fish), "set -gx RELO_CODEX_API_KEY") {
		t.Fatalf("relo.fish = %s, want fish syntax", fish)
	}
	if err := codingclients.UnpublishEnv(paths, agent); err != nil {
		t.Fatalf("UnpublishEnv() error = %v", err)
	}
	for _, name := range []string{".zshenv", ".zprofile", ".bashrc", ".bash_profile", ".profile", filepath.Join(".config", "fish", "conf.d", "relo.fish")} {
		if _, err := os.Stat(filepath.Join(home, name)); !os.IsNotExist(err) {
			t.Fatalf("%s after unpublish = %v, want removed", name, err)
		}
	}
	if codingclients.EnvPublished(paths, agent) {
		t.Fatal("EnvPublished() = true, want false after UnpublishEnv")
	}
}
