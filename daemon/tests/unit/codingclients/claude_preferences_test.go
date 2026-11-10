package codingclients_test

import (
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

func TestClaudePreferencesRoundTripLeavesOtherSettings(t *testing.T) {
	existing := `{
  "permissions": {"allow": ["Bash(npm run lint)"]},
  "env": {"ANTHROPIC_BASE_URL": "http://127.0.0.1:1", "CLAUDE_CODE_SUBAGENT_MODEL": 1},
  "model": "opus",
  "effortLevel": "high",
  "modelSettings": {
    "opus": {"effortLevel": "medium", "maxEffortLevel": "high"},
    "claude-sonnet-4-5": {"autoCompactWindow": 200000}
  }
}`
	read, err := codingclients.ReadClaudePreferences(existing)
	if err != nil {
		t.Fatalf("ReadClaudePreferences() error = %v", err)
	}
	if read.SubagentModel != "" {
		t.Fatalf("read = %+v, want a non-text subagent model ignored", read)
	}
	updated, err := codingclients.UpdateClaudePreferences(existing, codingclients.ClaudePreferenceUpdate{
		Model: "claude-relo-openai--gpt-4o", SubagentModel: "claude-sonnet-4-5", Effort: "xhigh", AutoCompact: "250000",
	})
	if err != nil {
		t.Fatalf("UpdateClaudePreferences() error = %v", err)
	}
	for _, want := range []string{`"allow"`, `"ANTHROPIC_BASE_URL"`, `"CLAUDE_CODE_SUBAGENT_MODEL": "claude-sonnet-4-5"`, `"effortLevel": "high"`, `"maxEffortLevel": "high"`, `"autoCompactWindow": 200000`} {
		if !strings.Contains(updated, want) {
			t.Fatalf("settings = %s, want %s kept", updated, want)
		}
	}
	if strings.Contains(updated, `"model": "opus"`) {
		t.Fatalf("settings = %s, want the selected model to replace opus", updated)
	}
	read, err = codingclients.ReadClaudePreferences(updated)
	if err != nil {
		t.Fatalf("ReadClaudePreferences() error = %v", err)
	}
	if read.Model != "claude-relo-openai--gpt-4o" || read.SubagentModel != "claude-sonnet-4-5" || read.Effort != "xhigh" || read.AutoCompact != "250000" {
		t.Fatalf("read = %+v, want the saved choice", read)
	}
	if read.Entries["opus"].Effort != "medium" || read.Entries["claude-sonnet-4-5"].AutoCompact != "200000" {
		t.Fatalf("entries = %+v, want the other models kept", read.Entries)
	}

	cleared, err := codingclients.UpdateClaudePreferences(updated, codingclients.ClaudePreferenceUpdate{
		Model: "claude-relo-openai--gpt-4o",
	})
	if err != nil {
		t.Fatalf("clear the overrides: %v", err)
	}
	read, err = codingclients.ReadClaudePreferences(cleared)
	if err != nil {
		t.Fatalf("ReadClaudePreferences() error = %v", err)
	}
	if read.Effort != "" || read.AutoCompact != "auto" {
		t.Fatalf("read = %+v, want the model default", read)
	}
	if _, found := read.Entries["claude-relo-openai--gpt-4o"]; found {
		t.Fatalf("entries = %+v, want the empty override removed", read.Entries)
	}
	if !strings.Contains(cleared, `"model": "claude-relo-openai--gpt-4o"`) || !strings.Contains(cleared, `"ANTHROPIC_BASE_URL"`) {
		t.Fatalf("settings = %s, want the model and gateway address kept", cleared)
	}
	if strings.Contains(cleared, "CLAUDE_CODE_SUBAGENT_MODEL") {
		t.Fatalf("settings = %s, want the subagent model cleared", cleared)
	}
}

func TestClaudePreferencesAutoClearsOnlyTheDefaultModel(t *testing.T) {
	updated, err := codingclients.UpdateClaudePreferences(`{"model":"opus","modelSettings":{"opus":{"effortLevel":"low"}}}`+"\n", codingclients.ClaudePreferenceUpdate{})
	if err != nil {
		t.Fatalf("UpdateClaudePreferences() error = %v", err)
	}
	read, err := codingclients.ReadClaudePreferences(updated)
	if err != nil {
		t.Fatalf("ReadClaudePreferences() error = %v", err)
	}
	if read.Model != "" || read.Entries["opus"].Effort != "low" {
		t.Fatalf("read = %+v, want no default and the saved opus effort", read)
	}
}

func TestClaudePreferencesRejectsABadShape(t *testing.T) {
	for _, content := range []string{`{"model":1}`, `{"env":[]}`, `{"modelSettings":[]}`, `{"modelSettings":{"opus":"high"}}`} {
		if _, err := codingclients.ReadClaudePreferences(content); err == nil {
			t.Fatalf("ReadClaudePreferences(%s) accepted a shape Relo cannot edit", content)
		}
	}
	if _, err := codingclients.UpdateClaudePreferences(`{}`, codingclients.ClaudePreferenceUpdate{Effort: "max"}); err == nil {
		t.Fatal("an effort without a model was accepted")
	}
	if _, err := codingclients.UpdateClaudePreferences(`{}`, codingclients.ClaudePreferenceUpdate{Model: "opus", Effort: "max"}); err == nil {
		t.Fatal("max effort was accepted")
	}
	if _, err := codingclients.UpdateClaudePreferences(`{}`, codingclients.ClaudePreferenceUpdate{Model: "opus", AutoCompact: "200"}); err == nil {
		t.Fatal("a window below 100000 was accepted")
	}
	if _, err := codingclients.UpdateClaudePreferences(`{"env":[]}`, codingclients.ClaudePreferenceUpdate{SubagentModel: "haiku"}); err == nil {
		t.Fatal("a subagent model was written into an env list")
	}
	if _, err := codingclients.ClearReloClaudePreferences(`{"env":[]}`); err == nil {
		t.Fatal("ClearReloClaudePreferences accepted an env list")
	}
}

func TestClaudeSubagentAliasClearsWhenItIsTheOnlyEnv(t *testing.T) {
	updated, err := codingclients.UpdateClaudePreferences("{}\n", codingclients.ClaudePreferenceUpdate{SubagentModel: "haiku"})
	if err != nil {
		t.Fatalf("UpdateClaudePreferences() error = %v", err)
	}
	if !strings.Contains(updated, `"CLAUDE_CODE_SUBAGENT_MODEL": "haiku"`) {
		t.Fatalf("settings = %s, want the haiku alias", updated)
	}
	cleared, err := codingclients.UpdateClaudePreferences(updated, codingclients.ClaudePreferenceUpdate{})
	if err != nil {
		t.Fatalf("clear the alias: %v", err)
	}
	if strings.Contains(cleared, "env") || strings.Contains(cleared, "CLAUDE_CODE_SUBAGENT_MODEL") {
		t.Fatalf("settings = %s, want the empty env removed", cleared)
	}
	kept, err := codingclients.ClearReloClaudePreferences(`{"env":{"CLAUDE_CODE_SUBAGENT_MODEL":1}}` + "\n")
	if err != nil {
		t.Fatalf("ClearReloClaudePreferences() error = %v", err)
	}
	if !strings.Contains(kept, "CLAUDE_CODE_SUBAGENT_MODEL") {
		t.Fatalf("settings = %s, want a non-text subagent model kept", kept)
	}
}

func TestClearReloClaudePreferencesKeepsNativeModels(t *testing.T) {
	content := `{
  "model": "claude-relo-openai--gpt-4o[1m]",
  "env": {"ANTHROPIC_BASE_URL": "http://127.0.0.1:1", "CLAUDE_CODE_SUBAGENT_MODEL": "claude-reloc-combo"},
  "permissions": {"allow": ["Bash(make ci)"]},
  "modelSettings": {
    "claude-relo-openai--gpt-4o[1m]": {"effortLevel": "high"},
    "claude-reloc-combo[1m]": {"autoCompactWindow": 300000},
    "opus": {"effortLevel": "low"}
  }
}`
	cleared, err := codingclients.ClearReloClaudePreferences(content)
	if err != nil {
		t.Fatalf("ClearReloClaudePreferences() error = %v", err)
	}
	if strings.Contains(cleared, "claude-relo-") || strings.Contains(cleared, "claude-reloc-") || strings.Contains(cleared, "CLAUDE_CODE_SUBAGENT_MODEL") {
		t.Fatalf("settings = %s, want Relo models gone", cleared)
	}
	if !strings.Contains(cleared, "ANTHROPIC_BASE_URL") {
		t.Fatalf("settings = %s, want the gateway address kept", cleared)
	}
	kept, err := codingclients.ClearReloClaudePreferences(`{"env":{"CLAUDE_CODE_SUBAGENT_MODEL":"opus"},"model":"opus"}` + "\n")
	if err != nil {
		t.Fatalf("ClearReloClaudePreferences() error = %v", err)
	}
	if !strings.Contains(kept, `"CLAUDE_CODE_SUBAGENT_MODEL": "opus"`) {
		t.Fatalf("settings = %s, want the native subagent model kept", kept)
	}
	for _, want := range []string{`"allow"`, `"modelSettings"`, `"opus"`, `"effortLevel": "low"`} {
		if !strings.Contains(cleared, want) {
			t.Fatalf("settings = %s, want %s kept", cleared, want)
		}
	}
}
