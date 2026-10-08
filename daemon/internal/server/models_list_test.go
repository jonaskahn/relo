package server

import (
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
)

func TestAnthropicModelIDSeparatesThePickerPrefix(t *testing.T) {
	window := int64(1_000_000)
	id := anthropicModelID(listedModel{
		ID:            "relo-google-antigravity-claude-sonnet-4-6",
		ProviderID:    "google-antigravity",
		SourceModelID: "claude-sonnet-4-6",
		Paired:        true,
		ContextWindow: &window,
	})
	const want = "claude-relo-google-antigravity--claude-sonnet-4-6[1m]"
	if id != want {
		t.Fatalf("id = %q, want %q", id, want)
	}
}

func TestAnthropicModelListStatesNullWhenUnknown(t *testing.T) {
	window := int64(200_000)
	entries := []listedModel{
		{ID: "known", Name: "Known", ContextWindow: &window},
		{ID: "unknown", Name: "Unknown"},
	}

	list := anthropicModelList(entries)
	data, _ := list["data"].([]map[string]any)
	if data[0]["max_input_tokens"] != window {
		t.Fatalf("known entry = %+v, want the context window", data[0])
	}
	if data[0]["max_tokens"] != nil {
		t.Fatalf("known entry = %+v, want a null output ceiling when unstated", data[0])
	}
	if data[1]["max_input_tokens"] != nil || data[1]["max_tokens"] != nil {
		t.Fatalf("unknown entry = %+v, want both counts null", data[1])
	}
}

func TestAppendListedVariantsPublishesDefaultAndMillionEntries(t *testing.T) {
	window := int64(catalog.MillionContext)
	entries := appendListedVariants(nil, listedModel{
		ID: "relo-claude-work-opus", Name: "Opus 1M on Claude",
		ProviderID: "claude-work", SourceModelID: "opus", ContextWindow: &window,
	}, true)
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want a default and 1M entry", entries)
	}
	if entries[0].ID != "relo-claude-work-opus" || *entries[0].ContextWindow != catalog.DefaultContext {
		t.Fatalf("default entry = %+v, want a %d-token entry", entries[0], catalog.DefaultContext)
	}
	if entries[0].Name != "Opus on Claude" {
		t.Fatalf("default name = %q, want the 1M marker removed", entries[0].Name)
	}
	if entries[1].ID != "relo-claude-work-opus-1m" || *entries[1].ContextWindow != catalog.MillionContext {
		t.Fatalf("1M entry = %+v, want the full window", entries[1])
	}
}

func TestAppendListedVariantsKeepsSubMillionEntrySingle(t *testing.T) {
	window := int64(500_000)
	entries := appendListedVariants(nil, listedModel{
		ID: "relo-openai-gpt-4o", Name: "GPT-4o on OpenAI",
		ProviderID: "openai", SourceModelID: "gpt-4o", ContextWindow: &window,
	}, true)
	if len(entries) != 1 {
		t.Fatalf("entries = %+v, want one entry", entries)
	}
	if entries[0].ID != "relo-openai-gpt-4o" || *entries[0].ContextWindow != window {
		t.Fatalf("entry = %+v, want its own window", entries[0])
	}
}

// TestAppendListedVariantsSuffixesASingleMillionEntry pins the listing for a
// million-token model off the Claude.ai sign-in: one entry, under the suffix
// alone, at its own window.
func TestAppendListedVariantsSuffixesASingleMillionEntry(t *testing.T) {
	window := int64(catalog.MillionContext)
	entries := appendListedVariants(nil, listedModel{
		ID: "relo-other-grok-4-7", Name: "Grok 4.7 1M on Other",
		ProviderID: "other", SourceModelID: "grok-4-7",
		Suffixed: true, ContextWindow: &window,
	}, false)
	if len(entries) != 1 {
		t.Fatalf("entries = %+v, want the single suffixed entry", entries)
	}
	if entries[0].ID != "relo-other-grok-4-7-1m" || *entries[0].ContextWindow != catalog.MillionContext {
		t.Fatalf("entry = %+v, want the full window under the suffix", entries[0])
	}
	if id := anthropicModelID(entries[0]); id != "claude-relo-other--grok-4-7[1m]" {
		t.Fatalf("id = %q, want the marked picker name", id)
	}
}

