package routing_test

import (
	"context"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestFailoverSkipsRefusedMembers pins the cross-request denylist: a refused
// member leaves the next plan, a success returns one, and a full refusal
// tries every member once more rather than answering with none.
func TestFailoverSkipsRefusedMembers(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	failover := routing.NewFailover(clock, []time.Duration{15 * time.Minute})
	members := []routing.Candidate{
		{ProviderID: "openai", ModelID: "alpha"},
		{ProviderID: "openai", ModelID: "beta"},
	}

	failover.MarkFailed("openai alpha")
	if kept := failover.Filter(members); len(kept) != 1 || kept[0].ModelID != "beta" {
		t.Fatalf("Filter() = %v, want only beta", kept)
	}

	failover.MarkSucceeded("openai alpha")
	if kept := failover.Filter(members); len(kept) != 2 {
		t.Fatalf("Filter() = %v, want both members after a success", kept)
	}

	failover.MarkFailed("openai alpha")
	failover.MarkFailed("openai beta")
	if kept := failover.Filter(members); len(kept) != 2 {
		t.Fatalf("Filter() = %v, want every member when all are refused", kept)
	}

	clock.Add(16 * time.Minute)
	if kept := failover.Filter(members); len(kept) != 2 {
		t.Fatalf("Filter() = %v, want the wait to expire", kept)
	}
}

// TestDisabledFailoverKeepsEveryMember pins the zero wait: a refusal is
// forgotten at once, and disabling the wait returns waiting members to the
// next plans immediately.
func TestDisabledFailoverKeepsEveryMember(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	members := []routing.Candidate{
		{ProviderID: "openai", ModelID: "alpha"},
		{ProviderID: "openai", ModelID: "beta"},
	}

	withoutDenylist := routing.NewFailover(clock, nil)
	withoutDenylist.MarkFailed("openai alpha")
	if kept := withoutDenylist.Filter(members); len(kept) != 2 {
		t.Fatalf("Filter() = %v, want both members without a denylist", kept)
	}

	waiting := routing.NewFailover(clock, []time.Duration{15 * time.Minute})
	waiting.MarkFailed("openai alpha")
	if kept := waiting.Filter(members); len(kept) != 1 {
		t.Fatalf("Filter() = %v, want only beta while the wait runs", kept)
	}
	waiting.SetBackoff(nil)
	if kept := waiting.Filter(members); len(kept) != 2 {
		t.Fatalf("Filter() = %v, want the waiting member back at once", kept)
	}
}

// TestFailoverEscalatesRepeatedFailures pins the ladder: the first refusal of
// a member may keep it available, the next walks a longer step, and a success
// resets the walk.
func TestFailoverEscalatesRepeatedFailures(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	failover := routing.NewFailover(clock, []time.Duration{0, 3 * time.Minute, 5 * time.Minute})
	members := []routing.Candidate{
		{ProviderID: "openai", ModelID: "alpha"},
		{ProviderID: "openai", ModelID: "beta"},
	}

	failover.MarkFailed("openai alpha")
	if kept := failover.Filter(members); len(kept) != 2 {
		t.Fatalf("Filter() = %v, want the first step to keep the member", kept)
	}
	failover.MarkFailed("openai alpha")
	clock.Add(3*time.Minute - time.Second)
	if kept := failover.Filter(members); len(kept) != 1 || kept[0].ModelID != "beta" {
		t.Fatalf("Filter() = %v, want only beta during the three-minute step", kept)
	}
	clock.Add(2 * time.Second)
	if kept := failover.Filter(members); len(kept) != 2 {
		t.Fatalf("Filter() = %v, want the three-minute step to expire", kept)
	}
	failover.MarkFailed("openai alpha")
	clock.Add(5*time.Minute - time.Second)
	if kept := failover.Filter(members); len(kept) != 1 {
		t.Fatalf("Filter() = %v, want the third failure to wait its five-minute step", kept)
	}
	failover.MarkSucceeded("openai alpha")
	failover.MarkFailed("openai alpha")
	if kept := failover.Filter(members); len(kept) != 2 {
		t.Fatalf("Filter() = %v, want a success to reset the ladder", kept)
	}
}

// TestPlanSkipsAResfusedMember covers the router wiring: the plan after a
// refusal starts past the refused member, and the plan after a success
// serves it again.
func TestPlanSkipsARefusedMember(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"alpha", "beta"}, ModelsFormat: catalog.ModelsNone,
	})
	repo := sqlite.NewCatalogRepo(db)
	if err := repo.SaveGroup(ctx, sqlite.GroupRow{
		ID: "alias", Label: "Alias", Strategy: string(catalog.StrategyPriority), Enabled: true, Listed: true,
		Members: []sqlite.GroupMemberRow{
			{Position: 0, ProviderID: "openai", ModelID: "alpha", Weight: 1, Enabled: true},
			{Position: 1, ProviderID: "openai", ModelID: "beta", Weight: 1, Enabled: true},
		},
	}); err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}
	router := routing.New(routing.Options{
		Catalog: testkit.ReloadCatalog(t, db, accounts{}),
		Clock:   clock, FailoverBackoff: []time.Duration{15 * time.Minute},
	})

	plan, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if firstModel(plan) != "alpha" {
		t.Fatalf("first plan started at %q, want alpha", firstModel(plan))
	}
	router.MarkFailed(plan.Candidates[0])

	next, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if firstModel(next) != "beta" {
		t.Fatalf("refused plan started at %q, want beta", firstModel(next))
	}

	router.MarkSucceeded(routing.Candidate{ProviderID: "openai", ModelID: "alpha"})
	third, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(third.Candidates) != 2 {
		t.Fatalf("recovered plan = %v, want both members", third.Candidates)
	}
}
