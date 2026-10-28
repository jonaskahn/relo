// Cursor wire conversation generations: a fresh id after each handoff.
package server

import (
	"strconv"
	"sync"
)

// cursorWire tracks how many native executions each client conversation has
// handed to its client. Cursor keeps a pending in-stream call on the wire
// conversation that issued it, so the next request has to start from a fresh
// one while the client keeps its own stable anchor.
type cursorWire struct {
	mu          sync.Mutex
	generations map[string]int
}

// generation returns the wire generation of one conversation anchor.
func (w *cursorWire) generation(providerID, credentialID, anchor string) int {
	if w == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.generations[wireKey(providerID, credentialID, anchor)]
}

// rotate moves one conversation anchor onto the next wire generation.
func (w *cursorWire) rotate(providerID, credentialID, anchor string) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.generations == nil {
		w.generations = map[string]int{}
	}
	w.generations[wireKey(providerID, credentialID, anchor)]++
}

func wireKey(providerID, credentialID, anchor string) string {
	return providerID + "\x00" + credentialID + "\x00" + anchor
}

// generationSuffix names the wire generation in the conversation seed, so
// generation zero keeps the historical id and later ones never collide.
func generationSuffix(generation int) string {
	if generation <= 0 {
		return ""
	}
	return "\x00" + strconv.Itoa(generation)
}
