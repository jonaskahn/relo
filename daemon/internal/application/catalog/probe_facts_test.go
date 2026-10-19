package catalog

import (
	"testing"

	featurecatalog "github.com/jonaskahn/relo/internal/catalog"
)

func TestProbeProviderFactCarriesListingCapabilities(t *testing.T) {
	contextWindow := int64(1_000_000)
	maxOutput := int64(128_000)
	vision := true
	entry := rosterEntry{
		Source: SourceListing,
		Listed: featurecatalog.Listed{
			ID:             "vision-model",
			ContextWindow:  &contextWindow,
			MaxOutput:      &maxOutput,
			SupportsVision: &vision,
		},
	}

	facts := factsForEntry(entry, ProbeModel{}, featurecatalog.ModelsDevModel{}, false)
	if len(facts) != 1 {
		t.Fatalf("facts = %+v, want one provider fact", facts)
	}
	fact := facts[0]
	if fact.ContextWindow == nil || *fact.ContextWindow != contextWindow {
		t.Errorf("context window = %v, want %d", fact.ContextWindow, contextWindow)
	}
	if fact.MaxOutput == nil || *fact.MaxOutput != maxOutput {
		t.Errorf("max output = %v, want %d", fact.MaxOutput, maxOutput)
	}
	if fact.SupportsVision == nil || !*fact.SupportsVision {
		t.Errorf("vision = %v, want true", fact.SupportsVision)
	}
}

func TestProbeProviderFactLeavesUnstatedVisionUnset(t *testing.T) {
	entry := rosterEntry{
		Source: SourceListing,
		Listed: featurecatalog.Listed{ID: "plain-model", Name: "Plain"},
	}

	facts := factsForEntry(entry, ProbeModel{}, featurecatalog.ModelsDevModel{}, false)
	if len(facts) != 1 {
		t.Fatalf("facts = %+v, want one provider fact", facts)
	}
	if facts[0].SupportsVision != nil {
		t.Errorf("vision = %v, want unstated", facts[0].SupportsVision)
	}
}
