package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/server"
)

func TestEventBusDeliversEveryNamedChannel(t *testing.T) {
	bus := server.NewEventBus(server.EventBusOptions{})
	subscription, ok := bus.Subscribe(server.ChannelLogs, server.ChannelQuota)
	if !ok {
		t.Fatal("Subscribe() refused a subscriber on an empty bus")
	}
	defer subscription.Close()

	if err := bus.Publish(server.ChannelLogs, map[string]string{"request_id": "req-1"}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if err := bus.Publish(server.ChannelQuota, map[string]string{"window": "5h"}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if err := bus.Publish(server.ChannelStatus, map[string]string{"state": "running"}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	for i := 0; i < 2; i++ {
		event := receiveEvent(t, subscription)
		switch event.Channel {
		case server.ChannelLogs, server.ChannelQuota:
		default:
			t.Fatalf("channel = %q, want one of the subscribed channels", event.Channel)
		}
	}
	select {
	case event := <-subscription.Events:
		t.Fatalf("received %+v, want nothing on a channel the stream did not name", event)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestEventBusDelivery(t *testing.T) {
	bus := server.NewEventBus(server.EventBusOptions{})
	defer func() {
		if count := bus.Subscribers(); count != 0 {
			t.Fatalf("the bus still holds %d subscribers", count)
		}
	}()
	subscription, ok := bus.Subscribe(server.ChannelLogs)
	if !ok {
		t.Fatal("Subscribe() refused a subscriber on an empty bus")
	}
	defer subscription.Close()

	if err := bus.Publish(server.ChannelLogs, map[string]string{"request_id": "req-1"}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	event := receiveEvent(t, subscription)
	if event.Channel != server.ChannelLogs {
		t.Fatalf("channel = %q, want %q", event.Channel, server.ChannelLogs)
	}
	if !strings.Contains(string(event.Payload), "req-1") {
		t.Fatalf("payload = %s, want the published event", event.Payload)
	}

	t.Run("another channel is not delivered", func(t *testing.T) {
		if err := bus.Publish(server.ChannelQuota, map[string]string{"window": "5h"}); err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
		select {
		case event := <-subscription.Events:
			t.Fatalf("received %+v, want nothing on the logs channel", event)
		case <-time.After(50 * time.Millisecond):
		}
	})

	t.Run("a closed subscription is released", func(t *testing.T) {
		extra, _ := bus.Subscribe(server.ChannelStatus)
		extra.Close()
		extra.Close()
		if bus.Subscribers() != 1 {
			t.Fatalf("Subscribers() = %d, want only the open stream", bus.Subscribers())
		}
	})
}

func receiveEvent(t *testing.T, subscription *server.Subscription) server.Event {
	t.Helper()
	select {
	case event, open := <-subscription.Events:
		if !open {
			t.Fatal("the subscription was closed")
		}
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("no event arrived")
		return server.Event{}
	}
}

func TestEventBusDropsSlowSubscribers(t *testing.T) {
	bus := server.NewEventBus(server.EventBusOptions{Buffer: 2})
	subscription, ok := bus.Subscribe(server.ChannelLogs)
	if !ok {
		t.Fatal("Subscribe() refused a subscriber")
	}
	delivered := 0
	for index := 0; index < 50; index++ {
		if err := bus.Publish(server.ChannelLogs, index); err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
	}
	for {
		select {
		case _, open := <-subscription.Events:
			if !open {
				t.Logf("the bus dropped the subscriber after %d events", delivered)
				if bus.Subscribers() != 0 {
					t.Fatalf("Subscribers() = %d, want the dropped subscriber released", bus.Subscribers())
				}
				return
			}
			delivered++
			if delivered > 100 {
				t.Fatal("the subscriber was never dropped")
			}
		default:
			t.Fatal("the bus stopped delivering before it dropped the subscriber")
		}
	}
}

func TestEventBusNeverBlocksThePublisher(t *testing.T) {
	bus := server.NewEventBus(server.EventBusOptions{Buffer: 1, Subscribers: 8})
	for index := 0; index < 8; index++ {
		if _, ok := bus.Subscribe(server.ChannelLogs); !ok {
			t.Fatalf("Subscribe() refused subscriber %d", index)
		}
	}
	done := make(chan struct{})
	go func() {
		for index := 0; index < 5000; index++ {
			_ = bus.Publish(server.ChannelLogs, index)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a publisher blocked on a subscriber that never reads")
	}
}

func TestEventBusBoundsSubscribers(t *testing.T) {
	bus := server.NewEventBus(server.EventBusOptions{Subscribers: 2})
	first, ok := bus.Subscribe(server.ChannelLogs)
	if !ok {
		t.Fatal("the first subscriber was refused")
	}
	defer first.Close()
	if _, ok := bus.Subscribe(server.ChannelLogs); !ok {
		t.Fatal("the second subscriber was refused")
	}
	if _, ok := bus.Subscribe(server.ChannelLogs); ok {
		t.Fatal("Subscribe() accepted more subscribers than the bound allows")
	}
}

func TestEventStreamHandler(t *testing.T) {
	harness := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events/logs", nil).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer "+adminToken)
	recorder := newStreamRecorder()
	done := make(chan struct{})
	go func() {
		harness.server.Handler().ServeHTTP(recorder, request)
		close(done)
	}()
	waitForSubscriber(t, harness.events)

	if err := harness.events.Publish(server.ChannelLogs, map[string]string{"request_id": "stream-1"}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	waitFor(recorder, "stream-1")
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the stream never finished after its context was cancelled")
	}
	body := recorder.snapshot()
	if !strings.HasPrefix(body, "retry:") {
		t.Fatalf("body = %q, want the retry hint first", body)
	}
	if !strings.Contains(body, "event: logs") {
		t.Fatalf("body = %q, want the channel as the event name", body)
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", contentType)
	}
	if count := harness.events.Subscribers(); count != 0 {
		t.Fatalf("Subscribers() = %d after the stream closed, want 0", count)
	}
}

func TestEventStreamServesSeveralChannels(t *testing.T) {
	harness := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events/logs,quota", nil).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer "+adminToken)
	recorder := newStreamRecorder()
	done := make(chan struct{})
	go func() {
		harness.server.Handler().ServeHTTP(recorder, request)
		close(done)
	}()
	waitForSubscriber(t, harness.events)

	if err := harness.events.Publish(server.ChannelLogs, map[string]string{"request_id": "stream-1"}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if err := harness.events.Publish(server.ChannelQuota, map[string]string{"window": "5h"}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	waitFor(recorder, "stream-1")
	waitFor(recorder, "5h")
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the stream never finished after its context was cancelled")
	}
	body := recorder.snapshot()
	for _, want := range []string{"event: logs", "event: quota"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want %q", body, want)
		}
	}
	if count := harness.events.Subscribers(); count != 0 {
		t.Fatalf("Subscribers() = %d after the stream closed, want 0", count)
	}
}

func TestEventStreamRejections(t *testing.T) {
	harness := newHarness(t)
	response := harness.management(http.MethodGet, "/api/v1/events/nothing", adminToken, nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for an unknown channel", response.Code)
	}
	mixed := harness.management(http.MethodGet, "/api/v1/events/logs,nothing", adminToken, nil)
	if mixed.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a list naming an unknown channel", mixed.Code)
	}
	unauthorized := harness.management(http.MethodGet, "/api/v1/events/logs", "", nil)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 without a token", unauthorized.Code)
	}

	t.Run("too many streams", func(t *testing.T) {
		full := server.NewEventBus(server.EventBusOptions{Subscribers: 1})
		held, ok := full.Subscribe(server.ChannelLogs)
		if !ok {
			t.Fatal("the first subscriber was refused")
		}
		defer held.Close()
		if _, ok := full.Subscribe(server.ChannelLogs); ok {
			t.Fatal("the bus exceeded its subscriber bound")
		}
	})
}

func TestEventStreamsDoNotLeakGoroutines(t *testing.T) {
	harness := newHarness(t)
	before := runtime.NumGoroutine()
	for index := 0; index < 100; index++ {
		ctx, cancel := context.WithCancel(context.Background())
		request := httptest.NewRequest(http.MethodGet, "/api/v1/events/status", nil).WithContext(ctx)
		request.Header.Set("Authorization", "Bearer "+adminToken)
		recorder := newStreamRecorder()
		go harness.server.Handler().ServeHTTP(recorder, request)
		cancel()
	}
	waitForSubscriberCount(t, harness.events, 0)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before+10 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("goroutines grew from %d to %d across 100 streams", before, runtime.NumGoroutine())
}

// streamRecorder records a streamed response while the handler keeps writing
// to it, which is what a live SSE test needs.
type streamRecorder struct {
	mu      sync.Mutex
	header  http.Header
	status  int
	body    strings.Builder
	flushes int
}

func newStreamRecorder() *streamRecorder {
	return &streamRecorder{header: http.Header{}, status: http.StatusOK}
}

func (r *streamRecorder) Header() http.Header {
	return r.header
}

func (r *streamRecorder) WriteHeader(status int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status
}

func (r *streamRecorder) Write(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.Write(data)
}

func (r *streamRecorder) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flushes++
}

func (r *streamRecorder) snapshot() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.String()
}

func waitFor(recorder *streamRecorder, fragment string) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(recorder.snapshot(), fragment) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func waitForSubscriber(t *testing.T, bus *server.EventBus) {
	t.Helper()
	waitForSubscriberCount(t, bus, 1)
}

func waitForSubscriberCount(t *testing.T, bus *server.EventBus, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if bus.Subscribers() == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("Subscribers() = %d, want %d", bus.Subscribers(), want)
}
