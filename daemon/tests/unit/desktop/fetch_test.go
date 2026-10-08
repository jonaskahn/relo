package tray_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/desktop"
)

func TestFetchUpdates(t *testing.T) {
	t.Run("reads the update check", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/updates" {
				t.Errorf("path = %q, want /api/v1/updates", r.URL.Path)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer admin-token" {
				t.Errorf("Authorization = %q, want the admin token", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"current":"0.1.0","latest":"0.2.0","available":true,"url":"https://example.test/relo","method":"download"}`))
		}))
		defer server.Close()

		info, err := desktop.FetchUpdates(context.Background(), server.Client(), server.URL, "admin-token")
		if err != nil {
			t.Fatalf("FetchUpdates() error = %v", err)
		}
		if info.Latest != "0.2.0" || !info.Available {
			t.Fatalf("info = %+v, want the newer release", info)
		}
	})

	t.Run("a rejected read is an error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		if _, err := desktop.FetchUpdates(context.Background(), server.Client(), server.URL, "bad"); !errors.Is(err, desktop.ErrFetchStatus) {
			t.Fatalf("FetchUpdates() error = %v, want ErrFetchStatus", err)
		}
	})

	t.Run("a broken body is an error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{`))
		}))
		defer server.Close()

		if _, err := desktop.FetchUpdates(context.Background(), server.Client(), server.URL, "admin-token"); err == nil {
			t.Fatal("FetchUpdates() error = nil, want a decode failure")
		}
	})
}
