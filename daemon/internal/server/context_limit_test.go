package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/formats"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestAttemptRejectsExplicitlyUnsupportedCapabilities(t *testing.T) {
	no := false
	server := New(Options{})
	spec := attemptSpec{candidate: routing.Candidate{Model: catalog.Model{ID: "m", SupportsTools: &no}}}
	request := &inference.Request{Tools: []inference.Tool{{Name: "exec"}}}
	if err := server.validateOutboundRequest(spec, request); !errors.Is(err, wire.ErrUnsupportedFeature) {
		t.Fatalf("validateOutboundRequest() = %v, want unsupported tools", err)
	}
}

func TestAttemptValidatesEffortsForToggleOnlyModels(t *testing.T) {
	server := New(Options{})
	yes := true
	spec := attemptSpec{candidate: routing.Candidate{Model: catalog.Model{
		ID: "m", SupportsReasoning: &yes, ReasoningEfforts: []string{}, ReasoningToggle: true,
	}}}
	for _, effort := range []string{"low", "high", "max", "none"} {
		request := &inference.Request{Reasoning: &inference.ReasoningConfig{Effort: effort}}
		if err := server.validateOutboundRequest(spec, request); err != nil {
			t.Fatalf("validateOutboundRequest(%s) = %v, want a toggle-only model to accept it", effort, err)
		}
	}
	unknown := &inference.Request{Reasoning: &inference.ReasoningConfig{Effort: "turbo"}}
	if err := server.validateOutboundRequest(spec, unknown); !errors.Is(err, wire.ErrUnsupportedFeature) {
		t.Fatalf("validateOutboundRequest(turbo) = %v, want an unknown effort refused", err)
	}
	noEffort := attemptSpec{candidate: routing.Candidate{Model: catalog.Model{
		ID: "plain", SupportsReasoning: &yes, ReasoningEfforts: []string{},
	}}}
	request := &inference.Request{Reasoning: &inference.ReasoningConfig{Effort: "high"}}
	if err := server.validateOutboundRequest(noEffort, request); !errors.Is(err, wire.ErrUnsupportedFeature) {
		t.Fatalf("validateOutboundRequest(high) = %v, want an explicit no-effort model to refuse it", err)
	}
}

// TestOpencodeMiMoAcceptsAnyEffort pins the gateway the catalog rule was
// written for: its facts state an empty ladder and no toggle, which used to
// refuse every positive effort. The rule now makes it toggle-only, so a
// client can ask for high and the codec turns it on.
func TestOpencodeMiMoAcceptsAnyEffort(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewCatalogRepo(db)
	yes := true
	if err := repo.SaveProvider(ctx, sqlite.ProviderRow{
		ID: "opencode-go", TemplateID: "opencode-go", Origin: "template",
		Label: "OpenCode Go", Auth: "api_key",
	}); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	if err := repo.SaveModel(ctx, sqlite.ModelRow{
		ProviderID: "opencode-go", ModelID: "mimo-v2.6-pro", Source: "listing",
	}); err != nil {
		t.Fatalf("SaveModel() error = %v", err)
	}
	if err := repo.SaveModelFacts(ctx, sqlite.ModelFactsRow{
		ProviderID: "opencode-go", ModelID: "mimo-v2.6-pro", Layer: "modelsdev",
		SupportsReasoning: &yes, ReasoningEfforts: []string{},
	}); err != nil {
		t.Fatalf("SaveModelFacts() error = %v", err)
	}
	built := catalog.New(sqlite.NewCatalogReader(db), nil, formats.New())
	if err := built.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	snapshot, found := built.Snapshot()
	if !found {
		t.Fatal("Reload() stored no snapshot")
	}
	model, ok := snapshot.Model("opencode-go", "mimo-v2.6-pro")
	if !ok {
		t.Fatal("the resolved MiMo model is missing")
	}

	server := New(Options{})
	spec := attemptSpec{candidate: routing.Candidate{Model: model}}
	for _, effort := range []string{"none", "low", "high", "max"} {
		request := &inference.Request{Reasoning: &inference.ReasoningConfig{Effort: effort}}
		if err := server.validateOutboundRequest(spec, request); err != nil {
			t.Fatalf("validateOutboundRequest(%s) = %v, want the resolved model to accept it", effort, err)
		}
	}
}

func TestAttemptReducesOutputToFitProviderContext(t *testing.T) {
	server := New(Options{})
	maxInput := int64(1_000)
	contextWindow := int64(256)
	maxOutput := int64(128)
	spec := attemptSpec{candidate: routing.Candidate{Model: catalog.Model{
		ID: "m", MaxInput: &maxInput, ContextWindow: &contextWindow, MaxOutput: &maxOutput,
	}}}
	request := &inference.Request{
		Messages: []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{{
			Type: inference.ContentTypeText, Text: strings.Repeat("token ", 40),
		}}}}, MaxTokens: 1000,
	}
	if err := server.validateOutboundRequest(spec, request); err != nil {
		t.Fatalf("validateOutboundRequest() = %v, want a reduced but valid request", err)
	}
	if request.MaxTokens <= 0 || request.MaxTokens >= 1000 {
		t.Fatalf("MaxTokens = %d, want the request reduced to the available context", request.MaxTokens)
	}
}

