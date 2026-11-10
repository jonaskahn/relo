package codingclients_test

import (
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

func TestClaudePreferencesRoundTripLeavesOtherSettings(t *testing.T) {
	existing := `{
  "permissions": {"allow": ["Bash(npm run lint)"]},
  "model": "opus",
  "effortLevel": "high",
  "modelSettings": {
    "opus": {"effortLevel": "medium", "maxEffortLevel": "high"},
    "claude-sonnet-4-5": {"autoCompactWindow": 200000}
  }
}`
	updated, err := codingclients.UpdateClaudePreferences(existing, codingclients.ClaudePreferenceUpdate{
		Model: "claude-relo-openai--gpt-4o", Effort: "xhigh", AutoCompact: "250000",
	})
	if err != nil {
		t.Fatalf("UpdateClaudePreferences() error = %v", err)
	}
	for _, want := range []string{`"allow"`, `"effortLevel": "high"`, `"maxEffortLevel": "high"`, `"claude-sonnet-4-5"`, `"autoCompactWindow": 200000`} {
		if !strings.Contains(updated, want) {
			t.Fatalf("settings = %s, want %s kept", updated, want)
		}
	}
	if strings.Contains(updated, `"model": "opus"`) {
		t.Fatalf("settings = %s, want the selected model to replace opus", updated)
	}
	read, err := codingclients.ReadClaudePreferences(updated)
	if err != nil {
		t.Fatalf("ReadClaudePreferences() error = %v", err)
	}
	if read.Model != "claude-relo-openai--gpt-4o" || read.Effort != "xhigh" || read.AutoCompact != "250000" {
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
	if !strings.Contains(cleared, `"model": "claude-relo-openai--gpt-4o"`) {
		t.Fatalf("settings = %s, want the model kept", cleared)
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
}

func TestClearReloClaudePreferencesKeepsNativeModels(t *testing.T) {
	content := `{
  "model": "claude-relo-openai--gpt-4o[1m]",
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
	if strings.Contains(cleared, "claude-relo-") || strings.Contains(cleared, "claude-reloc-") {
		t.Fatalf("settings = %s, want Relo models gone", cleared)
	}
	for _, want := range []string{`"allow"`, `"modelSettings"`, `"opus"`, `"effortLevel": "low"`} {
		if !strings.Contains(cleared, want) {
			t.Fatalf("settings = %s, want %s kept", cleared, want)
		}
	}
}
