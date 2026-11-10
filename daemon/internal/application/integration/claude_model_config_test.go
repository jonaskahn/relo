package integration

import "testing"

func TestValidateClaudeUpdateAcceptsAListedModelAndItsSavedModel(t *testing.T) {
	models := []ModelRef{{ID: "claude-relo-openai--gpt-4o"}, {ID: "claude-sonnet-4-6"}}
	if err := validateClaudeShape(ClaudePreferenceUpdate{Model: "claude-relo-openai--gpt-4o", Effort: "xhigh", AutoCompact: "200000"}); err != nil {
		t.Fatalf("listed model shape: %v", err)
	}
	if err := validateClaudeModel(ClaudePreferenceUpdate{Model: "claude-relo-openai--gpt-4o"}, "", models); err != nil {
		t.Fatalf("listed model: %v", err)
	}
	if err := validateClaudeModel(ClaudePreferenceUpdate{Model: "opus"}, "opus", models); err != nil {
		t.Fatalf("saved model: %v", err)
	}
	if err := validateClaudeShape(ClaudePreferenceUpdate{}); err != nil {
		t.Fatalf("auto: %v", err)
	}
	if err := validateClaudeModel(ClaudePreferenceUpdate{Model: "missing"}, "opus", models); err == nil {
		t.Fatal("an unlisted model was accepted")
	}
	if err := validateClaudeShape(ClaudePreferenceUpdate{Effort: "high"}); err == nil {
		t.Fatal("effort without a model was accepted")
	}
	if err := validateClaudeShape(ClaudePreferenceUpdate{Model: "opus", Effort: "max"}); err == nil {
		t.Fatal("max effort was accepted")
	}
	if err := validateClaudeShape(ClaudePreferenceUpdate{Model: "opus", AutoCompact: "99999"}); err == nil {
		t.Fatal("a window below 100000 was accepted")
	}
}

func TestClaudeEffortLevelsFollowTheModel(t *testing.T) {
	if got := claudeEffortLevels("claude-opus-4-6[1m]"); len(got) != 3 || got[2] != "high" {
		t.Fatalf("opus 4.6 = %v, want no extra-high level", got)
	}
	if got := claudeEffortLevels("claude-sonnet-4-6"); len(got) != 3 {
		t.Fatalf("sonnet 4.6 = %v, want no extra-high level", got)
	}
	if got := claudeEffortLevels("claude-opus-4-8"); len(got) != 4 || got[3] != "xhigh" {
		t.Fatalf("opus 4.8 = %v, want extra high", got)
	}
}