func TestAttemptRejectsInputThatCannotFit(t *testing.T) {
	server := New(Options{})
	maxInput := int64(8)
	contextWindow := int64(8)
	spec := attemptSpec{candidate: routing.Candidate{Model: catalog.Model{
		ID: "m", MaxInput: &maxInput, ContextWindow: &contextWindow,
	}}}
	request := &inference.Request{
		Messages: []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{{
			Type: inference.ContentTypeText, Text: strings.Repeat("token ", 100),
		}}}}, MaxTokens: 64,
	}
	if err := server.validateOutboundRequest(spec, request); !errors.Is(err, errContextLimitExceeded) {
		t.Fatalf("validateOutboundRequest() = %v, want an input refusal", err)
	}
}

func TestAttemptRejectsInputLeavingNoRoomForOutput(t *testing.T) {
	server := New(Options{})
	request := &inference.Request{MaxTokens: 1, Messages: []inference.Message{{
		Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hello"}},
	}}}
	window := int64(server.estimateRequestTokens(request))
	spec := attemptSpec{candidate: routing.Candidate{Model: catalog.Model{ID: "m", ContextWindow: &window}}}

	cause := server.validateOutboundRequest(spec, request)
	if !errors.Is(cause, errContextLimitExceeded) {
		t.Fatalf("validateOutboundRequest() = %v, want a context refusal", cause)
	}
	limit, found := contextLimitOf(cause)
	if !found || limit.Estimated != window || limit.Limit != window {
		t.Fatalf("context limit = %+v, want estimate and limit %d", limit, window)
	}
}

func TestRoutingNeedsCountImagesSeparatelyFromText(t *testing.T) {
	request := &inference.Request{
		Tools: []inference.Tool{{Name: "exec"}},
		Messages: []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{
			{Type: inference.ContentTypeImage, ImageURL: "data:image/png;base64,AAAA"},
			{Type: inference.ContentTypeText, Text: "describe both images"},
			{Type: inference.ContentTypeImage, ImageURL: "https://example.com/image.png"},
		}}},
	}
	needs := needsOf(request)
	want := catalog.PromptTokensFrom(len("describe both images")) + 2*inference.ImagePromptTokens
	if !needs.Images || !needs.Tools || needs.PromptTokens != want {
		t.Fatalf("needs = %+v, want images, tools and %d prompt tokens", needs, want)
	}
}

// imageRequest is the shape a coding client sends after reading a screenshot:
// one text turn and one image whose payload dwarfs every word in it.
func imageRequest(payload int) *inference.Request {
	base64 := strings.Repeat("A", payload)
	return &inference.Request{
		Messages: []inference.Message{
			{Role: inference.RoleUser, Content: []inference.ContentPart{{
				Type: inference.ContentTypeText, Text: strings.Repeat("token ", 1000),
			}}},
			{Role: inference.RoleUser, Content: []inference.ContentPart{{
				Type: inference.ContentTypeImage, ImageURL: "data:image/png;base64," + base64,
			}}},
		},
	}
}

// TestAttemptCountsAnImageByItsPixelsNotItsBase64 guards the regression that
// made a 1M-token model refuse a request whose provider saw 99k tokens: an
// inline data URL measured as text overstates one image by two orders of
// magnitude.
func TestAttemptCountsAnImageByItsPixelsNotItsBase64(t *testing.T) {
	server := New(Options{})
	window := int64(1_000_000)
	request := imageRequest(2_000_000)

	estimated := server.estimateRequestTokens(request)
	if estimated >= 100_000 {
		t.Fatalf("estimate = %d, want an image charged far below its base64 length", estimated)
	}
	if estimated < inference.ImagePromptTokens {
		t.Fatalf("estimate = %d, want at least the image charge", estimated)
	}
	spec := attemptSpec{candidate: routing.Candidate{Model: catalog.Model{
		ID: "vision", ContextWindow: &window,
	}}}
	if err := server.validateOutboundRequest(spec, request); err != nil {
		t.Fatalf("validateOutboundRequest() = %v, want the request to fit", err)
	}
}

// TestContextRefusalNamesItsNumbers checks that a refusal keeps the estimate
// and the limit, which is what each client surface rewrites into its own
// vocabulary.
func TestContextRefusalNamesItsNumbers(t *testing.T) {
	server := New(Options{})
	window := int64(8)
	spec := attemptSpec{candidate: routing.Candidate{Model: catalog.Model{
		ID: "m", ContextWindow: &window,
	}}}
	request := &inference.Request{Messages: []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{{
		Type: inference.ContentTypeText, Text: strings.Repeat("token ", 100),
	}}}}}

	err := server.validateOutboundRequest(spec, request)
	if !errors.Is(err, errContextLimitExceeded) {
		t.Fatalf("validateOutboundRequest() = %v, want a context refusal", err)
	}
	limit, found := contextLimitOf(err)
	if !found {
		t.Fatalf("refusal = %v, want it to carry the numbers", err)
	}
	if limit.Estimated <= limit.Limit || limit.Limit != window || limit.Model != "m" {
		t.Fatalf("refusal = %+v, want estimate above the stated window", limit)
	}
}
