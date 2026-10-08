// In-process event bus: publishing channels and subscriber lifetimes.
package server

import (
	"encoding/json"
	"sync"
	"time"
)

// Event channels the dashboard subscribes to. A channel is a plain string so
// a new one never changes this package.
const (
	ChannelLogs   = "logs"
	ChannelQuota  = "quota"
	ChannelStatus = "status"
)

const (
	// SubscriberBuffer is how many events a subscriber may fall behind before
	// the bus drops it.
	SubscriberBuffer = 64
	// MaxSubscribers bounds the number of live event streams one process
	// serves.
	MaxSubscribers = 64
)

// Event is one message on a channel.
type Event struct {
	Channel string
	Payload []byte
	At      time.Time
}

// EventBusOptions tune a bus.
type EventBusOptions struct {
	// Subscribers bounds the live subscriptions.
	Subscribers int
	// Buffer is the per-subscriber queue depth.
	Buffer int
	// Clock keeps event timestamps testable.
	Now func() time.Time
}

// EventBus fans events out to the dashboard's event streams. A publisher
// never blocks: a subscriber that cannot keep up is dropped, and the browser
// resynchronizes when it reconnects, so no resync marker ever travels in the
// channel.
type EventBus struct {
	mu          sync.Mutex
	now         func() time.Time
	buffer      int
	max         int
	subscribers map[int]*subscription
	nextID      int
}

// Subscription is one live dashboard stream.
type Subscription struct {
	ID     int
	Events <-chan Event
	bus    *EventBus
	once   sync.Once
}

type subscription struct {
	id       int
	channels map[string]bool
	queue    chan Event
}

// NewEventBus returns a bus with no subscribers.
func NewEventBus(options EventBusOptions) *EventBus {
	settings := options
	if settings.Now == nil {
		settings.Now = time.Now
	}
	if settings.Buffer <= 0 {
		settings.Buffer = SubscriberBuffer
	}
	if settings.Subscribers <= 0 {
		settings.Subscribers = MaxSubscribers
	}
	return &EventBus{
		now: settings.Now, buffer: settings.Buffer, max: settings.Subscribers,
		subscribers: map[int]*subscription{},
	}
}

// Publish sends one JSON payload to every subscriber of a channel. A
// subscriber whose queue is full is dropped instead of delaying the caller.
func (b *EventBus) Publish(channel string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return b.publishEncoded(channel, encoded)
}

func (b *EventBus) publishEncoded(channel string, encoded []byte) error {
	event := Event{Channel: channel, Payload: encoded, At: b.now()}
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, subscriber := range b.subscribers {
		if !subscriber.channels[channel] {
			continue
		}
		select {
		case subscriber.queue <- event:
		default:
			delete(b.subscribers, id)
			close(subscriber.queue)
		}
	}
	return nil
}

// Subscribe returns a stream of the named channels' events, so one console
// connection can carry every channel it cares about. Calling Close on the
// subscription detaches it.
func (b *EventBus) Subscribe(channels ...string) (*Subscription, bool) {
	if len(channels) == 0 {
		return nil, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked()
	if len(b.subscribers) >= b.max {
		return nil, false
	}
	wanted := make(map[string]bool, len(channels))
	for _, channel := range channels {
		wanted[channel] = true
	}
	queue := make(chan Event, b.buffer)
	id := b.nextID
	b.nextID++
	b.subscribers[id] = &subscription{id: id, channels: wanted, queue: queue}
	return &Subscription{ID: id, Events: queue, bus: b}, true
}

// Subscribers returns how many streams are attached right now.
func (b *EventBus) Subscribers() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked()
	return len(b.subscribers)
}

// Close detaches a subscription. Closing twice is harmless.
func (s *Subscription) Close() {
	if s == nil || s.bus == nil {
		return
	}
	s.once.Do(func() { s.bus.unsubscribe(s.ID) })
}

func (b *EventBus) unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subscriber, found := b.subscribers[id]
	if !found {
		return
	}
	delete(b.subscribers, id)
	close(subscriber.queue)
}

func (b *EventBus) pruneLocked() {
	for id, subscriber := range b.subscribers {
		if len(subscriber.queue) < cap(subscriber.queue) {
			continue
		}
		delete(b.subscribers, id)
		close(subscriber.queue)
	}
}

// Publisher is what a producer of dashboard events needs: the pool, the
// quota worker, and the usage recorder all publish through it.
type Publisher interface {
	Publish(channel string, payload any) error
}

// PublishThreadSafe lets a handler report an event without holding a
// reference to the bus itself.
func (b *EventBus) PublishThreadSafe(channel string, payload any) {
	_ = b.Publish(channel, payload)
}
