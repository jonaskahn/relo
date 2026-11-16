package claudebindings_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/application/claudebindings"
	"github.com/jonaskahn/relo/internal/application/docstore"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

type accounts struct{}

func (accounts) Active(string) int                         { return 1 }
func (accounts) Available(string) bool                     { return true }
func (accounts) CooldownUntil(string) time.Time            { return time.Time{} }
func (accounts) ServesModel(string, catalog.Model) bool    { return true }
func (accounts) Coverage(string, catalog.Model) (int, int) { return 1, 1 }

// documentAdapter lets the tests run the collection over the real table.
type documentAdapter struct{ repo *sqlite.Documents }

func (a documentAdapter) List(ctx context.Context, namespace string) ([]docstore.Row, error) {
	rows, err := a.repo.List(ctx, namespace)
	listed := make([]docstore.Row, 0, len(rows))
	for _, row := range rows {
		listed = append(listed, docstore.Row{ID: row.ID, Version: row.Version, Body: row.Body})
	}
	return listed, err
}

func (a documentAdapter) Put(ctx context.Context, namespace string, row docstore.Row, nowMs int64) error {
	return a.repo.Put(ctx, namespace, sqlite.DocumentRow{ID: row.ID, Version: row.Version, Body: row.Body}, nowMs)
}

func (a documentAdapter) Delete(ctx context.Context, namespace, id string) error {
	return a.repo.Delete(ctx, namespace, id)
}

type fixture struct {
	db      *sqlite.DB
	catalog *catalog.Catalog
	service *claudebindings.Service
	changed []string
	docs    *docstore.Collection[claudebindings.Document]
}

// documents is the collection the service reads, so a second service can share it.
func (f *fixture) documents() *docstore.Collection[claudebindings.Document] { return f.docs }

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"alpha"}, ModelsFormat: catalog.ModelsNone,
	})
	saveRoute(t, db, "cheap")
	built := testkit.ReloadCatalog(t, db, accounts{})
	collection := docstore.NewCollection(documentAdapter{repo: sqlite.NewDocuments(db)}, claudebindings.Namespace,
		docstore.Codec[claudebindings.Document]{Decode: claudebindings.Decode, Encode: claudebindings.Encode},
		docstore.Options{})
	f := &fixture{db: db, catalog: built, docs: collection}
	f.service = claudebindings.New(claudebindings.Options{
		Documents: collection, Catalog: snapshots{built},
		OnChange: func(_ context.Context, id string) { f.changed = append(f.changed, id) },
	})
	return f
}

type snapshots struct{ catalog *catalog.Catalog }

func (s snapshots) Snapshot() (*catalog.Snapshot, bool) { return s.catalog.Snapshot() }

func saveRoute(t *testing.T, db *sqlite.DB, id string) {
	t.Helper()
	if err := sqlite.NewCatalogRepo(db).SaveRoute(context.Background(), catalog.RouteRecord{
		ID: id, Label: id, Strategy: string(catalog.StrategyPriority), Enabled: true, Listed: true,
		Members: []catalog.RouteMemberRecord{{
			ProviderID: "openai", ModelID: "alpha", Kind: catalog.MemberKindModel, Weight: 1, Enabled: true,
		}},
	}); err != nil {
		t.Fatalf("save route %s: %v", id, err)
	}
}

func modelBinding(nativeID string) claudebindings.Binding {
	return claudebindings.Binding{
		NativeID: nativeID, Enabled: true,
		Target: claudebindings.Target{Kind: claudebindings.TargetModel, ProviderID: "openai", ModelID: "alpha"},
	}
}

