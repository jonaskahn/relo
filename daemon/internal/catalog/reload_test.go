package catalog

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// overlappingReader reports when two reloads read rows at the same time. A
// catalog that serializes reloads never builds two snapshots concurrently.
type overlappingReader struct {
	active  atomic.Int32
	overlap atomic.Bool
}

func (r *overlappingReader) enter() {
	if r.active.Add(1) > 1 {
		r.overlap.Store(true)
	}
	time.Sleep(time.Millisecond)
	r.active.Add(-1)
}

func (r *overlappingReader) ListConnections(context.Context) ([]ConnectionRecord, error) {
	r.enter()
	return nil, nil
}

func (r *overlappingReader) ListModels(context.Context) ([]ModelRecord, error) {
	r.enter()
	return nil, nil
}

func (r *overlappingReader) ListModelFacts(context.Context) ([]FactsRecord, error) {
	r.enter()
	return nil, nil
}

func (r *overlappingReader) ListRoutes(context.Context) ([]RouteRecord, error) {
	r.enter()
	return nil, nil
}

func TestReloadSerializesConcurrentRebuilds(t *testing.T) {
	reader := &overlappingReader{}
	catalog := New(reader, nil, nil)

	var reloads sync.WaitGroup
	for range 8 {
		reloads.Add(1)
		go func() {
			defer reloads.Done()
			if err := catalog.Reload(context.Background()); err != nil {
				t.Errorf("Reload() error = %v", err)
			}
		}()
	}
	reloads.Wait()

	if reader.overlap.Load() {
		t.Fatal("concurrent reloads read the catalog at the same time")
	}
}
