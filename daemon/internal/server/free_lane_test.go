package server

import (
	"strconv"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

// TestFreeLaneSessionHoldsOneConversation covers the forged session binding:
// a lane keeps its session while the conversation runs, and lanes never
// share one.
func TestFreeLaneSessionHoldsOneConversation(t *testing.T) {
	server := &Server{}
	first := server.freeLaneSession("lane-a")
	if !wire.ValidSessionID(first) {
		t.Fatalf("session = %q, want the shape the gateway accepts", first)
	}
	if again := server.freeLaneSession("lane-a"); again != first {
		t.Fatalf("session changed from %q to %q", first, again)
	}
	if other := server.freeLaneSession("lane-b"); other == first {
		t.Fatal("lanes share a session")
	}
}

// TestFreeLaneSessionCacheStaysBounded covers a daemon that outlives many
// conversations: an evicted lane forges a new session instead of growing
// the cache without bound.
func TestFreeLaneSessionCacheStaysBounded(t *testing.T) {
	server := &Server{}
	for i := range freeSessionCacheCap + 10 {
		server.freeLaneSession("lane-" + strconv.Itoa(i))
	}
	if len(server.freeSessions) > freeSessionCacheCap {
		t.Fatalf("cache holds %d lanes, want at most %d", len(server.freeSessions), freeSessionCacheCap)
	}
}
