package docstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jonaskahn/relo/internal/application/docstore"
)

// memoryStore is a store that counts what the collection asks of it.
type memoryStore struct {
	mu         sync.Mutex
	rows       map[string]docstore.Row
	lists      int
	failPut    error
	failList   error
	failListAt int
}

func newMemoryStore(rows ...docstore.Row) *memoryStore {
	store := &memoryStore{rows: map[string]docstore.Row{}}
	for _, row := range rows {
		store.rows[row.ID] = row
	}
	return store
}

func (s *memoryStore) List(_ context.Context, _ string) ([]docstore.Row, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lists++
	if s.failList != nil && s.lists >= s.failListAt {
		return nil, s.failList
	}
	listed := make([]docstore.Row, 0, len(s.rows))
	for _, row := range s.rows {
		listed = append(listed, row)
	}
	return listed, nil
}

func (s *memoryStore) Put(_ context.Context, _ string, row docstore.Row, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failPut != nil {
		return s.failPut
	}
	s.rows[row.ID] = row
	return nil
}

func (s *memoryStore) Delete(_ context.Context, _, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, id)
	return nil
}

type note struct {
	Text string `json:"text"`
}

// noteCodec reads version 1 as {"text": ...} and version 0 as a bare string,
// so an older shape is migrated on read.
func noteCodec() docstore.Codec[note] {
	return docstore.Codec[note]{
		Decode: func(version int, body []byte) (note, error) {
			if version == 0 {
				var text string
				if err := json.Unmarshal(body, &text); err != nil {
					return note{}, err
				}
				return note{Text: text}, nil
			}
			var value note
			err := json.Unmarshal(body, &value)
			return value, err
		},
		Encode: func(value note) ([]byte, int, error) {
			body, err := json.Marshal(value)
			return body, 1, err
		},
	}
}

func row(id string, version int, body string) docstore.Row {
	return docstore.Row{ID: id, Version: version, Body: []byte(body)}
}

func TestFirstReadLoadsOnceAndLaterReadsStayInMemory(t *testing.T) {
	store := newMemoryStore(row("a", 1, `{"text":"one"}`))
	collection := docstore.NewCollection(store, "test.notes", noteCodec(), docstore.Options{})
	for range 5 {
		value, found, err := collection.Get(context.Background(), "a")
		if err != nil || !found || value.Text != "one" {
			t.Fatalf("Get() = %+v, %v, %v", value, found, err)
		}
	}
	if store.lists != 1 {
		t.Fatalf("store listed %d times, want 1", store.lists)
	}
}

func TestPutStoresThenReloadsTheNamespace(t *testing.T) {
	store := newMemoryStore()
	collection := docstore.NewCollection(store, "test.notes", noteCodec(), docstore.Options{})
	if _, found, _ := collection.Get(context.Background(), "a"); found {
		t.Fatal("an empty namespace answered a document")
	}
	if err := collection.Put(context.Background(), "a", note{Text: "two"}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	value, found, err := collection.Get(context.Background(), "a")
	if err != nil || !found || value.Text != "two" {
		t.Fatalf("Get() after Put = %+v, %v, %v", value, found, err)
	}
	if store.lists != 2 {
		t.Fatalf("store listed %d times, want the first load and one reload", store.lists)
	}
}

func TestFailedWriteKeepsTheDocumentsInMemory(t *testing.T) {
	store := newMemoryStore(row("a", 1, `{"text":"one"}`))
	collection := docstore.NewCollection(store, "test.notes", noteCodec(), docstore.Options{})
	if _, _, err := collection.Get(context.Background(), "a"); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	store.failPut = errors.New("disk full")
	if err := collection.Put(context.Background(), "a", note{Text: "two"}); err == nil {
		t.Fatal("Put() error = nil, want the store's refusal")
	}
	value, _, _ := collection.Get(context.Background(), "a")
	if value.Text != "one" {
		t.Fatalf("text = %q, want the document unchanged", value.Text)
	}
}

func TestFailedReloadAfterAGoodWriteKeepsServingAndSaysSo(t *testing.T) {
	store := newMemoryStore(row("a", 1, `{"text":"one"}`))
	collection := docstore.NewCollection(store, "test.notes", noteCodec(), docstore.Options{})
	if _, _, err := collection.Get(context.Background(), "a"); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	store.failList, store.failListAt = errors.New("locked"), 2
	err := collection.Put(context.Background(), "a", note{Text: "two"})
	if err == nil {
		t.Fatal("Put() error = nil, want the reload failure reported")
	}
	value, _, getErr := collection.Get(context.Background(), "a")
	if getErr != nil || value.Text != "one" {
		t.Fatalf("Get() = %+v, %v, want the previous document", value, getErr)
	}
	store.failList = nil
	if err := collection.Reload(context.Background()); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if value, _, _ := collection.Get(context.Background(), "a"); value.Text != "two" {
		t.Fatalf("text = %q, want the stored document after a reload", value.Text)
	}
}

func TestRowThatDoesNotDecodeIsSkipped(t *testing.T) {
	store := newMemoryStore(row("bad", 1, `{not json`), row("good", 1, `{"text":"ok"}`))
	collection := docstore.NewCollection(store, "test.notes", noteCodec(), docstore.Options{})
	all, err := collection.All(context.Background())
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	if len(all) != 1 || all["good"].Text != "ok" {
		t.Fatalf("All() = %v, want only the readable document", all)
	}
}

func TestOlderSchemaVersionIsMigratedOnRead(t *testing.T) {
	store := newMemoryStore(row("old", 0, `"legacy"`))
	collection := docstore.NewCollection(store, "test.notes", noteCodec(), docstore.Options{})
	value, found, err := collection.Get(context.Background(), "old")
	if err != nil || !found || value.Text != "legacy" {
		t.Fatalf("Get() = %+v, %v, %v", value, found, err)
	}
}

func TestDeleteRemovesTheDocumentAndReloads(t *testing.T) {
	store := newMemoryStore(row("a", 1, `{"text":"one"}`))
	collection := docstore.NewCollection(store, "test.notes", noteCodec(), docstore.Options{})
	if err := collection.Delete(context.Background(), "a"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, found, _ := collection.Get(context.Background(), "a"); found {
		t.Fatal("a deleted document is still served")
	}
}

func TestEncodeFailureIsReportedBeforeAnyWrite(t *testing.T) {
	store := newMemoryStore()
	collection := docstore.NewCollection(store, "test.notes", docstore.Codec[note]{
		Decode: noteCodec().Decode,
		Encode: func(note) ([]byte, int, error) { return nil, 0, fmt.Errorf("invalid") },
	}, docstore.Options{})
	if err := collection.Put(context.Background(), "a", note{}); err == nil {
		t.Fatal("Put() error = nil, want the encode failure")
	}
	if len(store.rows) != 0 {
		t.Fatal("a document that failed to encode was written")
	}
}

func TestFirstLoadFailureIsReturnedAndRetried(t *testing.T) {
	store := newMemoryStore(row("a", 1, `{"text":"one"}`))
	store.failList, store.failListAt = errors.New("locked"), 1
	collection := docstore.NewCollection(store, "test.notes", noteCodec(), docstore.Options{})
	if _, _, err := collection.Get(context.Background(), "a"); err == nil {
		t.Fatal("Get() error = nil, want the load failure")
	}
	store.failList = nil
	if _, found, err := collection.Get(context.Background(), "a"); err != nil || !found {
		t.Fatalf("Get() after recovery = %v, %v", found, err)
	}
}
