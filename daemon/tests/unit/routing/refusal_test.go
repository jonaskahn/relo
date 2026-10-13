package routing_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestCoolingRefusalSaysWhenToAskAgain pins the answer a request gets when
// its route is waiting out a failure: it is refused as a no-route so a caller
// treats it like any other refusal, and it names the moment to come back.
func TestCoolingRefusalSaysWhenToAskAgain(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	ctx := context.Background()
	router, _ := newClockedRouter(t, catalog.StrategyPriority, clock)

	plan, err := router.Preview(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	router.SetFailoverBackoff([]time.Duration{15 * time.Minute})
	router.MarkGroupFailed(plan)

	refused, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if !errors.Is(err, routing.ErrNoRoute) {
		t.Fatalf("Plan() error = %v, want a no-route refusal", err)
	}
	if len(refused.Candidates) != 0 {
		t.Fatalf("Plan() = %v, want no candidate while the route waits", refused.Candidates)
	}
	// The refusal reads as a no-route to a caller that only checks for one, and
	// as itself to a caller that wants to say when to come back.
	var cooling *routing.CoolingError
	if !errors.As(err, &cooling) {
		t.Fatalf("Plan() error = %v, want a cooling refusal", err)
	}
	if cooling.Until.IsZero() || cooling.Until.After(clock.Now()) == false {
		t.Fatalf("Plan() error = %v, want a moment after now", cooling)
	}
	if !strings.Contains(cooling.Error(), "alias") {
		t.Fatalf("cooling.Error() = %q, want it to name the route", cooling.Error())
	}
}

// TestCoolingRefusalCarriesWhatTheCallerKnows pins the reason a caller supplies
// when it knows more than the routing layer does: a connection whose accounts
// are all waiting is named instead of the route's own wording.
func TestCoolingRefusalCarriesWhatTheCallerKnows(t *testing.T) {
	until := time.Date(2026, 9, 27, 15, 4, 5, 0, time.UTC)
	refusal := &routing.CoolingError{Name: "alias", Until: until, Reason: "openai: every account is cooling down"}
	if refusal.Error() != refusal.Reason {
		t.Fatalf("Error() = %q, want the reason the caller supplied", refusal.Error())
	}
	if !errors.Is(refusal, routing.ErrNoRoute) {
		t.Fatalf("errors.Is(%v, ErrNoRoute) = false, want a cooling refusal to read as a no-route", refusal)
	}
	plain := &routing.CoolingError{Name: "alias", Until: until}
	if !strings.Contains(plain.Error(), "15:04:05 UTC") {
		t.Fatalf("Error() = %q, want the moment to ask again", plain.Error())
	}
}

// TestGroupFailureLadderWalksLikeAMembers pins that a route whose every member
// failed walks the same escalating waits a member does, and that a member
// answering again ends the walk.
func TestGroupFailureLadderWalksLikeAMembers(t *testing.T) {
	clock := testkit.NewFakeClock(time.Now())
	ctx := context.Background()
	router, _ := newClockedRouter(t, catalog.StrategyPriority, clock)
	plan, err := router.Preview(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	router.SetFailoverBackoff([]time.Duration{0, 3 * time.Minute})

	router.MarkGroupFailed(plan)
	if _, err := router.Plan(ctx, routing.Request{Model: "alias"}); err != nil {
		t.Fatalf("Plan() error = %v, want the first step to keep the route usable", err)
	}

	router.MarkGroupFailed(plan)
	refused, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err == nil {
		t.Fatalf("Plan() = %v, want the route refused during its wait", refused)
	}
	clock.Add(3*time.Minute - time.Second)
	if _, err := router.Plan(ctx, routing.Request{Model: "alias"}); err == nil {
		t.Fatalf("Plan() succeeded, want the route still refused one second before its wait ends")
	}
	clock.Add(2 * time.Second)
	if _, err := router.Plan(ctx, routing.Request{Model: "alias"}); err != nil {
		t.Fatalf("Plan() error = %v, want the wait to expire", err)
	}

	router.MarkGroupFailed(plan)
	router.MarkGroupSucceeded(plan, plan.Candidates[0])
	if _, err := router.Plan(ctx, routing.Request{Model: "alias"}); err != nil {
		t.Fatalf("Plan() error = %v, want a member answering to end the walk", err)
	}
}

// TestFailoverBackoffCanBeTurnedOff pins the operator's switch: with no waits
// configured there is no denylist, so a member or a route that was waiting
// returns to the next plans at once.
func TestFailoverBackoffCanBeTurnedOff(t *testing.T) {
	ctx := context.Background()
	router, _ := newRouter(t, catalog.StrategyPriority)
	plan, err := router.Preview(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	router.SetFailoverBackoff([]time.Duration{time.Hour})
	router.MarkGroupFailed(plan)
	router.MarkFailed(plan.Candidates[0])
	if _, err := router.Plan(ctx, routing.Request{Model: "alias"}); err == nil {
		t.Fatalf("Plan() succeeded, want the route refused while its wait runs")
	}

	router.SetFailoverBackoff(nil)
	if _, err := router.Plan(ctx, routing.Request{Model: "alias"}); err != nil {
		t.Fatalf("Plan() error = %v, want the route back once the denylist is off", err)
	}
	if _, err := router.Plan(ctx, routing.Request{Model: "openai/alpha"}); err != nil {
		t.Fatalf("Plan() error = %v, want the member back once the denylist is off", err)
	}
}

// TestFailoverKeepsALoneCandidate covers the member filter on a request with
// nothing to choose between: a lone candidate stands on its own even once it
// is refused, because dropping it would leave the request with no answer at
// all rather than a different one.
func TestFailoverKeepsALoneCandidate(t *testing.T) {
	ctx := context.Background()
	router, _ := newRouter(t, catalog.StrategyPriority)
	router.SetFailoverBackoff([]time.Duration{15 * time.Minute})

	plan, err := router.Plan(ctx, routing.Request{Model: "alpha"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	// alpha is the only connection serving it, so there is no choice to make.
	router.MarkFailed(plan.Candidates[0])
	lone, err := router.Plan(ctx, routing.Request{Model: "alpha"})
	if err != nil {
		t.Fatalf("Plan() error = %v, want a lone candidate to stand on its own", err)
	}
	if len(lone.Candidates) != 1 {
		t.Fatalf("Plan() = %v, want the lone candidate kept", lone.Candidates)
	}
}

// TestRouterWithoutAFailoverRefusesNothing pins that a router built without a
// denylist takes the operators refusal as no refusal at all: the calls are
// safe to make on any router, and none of them reach a denylist that is not
// there.
func TestRouterWithoutAFailoverRefusesNothing(t *testing.T) {
	ctx := context.Background()
	router, _ := newRouter(t, catalog.StrategyPriority)
	plan, err := router.Preview(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	// A plan that named no route has no group to keep out of the next plans.
	router.MarkGroupFailed(routing.Plan{})
	router.MarkGroupSucceeded(routing.Plan{}, plan.Candidates[0])
	router.MarkFailed(routing.Candidate{})
	router.MarkSucceeded(routing.Candidate{})
	router.SetFailoverBackoff([]time.Duration{time.Hour})

	if _, err := router.Plan(ctx, routing.Request{Model: "alias"}); err != nil {
		t.Fatalf("Plan() error = %v, want a request that named nothing refused nothing", err)
	}
}

// TestUnconfiguredModelNamesWhatToLink pins the refusal a request gets when
// the catalog knows the model but no connection can serve it: the answer is
// still a no-route, and it carries the reason the connection was refused so an
// operator knows what to fix rather than being told the identifier was wrong.
func TestUnconfiguredModelNamesWhatToLink(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"alpha"}, ModelsFormat: catalog.ModelsNone,
	})
	// The connection is known but no account can serve it.
	router := routing.New(routing.Options{Catalog: testkit.ReloadCatalog(t, db, unavailable{})})

	refused, err := router.Plan(ctx, routing.Request{Model: "alpha"})
	if !errors.Is(err, routing.ErrNoRoute) {
		t.Fatalf("Plan() error = %v, want a no-route refusal", err)
	}
	if len(refused.Skipped) != 1 || refused.Skipped[0].ProviderID != "openai" {
		t.Fatalf("Plan() = %+v, want the refused connection named", refused)
	}
	if !strings.Contains(err.Error(), "no account") {
		t.Fatalf("Plan() error = %v, want the reason the connection was refused", err)
	}
}

// TestUnknownModelIsRefusedAsNotFound pins the identifier no connection
// serves at all, which is a different answer from one that is merely
// unconfigured.
func TestUnknownModelIsRefusedAsNotFound(t *testing.T) {
	ctx := context.Background()
	router, _ := newRouter(t, catalog.StrategyPriority)

	if _, err := router.Plan(ctx, routing.Request{Model: "nothing-serves-this"}); !errors.Is(err, routing.ErrModelNotFound) {
		t.Fatalf("Plan() error = %v, want the not-found refusal", err)
	}
}

// unavailable answers that no connection can serve anything, which is how a
// switched-off connection reads to the catalog.
type unavailable struct{}

func (unavailable) Active(string) int     { return 0 }
func (unavailable) Available(string) bool { return false }
func (unavailable) CooldownUntil(string) time.Time {
	return time.Time{}
}
func (unavailable) ServesModel(string, catalog.Model) bool { return false }
func (unavailable) Coverage(string, catalog.Model) (int, int) {
	return 0, 0
}

// TestRouterWithoutACatalogRefusesEveryRequest pins the router that was never
// given one: it answers no-route rather than a plan nothing could serve.
func TestRouterWithoutACatalogRefusesEveryRequest(t *testing.T) {
	router := routing.New(routing.Options{})

	if _, err := router.Plan(context.Background(), routing.Request{Model: "alpha"}); !errors.Is(err, routing.ErrNoCatalog) {
		t.Fatalf("Plan() error = %v, want the catalog refusal", err)
	}
	if _, err := router.Preview(context.Background(), routing.Request{Model: "alpha"}); !errors.Is(err, routing.ErrNoCatalog) {
		t.Fatalf("Preview() error = %v, want the catalog refusal", err)
	}
}

// TestRequestWithNoModelIsRefused pins the request that named nothing: it is
// refused before the catalog is asked what it might serve.
func TestRequestWithNoModelIsRefused(t *testing.T) {
	ctx := context.Background()
	router, _ := newRouter(t, catalog.StrategyPriority)

	for _, model := range []string{"", "   ", "\t\n"} {
		if _, err := router.Plan(ctx, routing.Request{Model: model}); !errors.Is(err, routing.ErrModelRequired) {
			t.Fatalf("Plan(%q) error = %v, want the required-model refusal", model, err)
		}
	}
}
