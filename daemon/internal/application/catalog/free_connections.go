package catalog

import (
	"context"
	"errors"
	"fmt"
	"github.com/jonaskahn/relo/internal/catalog"
)

// ErrFreeConnectionsDisabled refuses a free-pool operation without opt-in.
var ErrFreeConnectionsDisabled = errors.New("free connections are disabled; enable them and accept responsibility before using them")

// ErrFreeConnectionExists refuses a second keyless free connection, which is
// the one shape a pool cannot serve twice.
var ErrFreeConnectionExists = errors.New("this free connection is already added")

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

// checkSingleFreeConnection refuses a new keyless free connection when its
// template already has a stored row. Adding a key to an existing row is not a
// second connection and stays allowed.
func (s *Service) checkSingleFreeConnection(ctx context.Context, templateID, providerID string) error {
	if !catalog.IsFreeTemplate(templateID) || providerID != "" {
		return nil
	}
	row, found, err := s.findStoredTemplateConnection(ctx, templateID)
	if err != nil {
		return err
	}
	if found {
		return fmt.Errorf("%s: %w", row.ID, ErrFreeConnectionExists)
	}
	return nil
}

// findStoredTemplateConnection reads the stored rows rather than the snapshot,
// so a free row the management projection hides is still visible to the
// single-instance rule.
func (s *Service) findStoredTemplateConnection(ctx context.Context, templateID string) (catalog.ConnectionRecord, bool, error) {
	rows, err := s.store.ListConnections(ctx)
	if err != nil {
		return catalog.ConnectionRecord{}, false, err
	}
	for _, row := range rows {
		if row.TemplateID == templateID {
			return row, true, nil
		}
	}
	return catalog.ConnectionRecord{}, false, nil
}