func apply(t *testing.T, f *fixture, set ...claudebindings.Binding) {
	t.Helper()
	if err := f.service.Apply(context.Background(), "claude-code", claudebindings.Change{Set: set}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
}

func TestADatedOrMarkedOrUpperCaseIdHitsTheBinding(t *testing.T) {
	f := newFixture(t)
	apply(t, f, modelBinding("claude-haiku-4-5"))
	for _, spelled := range []string{
		"claude-haiku-4-5", "claude-haiku-4-5-20251001", "claude-haiku-4-5[1m]", "Claude-Haiku-4-5",
	} {
		resolution := f.service.Resolve("claude-code", spelled)
		if !resolution.Matched || resolution.Rewrite == "" {
			t.Fatalf("Resolve(%q) = %+v, want the binding", spelled, resolution)
		}
	}
	if resolution := f.service.Resolve("claude-code", "claude-sonnet-4-6"); resolution.Matched || resolution.Reason != "" {
		t.Fatalf("an unbound id resolved to %+v", resolution)
	}
}

func TestTheRewriteIsAnAliasTheDataPlaneResolves(t *testing.T) {
	f := newFixture(t)
	apply(t, f, modelBinding("claude-haiku-4-5"), claudebindings.Binding{
		NativeID: "claude-opus-5", Enabled: true,
		Target: claudebindings.Target{Kind: claudebindings.TargetRoute, GroupID: "cheap"},
	})
	snapshot, _ := f.catalog.Snapshot()
	for id, want := range map[string]string{
		"claude-haiku-4-5": "claude-relo-openai--alpha",
		"claude-opus-5":    "claude-reloc-cheap",
	} {
		resolution := f.service.Resolve("claude-code", id)
		if resolution.Rewrite != want {
			t.Fatalf("rewrite for %s = %q, want %q", id, resolution.Rewrite, want)
		}
		if _, found := snapshot.ResolveClientID(resolution.Rewrite); !found {
			t.Fatalf("the data plane cannot resolve %q", resolution.Rewrite)
		}
	}
}

func TestABindingWhoseTargetWentAwayIsLeftToTheVendor(t *testing.T) {
	f := newFixture(t)
	apply(t, f, claudebindings.Binding{
		NativeID: "claude-opus-5", Enabled: true,
		Target: claudebindings.Target{Kind: claudebindings.TargetRoute, GroupID: "cheap"},
	})
	if err := sqlite.NewCatalogRepo(f.db).DeleteGroup(context.Background(), "cheap"); err != nil {
		t.Fatalf("delete route: %v", err)
	}
	if err := f.catalog.Reload(context.Background()); err != nil {
		t.Fatalf("reload catalog: %v", err)
	}
	resolution := f.service.Resolve("claude-code", "claude-opus-5")
	if resolution.Matched || resolution.Reason != claudebindings.ReasonDangling {
		t.Fatalf("Resolve() = %+v, want a dangling binding left alone", resolution)
	}
	view, err := f.service.View(context.Background(), "claude-code")
	if err != nil || len(view.Entries) != 1 || view.Entries[0].State != claudebindings.StateDangling {
		t.Fatalf("View() = %+v, %v, want the dangling state shown", view, err)
	}
}

func TestADisabledBindingIsLeftToTheVendor(t *testing.T) {
	f := newFixture(t)
	disabled := modelBinding("claude-haiku-4-5")
	disabled.Enabled = false
	apply(t, f, disabled)
	if resolution := f.service.Resolve("claude-code", "claude-haiku-4-5"); resolution.Matched || resolution.Reason != claudebindings.ReasonDisabled {
		t.Fatalf("Resolve() = %+v, want a disabled binding left alone", resolution)
	}
}

func TestDirectionAndFailurePolicyDefaultToTheCautiousChoice(t *testing.T) {
	f := newFixture(t)
	apply(t, f, modelBinding("claude-haiku-4-5"))
	resolution := f.service.Resolve("claude-code", "claude-haiku-4-5")
	if resolution.Direction != claudebindings.DirectionReplace || resolution.OnFailure != claudebindings.OnFailureNone {
		t.Fatalf("Resolve() = %+v, want replace with no vendor fallback", resolution)
	}
}

func TestApplyRefusesWhatItCannotServe(t *testing.T) {
	f := newFixture(t)
	missing := modelBinding("claude-haiku-4-5")
	missing.Target.ModelID = "nothing"
	badDirection := modelBinding("claude-haiku-4-5")
	badDirection.Direction = "sideways"
	notNative := modelBinding("gpt-5")
	relo := modelBinding("claude-relo-openai--alpha")
	noTarget := claudebindings.Binding{NativeID: "claude-haiku-4-5", Enabled: true}
	for name, bad := range map[string]claudebindings.Binding{
		"a missing model": missing, "a bad direction": badDirection, "a non-Claude id": notNative,
		"a Relo alias": relo, "no target": noTarget,
	} {
		err := f.service.Apply(context.Background(), "claude-code", claudebindings.Change{Set: []claudebindings.Binding{bad}})
		if err == nil {
			t.Fatalf("Apply() with %s error = nil, want a refusal", name)
		}
	}
	if len(f.changed) != 0 {
		t.Fatal("a refused write ran the change hook")
	}
}

func TestApplyKeepsWhatItWasNotAskedToChange(t *testing.T) {
	f := newFixture(t)
	apply(t, f, modelBinding("claude-haiku-4-5"), modelBinding("claude-opus-5"))
	if err := f.service.Apply(context.Background(), "claude-code", claudebindings.Change{
		Remove: []string{"claude-haiku-4-5-20251001"},
	}); err != nil {
		t.Fatalf("Apply() remove error = %v", err)
	}
	view, err := f.service.View(context.Background(), "claude-code")
	if err != nil || len(view.Entries) != 1 || view.Entries[0].NativeID != "claude-opus-5" {
		t.Fatalf("View() = %+v, %v, want only the other binding", view.Entries, err)
	}
	if len(f.changed) != 2 {
		t.Fatalf("change hook ran %d times, want once per stored write", len(f.changed))
	}
}

func TestViewOffersTheSeedAndTheBoundIds(t *testing.T) {
	f := newFixture(t)
	apply(t, f, modelBinding("claude-future-9"))
	view, err := f.service.View(context.Background(), "claude-code")
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
	sources := map[string]string{}
	for _, official := range view.Official {
		sources[official.ID] = official.Source
	}
	if sources["claude-opus-5-5"] != claudebindings.SourceSeed || sources["claude-future-9"] != claudebindings.SourceBinding {
		t.Fatalf("official = %v, want the seed and the bound id", sources)
	}
}

func TestADocumentFromANewerBuildIsNotHalfRead(t *testing.T) {
	if _, err := claudebindings.Decode(claudebindings.SchemaVersion+1, []byte(`{"bindings":[]}`)); err == nil {
		t.Fatal("Decode() of an unknown schema error = nil, want a refusal")
	}
	document, err := claudebindings.Decode(claudebindings.SchemaVersion,
		[]byte(`{"bindings":[{"native_id":"gpt-5","target":{"kind":"model","provider_id":"a","model_id":"b"}},`+
			`{"native_id":"claude-opus-5","target":{"kind":"model","provider_id":"a","model_id":"b"},"enabled":true}]}`))
	if err != nil || len(document.Bindings) != 1 || document.Bindings[0].NativeID != "claude-opus-5" {
		t.Fatalf("Decode() = %+v, %v, want the malformed entry dropped", document, err)
	}
}

func TestTooManyBindingsAreRefused(t *testing.T) {
	f := newFixture(t)
	var many []claudebindings.Binding
	for i := range 201 {
		many = append(many, modelBinding("claude-m-"+string(rune('a'+i%26))+string(rune('a'+i/26%26))+string(rune('a'+i/676))))
	}
	err := f.service.Apply(context.Background(), "claude-code", claudebindings.Change{Set: many})
	if !errors.Is(err, claudebindings.ErrTooMany) {
		t.Fatalf("Apply() error = %v, want %v", err, claudebindings.ErrTooMany)
	}
}

func TestViewListsTheRoutesAndModelsABindingCanPointAt(t *testing.T) {
	f := newFixture(t)
	view, err := f.service.View(context.Background(), "claude-code")
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
	var route, model bool
	for _, option := range view.Targets {
		route = route || option.Kind == claudebindings.TargetRoute && option.GroupID == "cheap"
		model = model || option.Kind == claudebindings.TargetModel && option.ProviderID == "openai" && option.ModelID == "alpha"
	}
	if !route || !model {
		t.Fatalf("targets = %+v, want the route and the model", view.Targets)
	}
}

func TestARouteWithoutALabelIsOfferedUnderItsId(t *testing.T) {
	f := newFixture(t)
	for id, label := range map[string]string{"coder-free": "", "planner": "  "} {
		if err := sqlite.NewCatalogRepo(f.db).SaveRoute(context.Background(), catalog.RouteRecord{
			ID: id, Label: label, Strategy: string(catalog.StrategyPriority), Enabled: true, Listed: true,
			Members: []catalog.RouteMemberRecord{{
				ProviderID: "openai", ModelID: "alpha", Kind: catalog.MemberKindModel, Weight: 1, Enabled: true,
			}},
		}); err != nil {
			t.Fatalf("save route %s: %v", id, err)
		}
	}
	if err := f.catalog.Reload(context.Background()); err != nil {
		t.Fatalf("reload the catalog: %v", err)
	}
	view, err := f.service.View(context.Background(), "claude-code")
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
	labels := map[string]string{}
	for _, option := range view.Targets {
		if strings.TrimSpace(option.Label) == "" {
			t.Fatalf("targets = %+v, want no blank label", view.Targets)
		}
		if option.Kind == claudebindings.TargetRoute {
			labels[option.GroupID] = option.Label
		}
	}
	if labels["coder-free"] != "coder-free" || labels["planner"] != "planner" {
		t.Fatalf("route labels = %v, want the ids of the unlabeled routes", labels)
	}
}

func TestTheMillionTokenMarkerTravelsWithTheRewrite(t *testing.T) {
	f := newFixture(t)
	apply(t, f, modelBinding("claude-haiku-4-5"))
	if rewrite := f.service.Resolve("claude-code", "claude-haiku-4-5[1m]").Rewrite; rewrite != "claude-relo-openai--alpha[1m]" {
		t.Fatalf("rewrite = %q, want the marker kept", rewrite)
	}
}

func TestReloadReadsWhatAnotherWriterStored(t *testing.T) {
	f := newFixture(t)
	if err := f.service.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	body := `{"bindings":[{"native_id":"claude-opus-5","target":{"kind":"model","provider_id":"openai","model_id":"alpha"},"enabled":true}]}`
	if err := sqlite.NewDocuments(f.db).Put(context.Background(), claudebindings.Namespace,
		sqlite.DocumentRow{ID: "claude-code", Version: claudebindings.SchemaVersion, Body: []byte(body)}, 1); err != nil {
		t.Fatalf("store the document: %v", err)
	}
	if f.service.Resolve("claude-code", "claude-opus-5").Matched {
		t.Fatal("a document stored behind the service's back was read before a reload")
	}
	if err := f.service.Reload(context.Background()); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if !f.service.Resolve("claude-code", "claude-opus-5").Matched {
		t.Fatal("the stored document is not served after a reload")
	}
}

func TestWithoutACatalogABindingIsLeftToTheVendor(t *testing.T) {
	f := newFixture(t)
	apply(t, f, modelBinding("claude-haiku-4-5"))
	bare := claudebindings.New(claudebindings.Options{Documents: f.documents()})
	if resolution := bare.Resolve("claude-code", "claude-haiku-4-5"); resolution.Matched || resolution.Reason != claudebindings.ReasonDangling {
		t.Fatalf("Resolve() = %+v, want a dangling binding left alone", resolution)
	}
	view, err := bare.View(context.Background(), "claude-code")
	if err != nil || len(view.Targets) != 0 || view.Entries[0].State != claudebindings.StateDangling {
		t.Fatalf("View() = %+v, %v, want no targets and a dangling binding", view, err)
	}
}

func TestTheRefusalNamesTheRouteThatIsMissing(t *testing.T) {
	f := newFixture(t)
	err := f.service.Apply(context.Background(), "claude-code", claudebindings.Change{Set: []claudebindings.Binding{{
		NativeID: "claude-opus-5", Enabled: true,
		Target: claudebindings.Target{Kind: claudebindings.TargetRoute, GroupID: "nothing"},
	}}})
	if err == nil || !strings.Contains(err.Error(), "route nothing") {
		t.Fatalf("Apply() error = %v, want the missing route named", err)
	}
}

func TestAClaudeConnectionAddsTheIDsItLists(t *testing.T) {
	f := newFixture(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, f.db, testkit.Upstream{
		ID: "claude", Label: "Claude", APIFormat: catalog.FormatAnthropic,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey, ModelsFormat: catalog.ModelsNone,
		Models: []string{"claude-future-7", "not-a-claude-id"},
	})
	if _, err := f.db.SQL().Exec("UPDATE providers SET template_id = 'claude' WHERE id = 'claude'"); err != nil {
		t.Fatalf("set provider template: %v", err)
	}
	if err := f.catalog.Reload(context.Background()); err != nil {
		t.Fatalf("reload catalog: %v", err)
	}
	view, err := f.service.View(context.Background(), "claude-code")
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
	sources := map[string]string{}
	for _, official := range view.Official {
		sources[official.ID] = official.Source
	}
	if sources["claude-future-7"] != claudebindings.SourceConnection {
		t.Fatalf("official = %v, want the connection's Claude id", sources)
	}
	if _, listed := sources["not-a-claude-id"]; listed {
		t.Fatal("a model that is not a Claude id was offered for binding")
	}
}
