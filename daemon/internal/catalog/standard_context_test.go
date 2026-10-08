package catalog

import (
	"testing"
)

func TestStandardContextPairsMillionWindowWhenOffered(t *testing.T) {
	million := int64(MillionContext)
	below := int64(MillionContext - 1)
	cases := []struct {
		name     string
		window   *int64
		offers   bool
		paired   bool
		standard int64
	}{
		{name: "million with switch on pairs at default", window: &million, offers: true, paired: true, standard: 200_000},
		{name: "million with switch off stays single", window: &million, offers: false, paired: false, standard: MillionContext},
		{name: "below million with switch on stays single", window: &below, offers: true, paired: false, standard: MillionContext - 1},
		{name: "nil window stays single", window: nil, offers: true, paired: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, paired := StandardContext(testCase.window, testCase.offers)
			if paired != testCase.paired {
				t.Fatalf("paired = %v, want %v", paired, testCase.paired)
			}
			if !paired {
				if testCase.window == nil && got != nil {
					t.Fatalf("window = %v, want nil", got)
				}
				if testCase.window != nil && (got == nil || *got != *testCase.window) {
					t.Fatalf("window = %v, want its own window", got)
				}
				return
			}
			if got == nil || *got != testCase.standard {
				t.Fatalf("window = %v, want %d", got, testCase.standard)
			}
		})
	}
}

func TestDefaultContextStaysAtBaseRateBoundary(t *testing.T) {
	// The console copy says 200K, so changing the constant without changing
	// the label would make the Settings tab lie.
	if DefaultContext != 200_000 {
		t.Fatalf("DefaultContext = %d, want 200000", DefaultContext)
	}
}

// TestNativeMillionContextSeparatesBetaFromNative pins the two classes apart,
// because Relo publishes them differently and a mistake in either is silent:
// a native model published twice has an unreachable second entry, and a beta
// model published once loses the base-rate window.
func TestNativeMillionContextSeparatesBetaFromNative(t *testing.T) {
	million := int64(MillionContext)
	twoHundred := int64(DefaultContext)
	cases := []struct {
		name    string
		modelID string
		window  *int64
		want    bool
	}{
		{name: "a sonnet 5.5 snapshot is native", modelID: "claude-sonnet-5-5-20260114", window: &million, want: true},
		{name: "an opus 5.5 is native", modelID: "claude-opus-5-5", window: &million, want: true},
		{name: "a haiku 5.5 is native", modelID: "claude-haiku-5-5", window: &million, want: true},
		{name: "a mythos 5.1 is native", modelID: "claude-mythos-5-1", window: &million, want: true},
		{name: "an opus 4.6 reaches a million on the beta", modelID: "claude-opus-4-6", window: &million},
		{name: "a sonnet 4.6 reaches a million on the beta", modelID: "claude-sonnet-4-6", window: &million},
		{name: "an opus 4.5 has no million window at all", modelID: "claude-opus-4-5", window: &million},
		{name: "a native model narrowed below a million is not native", modelID: "claude-opus-5-5", window: &twoHundred},
		{name: "a native model with no stated window is not native", modelID: "claude-opus-5-5"},
		{name: "a provider's own 1M model is not Anthropic's", modelID: "grok-4.7", window: &million},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := NativeMillionContext(testCase.modelID, testCase.window); got != testCase.want {
				t.Fatalf("NativeMillionContext(%q) = %v, want %v", testCase.modelID, got, testCase.want)
			}
		})
	}
}

// TestEveryNativeMillionGenerationIsListed keeps the predicate honest: a native
// generation missing from the set publishes a second entry a client cannot
// reach. Failures name the generation, so adding it is the fix.
func TestEveryNativeMillionGenerationIsListed(t *testing.T) {
	want := []string{
		"claude-sonnet-5", "claude-sonnet-5-5", "claude-haiku-5-5",
		"claude-opus-4-7", "claude-opus-4-8", "claude-opus-5", "claude-opus-5-5",
		"claude-fable-5", "claude-fable-5-1",
		"claude-mythos-5", "claude-mythos-5-1",
	}
	for _, id := range want {
		if !nativeMillionContext[id] {
			t.Errorf("nativeMillionContext is missing %s", id)
		}
	}
	if len(nativeMillionContext) != len(want) {
		t.Errorf("nativeMillionContext has %d entries, want the %d listed above",
			len(nativeMillionContext), len(want))
	}
}

// TestProviderMillionMarkerNamesTheUpstreamSpelling pins the models Relo
// publishes under a single name because the provider already did. A router
// sells its large variant as kimi-k3[1M] rather than as a flag on a beta, so
// the identifier itself says the window is a million tokens and a twin beside
// it would only be a second name for the same model.
func TestProviderMillionMarkerNamesTheUpstreamSpelling(t *testing.T) {
	million := int64(MillionContext)
	twoHundred := int64(DefaultContext)
	cases := []struct {
		name    string
		modelID string
		window  *int64
		want    bool
	}{
		{name: "an uppercase marker is one million entry", modelID: "kimi-k3[1M]", window: &million, want: true},
		{name: "a lowercase marker is one million entry", modelID: "kimi-k3[1m]", window: &million, want: true},
		{name: "an unmarked model is not", modelID: "kimi-k3", window: &million},
		{name: "a marked model the operator narrowed is not", modelID: "kimi-k3[1M]", window: &twoHundred},
		{name: "a marked model with no stated window is not", modelID: "kimi-k3[1M]"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := ProviderMillionMarker(testCase.modelID, testCase.window); got != testCase.want {
				t.Fatalf("ProviderMillionMarker(%q) = %v, want %v", testCase.modelID, got, testCase.want)
			}
		})
	}

	// The marker is stripped before Relo applies its own, or the published
	// identifier ends in two of them and resolves to nothing.
	stripped := map[string]string{
		"kimi-k3[1M]":     "kimi-k3",
		"kimi-k3[1m]":     "kimi-k3",
		"kimi-k3":         "kimi-k3",
		"claude-sonnet-4": "claude-sonnet-4",
		"kimi-3[1m]-beta": "kimi-3[1m]-beta",
	}
	for id, want := range stripped {
		if got := StripContextMarker(id); got != want {
			t.Errorf("StripContextMarker(%q) = %q, want %q", id, got, want)
		}
	}
}

