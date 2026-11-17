package proxy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

type memoryStore struct{ rows map[string]Entry }

func newMemoryStore() *memoryStore { return &memoryStore{rows: map[string]Entry{}} }

func (s *memoryStore) List(context.Context) ([]Entry, error) {
	rows := make([]Entry, 0, len(s.rows))
	for _, row := range s.rows {
		rows = append(rows, row)
	}
	return rows, nil
}

func (s *memoryStore) Get(_ context.Context, id string) (Entry, bool, error) {
	row, found := s.rows[id]
	return row, found, nil
}

func (s *memoryStore) Create(_ context.Context, entry Entry) (Entry, error) {
	s.rows[entry.ID] = entry
	return entry, nil
}

func (s *memoryStore) Update(_ context.Context, entry Entry) error {
	if _, found := s.rows[entry.ID]; !found {
		return ErrNotFound
	}
	s.rows[entry.ID] = entry
	return nil
}

func (s *memoryStore) Delete(_ context.Context, id string) error {
	delete(s.rows, id)
	return nil
}

func (s *memoryStore) Count(context.Context) (int, error) { return len(s.rows), nil }

type memorySecrets struct{ values map[string]string }

func newMemorySecrets() *memorySecrets { return &memorySecrets{values: map[string]string{}} }

func (s *memorySecrets) Set(ref, value string) error { s.values[ref] = value; return nil }
func (s *memorySecrets) Get(ref string) (string, error) {
	value, found := s.values[ref]
	if !found {
		return "", errors.New("missing")
	}
	return value, nil
}
func (s *memorySecrets) Delete(ref string) error { delete(s.values, ref); return nil }
func (s *memorySecrets) Mode() string            { return "encrypted_file" }

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time                         { return c.now }
func (c *testClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (c *testClock) NewTicker(d time.Duration) clock.Ticker { return clock.New().NewTicker(d) }

func testService(t *testing.T) (*Service, *memoryStore, *memorySecrets, *testClock) {
	t.Helper()
	store, vault, clk := newMemoryStore(), newMemorySecrets(), &testClock{now: time.Unix(1000, 0)}
	service := New(Options{
		Store: store, Secrets: vault, Clock: clk,
		Backoff: func() []time.Duration { return []time.Duration{time.Minute, 2 * time.Minute} },
		Check:   func(context.Context, string) error { return nil },
	})
	return service, store, vault, clk
}

func TestCreateWritesVaultAndUpdateInvalidatesCache(t *testing.T) {
	service, _, vault, _ := testService(t)
	created, err := service.Create(context.Background(), Draft{
		Scheme: "http", Host: "127.0.0.1", Port: 7890, Username: "alice", Password: "secret", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if vault.values["proxy/"+created.ID] != "secret" {
		t.Fatalf("vault = %#v", vault.values)
	}
	url, err := service.URL(context.Background(), created.ID)
	if err != nil || !strings.Contains(url, "alice:secret@") {
		t.Fatalf("URL() = %q, %v", url, err)
	}
	password := "rotated"
	if _, err := service.Update(context.Background(), created.ID, Patch{Password: &password}); err != nil {
		t.Fatal(err)
	}
	url, err = service.URL(context.Background(), created.ID)
	if err != nil || !strings.Contains(url, "alice:rotated@") {
		t.Fatalf("URL() after update = %q, %v", url, err)
	}
}

func TestDeleteRemovesTheVaultEntry(t *testing.T) {
	service, _, vault, _ := testService(t)
	created, err := service.Create(context.Background(), Draft{Scheme: "socks5", Host: "127.0.0.1", Port: 1080, Password: "secret", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	if _, found := vault.values["proxy/"+created.ID]; found {
		t.Fatal("vault entry survived the delete")
	}
}

func TestImportKeepsGoodLines(t *testing.T) {
	service, _, _, _ := testService(t)
	results, err := service.Import(context.Background(), strings.Join([]string{
		"# comment",
		"http://127.0.0.1:7890",
		"http://127.0.0.1:7890",
		"not a proxy",
		"socks5://user:pass@127.0.0.1:1080",
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	statuses := []string{}
	for _, result := range results {
		statuses = append(statuses, result.Status)
	}
	if strings.Join(statuses, ",") != "accepted,duplicate,invalid,accepted" {
		t.Fatalf("statuses = %v", statuses)
	}
}

func TestSelectionCoolsAndHonoursRetryAfter(t *testing.T) {
	service, _, _, clk := testService(t)
	first, _ := service.Create(context.Background(), Draft{Scheme: "http", Host: "127.0.0.1", Port: 7890, Enabled: true, Rank: 0})
	second, _ := service.Create(context.Background(), Draft{Scheme: "http", Host: "127.0.0.1", Port: 7891, Enabled: true, Rank: 1})
	selected, err := service.Select(context.Background(), nil)
	if err != nil || selected.ID != first.ID {
		t.Fatalf("first select = %#v, %v", selected, err)
	}
	service.Cool(first.ID, 5*time.Minute)
	selected, err = service.Select(context.Background(), nil)
	if err != nil || selected.ID != second.ID {
		t.Fatalf("second select = %#v, %v", selected, err)
	}
	service.Cool(second.ID, 0)
	clk.now = clk.now.Add(10 * time.Minute)
	selected, err = service.Select(context.Background(), nil)
	if err != nil || selected.ID != first.ID {
		t.Fatalf("after expiry = %#v, %v", selected, err)
	}
}

func TestAllCoolingReportsWait(t *testing.T) {
	service, _, _, _ := testService(t)
	created, _ := service.Create(context.Background(), Draft{Scheme: "http", Host: "127.0.0.1", Port: 7890, Enabled: true})
	service.Cool(created.ID, 2*time.Minute)
	_, err := service.Select(context.Background(), nil)
	var all *AllCoolingError
	if !errors.As(err, &all) || all.RetryAfter <= 0 {
		t.Fatalf("Select() = %v, want AllCoolingError", err)
	}
}

func TestStickySelectionReusesTheConnectionProxy(t *testing.T) {
	service, _, _, _ := testService(t)
	first, _ := service.Create(context.Background(), Draft{Scheme: "http", Host: "127.0.0.1", Port: 7890, Enabled: true, Rank: 0})
	second, _ := service.Create(context.Background(), Draft{Scheme: "http", Host: "127.0.0.1", Port: 7891, Enabled: true, Rank: 1})
	selected, err := service.SelectSticky(context.Background(), second.ID, nil)
	if err != nil || selected.ID != second.ID {
		t.Fatalf("sticky = %#v, %v", selected, err)
	}
	service.Cool(second.ID, time.Minute)
	selected, err = service.SelectSticky(context.Background(), second.ID, nil)
	if err != nil || selected.ID != first.ID {
		t.Fatalf("after cooling sticky = %#v, %v", selected, err)
	}
}

func TestBackfillIsIdempotent(t *testing.T) {
	service, _, _, _ := testService(t)
	cleared := 0
	created, err := service.Backfill(context.Background(), "http://127.0.0.1:7890", func() error { cleared++; return nil })
	if err != nil || !created || cleared != 1 {
		t.Fatalf("Backfill() = %v, %v (cleared %d)", created, err, cleared)
	}
	created, err = service.Backfill(context.Background(), "http://127.0.0.1:7890", func() error { cleared++; return nil })
	if err != nil || created || cleared != 1 {
		t.Fatalf("second Backfill() = %v, %v (cleared %d)", created, err, cleared)
	}
}
