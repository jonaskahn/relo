package integration

import (
	"context"
	"errors"
)

// ModeRequest is the mode an operator asks one integration to run in, and
// whether Claude Desktop may bring Claude Code's proxy wiring up with it.
type ModeRequest struct {
	// Mode is ModeEnv or ModeProxy. Empty keeps the mode the integration has.
	Mode string
	// WithClaudeCode lets a Claude Desktop proxy request set Claude Code up in
	// proxy mode, or move it there, instead of being refused. Desktop's
	// processes read Claude Code's settings file, so the wiring is shared.
	WithClaudeCode bool
}

// withProxyOwner runs one Desktop mode change after making sure Claude Code's
// proxy wiring is there, and puts Claude Code back as it was when the change
// fails. Any other request runs untouched.
func (s *Service) withProxyOwner(ctx context.Context, id string, choice ModeRequest, run func() error) error {
	undo, err := s.raiseProxyOwner(ctx, id, choice)
	if err != nil {
		return err
	}
	if err := run(); err != nil {
		return errors.Join(err, undo())
	}
	return nil
}

// raiseProxyOwner brings Claude Code's proxy wiring up for a Desktop request
// that asked for it and answers how to undo what it did. Claude Code already in
// proxy mode needs nothing; one in the settings mode is moved over; one that is
// not set up is set up in proxy mode, which mints its key.
func (s *Service) raiseProxyOwner(ctx context.Context, id string, choice ModeRequest) (func() error, error) {
	untouched := func() error { return nil }
	if id != ProxyDesktop || !choice.WithClaudeCode {
		return untouched, nil
	}
	if mode, err := NormalizeMode(choice.Mode); err != nil || mode != ModeProxy {
		return untouched, err
	}
	manager, err := s.manager()
	if err != nil {
		return untouched, err
	}
	record, found, err := manager.store.Integration(ctx, ProxyOwner)
	if err != nil {
		return untouched, err
	}
	current, _ := NormalizeMode(record.Mode)
	switch {
	case found && record.Enabled && current == ModeProxy:
		return untouched, nil
	case found && record.Enabled:
		return moveOwnerToProxy(ctx, manager)
	}
	if _, err := s.enableAgent(ctx, ProxyOwner, enableRequest{mode: ModeRequest{Mode: ModeProxy}}); err != nil {
		return untouched, err
	}
	return func() error { return s.DisableIntegration(ctx, ProxyOwner, false) }, nil
}

// moveOwnerToProxy switches a Claude Code set up in the settings mode, and
// answers the switch back.
func moveOwnerToProxy(ctx context.Context, manager *Manager) (func() error, error) {
	if _, err := manager.SetMode(ctx, ProxyOwner, ModeProxy); err != nil {
		return func() error { return nil }, err
	}
	return func() error {
		_, err := manager.SetMode(ctx, ProxyOwner, ModeEnv)
		return err
	}, nil
}
