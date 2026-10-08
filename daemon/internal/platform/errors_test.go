package platform

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jonaskahn/relo/internal/server"
)

// TestStopRefusalsAreInspectable covers the stop-time refusals with
// errors.Is, so a caller can tell a missing token from a refusing daemon.
func TestStopRefusalsAreInspectable(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	tokenPath := server.TokenPath(home, server.AdminTokenFile)
	if err := os.WriteFile(tokenPath, []byte("  "), 0o600); err != nil {
		t.Fatal(err)
	}
	published := Runtime{Address: "127.0.0.1:1", InstanceID: "test"}
	if err := RequestStop(t.Context(), home, published); !errors.Is(err, ErrAdminTokenMissing) {
		t.Fatalf("RequestStop with blank token = %v, want ErrAdminTokenMissing", err)
	}

	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	t.Cleanup(refusing.Close)
	if err := os.WriteFile(tokenPath, []byte("token"), 0o600); err != nil {
		t.Fatal(err)
	}
	address := refusing.URL[len("http://"):]
	published = Runtime{Address: address, InstanceID: "test"}
	if err := RequestStop(t.Context(), home, published); !errors.Is(err, ErrStopRefused) {
		t.Fatalf("RequestStop refused = %v, want ErrStopRefused", err)
	}
}