// TestAnthropicModelIDOmitsTheMarkerForANativeModel pins the spelling a client
// will not rewrite. Claude Code strips [1m] from a model it knows to be natively
// a million tokens, so listing it marked only produces a second row that resolves
// back to the first.
func TestAnthropicModelIDOmitsTheMarkerForANativeModel(t *testing.T) {
	window := int64(catalog.MillionContext)
	native := anthropicModelID(listedModel{
		ID: "relo-claude-claude-sonnet-5-5", ProviderID: "claude", SourceModelID: "claude-sonnet-5-5",
		NativeMillion: true, ContextWindow: &window,
	})
	if native != "claude-relo-claude--claude-sonnet-5-5" {
		t.Fatalf("native id = %q, want the bare picker name", native)
	}
	beta := anthropicModelID(listedModel{
		ID: "relo-claude-claude-sonnet-4-6", ProviderID: "claude", SourceModelID: "claude-sonnet-4-6",
		Paired: true, ContextWindow: &window,
	})
	if beta != "claude-relo-claude--claude-sonnet-4-6[1m]" {
		t.Fatalf("beta id = %q, want the marked picker name", beta)
	}
}

// TestTheProviderOwnMillionTokenSpellingListsOnce pins the listing for a router
// that publishes kimi-k3 and kimi-k3[1M]. Slugging the second lands on the name
// Relo derives for a twin of the first, so the twin is not published: three rows
// would claim one name twice, and the Claude-shaped row would carry the marker
// twice, naming an identifier nothing resolves.
func TestTheProviderOwnMillionTokenSpellingListsOnce(t *testing.T) {
	window := int64(catalog.MillionContext)
	entries := appendListedVariants(nil, listedModel{
		ID: "relo-teamorouter-kimi-k3", Name: "Kimi K3 on Teamo Router",
		ProviderID: "teamorouter", SourceModelID: "kimi-k3", ContextWindow: &window,
	}, false)
	entries = appendListedVariants(entries, listedModel{
		ID: "relo-teamorouter-kimi-k3-1m", Name: "Kimi K3[1M] on Teamo Router",
		ProviderID: "teamorouter", SourceModelID: "kimi-k3[1M]",
		NativeMillion: true, ContextWindow: &window,
	}, false)

	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want one row per model", entries)
	}
	// The marker the provider spelled stays, because dropping it would name the
	// other model; the marker Relo would add beside it does not.
	if id := anthropicModelID(entries[1]); id != "claude-relo-teamorouter--kimi-k3[1M]" {
		t.Fatalf("id = %q, want the provider's own marker spelled once", id)
	}
	if id := anthropicModelID(entries[0]); id != "claude-relo-teamorouter--kimi-k3" {
		t.Fatalf("id = %q, want the other model's name left alone", id)
	}
	if entries[1].NativeMillion != true || *entries[1].ContextWindow != window {
		t.Fatalf("entry = %+v, want the full window with no twin", entries[1])
	}

	// The same model narrowed below a million loses its twin, so it carries no
	// marker of Relo's to double up on the one the provider spelled.
	narrowed := int64(catalog.DefaultContext)
	if id := anthropicModelID(listedModel{
		ProviderID: "teamorouter", SourceModelID: "kimi-k3[1M]", ContextWindow: &narrowed,
	}); id != "claude-relo-teamorouter--kimi-k3[1M]" {
		t.Fatalf("id = %q, want the provider's marker and none of ours", id)
	}
	// A paired model whose provider spelled a marker keeps only ours, so one
	// identifier never carries two.
	if id := anthropicModelID(listedModel{
		ProviderID: "teamorouter", SourceModelID: "kimi-k3[1M]",
		Paired: true, ContextWindow: &window,
	}); id != "claude-relo-teamorouter--kimi-k3[1m]" {
		t.Fatalf("id = %q, want the provider's marker dropped when ours goes on", id)
	}
}
