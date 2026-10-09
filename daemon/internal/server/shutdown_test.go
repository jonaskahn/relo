package server

import (
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/config"
)

func TestShutdownClosesEventStreams(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Port = 0
	cfg.Server.DataPlane.OpenAI = 0
	cfg.Server.DataPlane.Anthropic = 0
	served := New(Options{Config: &cfg})
	ready := make(chan Addrs, 1)
	served.OnReady(func(addrs Addrs) { ready <- addrs })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- served.Start(ctx) }()
	address := <-ready
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + address.Management + "/api/v1/events/logs,quota,status")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("stream status: %d", response.StatusCode)
	}
	ended := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, response.Body); close(ended) }()
	cancel()
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("event stream remained open")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server remained running")
	}
	if served.events.Subscribers() != 0 {
		t.Fatal("stream subscription leaked")
	}
}

func TestShutdownCancelsWorkAndBoundsPersistence(t *testing.T) {
	served := New(Options{})
	started := make(chan struct{})
	cancelled := make(chan struct{})
	served.runOperation(func(ctx context.Context) { close(started); <-ctx.Done(); close(cancelled) })
	<-started
	blocked := make(chan struct{})
	release := make(chan struct{})
	served.bookkeeping.enqueue(func() { close(blocked); <-release })
	<-blocked
	var queued atomic.Bool
	served.bookkeeping.enqueue(func() { queued.Store(true) })
	stopped := make(chan error, 1)
	began := time.Now()
	go func() { stopped <- served.Shutdown(context.Background()) }()
	select {
	case <-cancelled:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("operational work was not cancelled immediately")
	}
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
		if elapsed := time.Since(began); elapsed < 400*time.Millisecond || elapsed > time.Second {
			t.Fatalf("save budget: %v", elapsed)
		}
	case <-time.After(time.Second):
		t.Fatal("persistence blocked shutdown")
	}
	close(release)
	<-served.bookkeeping.done
	if queued.Load() {
		t.Fatal("queued work ran after the persistence deadline")
	}
	if served.persistence.Err() == nil {
		t.Fatal("persistence remained writable")
	}
	if served.admit() {
		t.Fatal("new work accepted after shutdown")
	}
}

func TestShutdownSavesRecordsAndKeepsRequestValues(t *testing.T) {
	served := New(Options{})
	request, cancel := context.WithCancel(context.WithValue(context.Background(), identityKey{}, clientIdentity{ID: "client"}))
	cancel()
	record := served.recordContext(request)
	if record.Err() != nil {
		t.Fatal("client cancellation cancelled final records")
	}
	if identity, ok := clientIdentityFrom(record); !ok || identity.ID != "client" {
		t.Fatal("client identity was lost")
	}
	var saved atomic.Bool
	served.bookkeeping.enqueue(func() { saved.Store(record.Err() == nil) })
	began := time.Now()
	if err := served.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !saved.Load() {
		t.Fatal("pending record was discarded")
	}
	if time.Since(began) >= 400*time.Millisecond {
		t.Fatal("idle shutdown introduced a fixed wait")
	}
	if err := served.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStopReceiptSurvivesImmediateConnectionClosure(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Port = 0
	cfg.Server.DataPlane.OpenAI = 0
	cfg.Server.DataPlane.Anthropic = 0
	served := New(Options{Config: &cfg, AdminToken: "shutdown-test"})
	served.opts.Shutdown = func() { _ = served.Shutdown(context.Background()) }
	ready := make(chan Addrs, 1)
	served.OnReady(func(addrs Addrs) { ready <- addrs })
	done := make(chan error, 1)
	go func() { done <- served.Start(context.Background()) }()
	address := <-ready
	request, err := http.NewRequest(http.MethodPost, "http://"+address.Management+"/api/v1/daemon/shutdown", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer shutdown-test")
	response, err := (&http.Client{Timeout: time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("stop receipt was truncated: %v", err)
	}
	if response.StatusCode != http.StatusAccepted || string(body) != "{\"status\":\"stopping\"}\n" {
		t.Fatalf("receipt: %d %s", response.StatusCode, body)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
