package templates

import (
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
)

func TestKiloFreeKeepsModelsTheGatewayMarksFree(t *testing.T) {
	for _, entry := range []struct {
		id     string
		isFree *bool
		kept   bool
		why    string
	}{
		{"stepfun/step-5-preview-free", new(true), true, "the flag states the lane"},
		{"kilo-auto/free", new(true), true, "a bare name can still be free access"},
		{"inclusionai/ling-3.1-flash", new(true), true, "free under a name that states no price"},
		{"stealth/glyph-cluster", new(true), true, "free under a name that states no price"},
		{"anthropic/claude-opus-5.5", new(false), false, "the gateway marks it paid"},
		{"vendor/unmarked", nil, false, "a listing that never states the flag proves nothing"},
	} {
		model := catalog.Listed{ID: entry.id, IsFree: entry.isFree}
		if got := kiloFreeModel(model); got != entry.kept {
			t.Errorf("kiloFreeModel(%q) = %t, want %t: %s", entry.id, got, entry.kept, entry.why)
		}
	}
}

// TestKiloFreeNeedsNoCredential covers what lets the pool answer at all: the
// gateway serves the free models with no key, so the connection must not ask
// an operator for one.
func TestKiloFreeNeedsNoCredential(t *testing.T) {
	template, found := Get("kilo-free", nil)
	if !found {
		t.Fatal("kilo-free is not registered")
	}
	if template.Auth != catalog.AuthNone || template.KeyHeader != catalog.KeyHeaderNone {
		t.Fatalf("kilo-free asks for a credential: auth %q, key header %q", template.Auth, template.KeyHeader)
	}
	if template.ModelsDevProviderID != "kilo" {
		t.Fatalf("modelsdev provider = %q, want the gateway Relo resolves capabilities from", template.ModelsDevProviderID)
	}
	if template.RequiresStream {
		t.Fatal("kilo-free streams only, but the gateway serves a one-shot request")
	}
}

// TestKiloFreeFilterRunsWithoutTheModelCatalog asserts the pool is kept from
// the gateway's own listing, so a connection still resolves its roster before
// the model catalog is fetched.
func TestKiloFreeFilterRunsWithoutTheModelCatalog(t *testing.T) {
	template, found := Get("kilo-free", nil)
	if !found || template.FilterModels == nil {
		t.Fatal("kilo-free has no model filter")
	}
	if !template.FilterModels(catalog.Listed{ID: "stealth/glyph-cluster", IsFree: new(true)}) {
		t.Fatal("a free model was dropped without a catalog copy")
	}
	if template.FilterModels(catalog.Listed{ID: "anthropic/claude-opus-5.5", IsFree: new(false)}) {
		t.Fatal("a paid model was kept without a catalog copy")
	}
}
