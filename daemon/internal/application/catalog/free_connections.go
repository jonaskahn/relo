package catalog

import (
	"context"
	"errors"
	"github.com/jonaskahn/relo/internal/catalog"
)

// ErrFreeConnectionsDisabled refuses a free-pool operation without opt-in.
var ErrFreeConnectionsDisabled = errors.New("free connections are disabled; enable them and accept responsibility before using them")

// FreeConnectionsEnabled reports the operator's current choice.
func (s *Service) FreeConnectionsEnabled() bool {
	return s.freeConnections == nil || s.freeConnections()
}

func (s *Service) checkFreeConnection(ctx context.Context, templateID, providerID string) error {
	if s.FreeConnectionsEnabled() {
		return nil
	}
	if providerID != "" {
		row, err := s.store.GetProvider(ctx, providerID)
		if err != nil {
			return err
		}
		templateID = row.TemplateID
	}
	if catalog.IsFreeTemplate(templateID) {
		return ErrFreeConnectionsDisabled
	}
	return nil
}
