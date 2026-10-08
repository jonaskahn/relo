// Package routing owns route save and delete: validating members against the
// live catalog snapshot and persisting the route the router reads.
package routing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jonaskahn/relo/internal/catalog"
)

// ErrInvalidRouteMember reports a route member the catalog cannot serve.
var ErrInvalidRouteMember = errors.New("invalid route member")

// MemberWrite is one member an operator saves on a route.
type MemberWrite struct {
	ProviderID string
	ModelID    string
	Kind       string
	Weight     int
	Enabled    bool
}

// Write is the editable fields of one route.
type Write struct {
	Label    string
	Strategy string
	Enabled  bool
	Listed   bool
	Members  []MemberWrite
	// SwitchOn4xx and SwitchOn5xx move the route to its next member on an
	// unlisted status of that class.
	SwitchOn4xx bool
	SwitchOn5xx bool
}

// Snapshot reads the catalog state route validation runs against.
type Snapshot interface {
	Snapshot() (*catalog.Snapshot, error)
}

// Routes persists routes after validation.
type Routes interface {
	SaveRoute(ctx context.Context, route catalog.RouteRecord) error
	DeleteRoute(ctx context.Context, id string) error
}

// Reloader rebuilds the runtime catalog after a route changes.
type Reloader interface {
	Reload(ctx context.Context) error
}

// Options wires one routing service.
type Options struct {
	Snapshot Snapshot
	Routes   Routes
	Reload   Reloader
}

// Service saves and deletes routes.
type Service struct {
	snapshot Snapshot
	routes   Routes
	reload   Reloader
}

// New returns routing use cases over the given ports.
func New(opts Options) *Service {
	return &Service{snapshot: opts.Snapshot, routes: opts.Routes, reload: opts.Reload}
}

// Save validates and stores one route, then reloads the catalog.
func (s *Service) Save(ctx context.Context, id string, write Write) error {
	snapshot, err := s.snapshot.Snapshot()
	if err != nil {
		return err
	}
	now := catalog.NowMs()
	route := catalog.RouteRecord{
		ID: id, Label: write.Label, Strategy: write.Strategy,
		Enabled: write.Enabled, Listed: write.Listed,
		CreatedAtMs: now, UpdatedAtMs: now,
		SwitchOn4xx: new(write.SwitchOn4xx), SwitchOn5xx: new(write.SwitchOn5xx),
	}
	for i, member := range write.Members {
		validated, err := validateMember(snapshot, id, member)
		if err != nil {
			return err
		}
		validated.Position = i
		route.Members = append(route.Members, validated)
	}
	if err := validateToolMembers(snapshot, id, route.Members); err != nil {
		return err
	}
	if err := s.routes.SaveRoute(ctx, route); err != nil {
		return err
	}
	return s.reload.Reload(ctx)
}

// Delete removes one route and reloads the catalog.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.routes.DeleteRoute(ctx, id); err != nil {
		return err
	}
	return s.reload.Reload(ctx)
}

func validateMember(snapshot *catalog.Snapshot, groupID string, write MemberWrite) (catalog.RouteMemberRecord, error) {
	kind := catalog.MemberKindOf(write.Kind)
	modelID := strings.TrimSpace(write.ModelID)
	if modelID == "" {
		return catalog.RouteMemberRecord{}, fmt.Errorf("%s: %w (a model is required)", groupID, ErrInvalidRouteMember)
	}
	providerID := strings.TrimSpace(write.ProviderID)
	if kind == catalog.MemberKindAuto {
		providerID = ""
		if len(snapshot.ModelsNamed(modelID)) == 0 {
			return catalog.RouteMemberRecord{}, fmt.Errorf("%s: %w (no connection serves %s)", groupID, ErrInvalidRouteMember, modelID)
		}
	} else if _, found := snapshot.Model(providerID, modelID); !found {
		return catalog.RouteMemberRecord{}, fmt.Errorf("%s/%s: %w", providerID, modelID, ErrInvalidRouteMember)
	}
	return catalog.RouteMemberRecord{
		ProviderID: providerID, ModelID: modelID, Kind: kind,
		Weight: write.Weight, Enabled: write.Enabled,
	}, nil
}

func validateToolMembers(snapshot *catalog.Snapshot, groupID string, members []catalog.RouteMemberRecord) error {
	var calling, refusing []string
	for _, member := range members {
		if !member.Enabled {
			continue
		}
		for _, model := range memberModels(snapshot, member) {
			if model.SupportsTools == nil {
				continue
			}
			name := model.ProviderID + "/" + model.ID
			if *model.SupportsTools {
				calling = append(calling, name)
			} else {
				refusing = append(refusing, name)
			}
		}
	}
	if len(calling) == 0 || len(refusing) == 0 {
		return nil
	}
	return fmt.Errorf("%s: %w (tools: %s cannot share a route with %s)",
		groupID, ErrInvalidRouteMember, strings.Join(calling, ", "), strings.Join(refusing, ", "))
}

func memberModels(snapshot *catalog.Snapshot, member catalog.RouteMemberRecord) []catalog.Model {
	if catalog.MemberKindOf(member.Kind) == catalog.MemberKindAuto {
		return snapshot.ModelsNamed(member.ModelID)
	}
	model, found := snapshot.Model(member.ProviderID, member.ModelID)
	if !found {
		return nil
	}
	return []catalog.Model{model}
}
