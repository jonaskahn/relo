package catalog

import "testing"

// TestResolveModelWindows pins provider authority and exact presence: the
// provider wins over models.dev and a manual completion, and a model no layer
// describes keeps its output ceiling unknown instead of inventing a default.
func TestResolveModelWindows(t *testing.T) {
	t.Run("every layer silent leaves the ceiling unknown", func(t *testing.T) {
		res := &Model{}
		resolveModelWindows(res, Provider{AllowOverrides: true}, factLayers{})
		if res.MaxOutput != nil {
			t.Fatalf("MaxOutput = %v, want unknown", res.MaxOutput)
		}
		if res.ContextWindow != nil {
			t.Fatalf("ContextWindow = %v, want nil so routing keeps DefaultContext", res.ContextWindow)
		}
		if sources := factSources(Provider{AllowOverrides: true}, FactsRecord{}, FactsRecord{}, FactsRecord{}); sources.MaxOutput != "" {
			t.Fatalf("FactSources.MaxOutput = %q, want empty so the wire keeps a silent client's ceiling unstated", sources.MaxOutput)
		}
	})

	t.Run("the model index states a ceiling", func(t *testing.T) {
		stated := int64(131_000)
		res := &Model{}
		resolveModelWindows(res, Provider{AllowOverrides: true}, factLayers{models: FactsRecord{MaxOutput: &stated}})
		if res.MaxOutput == nil || *res.MaxOutput != stated {
			t.Fatalf("MaxOutput = %v, want the stated %d", res.MaxOutput, stated)
		}
	})

	t.Run("the provider wins over a manual completion", func(t *testing.T) {
		stated, over := int64(32_000), int64(128_000)
		res := &Model{}
		resolveModelWindows(res, Provider{AllowOverrides: true}, factLayers{
			override: FactsRecord{MaxOutput: &over}, provider: FactsRecord{MaxOutput: &stated},
		})
		if res.MaxOutput == nil || *res.MaxOutput != over {
			t.Fatalf("MaxOutput = %v, want the operator's %d", res.MaxOutput, over)
		}
	})
}
