package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
	if err := validateEffort(unknown); !errors.Is(err, inference.ErrUnknownEffort) {
		t.Fatalf("validateEffort(turbo) = %v, want an unknown effort refused", err)
	}
	noEffort := attemptSpec{candidate: routing.Candidate{Model: catalog.Model{
		ID: "plain", SupportsReasoning: &yes, ReasoningEfforts: []string{},
	}}}
	request := &inference.Request{Reasoning: &inference.ReasoningConfig{Effort: "high"}}
	if err := server.validateOutboundRequest(noEffort, request); err != nil {
		t.Fatalf("validateOutboundRequest(high) = %v, want an explicit no-effort model to ignore it", err)
	}
	if request.Reasoning != nil {
		t.Fatalf("Reasoning = %+v, want the control omitted", request.Reasoning)
	}
	request = &inference.Request{Reasoning: &inference.ReasoningConfig{Effort: "turbo"}}
	if err := server.validateOutboundRequest(noEffort, request); err != nil {
		t.Fatalf("validateOutboundRequest(turbo) = %v, want an internal unknown value dropped", err)
	}
	if request.Reasoning != nil {
		t.Fatalf("Reasoning = %+v, want an unknown internal value dropped", request.Reasoning)
	}
}

// TestAttemptCoercesReasoningOntoTheModelCapability pins the resolution table
// the attempt applies: a recognized value is coerced, ignored, or turned into
// the model's own control, never refused.
func TestAttemptCoercesReasoningOntoTheModelCapability(t *testing.T) {
	server := New(Options{})
	yes, no := true, false
	cases := []struct {
		name     string
		request  string
		ladder   []string
		toggle   bool
		supports *bool
		effort   string
		dropped  bool
	}{
		{name: "a ladder coerces to the nearest stronger rung", request: "medium", ladder: []string{"low", "high"}, supports: &yes, effort: "high"},
		{name: "an exact level keeps its rung", request: "low", ladder: []string{"low", "high"}, supports: &yes, effort: "low"},
		{name: "a level above the ladder stays at the highest", request: "max", ladder: []string{"low", "high"}, supports: &yes, effort: "high"},
		{name: "a non-reasoning model drops the control", request: "medium", ladder: []string{"low", "high"}, supports: &no, dropped: true},
		{name: "a toggle-only model disables for default", request: "default", ladder: []string{}, toggle: true, supports: &yes, effort: "none"},
		{name: "a toggle-only model disables for none", request: "none", ladder: []string{}, toggle: true, supports: &yes, effort: "none"},
		{name: "a toggle-only model disables for off", request: "off", ladder: []string{}, toggle: true, supports: &yes, effort: "none"},
		{name: "a toggle-only model enables for on", request: "on", ladder: []string{}, toggle: true, supports: &yes, effort: "on"},
		{name: "a toggle-only model enables for a level", request: "high", ladder: []string{}, toggle: true, supports: &yes, effort: "on"},
		{name: "a toggle-only model omits auto", request: "auto", ladder: []string{}, toggle: true, supports: &yes, effort: ""},
		{name: "an explicit empty ladder omits a level", request: "high", ladder: []string{}, supports: &yes, dropped: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := attemptSpec{candidate: routing.Candidate{Model: catalog.Model{
				ID: "m", SupportsReasoning: tc.supports, ReasoningEfforts: tc.ladder, ReasoningToggle: tc.toggle,
			}}}
			request := &inference.Request{Reasoning: &inference.ReasoningConfig{Effort: tc.request}}
			if err := server.validateOutboundRequest(spec, request); err != nil {
				t.Fatalf("validateOutboundRequest() = %v, want the value accepted", err)
			}
			if tc.dropped {
				if request.Reasoning != nil {
					t.Fatalf("Reasoning = %+v, want the control dropped", request.Reasoning)
				}
				return
			}
			if request.Reasoning == nil || request.Reasoning.Effort != tc.effort {
				t.Fatalf("Reasoning = %+v, want effort %q", request.Reasoning, tc.effort)
			}
		})
	}
}

// TestAttemptReasoningMutationStaysLocal pins that coercion changes only the
// attempt's copy, so a retry or another route member still sees the request
// the client sent.
func TestAttemptReasoningMutationStaysLocal(t *testing.T) {
	server := New(Options{})
	yes := true
	request := &inference.Request{
		Model:     "deepseek",
		Reasoning: &inference.ReasoningConfig{Effort: "medium", ID: "rs_1"},
	}
	spec := attemptSpec{
		candidate: routing.Candidate{Model: catalog.Model{
			ID: "deepseek", UpstreamID: "deepseek", SupportsReasoning: &yes,
			ReasoningEfforts: []string{"low", "high"},
		}},
		outcome: &requestOutcome{request: request},
	}
	attempt := modelRequest(request, spec.candidate.Model.UpstreamID)
	if err := server.validateOutboundRequest(spec, attempt); err != nil {
		t.Fatalf("validateOutboundRequest() = %v", err)
	}
	if attempt.Reasoning == nil || attempt.Reasoning.Effort != "high" {
		t.Fatalf("attempt reasoning = %+v, want the coerced high value", attempt.Reasoning)
	}
	if request.Reasoning.Effort != "medium" || request.Reasoning.ID != "rs_1" {
		t.Fatalf("shared reasoning = %+v, want the client's request unchanged", request.Reasoning)
	}
}

// TestAnthropicReasoningRefusalUsesTheInvalidRequestEnvelope pins the Claude
// surface's mapping for an unknown effort: a 400 with the error type Claude
// Code reads as a client mistake.
func TestAnthropicReasoningRefusalUsesTheInvalidRequestEnvelope(t *testing.T) {
	server := New(Options{Formats: formats.New()})
	recorder := httptest.NewRecorder()
	err := &inference.UnknownEffortError{Value: "invent"}
	server.writeAnthropicRefusal(recorder, failureStatus(err), err)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	var payload struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode refusal: %v", err)
	}
	if payload.Error.Type != "invalid_request_error" || !strings.Contains(payload.Error.Message, "invent") {
		t.Fatalf("refusal = %+v, want the invalid-request envelope naming the value", payload)
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
