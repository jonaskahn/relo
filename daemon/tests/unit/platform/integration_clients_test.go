package platform_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
	"github.com/jonaskahn/relo/internal/platform"
)

func TestClientFilesReportWhetherTheGatewayCacheListsTheModels(t *testing.T) {
	const base = "http://127.0.0.1:10202"
	dir := t.TempDir()
	files := platform.NewClientFiles(codingclients.Paths{Home: t.TempDir()})
	models := []appintegration.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o"}}
	if files.GatewayCacheLists(dir, base, models) {
		t.Fatal("GatewayCacheLists() = true before a cache was written")
	}
	if err := files.WriteGatewayCache(dir, base, models); err != nil {
		t.Fatalf("WriteGatewayCache() error = %v", err)
	}
	if !files.GatewayCacheLists(dir, base, models) {
		t.Fatal("GatewayCacheLists() = false for the cache just written")
	}
}

// TestClientFilesForwardTheDesktopBehaviorComparison pins the port the
// integration workflows read: the rendered rows, including behavesAs, decide
// whether a profile is current.
func TestClientFilesForwardTheDesktopBehaviorComparison(t *testing.T) {
	const base = "http://127.0.0.1:10202"
	files := platform.NewClientFiles(codingclients.Paths{Home: t.TempDir()})
	models := []appintegration.ModelRef{{
		ID: "claude-relo-openai--gpt-5.5-pro", Name: "GPT Pro",
		Connection: "OpenAI", Upstream: "gpt-5.5-pro",
	}}
	content, err := files.DesktopProfileContent(base, "rlo_ak_test", models)
	if err != nil {
		t.Fatalf("DesktopProfileContent() error = %v", err)
	}
	if !files.DesktopProfileLists(string(content), base, models) {
		t.Fatal("DesktopProfileLists() = false for the profile just rendered")
	}
	changed := []appintegration.ModelRef{{
		ID: "claude-relo-openai--gpt-5.5-mini", Name: "GPT Mini",
		Connection: "OpenAI", Upstream: "gpt-5.5-mini",
	}}
	if files.DesktopProfileLists(string(content), base, changed) {
		t.Fatal("DesktopProfileLists() = true for a different model roster")
	}
}

func TestClientFilesRoundTripClaudePreferences(t *testing.T) {
	files := platform.NewClientFiles(codingclients.Paths{Home: t.TempDir()})
	updated, err := files.UpdateClaudePreferences(`{"env":{"CLAUDE_CODE_SUBAGENT_MODEL":"haiku"}}`+"\n", appintegration.ClaudePreferenceUpdate{
		Model: "opus", SubagentModel: "opusplan",
	})
	if err != nil {
		t.Fatalf("UpdateClaudePreferences() error = %v", err)
	}
	read, err := files.ReadClaudePreferences(updated)
	if err != nil {
		t.Fatalf("ReadClaudePreferences() error = %v", err)
	}
	if read.Model != "opus" || read.SubagentModel != "opusplan" {
		t.Fatalf("read = %+v, want opus and the opusplan alias", read)
	}
	kept, err := files.ClearReloClaudePreferences(updated)
	if err != nil {
		t.Fatalf("ClearReloClaudePreferences() error = %v", err)
	}
	if !strings.Contains(kept, `"CLAUDE_CODE_SUBAGENT_MODEL": "opusplan"`) {
		t.Fatalf("settings = %s, want the alias kept", kept)
	}
}

func TestAgentProcessesDriveTheOpenCodeLauncher(t *testing.T) {
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + marker + "\nif [ \"$1 $2\" = \"service get\" ]; then echo stored-key; fi\n"
	if err := os.WriteFile(filepath.Join(binDir, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatalf("write the stub launcher: %v", err)
	}
	t.Setenv("PATH", binDir)
	processes := platform.NewAgentProcesses()
	ctx := context.Background()
	home := t.TempDir()
	if err := processes.ReloadOpenCodeService(ctx, home); err != nil {
		t.Fatalf("ReloadOpenCodeService() error = %v", err)
	}
	key, err := processes.OpenCodeServiceKey(ctx, home, "RELO_OPENCODE_API_KEY")
	if err != nil || key != "stored-key" {
		t.Fatalf("OpenCodeServiceKey() = %q, %v, want the stored key", key, err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read the stub marker: %v", err)
	}
	if want := "reload\nservice get env RELO_OPENCODE_API_KEY\n"; string(got) != want {
		t.Fatalf("launcher args = %q, want %q", got, want)
	}
}
