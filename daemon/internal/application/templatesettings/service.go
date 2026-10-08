// Package templatesettings owns the choices stored for one provider template
// and read from a connection: token refresh.
package templatesettings

import (
	"context"
	"errors"
	"fmt"

	"github.com/jonaskahn/relo/internal/clock"
)

var supportedTemplates = map[string]Setting{
	"claude":       {AutoRefresh: true},
	"openai-codex": {AutoRefresh: true},
}

var (
	// ErrNotFound reports a connection the catalog does not have.
	ErrNotFound = errors.New("connection not found")
	// ErrUnsupported reports a connection whose template stores nothing here.
	ErrUnsupported = errors.New("this connection has no template settings")
)

// Setting is what the connection settings tab reads and writes. Whether a model
// is published twice, at 200K and at 1M, follows from the model itself now, so
// no stored choice governs it.
type Setting struct {
	AutoRefresh bool
}

// Patch changes only the fields the caller sent.
type Patch struct {
	AutoRefresh *bool
}

// Store persists one template's choices. The long-context column is still read
// and written so a row written by an earlier build keeps its place, but no
// decision is taken from it any more.
type Store interface {
	Get(ctx context.Context, templateID string) (longContext, autoRefresh bool, found bool, err error)
	Save(ctx context.Context, templateID string, longContext, autoRefresh *bool, nowMs int64) error
}

// Connections names the template behind one connection.
type Connections interface {
	TemplateID(providerID string) (string, bool)
}

// Options configure the use cases.
type Options struct {
	Store       Store
	Connections Connections
	Clock       clock.Clock
}

// Service is the template-settings use cases.
type Service struct {
	store       Store
	connections Connections
	clock       clock.Clock
}

// New returns the use cases over the given store.
func New(options Options) *Service {
	clk := options.Clock
	if clk == nil {
		clk = clock.New()
	}
	return &Service{
		store: options.Store, connections: options.Connections, clock: clk,
	}
}

// Read returns the choices for one connection. A template other than a
// supported sign-in is refused, and a connection with no stored row keeps that
// template's defaults.
func (s *Service) Read(ctx context.Context, providerID string) (Setting, error) {
	templateID, err := s.template(providerID)
	if err != nil {
		return Setting{}, err
	}
	_, autoRefresh, found, err := s.store.Get(ctx, templateID)
	if err != nil {
		return Setting{}, err
	}
	if !found {
		return supportedTemplates[templateID], nil
	}
	return Setting{AutoRefresh: autoRefresh}, nil
}

// Update stores the fields the patch names. Both stored columns are written
// explicitly, so a row keeps its place and only the choice the console owns
// changes.
func (s *Service) Update(ctx context.Context, providerID string, patch Patch) (Setting, error) {
	templateID, err := s.template(providerID)
	if err != nil {
		return Setting{}, err
	}
	if patch.AutoRefresh == nil {
		return s.Read(ctx, providerID)
	}
	longContext, autoRefresh, _, err := s.store.Get(ctx, templateID)
	if err != nil {
		return Setting{}, err
	}
	if patch.AutoRefresh != nil {
		autoRefresh = *patch.AutoRefresh
	}
	if err := s.store.Save(ctx, templateID, &longContext, &autoRefresh, s.clock.Now().UnixMilli()); err != nil {
		return Setting{}, err
	}
	return s.Read(ctx, providerID)
}

func (s *Service) template(providerID string) (string, error) {
	templateID, found := s.connections.TemplateID(providerID)
	if !found {
		return "", fmt.Errorf("%s: %w", providerID, ErrNotFound)
	}
	if _, supported := supportedTemplates[templateID]; !supported {
		return "", fmt.Errorf("%s: %w", templateID, ErrUnsupported)
	}
	return templateID, nil
}
