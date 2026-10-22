package catalog

import "testing"

// TestResolveModelWindows pins the output-ceiling default: a model no fact
// layer describes keeps the 64K default so the wire, /v1/models, and the
// generated client configs all bound the same way, while a layer that stated
// a ceiling — override, provider, or model index — is left untouched.
func TestResolveModelWindows(t *testing.T) {
	t.Run("every layer silent carries the default ceiling", func(t *testing.T) {
		res := &Model{}
		resolveModelWindows(res, FactsRecord{}, FactsRecord{}, FactsRecord{})
		if res.MaxOutput == nil || *res.MaxOutput != DefaultMaxOutput {
			t.Fatalf("MaxOutput = %v, want the %d default", res.MaxOutput, DefaultMaxOutput)
		}
		if res.ContextWindow != nil {
			t.Fatalf("ContextWindow = %v, want nil so routing keeps DefaultContext", res.ContextWindow)
		}
		if sources := factSources(FactsRecord{}, FactsRecord{}, FactsRecord{}); sources.MaxOutput != "" {
			t.Fatalf("FactSources.MaxOutput = %q, want empty so the wire keeps a silent client's ceiling unstated", sources.MaxOutput)
		}
	})

	t.Run("the model index states a ceiling", func(t *testing.T) {
		stated := int64(131_000)
		res := &Model{}
		resolveModelWindows(res, FactsRecord{}, FactsRecord{}, FactsRecord{MaxOutput: &stated})
		if res.MaxOutput == nil || *res.MaxOutput != stated {
			t.Fatalf("MaxOutput = %v, want the stated %d", res.MaxOutput, stated)
		}
	})

	t.Run("an override wins over the model index", func(t *testing.T) {
		stated, over := int64(32_000), int64(128_000)
		res := &Model{}
		resolveModelWindows(res, FactsRecord{MaxOutput: &over}, FactsRecord{}, FactsRecord{MaxOutput: &stated})
		if res.MaxOutput == nil || *res.MaxOutput != over {
			t.Fatalf("MaxOutput = %v, want the override %d", res.MaxOutput, over)
		}
	})
}
