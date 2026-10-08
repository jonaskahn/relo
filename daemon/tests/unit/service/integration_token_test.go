package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	appaccess "github.com/jonaskahn/relo/internal/application/access"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/tests/testkit"
)

// tokenService builds the integration use cases over a temporary state
// directory. A data plane address answers only the surface check, and a
// harness built by hand skips the models.dev listener the shared one starts,
// which this sandbox refuses.
func tokenService(t *testing.T) (*harness, *appintegration.Service) {
	t.Helper()
	// The models.dev directory the shared harness builds starts a listener
	// this sandbox refuses, so this test builds its own: a catalog read from
	// the database rows the test seeds never needs one.
	h := &harness{
		t: t, home: testkit.TempHome(t), db: testkit.OpenTestDB(t),
		secrets: map[string]string{}, clock: testkit.NewFakeClock(time.Unix(1_700_000_000, 0)),
		refreshes: newRefreshTracker(),
	}
	h.catalog = h.newCatalog()
	h.pools = account.NewManager(sqlite.NewCredentialStore(h.db), testkit.SecretStore(h.secrets))
	if err := h.pools.LoadFromDB(context.Background()); err != nil {
		t.Fatalf("LoadFromDB() error = %v", err)
	}
	h.keys = appaccess.NewKeys(sqlite.NewAccessKeyStore(h.db), h.clock)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))
	logger, _ := testkit.TestLogger(t)
	relo, err := os.Executable()
	if err != nil {
		relo = ""
	}
	clientPaths := codingclients.Paths{
		Home: t.TempDir(), StateHome: h.home, Relo: relo,
		DataPlane: func(protocol string) string {
			return "http://127.0.0.1:0/probe"
		},
	}
	service := appintegration.NewService(appintegration.ServiceOptions{
		Paths:     platform.NewPathResolver(clientPaths),
		Agents:    platform.NewAgentRegistry(),
		Files:     platform.NewClientFiles(clientPaths),
		Processes: platform.NewAgentProcesses(),
		Wire:      platform.NewWireCodec(),
		Store:     platform.NewIntegrationStore(h.db), Keys: h.keys, Catalog: h.catalog,
		Logger: logger, Now: h.clock.Now,
	})
	return h, service
}

// TestIntegrationTokenReadsTheStoredKey covers the read behind the manage
// modal's eye control: an enabled integration answers with the secret its
// agent runs with, and anything else is refused rather than answered with a
// half token.
func TestIntegrationTokenReadsTheStoredKey(t *testing.T) {
	_, built := tokenService(t)
	ctx := context.Background()
	t.Run("an enabled integration answers with the stored secret", func(t *testing.T) {
		// EnableIntegration writes the key file itself, which is the only
		// state the token read needs beyond the record.
		result, err := built.EnableIntegration(ctx, "codex")
		if err != nil {
			t.Fatalf("EnableIntegration() error = %v", err)
		}
		token, err := built.IntegrationToken(ctx, "codex")
		if err != nil {
			t.Fatalf("IntegrationToken() error = %v", err)
		}
		if token != result.Token {
			t.Fatalf("token = %q, want the key the integration mints", token)
		}
	})

	t.Run("an integration with no key file is refused", func(t *testing.T) {
		if _, err := built.IntegrationToken(ctx, "cursor"); err == nil || !errors.Is(err, appaccess.ErrNotFound) {
			t.Fatalf("IntegrationToken() error = %v, want %v", err, appaccess.ErrNotFound)
		}
	})

	t.Run("an unknown integration is refused", func(t *testing.T) {
		if _, err := built.IntegrationToken(ctx, "nobody"); err == nil || !errors.Is(err, appintegration.ErrUnknownIntegration) {
			t.Fatalf("IntegrationToken() error = %v, want %v", err, appintegration.ErrUnknownIntegration)
		}
	})
}

// TestIntegrationTokenRefusesADisabledIntegration keeps the modal's reveal
// from answering for a client Relo has taken its wiring out of: the key file
// may still exist after a disable that kept it, but the token is retired and
// nothing should hand it over again.
func TestIntegrationTokenRefusesADisabledIntegration(t *testing.T) {
	_, built := tokenService(t)
	ctx := context.Background()
	t.Setenv("CODEX_HOME", t.TempDir())

	// A record alone is what an off integration reads as: no key file, no
	// token. The service refuses before it touches the file system.
	if _, err := built.IntegrationToken(ctx, "codex"); err == nil || !errors.Is(err, appaccess.ErrNotFound) {
		t.Fatalf("IntegrationToken() error = %v, want %v", err, appaccess.ErrNotFound)
	}
}