// millionPolicySnapshot holds one Claude.ai sign-in and one other connection,
// so the connection rule is measured rather than assumed.
func millionPolicySnapshot() *Snapshot {
	return &Snapshot{
		byProvider: map[string]Provider{
			"claude": {ID: "claude", TemplateID: ClaudeSubscriptionTemplate},
			"other":  {ID: "other", TemplateID: "custom"},
		},
		paired: map[string]bool{},
	}
}

func millionPolicyModel(providerID, modelID string, format APIFormat, window *int64) Model {
	return Model{ProviderID: providerID, ID: modelID, APIFormat: format, ContextWindow: window}
}

// TestMillionAloneNeedsTheClaudeSignIn pins the connection rule: a natively
// million-token Claude generation is a bare entry only on the Claude.ai
// sign-in. On any other connection it takes the generated suffix instead.
func TestMillionAloneNeedsTheClaudeSignIn(t *testing.T) {
	million := int64(MillionContext)
	twoHundred := int64(DefaultContext)
	snapshot := millionPolicySnapshot()
	cases := []struct {
		name   string
		model  Model
		alone  bool
		suffix bool
	}{
		{name: "a native generation on the sign-in is alone", model: millionPolicyModel("claude", "claude-opus-5-5", FormatAnthropic, &million), alone: true},
		{name: "a native snapshot on the sign-in is alone", model: millionPolicyModel("claude", "claude-sonnet-5-5-20260114", FormatAnthropic, &million), alone: true},
		{name: "the same generation elsewhere takes the suffix", model: millionPolicyModel("other", "claude-opus-5-5", FormatAnthropic, &million), suffix: true},
		{name: "a beta generation on the sign-in pairs, so neither", model: millionPolicyModel("claude", "claude-opus-4-6", FormatAnthropic, &million)},
		{name: "a beta generation elsewhere takes the suffix", model: millionPolicyModel("other", "claude-sonnet-4-6", FormatAnthropic, &million), suffix: true},
		{name: "a provider marker is alone on any connection", model: millionPolicyModel("other", "kimi-k3[1M]", FormatOpenAIChat, &million), alone: true},
		{name: "an unmarked model elsewhere takes the suffix", model: millionPolicyModel("other", "kimi-k3", FormatOpenAIChat, &million), suffix: true},
		{name: "a native model narrowed below a million is neither", model: millionPolicyModel("claude", "claude-opus-5-5", FormatAnthropic, &twoHundred)},
		{name: "a model with no stated window is neither", model: millionPolicyModel("other", "kimi-k3", FormatOpenAIChat, nil)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := snapshot.MillionAlone(testCase.model); got != testCase.alone {
				t.Fatalf("MillionAlone(%s/%s) = %v, want %v", testCase.model.ProviderID, testCase.model.ID, got, testCase.alone)
			}
			if got := snapshot.MillionSuffixed(testCase.model); got != testCase.suffix {
				t.Fatalf("MillionSuffixed(%s/%s) = %v, want %v", testCase.model.ProviderID, testCase.model.ID, got, testCase.suffix)
			}
		})
	}
}

// TestOnlyTheClaudeSignInPairsWithOneMillion pins the format the second entry
// is for. A million-token window is an opt-in on the Anthropic wire of a
// Claude.ai sign-in: the twin is the entry that claims the context-1m beta,
// and the base entry is held at the base rate so an agent picks between them.
// Everywhere else a million-token model is published once, under the suffix
// alone, so pairing one would cap a model the upstream already serves in full
// and publish a twin that changes nothing.
func TestOnlyTheClaudeSignInPairsWithOneMillion(t *testing.T) {
	million := int64(MillionContext)
	snapshot := millionPolicySnapshot()
	cases := []struct {
		name  string
		model Model
		want  bool
	}{
		{name: "the sign-in pairs", model: millionPolicyModel("claude", "claude-opus-4-6", FormatAnthropic, &million), want: true},
		{name: "another anthropic connection does not", model: millionPolicyModel("other", "claude-opus-4-6", FormatAnthropic, &million)},
		{name: "openai chat does not", model: millionPolicyModel("other", "some-model", FormatOpenAIChat, &million)},
		{name: "openai responses does not", model: millionPolicyModel("other", "some-model", FormatOpenAIResp, &million)},
		{name: "a native generation on the sign-in does not", model: millionPolicyModel("claude", "claude-opus-5-5", FormatAnthropic, &million)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := snapshot.pairsWithOneMillion(testCase.model); got != testCase.want {
				t.Fatalf("pairsWithOneMillion(%s/%s) = %v, want %v", testCase.model.ProviderID, testCase.model.ID, got, testCase.want)
			}
		})
	}

	// A window under a million never pairs, whatever the format.
	small := int64(DefaultContext)
	model := millionPolicyModel("claude", "claude-opus-4-6", FormatAnthropic, &small)
	if snapshot.pairsWithOneMillion(model) {
		t.Fatal("a model below a million tokens must not pair")
	}
}
