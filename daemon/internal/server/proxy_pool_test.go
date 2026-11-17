package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
	appproxy "github.com/jonaskahn/relo/internal/proxy"
)

type proxyTestStore struct{ rows map[string]appproxy.Entry }

func (s *proxyTestStore) List(context.Context) ([]appproxy.Entry, error) {
	rows := make([]appproxy.Entry, 0, len(s.rows))
	for _, row := range s.rows {
		rows = append(rows, row)
	}
	return rows, nil
}
func (s *proxyTestStore) Get(_ context.Context, id string) (appproxy.Entry, bool, error) {
	row, found := s.rows[id]
	return row, found, nil
}
func (s *proxyTestStore) Create(_ context.Context, entry appproxy.Entry) (appproxy.Entry, error) {
	s.rows[entry.ID] = entry
	return entry, nil
}
func (s *proxyTestStore) Update(_ context.Context, entry appproxy.Entry) error {
	s.rows[entry.ID] = entry
	return nil
}
func (s *proxyTestStore) Delete(_ context.Context, id string) error { delete(s.rows, id); return nil }
func (s *proxyTestStore) Count(context.Context) (int, error)        { return len(s.rows), nil }

type proxyTestSecrets struct{ values map[string]string }

func (s *proxyTestSecrets) Set(ref, value string) error { s.values[ref] = value; return nil }
func (s *proxyTestSecrets) Get(ref string) (string, error) {
	value, found := s.values[ref]
	if !found {
		return "", errors.New("missing")
	}
	return value, nil
}
func (s *proxyTestSecrets) Delete(ref string) error { delete(s.values, ref); return nil }
func (s *proxyTestSecrets) Mode() string            { return "encrypted_file" }

func testProxyService(t *testing.T) (*appproxy.Service, *proxyTestStore) {
	t.Helper()
	store := &proxyTestStore{rows: map[string]appproxy.Entry{}}
	service := appproxy.New(appproxy.Options{
		Store: store, Secrets: &proxyTestSecrets{values: map[string]string{}}, Clock: clock.New(),
		Backoff: func() []time.Duration { return []time.Duration{time.Minute} },
	})
	return service, store
}

func TestSelectProxyUsesOneStickyEntryPerConnection(t *testing.T) {
	service, _ := testProxyService(t)
	first, err := service.Create(context.Background(), appproxy.Draft{
		Label: "one", Scheme: "http", Host: "127.0.0.1", Port: 7890, Enabled: true, Rank: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(context.Background(), appproxy.Draft{
		Label: "two", Scheme: "http", Host: "127.0.0.1", Port: 7891, Enabled: true, Rank: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := New(Options{Proxies: service})
	ctx := withProxyConn(context.Background(), nil)

	selection, err := server.selectProxy(ctx, true, nil)
	if err != nil || selection.ID != first.ID {
		t.Fatalf("first selection = %#v, %v", selection, err)
	}
	server.pinProxy(ctx, second.ID)
	selection, err = server.selectProxy(ctx, true, nil)
	if err != nil || selection.ID != second.ID {
		t.Fatalf("sticky selection = %#v, %v", selection, err)
	}

	server.coolProxy(ctx, selection, 0)
	selection, err = server.selectProxy(ctx, true, []string{second.ID})
	if err != nil || selection.ID != first.ID {
		t.Fatalf("rotation selection = %#v, %v", selection, err)
	}
	if server.stickyProxy(ctx) != "" {
		t.Fatal("the cooled proxy stayed pinned")
	}
}

func TestSelectProxyGoesDirectForAnEmptyPoolOrDisabledConnections(t *testing.T) {
	service, _ := testProxyService(t)
	server := New(Options{Proxies: service})
	if selection, err := server.selectProxy(context.Background(), true, nil); err != nil || selection.ID != "" {
		t.Fatalf("empty pool = %#v, %v", selection, err)
	}
	if selection, err := server.selectProxy(context.Background(), false, nil); err != nil || selection.ID != "" {
		t.Fatalf("connection without proxy = %#v, %v", selection, err)
	}
}

func TestAllCoolingAnswersReloOwn503WithRetryAfter(t *testing.T) {
	service, _ := testProxyService(t)
	created, err := service.Create(context.Background(), appproxy.Draft{
		Scheme: "http", Host: "127.0.0.1", Port: 7890, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	service.Cool(created.ID, 2*time.Minute)
	server := New(Options{Proxies: service})
	_, err = server.selectProxy(context.Background(), true, nil)
	if !errors.Is(err, appproxy.ErrAllCooling) {
		t.Fatalf("selectProxy() = %v, want all cooling", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	server.proxyCoolingRefusal(recorder, request, false, err)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
	if recorder.Header().Get("Retry-After") == "" {
		t.Fatal("the cooling refusal carried no Retry-After")
	}
}
