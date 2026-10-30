package catalog

import "testing"

func TestResolveCapabilitiesUsesProviderAuthority(t *testing.T) {
	providerWindow, modelsWindow, manualWindow := int64(200_000), int64(100_000), int64(400_000)
	providerOutput, modelsOutput, manualOutput := int64(8_192), int64(4_096), int64(32_000)
	no, yes := false, true

	res := ResolveCapabilities(CapabilityInputs{
		AllowOverrides: true,
		Provider: FactsRecord{
			ContextWindow: &providerWindow, MaxOutput: &providerOutput,
			SupportsTools: &no, SupportsReasoning: &no, SupportsVision: &no,
		},
		ModelsDev: FactsRecord{
			ContextWindow: &modelsWindow, MaxOutput: &modelsOutput,
			SupportsTools: &yes, SupportsReasoning: &yes, SupportsVision: &yes,
		},
		Manual: FactsRecord{
			ContextWindow: &manualWindow, MaxOutput: &manualOutput,
			SupportsTools: &yes, SupportsReasoning: &yes, SupportsVision: &yes,
		},
	})

	if res.ContextWindow == nil || *res.ContextWindow != manualWindow || res.Sources.ContextWindow != "override" {
		t.Fatalf("context = %v from %q, want the operator's %d", res.ContextWindow, res.Sources.ContextWindow, manualWindow)
	}
	if res.MaxOutput == nil || *res.MaxOutput != manualOutput || res.Sources.MaxOutput != "override" {
		t.Fatalf("output = %v from %q, want the operator's %d", res.MaxOutput, res.Sources.MaxOutput, manualOutput)
	}
	if res.SupportsTools == nil || !*res.SupportsTools || res.SupportsReasoning == nil || !*res.SupportsReasoning ||
		res.SupportsVision == nil || !*res.SupportsVision {
		t.Fatalf("capabilities = %+v, want the operator's explicit true values", res)
	}
	if res.ManualInactive.ContextWindow || res.ManualInactive.MaxOutput || res.ManualInactive.Tools ||
		res.ManualInactive.Reasoning || res.ManualInactive.Vision {
		t.Fatalf("inactive manuals = %+v, want the live manual values marked active", res.ManualInactive)
	}
}

func TestResolveCapabilitiesAccountBeatsProviderAndKeepsPresence(t *testing.T) {
	accountWindow, providerWindow := int64(1_000_000), int64(200_000)
	accountOutput := int64(65_536)
	no, yes := false, true
	account := FactsRecord{
		ContextWindow: &accountWindow, MaxOutput: &accountOutput,
		SupportsTools: &no, SupportsReasoning: &yes, ReasoningEfforts: []string{},
		ReasoningToggle: &yes,
	}
	res := ResolveCapabilities(CapabilityInputs{
		Account:  &account,
		Provider: FactsRecord{ContextWindow: &providerWindow, SupportsTools: &yes, ReasoningEfforts: []string{"low", "high"}},
	})

	if res.ContextWindow == nil || *res.ContextWindow != accountWindow || res.Sources.ContextWindow != "account" {
		t.Fatalf("context = %v from %q, want the account's %d", res.ContextWindow, res.Sources.ContextWindow, accountWindow)
	}
	if res.SupportsTools == nil || *res.SupportsTools {
		t.Fatalf("tools = %v, want the account's explicit false", res.SupportsTools)
	}
	if res.ReasoningEfforts == nil || len(res.ReasoningEfforts) != 0 {
		t.Fatalf("efforts = %v, want a stated empty ladder", res.ReasoningEfforts)
	}
	if res.ReasoningToggle == nil || !*res.ReasoningToggle {
		t.Fatalf("toggle = %v, want the account's toggle", res.ReasoningToggle)
	}
}

