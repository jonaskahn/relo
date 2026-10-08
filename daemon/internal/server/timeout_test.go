package server

import (
	"path/filepath"
	"testing"
	"time"

	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
)

// TestCallWaitPrefersTheConnectionThenGlobal covers the resolved call wait:
// a connection's own value wins, otherwise the global stored value decides,
// and a build without either answers the shipped default.
func TestCallWaitPrefersTheConnectionThenGlobal(t *testing.T) {
	stored := filepath.Join(t.TempDir(), "config.toml")
	if err := config.UpdateUpstreamTimeout(stored, 300); err != nil {
		t.Fatal(err)
	}
	served := New(Options{Settings: appsettings.New(appsettings.Options{ConfigPath: stored})})
	if got := served.callWait(catalog.Provider{}); got != 300*time.Second {
		t.Fatalf("callWait(no override) = %v, want the global 300s", got)
	}

	override := 180
	if got := served.callWait(catalog.Provider{TimeoutSeconds: &override}); got != 180*time.Second {
		t.Fatalf("callWait(override) = %v, want the connection's 180s", got)
	}

	bare := New(Options{})
	if got := bare.callWait(catalog.Provider{}); got != config.DefaultUpstreamTimeoutSeconds*time.Second {
		t.Fatalf("callWait(bare) = %v, want the default", got)
	}
}
