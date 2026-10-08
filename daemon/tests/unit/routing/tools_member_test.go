package routing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	approuting "github.com/jonaskahn/relo/internal/application/routing"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

// toolRoute is a route service over two models whose tool capability a test
// then sets, so that the refusal can be measured against one difference.
type toolRoute struct {
	service *approuting.Service
	catalog *catalog.Catalog
	repo    *sqlite.CatalogRepo
}

func newToolRoute(t *testing.T) *toolRoute {
	t.Helper()
	db := testkit.OpenTestDB(t)
	repo := sqlite.NewCatalogRepo(db)
	if err := repo.SaveProvider(context.Background(), sqlite.ProviderRow{
		ID: "openai", Origin: string(catalog.OriginCustom), Label: "OpenAI",
		Auth: string(catalog.AuthAPIKey), APIFormat: string(catalog.FormatOpenAIChat),
		BaseURL: "https://openai.example/v1", ModelsFormat: string(catalog.ModelsNone),
		Headers: map[string]string{}, Variables: map[string]string{},
		Enabled: true, Rank: 100, PoolStrategy: sqlite.StrategyLeastLoaded,
	}); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	for _, id := range []string{"alpha", "beta"} {
		if err := repo.SaveModel(context.Background(), sqlite.ModelRow{
			ProviderID: "openai", ModelID: id, Source: "manual", Enabled: true,
		}); err != nil {
			t.Fatalf("SaveModel(%s) error = %v", id, err)
		}
		category := string(catalog.CategoryChat)
		if err := repo.SaveModelFacts(context.Background(), sqlite.ModelFactsRow{
			ProviderID: "openai", ModelID: id, Layer: "override", Category: &category,
		}); err != nil {
			t.Fatalf("SaveModelFacts(%s) error = %v", id, err)
		}
	}
	built := testkit.ReloadCatalog(t, db, accounts{})
	return &toolRoute{
		service: approuting.New(approuting.Options{Snapshot: snapshotPort{built}, Routes: repo, Reload: built}),
		catalog: built,
		repo:    repo,
	}
}

// snapshotPort adapts the catalog to the error-returning snapshot port the
// route service reads.
type snapshotPort struct{ built *catalog.Catalog }

func (p snapshotPort) Snapshot() (*catalog.Snapshot, error) {
	snapshot, found := p.built.Snapshot()
	if !found {
		return nil, catalog.ErrNoCatalog
	}
	return snapshot, nil
}

func (r *toolRoute) stateTools(t *testing.T, modelID string, value bool) {
	t.Helper()
	if err := r.repo.SaveModelFacts(context.Background(), sqlite.ModelFactsRow{
		ProviderID: "openai", ModelID: modelID, Layer: "override", SupportsTools: &value,
	}); err != nil {
		t.Fatalf("SaveModelFacts(%s) error = %v", modelID, err)
	}
	if err := r.catalog.Reload(context.Background()); err != nil {
		t.Fatalf("reload catalog: %v", err)
	}
}

func (r *toolRoute) save(members ...approuting.MemberWrite) error {
	return r.service.Save(context.Background(), "mixed", approuting.Write{
		Strategy: "priority", Enabled: true, Listed: true, Members: members,
	})
}

func toolMember(providerID, modelID string, enabled bool) approuting.MemberWrite {
	return approuting.MemberWrite{ProviderID: providerID, ModelID: modelID, Weight: 1, Enabled: enabled}
}

// TestSaveExpandsAnAutoMemberInTheToolRule keeps a bare identifier from
// bypassing the rule: every connection serving it answers for the member.
func TestSaveExpandsAnAutoMemberInTheToolRule(t *testing.T) {
	route := newToolRoute(t)
	route.stateTools(t, "alpha", true)
	route.stateTools(t, "beta", false)
	auto := approuting.MemberWrite{ModelID: "alpha", Kind: "auto", Weight: 1, Enabled: true}
	if err := route.save(auto, toolMember("openai", "beta", true)); !errors.Is(err, approuting.ErrInvalidRouteMember) {
		t.Fatalf("Save() error = %v, want an auto member expanded into the rule", err)
	}
}

// TestSaveRefusesMixingToolAndToolLessMembers is the rule the editor mirrors:
// a client cannot be promised a tool call when one member states it cannot
// make one.
func TestSaveRefusesMixingToolAndToolLessMembers(t *testing.T) {
	route := newToolRoute(t)
	route.stateTools(t, "alpha", true)
	route.stateTools(t, "beta", false)
	want := approuting.ErrInvalidRouteMember
	if err := route.save(toolMember("openai", "alpha", true), toolMember("openai", "beta", true)); !errors.Is(err, want) {
		t.Fatalf("Save() error = %v, want a tool conflict refusal", err)
	}
	if _, err := route.repo.GetGroup(context.Background(), "mixed"); err == nil {
		t.Fatal("a refused save stored the route")
	}
}

// TestSaveKeepsAGroupWhenOnlyOneSideStatesTools lets a silent capability pass,
// because an unstated tool capability is not a model that refuses tools.
func TestSaveKeepsAGroupWhenOnlyOneSideStatesTools(t *testing.T) {
	route := newToolRoute(t)
	route.stateTools(t, "alpha", true)
	if err := route.save(toolMember("openai", "alpha", true), toolMember("openai", "beta", true)); err != nil {
		t.Fatalf("Save() error = %v, want a silent member to raise nothing", err)
	}
}

// TestSaveIgnoresDisabledMembersInTheToolRule keeps a switched-off member from
// blocking a route whose live members agree.
func TestSaveIgnoresDisabledMembersInTheToolRule(t *testing.T) {
	route := newToolRoute(t)
	route.stateTools(t, "alpha", false)
	route.stateTools(t, "beta", true)
	if err := route.save(toolMember("openai", "alpha", false), toolMember("openai", "beta", true)); err != nil {
		t.Fatalf("Save() error = %v, want a disabled member left out of the rule", err)
	}
}
