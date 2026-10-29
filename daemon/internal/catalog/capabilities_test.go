package catalog

import "testing"

func TestResolveCapabilitiesUsesProviderAuthority(t *testing.T) {
	providerWindow, modelsWindow, manualWindow := int64(200_000), int64(100_000), int64(400_000)
	providerOutput, modelsOutput, manualOutput := int64(8_192), int64(4_096), int64(32_000)
	no, yes := false, true

	res := ResolveCapabilities(CapabilityInputs{
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

	if res.ContextWindow == nil || *res.ContextWindow != providerWindow || res.Sources.ContextWindow != "provider" {
		t.Fatalf("context = %v from %q, want the provider's %d", res.ContextWindow, res.Sources.ContextWindow, providerWindow)
	}
	if res.MaxOutput == nil || *res.MaxOutput != providerOutput || res.Sources.MaxOutput != "provider" {
		t.Fatalf("output = %v from %q, want the provider's %d", res.MaxOutput, res.Sources.MaxOutput, providerOutput)
	}
	if res.SupportsTools == nil || *res.SupportsTools || res.SupportsReasoning == nil || *res.SupportsReasoning ||
		res.SupportsVision == nil || *res.SupportsVision {
		t.Fatalf("capabilities = %+v, want the provider's explicit false values", res)
	}
	if !res.ManualInactive.ContextWindow || !res.ManualInactive.MaxOutput || !res.ManualInactive.Tools ||
		!res.ManualInactive.Reasoning || !res.ManualInactive.Vision {
		t.Fatalf("inactive manuals = %+v, want every superseded manual value marked", res.ManualInactive)
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
		ModelsDev: FactsRecord{ContextWindow: &modelsWindow},
		Manual:    FactsRecord{MaxOutput: &manualOutput},
	})
	if res.ContextWindow == nil || *res.ContextWindow != modelsWindow || res.Sources.ContextWindow != "modelsdev" {
		t.Fatalf("context = %v from %q, want models.dev", res.ContextWindow, res.Sources.ContextWindow)
	}
	if res.MaxOutput == nil || *res.MaxOutput != manualOutput || res.Sources.MaxOutput != "override" {
		t.Fatalf("output = %v from %q, want manual completion", res.MaxOutput, res.Sources.MaxOutput)
	}
	if res.Editable.ContextWindow || !res.Editable.MaxOutput {
		t.Fatalf("editable = %+v, want models.dev locked and the manual completion editable", res.Editable)
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
