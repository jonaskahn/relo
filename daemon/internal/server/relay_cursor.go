// Cursor attempt selection: resolving one request to one account's tuple.
package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/routing"
)

// CursorConfigSource reads one account's Cursor request tuples. The
// composition root backs it with the catalog store.
type CursorConfigSource interface {
	CursorConfigs(ctx context.Context, providerID, credentialID string) ([]catalog.CursorVariant, error)
}

// selectCursorAttempt resolves the exact Cursor tuple one attempt sends. An
// account that serves the group but not the requested effort is excluded
// and the next account is authorized, so failover keeps the requested
// selection instead of substituting another effort.
func (s *Server) selectCursorAttempt(ctx context.Context, spec *attemptSpec, tried map[string][]string) (*wire.CursorRequest, error) {
	effort := ""
	if spec.outcome.request != nil {
		effort, _ = catalog.RequestEffort(spec.outcome.request.Reasoning)
	}
	if effort != "" {
		if _, ok := catalog.NormalizeCursorEffort(effort); !ok {
			return nil, fmt.Errorf("cursor effort %q: %w", effort, wire.ErrUnsupportedFeature)
		}
	}
	for range maxCursorAccounts {
		selected, ok, err := s.matchCursorAccount(ctx, spec, tried, effort)
		if err != nil || ok {
			return selected, err
		}
	}
	return nil, cursorSelectionError(spec.candidate, effort)
}

func (s *Server) matchCursorAccount(ctx context.Context, spec *attemptSpec, tried map[string][]string, effort string) (*wire.CursorRequest, bool, error) {
	candidate, outcome := spec.candidate, spec.outcome
	configs, err := s.cursorConfigs(ctx, candidate.ProviderID, spec.authorized.CredentialID)
	if err != nil {
		return nil, false, err
	}
	if selected, ok := catalog.SelectCursorVariant(configs, candidate.ModelID, effort); ok {
		return cursorRequestOf(s.conversationAnchor(outcome, spec.request), spec, selected), true, nil
	}
	if !s.excludeCursorAccount(spec, tried) {
		return nil, false, cursorSelectionError(candidate, effort)
	}
	authorized, err := s.opts.Relay.Authorize(ctx, catalog.AuthRequest{
		ProviderID: candidate.ProviderID, Auth: spec.host.Auth,
		Selection: account.Selection{
			ConversationID: s.conversationAnchor(outcome, spec.request), Surface: string(outcome.kind),
			RequestID: outcome.requestID, Exclude: tried[candidate.ProviderID], Prefer: spec.prefer,
			Models: capabilityIDs(candidate.Model),
		},
	})
	if err != nil {
		return nil, false, cursorSelectionError(candidate, effort)
	}
	spec.authorized = authorized
	return nil, false, nil
}

const maxCursorAccounts = 32

func (s *Server) cursorConfigs(ctx context.Context, providerID, credentialID string) ([]catalog.CursorVariant, error) {
	if s.opts.CursorConfigs == nil || credentialID == "" {
		return nil, fmt.Errorf("cursor selection unavailable: no configuration source")
	}
	configs, err := s.opts.CursorConfigs.CursorConfigs(ctx, providerID, credentialID)
	if err != nil {
		return nil, fmt.Errorf("read the cursor configurations: %w", err)
	}
	return configs, nil
}

func (s *Server) excludeCursorAccount(spec *attemptSpec, tried map[string][]string) bool {
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

func cursorRequestOf(anchor string, spec *attemptSpec, selected catalog.CursorVariant) *wire.CursorRequest {
	params := make([]wire.CursorParam, 0, len(selected.Params))
	for _, param := range selected.Params {
		params = append(params, wire.CursorParam{ID: param.ID, Value: param.Value})
	}
	return &wire.CursorRequest{
		ConversationID: cursorConversationID(spec.candidate.ProviderID, spec.authorized.CredentialID, anchor),
		ModelID:        selected.ModelID,
		DetailsID:      selected.DetailsID,
		MaxMode:        selected.MaxMode,
		Params:         params,
	}
}

// cursorConversationID derives the stable upstream conversation id from the
// inbound anchor, namespaced by connection and account: one conversation
// keeps its thread across turns and restarts, and accounts never share
// server state. It carries no secret, only stored identifiers.
func cursorConversationID(providerID, credentialID, anchor string) string {
	sum := sha256.Sum256([]byte(providerID + "\x00" + credentialID + "\x00" + anchor))
	raw := sum[:16]
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	return hex.EncodeToString(raw[:4]) + "-" + hex.EncodeToString(raw[4:6]) + "-" +
		hex.EncodeToString(raw[6:8]) + "-" + hex.EncodeToString(raw[8:10]) + "-" + hex.EncodeToString(raw[10:])
}

func cursorSelectionError(candidate routing.Candidate, effort string) error {
	if effort == "" {
		return fmt.Errorf("%s: %w", candidate.ModelID, account.ErrNoModelAccount)
	}
	return fmt.Errorf("%s effort %s: %w", candidate.ModelID, effort, account.ErrNoModelAccount)
}
