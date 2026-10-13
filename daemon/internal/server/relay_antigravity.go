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
func (s *Server) selectAntigravityAttempt(ctx context.Context, spec *attemptSpec, tried map[string][]string) (*wire.AntigravityRequest, error) {
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
		return &wire.AntigravityRequest{WireModel: selected.WireModel, ModelEnum: selected.ModelEnum, MaxOutput: selected.MaxOutput}, true, nil
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
