package catalog

import (
	"context"
	"testing"
)

type emptyCatalogReader struct{}

func (emptyCatalogReader) ListConnections(context.Context) ([]ConnectionRecord, error) {
	return nil, nil
}
func (emptyCatalogReader) ListModels(context.Context) ([]ModelRecord, error)     { return nil, nil }
func (emptyCatalogReader) ListModelFacts(context.Context) ([]FactsRecord, error) { return nil, nil }
func (emptyCatalogReader) ListRoutes(context.Context) ([]RouteRecord, error)     { return nil, nil }

func TestWithFreeConnectionsHidesUnconsentedFreePools(t *testing.T) {
	catalog := New(emptyCatalogReader{}, nil, nil).WithFreeConnections(func() bool { return false })
	if err := catalog.Reload(context.Background()); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	snapshot, found := catalog.Snapshot()
	if !found || snapshot.freeConnectionsEnabled == nil || snapshot.freeConnectionsEnabled() {
		t.Fatal("the reloaded snapshot did not keep the operator's opt-out")
	}
}

func freeVisibilitySnapshot() *Snapshot {
	freeHost := Provider{
		ID: "free", TemplateID: OpenCodeFreeTemplate, Auth: AuthNone,
		Configured: true, Enabled: true, Routable: true,
	}
	paidHost := Provider{
		ID: "paid", TemplateID: "custom", Auth: AuthNone,
		Configured: true, Enabled: true, Routable: true,
	}
	freeModel := Model{ProviderID: "free", ID: "shared", Enabled: true, Routable: true, Category: CategoryChat}
	paidModel := Model{ProviderID: "paid", ID: "shared", Enabled: true, Routable: true, Category: CategoryChat}
	return &Snapshot{
		Providers: []Provider{freeHost, paidHost},
		Groups: []Group{
			{ID: "free-route", Enabled: true, Listed: true, Members: []Member{{ProviderID: "free", ModelID: "shared", Kind: MemberKindModel, Enabled: true}}},
			{ID: "paid-route", Enabled: true, Listed: true, Members: []Member{{ProviderID: "paid", ModelID: "shared", Kind: MemberKindModel, Enabled: true}}},
		},
		freeConnectionsEnabled: func() bool { return false },
		byProvider: map[string]Provider{
			"free": freeHost, "paid": paidHost,
		},
		byModelKey: map[key]Model{
			{providerID: "free", modelID: "shared"}: freeModel,
			{providerID: "paid", modelID: "shared"}: paidModel,
		},
		byProviderM: map[string][]Model{"free": {freeModel}, "paid": {paidModel}},
		byModelID:   map[string][]Model{"shared": {freeModel, paidModel}},
	}
}

func TestFreeVisibilityKeepsRoutingButHidesUnconsentedPools(t *testing.T) {
	snapshot := freeVisibilitySnapshot()
	freeHost, _ := snapshot.Provider("free")
	paidHost, _ := snapshot.Provider("paid")
	if snapshot.ProviderVisible(freeHost) {
		t.Fatal("an unconsented free pool stayed visible")
	}
	if !snapshot.ProviderVisible(paidHost) {
		t.Fatal("a paid connection was hidden")
	}
	if listed := snapshot.Listed(); len(listed) != 1 || listed[0].ProviderID != "paid" {
		t.Fatalf("Listed() = %+v, want the paid model alone", listed)
	}
	if groups := snapshot.ListedGroups(); len(groups) != 1 || groups[0].ID != "paid-route" {
		t.Fatalf("ListedGroups() = %+v, want the paid route alone", groups)
	}
}

func TestGroupVisibleReadsEveryMemberShape(t *testing.T) {
	snapshot := freeVisibilitySnapshot()
	cases := []struct {
		name  string
		group Group
		want  bool
	}{
		{name: "a named paid member", group: Group{Members: []Member{{ProviderID: "paid", ModelID: "shared"}}}, want: true},
		{name: "a named free member", group: Group{Members: []Member{{ProviderID: "free", ModelID: "shared"}}}},
		{name: "a named member the catalog lost", group: Group{Members: []Member{{ProviderID: "missing", ModelID: "shared"}}}, want: true},
		{name: "an auto member on a paid pool", group: Group{Members: []Member{{ModelID: "missing-paid"}}}, want: true},
		{name: "an auto member the catalog lost", group: Group{Members: []Member{{ModelID: "shared-missing"}}}, want: false},
	}
	snapshot.byModelID["missing-paid"] = []Model{{ProviderID: "paid", ID: "missing-paid"}}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := snapshot.groupVisible(testCase.group); got != testCase.want {
				t.Fatalf("groupVisible() = %v, want %v", got, testCase.want)
			}
		})
	}
}
