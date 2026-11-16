package claudebindings

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jonaskahn/relo/internal/application/docstore"
	"github.com/jonaskahn/relo/internal/catalog"
)

// Snapshots answers the catalog snapshot a binding's target is checked against.
type Snapshots interface {
	Snapshot() (*catalog.Snapshot, bool)
}

// Options wire the bindings service.
type Options struct {
	Documents *docstore.Collection[Document]
	Catalog   Snapshots
	// OnChange runs after a write is stored, so the caller can drop what it
	// derived from the old bindings, such as a catalog the client cached.
	OnChange func(ctx context.Context, integrationID string)
}

// Service reads and writes the bindings of the integrations that have them.
type Service struct {
	documents *docstore.Collection[Document]
	catalog   Snapshots
	onChange  func(ctx context.Context, integrationID string)
}

// New returns the bindings service.
func New(options Options) *Service {
	return &Service{documents: options.Documents, catalog: options.Catalog, onChange: options.OnChange}
}

// Load reads every document into memory, so the first request never waits on
// the store.
func (s *Service) Load(ctx context.Context) error {
	return s.documents.Reload(ctx)
}

// Reload reads the documents again, which is what a catalog change asks for.
func (s *Service) Reload(ctx context.Context) error {
	return s.documents.Reload(ctx)
}

// Resolve answers what a request's model resolves to for one integration. It
// reads memory only: the document was loaded at startup and is replaced whole
// on every write.
func (s *Service) Resolve(integrationID, model string) Resolution {
	document, found, err := s.documents.Get(context.Background(), integrationID)
	if err != nil || !found {
		return Resolution{}
	}
	var snapshot *catalog.Snapshot
	if s.catalog != nil {
		if current, ok := s.catalog.Snapshot(); ok {
			snapshot = current
		}
	}
	return resolve(document, snapshot, model)
}

// Change is one write: the bindings to set, and the native ids to remove.
type Change struct {
	Set    []Binding
	Remove []string
}

// State is where one binding stands against the live catalog.
type State string

// The states a binding reports.
const (
	StateBound    State = "bound"
	StateDangling State = "dangling"
	StateDisabled State = "disabled"
)

// Entry is one binding with its state.
type Entry struct {
	Binding
	State State
}

// Official is one native Claude id an operator may bind.
type Official struct {
	ID     string
	Family string
	// Source says where the id was learned: seed, connection, or binding for an
	// id the operator typed.
	Source string
}

// TargetOption is one Relo route or model a binding can point at, with the
// label an operator reads.
type TargetOption struct {
	Target
	Label string
}

// View is everything the bindings screen shows for one integration.
type View struct {
	Entries  []Entry
	Official []Official
	Targets  []TargetOption
}

// Sources an official id is learned from.
const (
	SourceSeed       = "seed"
	SourceConnection = "connection"
	SourceBinding    = "binding"
)

// View answers the bindings of one integration and the ids offered for more.
func (s *Service) View(ctx context.Context, integrationID string) (View, error) {
	document, _, err := s.documents.Get(ctx, integrationID)
	if err != nil {
		return View{}, err
	}
	snapshot := s.snapshot()
	view := View{Official: s.official(snapshot, document), Targets: targets(snapshot)}
	for _, binding := range document.Bindings {
		view.Entries = append(view.Entries, Entry{Binding: binding, State: stateOf(snapshot, binding)})
	}
	return view, nil
}

// Apply validates a change against the live catalog and stores the resulting
// document whole.
func (s *Service) Apply(ctx context.Context, integrationID string, change Change) error {
	document, _, err := s.documents.Get(ctx, integrationID)
	if err != nil {
		return err
	}
	merged, err := s.merge(document, change)
	if err != nil {
		return err
	}
	if err := s.documents.Put(ctx, integrationID, NewDocument(merged)); err != nil {
		return err
	}
	if s.onChange != nil {
		s.onChange(ctx, integrationID)
	}
	return nil
}

