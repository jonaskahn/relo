// Conversation pinning: sticking a chat to its credential.
package account

import (
	"container/list"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

const (
	pinMaxEntries = 10000
	pinTTL        = time.Hour
)

// PinCache remembers which credential served a conversation, so a session
// never switches accounts halfway through.
type PinCache struct {
	mu      sync.Mutex
	clock   clock.Clock
	max     int
	ttl     time.Duration
	entries map[string]*pinEntry
	order   *list.List
}

type pinEntry struct {
	credentialID string
	expiresAt    time.Time
	element      *list.Element
}

// NewPinCache returns a cache bounded to maxEntries credentials and ttl
// per conversation. Zero values keep the documented defaults.
func NewPinCache(maxEntries int, ttl time.Duration, clk clock.Clock) *PinCache {
	if clk == nil {
		clk = clock.New()
	}
	if maxEntries <= 0 {
		maxEntries = pinMaxEntries
	}
	if ttl <= 0 {
		ttl = pinTTL
	}
	return &PinCache{
		clock:   clk,
		max:     maxEntries,
		ttl:     ttl,
		entries: map[string]*pinEntry{},
		order:   list.New(),
	}
}

// Pin records the credential a conversation runs on.
func (c *PinCache) Pin(conversationID, credentialID string) {
	if conversationID == "" || credentialID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, found := c.entries[conversationID]; found {
		existing.credentialID = credentialID
		existing.expiresAt = c.clock.Now().Add(c.ttl)
		c.order.MoveToFront(existing.element)
		return
	}
	c.evictOverflow()
	entry := &pinEntry{credentialID: credentialID, expiresAt: c.clock.Now().Add(c.ttl)}
	entry.element = c.order.PushFront(conversationID)
	c.entries[conversationID] = entry
}

// Get returns the credential a conversation is pinned to.
func (c *PinCache) Get(conversationID string) (string, bool) {
	if conversationID == "" {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, found := c.entries[conversationID]
	if !found {
		return "", false
	}
	if !c.clock.Now().Before(entry.expiresAt) {
		c.remove(conversationID)
		return "", false
	}
	c.order.MoveToFront(entry.element)
	return entry.credentialID, true
}

// Evict forgets one conversation.
func (c *PinCache) Evict(conversationID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.remove(conversationID)
}

// EvictByCredential forgets every conversation that ran on a credential,
// which is what taking it out of rotation requires.
func (c *PinCache) EvictByCredential(credentialID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for conversationID, entry := range c.entries {
		if entry.credentialID == credentialID {
			c.remove(conversationID)
		}
	}
}

// Len returns how many conversations are pinned right now.
func (c *PinCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *PinCache) evictOverflow() {
	for len(c.entries) >= c.max {
		oldest := c.order.Back()
		if oldest == nil {
			return
		}
		c.remove(oldest.Value.(string))
	}
}

func (c *PinCache) remove(conversationID string) {
	entry, found := c.entries[conversationID]
	if !found {
		return
	}
	c.order.Remove(entry.element)
	delete(c.entries, conversationID)
}
