// Antigravity attempt selection: resolving one request to one account's SKU.
package server

import (
	"context"
	"fmt"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/routing"
)

// selectAntigravityAttempt resolves the exact Cloud Code SKU one attempt sends.
// A Claude generation the vendor serves under its own id passes through: any
// thinking the request states is ignored rather than resolved onto a SKU. A
// collapsed Claude generation resolves an effort the way a Gemini model does.
func (s *Server) selectAntigravityAttempt(ctx context.Context, spec *attemptSpec, tried map[string][]string) (*wire.AntigravityRequest, error) {
	if isClaudeAntigravityPassthrough(spec.candidate) {
		return nil, nil
	}
	effort := ""
	if spec.outcome.request != nil {
		effort, _ = catalog.RequestEffort(spec.outcome.request.Reasoning)
	}
	for range maxAntigravityAccounts {
		selected, ok, err := s.matchAntigravityAccount(ctx, spec, tried, effort)
		if err != nil || ok {
			return selected, err
		}
	}
	return nil, antigravitySelectionError(spec.candidate, effort)
}

func (s *Server) matchAntigravityAccount(ctx context.Context, spec *attemptSpec, tried map[string][]string, effort string) (*wire.AntigravityRequest, bool, error) {
	candidate, outcome := spec.candidate, spec.outcome
	configs, err := s.antigravityConfigs(ctx, candidate.ProviderID, spec.authorized.CredentialID)
	if err != nil {
		return nil, false, err
	}
	if len(configs) == 0 {
		return nil, true, nil
	}
	if selected, ok := catalog.SelectAntigravityVariant(configs, candidate.ModelID, effort); ok {
		return antigravityRequestOf(selected), true, nil
	}
	if !s.excludeAntigravityAccount(spec, tried) {
		return nil, false, antigravitySelectionError(candidate, effort)
	}
	authorized, err := s.opts.Relay.Authorize(ctx, catalog.AuthRequest{
		ProviderID: candidate.ProviderID, Auth: spec.host.Auth,
		KeyHeader: spec.host.KeyHeader, Region: spec.host.Variables["region"], Service: spec.host.Variables["service"],
		Selection: account.Selection{
			ConversationID: s.conversationAnchor(outcome, spec.request), Surface: string(outcome.kind),
			RequestID: outcome.requestID, Exclude: tried[candidate.ProviderID], Prefer: spec.prefer,
			Models: capabilityIDs(candidate.Model),
		},
	})
	if err != nil {
		return nil, false, antigravitySelectionError(candidate, effort)
	}
	spec.authorized = authorized
	return nil, false, nil
}

const maxAntigravityAccounts = 32

// antigravityRequestOf carries one account's selected configuration onto the
// wire: the SKU the attempt sends, the enum that labels it, the output ceiling
// it serves, and the thinking budget it publishes.
func antigravityRequestOf(selected catalog.AntigravityVariant) *wire.AntigravityRequest {
	return &wire.AntigravityRequest{
		WireModel: selected.WireModel, ModelEnum: selected.ModelEnum,
		MaxOutput: selected.MaxOutput, Context: selected.Context, ThinkingBudget: selected.ThinkingBudget,
	}
}

func (s *Server) antigravityConfigs(ctx context.Context, providerID, credentialID string) ([]catalog.AntigravityVariant, error) {
	if s.opts.RequestConfigs == nil || credentialID == "" {
		return nil, nil
	}
	stored, err := s.opts.RequestConfigs.ModelRequestConfigs(ctx, providerID, credentialID)
	if err != nil {
		return nil, fmt.Errorf("read the antigravity configurations: %w", err)
	}
	configs := make([]catalog.AntigravityVariant, 0, len(stored))
	for _, config := range stored {
		if variant, ok := catalog.DecodeAntigravityConfig(config); ok {
			configs = append(configs, variant)
		}
	}
	return configs, nil
}

func (s *Server) excludeAntigravityAccount(spec *attemptSpec, tried map[string][]string) bool {
	credential := spec.authorized.CredentialID
	if credential == "" {
		return false
	}
	for _, seen := range tried[spec.candidate.ProviderID] {
		if seen == credential {
			return false
		}
	}
	tried[spec.candidate.ProviderID] = append(tried[spec.candidate.ProviderID], credential)
	return true
}

func antigravitySelectionError(candidate routing.Candidate, _ string) error {
	return fmt.Errorf("no account serves the requested effort for %s: %w", candidate.ModelID, wire.ErrUnsupportedFeature)
}

// isClaudeAntigravityPassthrough reports whether one attempt names a Claude
// generation the vendor serves under its own id, which has no thinking to
// resolve. A collapsed Claude generation is not one of these.
func isClaudeAntigravityPassthrough(candidate routing.Candidate) bool {
	return catalog.ClaudeAntigravityPassthrough(candidate.ModelID) ||
		catalog.ClaudeAntigravityPassthrough(candidate.Model.ID) ||
		catalog.ClaudeAntigravityPassthrough(candidate.Model.UpstreamID)
}
