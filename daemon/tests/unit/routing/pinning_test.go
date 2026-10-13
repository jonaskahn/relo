package routing_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/routing"
)

// TestConversationOfReadsTheHeaderAClientSent pins how a request's
// conversation is named: the two headers a client may use, the first one that
// carries something wins, and a request that names none has no conversation to
// pin to.
func TestConversationOfReadsTheHeaderAClientSent(t *testing.T) {
	if got := routing.ConversationOf(map[string]string{"session_id": "one"}); got != "one" {
		t.Fatalf("ConversationOf() = %q, want the session header", got)
	}
	if got := routing.ConversationOf(map[string]string{"conversation_id": "two"}); got != "two" {
		t.Fatalf("ConversationOf() = %q, want the conversation header", got)
	}
	// A client may send both; the session header is the one the router reads.
	both := map[string]string{"session_id": "one", "conversation_id": "two"}
	if got := routing.ConversationOf(both); got != "one" {
		t.Fatalf("ConversationOf() = %q, want the session header to win", got)
	}
	for _, headers := range []map[string]string{
		nil,
		{},
		{"session_id": ""},
		{"session_id": "", "conversation_id": ""},
		{"unrelated": "value"},
	} {
		if got := routing.ConversationOf(headers); got != "" {
			t.Fatalf("ConversationOf(%v) = %q, want no conversation", headers, got)
		}
	}
}

// TestPinPrefixNamesTheDecision pins which pin a conversation keeps, so a pin
// taken against a route does not also bind the bare model of the same name and
// a pin on a bare model survives a plan that named no route at all.
func TestPinPrefixNamesTheDecision(t *testing.T) {
	routed := routing.PinPrefix(routing.Plan{GroupID: "alias", Model: "alias"})
	bare := routing.PinPrefix(routing.Plan{Model: "alias"})
	if routed == bare {
		t.Fatalf("PinPrefix() = %q for both a route and a bare model, want them told apart", routed)
	}
	if again := routing.PinPrefix(routing.Plan{Model: "alias"}); again != bare {
		t.Fatalf("PinPrefix() = %q then %q for one bare model, want it stable", bare, again)
	}
	// A plan naming no route but a different model is a different pin again,
	// so one conversation's pin cannot bind another's.
	other := routing.PinPrefix(routing.Plan{Model: "beta"})
	if other == bare || other == routed {
		t.Fatalf("PinPrefix() = %q, want a second model to pin apart", other)
	}
}

// TestUnpinLetsTheNextRequestLandAnywhere pins what a failure does to a
// conversation: the provider it reached is forgotten, so the next request is
// free to go elsewhere rather than straight back to what refused.
func TestUnpinLetsTheNextRequestLandAnywhere(t *testing.T) {
	ctx := context.Background()
	router, _ := newRouter(t, catalog.StrategyPriority)

	plan, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	pinned := plan.Candidates[1]
	prefix := routing.PinPrefix(plan)
	router.Pin("conversation-one", prefix, pinned)

	request := routing.Request{Model: "alias", Conversation: "conversation-one"}
	before, err := router.Plan(ctx, request)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if before.Candidates[0].ModelID != pinned.ModelID {
		t.Fatalf("Plan() started at %q, want the pinned %q", before.Candidates[0].ModelID, pinned.ModelID)
	}

	router.Unpin("conversation-one", prefix)
	after, err := router.Plan(ctx, request)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if after.Candidates[0].ModelID == pinned.ModelID {
		t.Fatalf("Plan() started at the unpinned %q, want the stored order back", pinned.ModelID)
	}
}

// TestPinAndUnpinIgnoreAConversationWithNoName pins that a request that named
// no conversation keeps nothing: there is no identifier to hold a provider
// against, so pinning one is not an error but is not remembered either.
func TestPinAndUnpinIgnoreAConversationWithNoName(t *testing.T) {
	ctx := context.Background()
	router, _ := newRouter(t, catalog.StrategyPriority)

	plan, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	prefix := routing.PinPrefix(plan)
	router.Pin("", prefix, plan.Candidates[1])

	pinned, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if pinned.Candidates[0].ModelID != plan.Candidates[0].ModelID {
		t.Fatalf("Plan() started at %q, want a nameless conversation to pin nothing",
			pinned.Candidates[0].ModelID)
	}
	router.Unpin("", prefix)
}

// TestStickyMemberLeadsAPriorityRoute pins the member that last served a
// priority route: the next request starts there, which is what makes a
// conversation's provider stick without the route being rewritten.
func TestStickyMemberLeadsAPriorityRoute(t *testing.T) {
	ctx := context.Background()
	router, _ := newRouter(t, catalog.StrategyPriority)

	first, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	served := first.Candidates[1]
	router.MarkGroupSucceeded(first, served)

	next, err := router.Plan(ctx, routing.Request{Model: "alias"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if next.Candidates[0].ModelID != served.ModelID {
		t.Fatalf("Plan() started at %q, want the member that served last at %q",
			next.Candidates[0].ModelID, served.ModelID)
	}
}
