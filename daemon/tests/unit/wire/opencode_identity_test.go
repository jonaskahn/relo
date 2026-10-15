package codec

import (
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

var nativeIdentifier = regexp.MustCompile(`^(ses|msg)_[0-9a-f]{12}[0-9A-Za-z]{14}$`)

// TestNativeIdentifiersOrderLikeTheClient covers the sort direction the
// gateway relies on: sessions newest first, requests oldest first.
func TestNativeIdentifiersOrderLikeTheClient(t *testing.T) {
	first := wire.NativeSessionID()
	second := wire.NativeSessionID()
	if second >= first {
		t.Fatalf("sessions %q then %q do not sort newest first", first, second)
	}
	firstReq := wire.NativeRequestID()
	secondReq := wire.NativeRequestID()
	if secondReq <= firstReq {
		t.Fatalf("requests %q then %q do not sort oldest first", firstReq, secondReq)
	}
	for _, id := range []string{first, second, firstReq, secondReq} {
		if !nativeIdentifier.MatchString(id) {
			t.Fatalf("id = %q, want the client shape", id)
		}
	}
}

// TestNativeIdentifiersAreUniqueUnderConcurrency covers the per-millisecond
// counter: callers in the same millisecond still mint distinct ids.
func TestNativeIdentifiersAreUniqueUnderConcurrency(t *testing.T) {
	const callers = 64
	seen := make(chan string, 2*callers)
	var group sync.WaitGroup
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			seen <- wire.NativeSessionID()
			seen <- wire.NativeRequestID()
		}()
	}
	group.Wait()
	close(seen)
	distinct := map[string]bool{}
	for id := range seen {
		if distinct[id] {
			t.Fatalf("id %q was minted twice", id)
		}
		distinct[id] = true
	}
}

// TestProjectForgeMintsAShapedStableID covers the public contract: a 40-hex
// id that holds still until its epoch expires.
func TestProjectForgeMintsAShapedStableID(t *testing.T) {
	forge := &wire.ProjectForge{}
	first := forge.Get()
	if len(first) != 40 || strings.Trim(first, "0123456789abcdef") != "" {
		t.Fatalf("project = %q, want 40 hex chars", first)
	}
	if got := forge.Get(); got != first {
		t.Fatalf("project changed from %q to %q inside its epoch", first, got)
	}
}