func TestResolveCapabilitiesToggleOnlyDoesNotInheritLadder(t *testing.T) {
	yes := true
	res := ResolveCapabilities(CapabilityInputs{
		Provider:  FactsRecord{SupportsReasoning: &yes, ReasoningToggle: &yes},
		ModelsDev: FactsRecord{SupportsReasoning: &yes, ReasoningEfforts: []string{"low", "medium", "high"}},
	})
	if res.ReasoningEfforts != nil {
		t.Fatalf("efforts = %v, want nil so a toggle-only provider inherits no ladder", res.ReasoningEfforts)
	}
	if res.ReasoningToggle == nil || !*res.ReasoningToggle {
		t.Fatalf("toggle = %v, want the provider's toggle", res.ReasoningToggle)
	}
}

func TestResolveCapabilitiesFallsThroughFieldByField(t *testing.T) {
	modelsWindow, manualOutput := int64(100_000), int64(16_000)
	res := ResolveCapabilities(CapabilityInputs{
		AllowOverrides: true,
		ModelsDev:      FactsRecord{ContextWindow: &modelsWindow},
		Manual:         FactsRecord{MaxOutput: &manualOutput},
	})
	if res.ContextWindow == nil || *res.ContextWindow != modelsWindow || res.Sources.ContextWindow != "modelsdev" {
		t.Fatalf("context = %v from %q, want models.dev", res.ContextWindow, res.Sources.ContextWindow)
	}
	if res.MaxOutput == nil || *res.MaxOutput != manualOutput || res.Sources.MaxOutput != "override" {
		t.Fatalf("output = %v from %q, want manual completion", res.MaxOutput, res.Sources.MaxOutput)
	}
	if !res.Editable.ContextWindow || !res.Editable.MaxOutput {
		t.Fatalf("editable = %+v, want manual values saved regardless of the detected layers", res.Editable)
	}
}

func TestResolveCapabilitiesLeavesManualValuesDormantWhenOverridesAreOff(t *testing.T) {
	manualWindow, providerWindow := int64(400_000), int64(200_000)
	yes := true
	res := ResolveCapabilities(CapabilityInputs{
		Provider: FactsRecord{ContextWindow: &providerWindow},
		Manual:   FactsRecord{ContextWindow: &manualWindow, SupportsTools: &yes},
	})
	if res.ContextWindow == nil || *res.ContextWindow != providerWindow || res.Sources.ContextWindow != "provider" {
		t.Fatalf("context = %v from %q, want the provider's %d", res.ContextWindow, res.Sources.ContextWindow, providerWindow)
	}
	if res.SupportsTools != nil {
		t.Fatalf("tools = %v, want a dormant manual value to fill nothing", res.SupportsTools)
	}
	if !res.ManualInactive.ContextWindow || !res.ManualInactive.Tools {
		t.Fatalf("inactive manuals = %+v, want the stored values marked inactive", res.ManualInactive)
	}
}

func TestResolveCapabilitiesManualReasoningKeepsTheDetectedLadder(t *testing.T) {
	yes := true
	res := ResolveCapabilities(CapabilityInputs{
		AllowOverrides: true,
		Provider:       FactsRecord{SupportsReasoning: &yes, ReasoningEfforts: []string{"low", "high"}, ReasoningToggle: &yes},
		Manual:         FactsRecord{SupportsReasoning: &yes},
	})
	if res.Sources.Reasoning != "override" {
		t.Fatalf("reasoning source = %q, want the manual flag", res.Sources.Reasoning)
	}
	if len(res.ReasoningEfforts) != 2 || res.Sources.ReasoningEfforts != "provider" {
		t.Fatalf("efforts = %v from %q, want the detected ladder", res.ReasoningEfforts, res.Sources.ReasoningEfforts)
	}
	if res.ReasoningToggle == nil || !*res.ReasoningToggle {
		t.Fatalf("toggle = %v, want the detected toggle", res.ReasoningToggle)
	}
}

