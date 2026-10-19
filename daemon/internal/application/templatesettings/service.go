// Package templatesettings owns the choices stored for one provider template
// and read from a connection: token refresh.
package templatesettings

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jonaskahn/relo/internal/clock"
)

var supportedTemplates = map[string]Setting{
	"claude":             {AutoRefresh: true},
	"openai-codex":       {AutoRefresh: true},
	"google-antigravity": {AutoRefresh: true, ModelPrefixes: []string{"claude", "gemini"}},
	"grok":               {AutoRefresh: true},
	"opencode-free":      {ToolCutoff: true},
}

var (
	// ErrNotFound reports a connection the catalog does not have.
	ErrNotFound = errors.New("connection not found")
	// ErrUnsupported reports a connection whose template stores nothing here.
	ErrUnsupported = errors.New("this connection has no template settings")
	// ErrInvalidPrefixes reports a model prefix list the roster cannot use.
	ErrInvalidPrefixes = errors.New("model prefixes must be comma-separated names without spaces")
)

// Setting is what the connection settings tab reads and writes. Whether a model
// is published twice, at 200K and at 1M, follows from the model itself now, so
// no stored choice governs it.
type Setting struct {
	AutoRefresh   bool
	ToolCutoff    bool
	ModelPrefixes []string
}

// Patch changes only the fields the caller sent.
type Patch struct {
	AutoRefresh   *bool
	ToolCutoff    *bool
	ModelPrefixes *[]string
}

// Store persists one template's choices. The long-context column is still read
// and written so a row written by an earlier build keeps its place, but no
// decision is taken from it any more.
type Store interface {
	Get(ctx context.Context, templateID string) (longContext, autoRefresh, toolCutoff bool, found bool, err error)
	Save(ctx context.Context, templateID string, longContext, autoRefresh *bool, nowMs int64) error
	SaveToolCutoff(ctx context.Context, templateID string, toolCutoff bool, nowMs int64) error
	Prefixes(ctx context.Context, templateID string) ([]string, bool, error)
	SavePrefixes(ctx context.Context, templateID string, prefixes []string, nowMs int64) error
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
	_, autoRefresh, toolCutoff, found, err := s.store.Get(ctx, templateID)
	if err != nil {
		return Setting{}, err
	}
	defaults := supportedTemplates[templateID]
	if !found {
		return defaults, nil
	}
	prefixes, _, err := s.store.Prefixes(ctx, templateID)
	if err != nil {
		return Setting{}, err
	}
	if prefixes == nil {
		prefixes = defaults.ModelPrefixes
	}
	return Setting{AutoRefresh: autoRefresh, ToolCutoff: toolCutoff, ModelPrefixes: prefixes}, nil
}

// Update stores the fields the patch names and leaves the others as stored.
func (s *Service) Update(ctx context.Context, providerID string, patch Patch) (Setting, error) {
	templateID, err := s.template(providerID)
	if err != nil {
		return Setting{}, err
	}
	if patch.AutoRefresh == nil && patch.ToolCutoff == nil && patch.ModelPrefixes == nil {
		return s.Read(ctx, providerID)
	}
	if err := s.writePatch(ctx, templateID, patch); err != nil {
		return Setting{}, err
	}
	return s.Read(ctx, providerID)
}

func (s *Service) writePatch(ctx context.Context, templateID string, patch Patch) error {
	now := s.clock.Now().UnixMilli()
	longContext, _, _, _, err := s.store.Get(ctx, templateID)
	if err != nil {
		return err
	}
	if patch.AutoRefresh != nil {
		if err := s.store.Save(ctx, templateID, &longContext, patch.AutoRefresh, now); err != nil {
			return err
		}
	}
	if patch.ToolCutoff != nil {
		if err := s.store.SaveToolCutoff(ctx, templateID, *patch.ToolCutoff, now); err != nil {
			return err
		}
	}
	if patch.ModelPrefixes != nil {
		normalized, err := normalizePrefixes(*patch.ModelPrefixes)
		if err != nil {
			return err
		}
		if err := s.store.SavePrefixes(ctx, templateID, normalized, now); err != nil {
			return err
		}
	}
	return nil
}

func normalizePrefixes(prefixes []string) ([]string, error) {
	normalized := make([]string, 0, len(prefixes))
	seen := map[string]bool{}
	for _, prefix := range prefixes {
		trimmed := strings.ToLower(strings.TrimSpace(prefix))
		if trimmed == "" {
			continue
		}
		if strings.ContainsAny(trimmed, " \t,") {
			return nil, ErrInvalidPrefixes
		}
		if seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		normalized = append(normalized, trimmed)
	}
	return normalized, nil
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
