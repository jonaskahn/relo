// Package docstore keeps small JSON documents in memory over a store that
// holds them durably.
//
// A document belongs here only when all of these hold: it is small and owned
// by one package that validates it; it is read-mostly, loaded whole and
// replaced whole on a write; it is looked up by namespace and id and never by a
// field inside it; it is neither a secret nor high volume; and it needs no
// foreign key, so a document that names a route or a model tolerates the
// target disappearing. Anything else gets a table of its own.
package docstore

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// Row is one stored document: its id within the namespace, the schema version
// its owner stamped, and the raw JSON body.
type Row struct {
	ID      string
	Version int
	Body    []byte
}

// Store holds documents durably. The adapter backs it with the database.
type Store interface {
	List(ctx context.Context, namespace string) ([]Row, error)
	Put(ctx context.Context, namespace string, row Row, nowMs int64) error
	Delete(ctx context.Context, namespace, id string) error
}

// Codec is how one owner reads and writes its documents. Decode receives the
// version the row was stamped with, so an older shape is migrated on read, and
// Encode answers the body together with the version it was written at.
type Codec[T any] struct {
	Decode func(version int, body []byte) (T, error)
	Encode func(value T) (body []byte, version int, err error)
}

// Collection is one namespace held in memory. Reads touch only memory and take
// no lock. A write stores the document, then clears and fetches the namespace
// again, so what a reader sees is always what the store holds.
type Collection[T any] struct {
	store     Store
	namespace string
	codec     Codec[T]
	logger    *slog.Logger
	now       func() time.Time

	current atomic.Pointer[map[string]T]
	// load serializes the first load and every reload.
	load sync.Mutex
}

// Options configure one collection.
type Options struct {
	Logger *slog.Logger
	Now    func() time.Time
}

// NewCollection returns a collection over one namespace. Nothing is read until
// the first Get or an explicit Reload.
func NewCollection[T any](store Store, namespace string, codec Codec[T], options Options) *Collection[T] {
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	clock := options.Now
	if clock == nil {
		clock = time.Now
	}
	return &Collection[T]{store: store, namespace: namespace, codec: codec, logger: logger, now: clock}
}

// Get answers one document from memory. The first call loads the namespace.
func (c *Collection[T]) Get(ctx context.Context, id string) (T, bool, error) {
	documents, err := c.loaded(ctx)
	if err != nil {
		var zero T
		return zero, false, err
	}
	value, found := documents[id]
	return value, found, nil
}

// All answers every document from memory, keyed by id. The map belongs to the
// collection, so a caller reads it and never changes it.
func (c *Collection[T]) All(ctx context.Context) (map[string]T, error) {
	return c.loaded(ctx)
}

// Put stores one document, then reloads the namespace. A failed write leaves
// the documents in memory as they were. A reload that fails after a good write
// also keeps them, and the error says the document is stored but not yet read.
func (c *Collection[T]) Put(ctx context.Context, id string, value T) error {
	body, version, err := c.codec.Encode(value)
	if err != nil {
		return fmt.Errorf("encode %s document %s: %w", c.namespace, id, err)
	}
	if err := c.store.Put(ctx, c.namespace, Row{ID: id, Version: version, Body: body}, c.now().UnixMilli()); err != nil {
		return err
	}
	if err := c.Reload(ctx); err != nil {
		return fmt.Errorf("%s document %s is saved but not reloaded: %w", c.namespace, id, err)
	}
	return nil
}

// Delete removes one document, then reloads the namespace.
func (c *Collection[T]) Delete(ctx context.Context, id string) error {
	if err := c.store.Delete(ctx, c.namespace, id); err != nil {
		return err
	}
	if err := c.Reload(ctx); err != nil {
		return fmt.Errorf("%s document %s is removed but not reloaded: %w", c.namespace, id, err)
	}
	return nil
}

// Reload fetches the namespace again and swaps it in. A row that does not
// decode is skipped and logged, so one bad document cannot take its neighbours
// down. A failed fetch keeps what is in memory.
func (c *Collection[T]) Reload(ctx context.Context) error {
	c.load.Lock()
	defer c.load.Unlock()
	return c.fetch(ctx)
}

func (c *Collection[T]) loaded(ctx context.Context) (map[string]T, error) {
	if current := c.current.Load(); current != nil {
		return *current, nil
	}
	c.load.Lock()
	defer c.load.Unlock()
	if current := c.current.Load(); current != nil {
		return *current, nil
	}
	if err := c.fetch(ctx); err != nil {
		return nil, err
	}
	return *c.current.Load(), nil
}

func (c *Collection[T]) fetch(ctx context.Context) error {
	rows, err := c.store.List(ctx, c.namespace)
	if err != nil {
		return err
	}
	documents := make(map[string]T, len(rows))
	for _, row := range rows {
		value, err := c.codec.Decode(row.Version, row.Body)
		if err != nil {
			c.logger.Warn("skipping an unreadable document",
				"namespace", c.namespace, "id", row.ID, "version", row.Version, "error", err)
			continue
		}
		documents[row.ID] = value
	}
	c.current.Store(&documents)
	return nil
}