func TestResolveCapabilitiesManualReasoningOffClearsTheDetectedFamily(t *testing.T) {
	yes, no := true, false
	res := ResolveCapabilities(CapabilityInputs{
		AllowOverrides: true,
		Provider: FactsRecord{
			SupportsReasoning: &yes, ReasoningEfforts: []string{"low", "high"},
			ReasoningToggle: &yes, ReasoningBudget: &yes,
		},
		Manual: FactsRecord{SupportsReasoning: &no},
	})
	if res.SupportsReasoning == nil || *res.SupportsReasoning {
		t.Fatalf("SupportsReasoning = %v, want the operator's off switch", res.SupportsReasoning)
	}
	if len(res.ReasoningEfforts) != 0 || res.ReasoningToggle == nil || *res.ReasoningToggle ||
		res.ReasoningBudget == nil || *res.ReasoningBudget {
		t.Fatalf("reasoning = %+v, want the family cleared", res)
	}
	if res.Sources.Reasoning != "override" || res.Sources.ReasoningEfforts != "override" ||
		res.Sources.ReasoningToggle != "override" || res.Sources.ReasoningBudget != "override" ||
		res.Sources.ReasoningBudgetMin != "override" || res.Sources.ReasoningBudgetMax != "override" {
		t.Fatalf("sources = %+v, want the manual layer named", res.Sources)
	}
}

func TestConservativeCapabilitiesKeepsUnknownUnknown(t *testing.T) {
	narrow, wide := int64(100_000), int64(200_000)
	yes := true
	res := ConservativeCapabilities([]Model{
		{ContextWindow: &narrow, MaxInput: &narrow, SupportsTools: &yes, ReasoningEfforts: []string{"low", "high"}},
		{ContextWindow: &wide, SupportsTools: &yes, ReasoningEfforts: []string{"high", "max"}},
	})
	if res.ContextWindow == nil || *res.ContextWindow != narrow {
		t.Fatalf("context = %v, want the smaller stated window", res.ContextWindow)
	}
	if res.MaxInput != nil {
		t.Fatalf("max input = %v, want unknown because one member is unknown", res.MaxInput)
	}
	if len(res.ReasoningEfforts) != 1 || res.ReasoningEfforts[0] != "high" {
		t.Fatalf("efforts = %v, want the intersection", res.ReasoningEfforts)
	}
}

func TestClientReasoningProjectsToggleOnly(t *testing.T) {
	yes := true
	model := Model{SupportsReasoning: &yes, ReasoningEfforts: []string{}, ReasoningToggle: true, ReasoningToggleKnown: true}
	if !ToggleOnly(model) {
		t.Fatal("a reasoned empty-ladder toggle model is toggle-only")
	}
	if got := ClientReasoningEfforts(model); len(got) != 2 || got[0] != "none" || got[1] != "high" {
		t.Fatalf("efforts = %v, want the synthetic none/high pair", got)
	}
	plain := Model{SupportsReasoning: &yes, ReasoningEfforts: []string{}, ReasoningToggleKnown: true}
	if ToggleOnly(plain) || ClientReasoningEfforts(plain) == nil {
		t.Fatalf("a non-toggle empty ladder = %v, want no synthetic projection", ClientReasoningEfforts(plain))
	}
}

func TestSharedReasoningIntersectsRoutes(t *testing.T) {
	yes := true
	toggleOnly := Model{SupportsReasoning: &yes, ReasoningEfforts: []string{}, ReasoningToggle: true, ReasoningToggleKnown: true}
	stated := Model{SupportsReasoning: &yes, ReasoningEfforts: []string{"low", "high", "max"}, ReasoningToggleKnown: true}
	noEffort := Model{SupportsReasoning: &yes, ReasoningEfforts: []string{}, ReasoningToggleKnown: true}

	efforts, toggle := SharedReasoning([]Model{toggleOnly, stated})
	if len(efforts) != 3 || efforts[0] != "low" || efforts[2] != "max" || toggle {
		t.Fatalf("mixed efforts = %v toggle %v, want low/high/max and no shared toggle", efforts, toggle)
	}
	efforts, toggle = SharedReasoning([]Model{toggleOnly, toggleOnly})
	if len(efforts) != 2 || efforts[0] != "none" || efforts[1] != "high" || !toggle {
		t.Fatalf("toggle-only efforts = %v toggle %v, want none/high", efforts, toggle)
	}
	efforts, _ = SharedReasoning([]Model{stated, noEffort})
	if len(efforts) != 0 {
		t.Fatalf("efforts = %v, want an empty intersection", efforts)
	}
}
