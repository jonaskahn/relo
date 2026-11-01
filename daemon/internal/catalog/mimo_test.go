package catalog

import (
	"slices"
	"testing"
)

func TestIsMimoReasoningText(t *testing.T) {
	for _, id := range []string{
		"mimo-v2.6-flash", "mimo-v2-pro-free", "mimo-v2-omni-free", "mimo-v25",
		"mimo-v2-5-pro", "mimo-v2.6-pro-ultraspeed", "mimo-v2.6-pro-rl",
		"xiaomi/mimo-v2.6-pro", "XiaomiMiMo/MiMo-V2.6-Flash", "xiaomimimo/mimo-v2.5",
	} {
		if !IsMimoReasoningText(id) {
			t.Errorf("IsMimoReasoningText(%q) = false, want a MiMo chat model", id)
		}
	}
	for _, id := range []string{
		"mimo-v2.5-tts-voicedesign", "mimo-v2.5-tts-voiceclone", "mimo-asr",
		"mimo-v2.5-tts", "gpt-5", "", "mimo-v",
	} {
		if IsMimoReasoningText(id) {
			t.Errorf("IsMimoReasoningText(%q) = true, want only MiMo chat models", id)
		}
	}
}

func TestApplyMimoFamily(t *testing.T) {
	yes, no := true, false
	stated := func(efforts ...string) []string { return efforts }
	cases := []struct {
		name     string
		res      Model
		wantReas bool
		wantLad  []string
		wantTog  bool
	}{
		{
			name:     "the opencode shape states no toggle",
			res:      Model{ID: "mimo-v2.6-flash-free", SupportsReasoning: &yes, ReasoningEfforts: []string{}},
			wantReas: true, wantLad: []string{}, wantTog: true,
		},
		{
			name:     "a listing-only model resolves as reasoning",
			res:      Model{ID: "mimo-v2.6-flash"},
			wantReas: true, wantLad: []string{}, wantTog: true,
		},
		{
			name:     "a clone resolves on its upstream id",
			res:      Model{ID: "my-alias", UpstreamID: "MiMo-V2.6-Pro"},
			wantReas: true, wantLad: []string{}, wantTog: true,
		},
		{
			name:     "a stated toggle is unchanged",
			res:      Model{ID: "mimo-v2.6-pro", SupportsReasoning: &yes, ReasoningEfforts: []string{}, ReasoningToggle: true, ReasoningToggleKnown: true},
			wantReas: true, wantLad: []string{}, wantTog: true,
		},
		{
			name:     "a stated off/on ladder is kept",
			res:      Model{ID: "xiaomi/mimo-v2.6-pro", SupportsReasoning: &yes, ReasoningEfforts: stated("none", "high")},
			wantReas: true, wantLad: []string{"none", "high"},
		},
		{
			name:     "a stated full ladder is kept",
			res:      Model{ID: "mimo-v2.6-flash", SupportsReasoning: &yes, ReasoningEfforts: stated("low", "medium", "high", "max")},
			wantReas: true, wantLad: []string{"low", "medium", "high", "max"},
		},
		{
			name:     "an explicit opt-out is kept",
			res:      Model{ID: "mimo-v2.5", SupportsReasoning: &no, ReasoningEfforts: []string{}},
			wantReas: false, wantLad: []string{},
		},
		{
			name:     "a speech variant is not reasoning",
			res:      Model{ID: "mimo-v2.5-tts", SupportsReasoning: &no},
			wantReas: false, wantLad: nil,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			res := test.res
			applyMimoFamily(&res)
			if res.SupportsReasoning == nil || *res.SupportsReasoning != test.wantReas {
				t.Fatalf("SupportsReasoning = %v, want %v", res.SupportsReasoning, test.wantReas)
			}
			if !slices.Equal(res.ReasoningEfforts, test.wantLad) {
				t.Fatalf("ReasoningEfforts = %v, want %v", res.ReasoningEfforts, test.wantLad)
			}
			if res.ReasoningToggle != test.wantTog || res.ReasoningToggleKnown != test.wantTog {
				t.Fatalf("toggle = %v known %v, want %v", res.ReasoningToggle, res.ReasoningToggleKnown, test.wantTog)
			}
		})
	}
}

func TestMimoToolCallDialect(t *testing.T) {
	for _, res := range []Model{
		{ID: "mimo-v2.6-flash"},
		{ID: "alias", UpstreamID: "mimo-v2.6-pro"},
	} {
		if got := mimoToolCallDialect(res); got != ToolCallDialectText {
			t.Errorf("mimoToolCallDialect(%+v) = %q, want %q", res, got, ToolCallDialectText)
		}
	}
	for _, res := range []Model{{ID: "mimo-asr"}, {ID: "gpt-5"}, {ID: "mimo-v2.5-tts-voicedesign"}} {
		if got := mimoToolCallDialect(res); got != "" {
			t.Errorf("mimoToolCallDialect(%+v) = %q, want none", res, got)
		}
	}
}