// merge applies a change to the stored bindings, in id order.
func (s *Service) merge(document Document, change Change) ([]Binding, error) {
	snapshot := s.snapshot()
	kept := make(map[string]Binding, len(document.Bindings))
	for _, binding := range document.Bindings {
		kept[binding.NativeID] = binding
	}
	for _, id := range change.Remove {
		delete(kept, NormalizeID(id))
	}
	for _, requested := range change.Set {
		binding, err := normalize(requested)
		if err != nil {
			return nil, err
		}
		if snapshot != nil && binding.Enabled {
			if _, reachable := spell(snapshot, binding.Target, false); !reachable {
				return nil, fmt.Errorf("%s: %w (%s is not available)", binding.NativeID, ErrInvalidTarget, describe(binding.Target))
			}
		}
		kept[binding.NativeID] = binding
	}
	if len(kept) > maxBindings {
		return nil, fmt.Errorf("%w (at most %d)", ErrTooMany, maxBindings)
	}
	merged := make([]Binding, 0, len(kept))
	for _, binding := range kept {
		merged = append(merged, binding)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].NativeID < merged[j].NativeID })
	return merged, nil
}

func (s *Service) snapshot() *catalog.Snapshot {
	if s.catalog == nil {
		return nil
	}
	snapshot, found := s.catalog.Snapshot()
	if !found {
		return nil
	}
	return snapshot
}

func stateOf(snapshot *catalog.Snapshot, binding Binding) State {
	if !binding.Enabled {
		return StateDisabled
	}
	if snapshot == nil {
		return StateDangling
	}
	if _, reachable := spell(snapshot, binding.Target, false); !reachable {
		return StateDangling
	}
	return StateBound
}

func describe(target Target) string {
	if target.Kind == TargetRoute {
		return "route " + target.GroupID
	}
	return target.ProviderID + "/" + target.ModelID
}

// official merges the ids an operator can bind: the seed, the native ids a
// Claude connection lists, and the ids already bound.
func (s *Service) official(snapshot *catalog.Snapshot, document Document) []Official {
	seen := map[string]bool{}
	var listed []Official
	add := func(id, source string) {
		id = NormalizeID(id)
		if id == "" || seen[id] || validID(id) != nil {
			return
		}
		seen[id] = true
		listed = append(listed, Official{ID: id, Family: catalog.ClaudeFamily(id), Source: source})
	}
	for _, id := range catalog.ClaudeOfficialSeed() {
		add(id, SourceSeed)
	}
	if snapshot != nil {
		for _, host := range snapshot.Providers {
			if host.TemplateID != catalog.ClaudeSubscriptionTemplate {
				continue
			}
			for _, model := range snapshot.Models(host.ID) {
				if strings.HasPrefix(strings.ToLower(model.ID), catalog.ClaudePrefix) {
					add(model.ID, SourceConnection)
				}
			}
		}
	}
	for _, binding := range document.Bindings {
		add(binding.NativeID, SourceBinding)
	}
	return listed
}

// targets lists what a binding can point at now: every route that has a member
// able to answer, then every model a connection can serve.
func targets(snapshot *catalog.Snapshot) []TargetOption {
	if snapshot == nil {
		return nil
	}
	var options []TargetOption
	for _, group := range snapshot.Groups {
		if group.Enabled && len(snapshot.GroupModels(group)) > 0 {
			options = append(options, TargetOption{
				Target: Target{Kind: TargetRoute, GroupID: group.ID}, Label: group.Label,
			})
		}
	}
	for _, host := range snapshot.Providers {
		for _, model := range snapshot.Models(host.ID) {
			if !snapshot.Eligible(model, catalog.Requirements{}) {
				continue
			}
			name := model.Name
			if name == "" {
				name = model.ID
			}
			options = append(options, TargetOption{
				Target: Target{Kind: TargetModel, ProviderID: host.ID, ModelID: model.ID},
				Label:  host.Label + " / " + name,
			})
		}
	}
	return options
}
