package cli_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"

	"github.com/jonaskahn/relo/internal/access"
	"github.com/jonaskahn/relo/internal/adapters/secrets"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	appaccess "github.com/jonaskahn/relo/internal/application/access"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

// fakeManagement starts a management API stand-in that answers with the
// given status payload and stores the admin token the CLI needs.
func fakeManagement(t *testing.T, home string, payload map[string]any) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode status payload: %v", err)
	}
	return fakeManagementStatus(t, home, http.StatusOK, string(body))
}

func fakeManagementStatus(t *testing.T, home string, status int, body string) *httptest.Server {
	t.Helper()
	if _, err := server.EnsureToken(server.TokenPath(home, server.AdminTokenFile)); err != nil {
		t.Fatalf("EnsureToken() error = %v", err)
	}
	management := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(management.Close)
	return management
}

func statusPayload() map[string]any {
	return map[string]any{
		"status":         "running",
		"version":        "test-version",
		"uptime_seconds": 4980,
		"addr":           "127.0.0.1:10101",
		"secret_mode":    "keychain",
		"schema_version": sqlite.LatestSchemaVersion(),
	}
}

// preparedHome returns a state directory that has already been started
// once: configuration, database, tokens, and one client key are in place.
func preparedHome(t *testing.T) string {
	t.Helper()
	home := testkit.TempHome(t)
	useFreePorts(t, home)
	logger, _ := testkit.TestLogger(t)
	if _, err := config.LoadConfig(config.ConfigPath(home), logger); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	db, err := sqlite.OpenDB(config.DatabasePath(home), logger)
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := server.EnsureToken(server.TokenPath(home, server.AdminTokenFile)); err != nil {
		t.Fatalf("EnsureToken() error = %v", err)
	}
	// A healthy install can serve inference, which needs a client key: with
	// none, the daemon reports the data plane unreachable.
	ctx := context.Background()
	session, err := platform.OpenManagement(ctx, home, nil)
	if err != nil {
		t.Fatalf("OpenManagement() error = %v", err)
	}
	defer func() { _ = session.Close() }()
	if _, err := session.Keys().CreateAccessKey(ctx, appaccess.NewAccessKey{
		Name: "prepared-agent", Kind: appaccess.Agent, Client: access.CustomClient,
	}); err != nil {
		t.Fatalf("CreateAccessKey() error = %v", err)
	}
	seedProvider(t, session.DB(), "openai")
	return home
}

// seedProvider stores the provider a credential has to hang off, so a test
// has an install an operator would recognise.
func seedProvider(t *testing.T, db *sqlite.DB, id string) {
	t.Helper()
	host := sqlite.ProviderRow{
		ID: id, Origin: string(catalog.OriginCustom), Label: id,
		Auth: string(catalog.AuthAPIKey), APIFormat: string(catalog.FormatOpenAIChat),
		BaseURL: "https://" + id + "/v1", KeyHeader: string(catalog.KeyHeaderBearer),
		ModelsFormat: string(catalog.ModelsNone),
		Headers:      map[string]string{}, Variables: map[string]string{},
		Enabled: true, Rank: 100, PoolStrategy: sqlite.StrategyLeastLoaded,
	}
	if err := sqlite.NewCatalogRepo(db).SaveProvider(context.Background(), host); err != nil {
		t.Fatalf("save provider %s: %v", id, err)
	}
}

// corruptSecretKey puts a key file Relo cannot read into a state directory,
// which is how a test makes a command fail on the secret store.
func corruptSecretKey(t *testing.T, home string) {
	t.Helper()
	if err := os.WriteFile(secrets.KeyPath(home, secrets.DefaultKeyFile), []byte("not-a-key"), 0o600); err != nil {
		t.Fatalf("write a broken secret key: %v", err)
	}
}

// useFreePorts points the state directory's configuration at an unused
// loopback port, so a diagnostic never depends on what else runs on the
// machine.
func useFreePorts(t *testing.T, home string) {
	t.Helper()
	writePort(t, home, strconv.Itoa(freePort(t)))
}

// writePort replaces the state directory's configuration with one that
// listens on the given port, which is how a test points a command at the
// endpoint it stubbed. The data plane takes free ports of its own, because
// a daemon a test starts must never want the port another package's daemon
// is already serving on.
func writePort(t *testing.T, home, port string) {
	t.Helper()
	used := map[string]bool{port: true}
	next := func() string {
		t.Helper()
		for {
			candidate := strconv.Itoa(freePort(t))
			if !used[candidate] {
				used[candidate] = true
				return candidate
			}
		}
	}
	body := "[server]\nbind = \"127.0.0.1\"\nport = " + port + "\n" +
		"\n[server.data_plane]\nopenai = " + next() +
		"\nanthropic = " + next() +
		"\ngemini = " + next() + "\n"
	if err := os.WriteFile(config.ConfigPath(home), []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

func portOf(t *testing.T, rawURL string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %s: %v", rawURL, err)
	}
	return parsed.Port()
}
