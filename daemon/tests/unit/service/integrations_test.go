package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/wire/formats"
	appaccess "github.com/jonaskahn/relo/internal/application/access"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/tests/testkit"
)

// integrationService builds the integration use cases over a temporary state
// directory with a configuration that serves both client protocols, which is
// what an integration needs before it can point an agent anywhere.
func integrationService(t *testing.T) (*harness, *appintegration.Service) {
	t.Helper()
	h := newHarness(t)
	settings := &config.Config{}
	settings.Server.Bind = "127.0.0.1"
	settings.Server.DataPlane.OpenAI = 10201
	settings.Server.DataPlane.Anthropic = 10202
	return h, integrationUseCases(t, h, settings, t.TempDir())
}

// integrationUseCases wires the integration use cases the way the daemon
// does: the operator's home, this state directory, and the data plane the
// configuration serves.
func integrationUseCases(t *testing.T, h *harness, settings *config.Config, homeDirectory string) *appintegration.Service {
	t.Helper()
	logger, _ := testkit.TestLogger(t)
	relo, err := os.Executable()
	if err != nil {
		relo = ""
	}
	clientPaths := codingclients.Paths{
		Home: homeDirectory, StateHome: h.home, Relo: relo,
		DataPlane: func(protocol string) string { return dataPlaneURL(settings, protocol) },
	}
	return appintegration.NewService(appintegration.ServiceOptions{
		Paths:     platform.NewPathResolver(clientPaths),
		Agents:    platform.NewAgentRegistry(),
		Files:     platform.NewClientFiles(clientPaths),
		Processes: platform.NewAgentProcesses(),
		Wire:      platform.NewWireCodec(),
		Store:     platform.NewIntegrationStore(h.db), Keys: h.keys, Catalog: h.catalog,
		Logger: logger, Now: h.clock.Now,
	})
}

// dataPlaneURL answers the address one client protocol is served on.
func dataPlaneURL(cfg *config.Config, protocol string) string {
	address := cfg.DataPlaneAddr(protocol)
	if address == "" {
		return ""
	}
	return "http://" + address
}

// TestEnableIntegrationOwnsItsKeyAndDisableRetiresIt is the ownership rule: the
// integration mints the key, and turning it off retires that key.
func TestEnableIntegrationOwnsItsKeyAndDisableRetiresIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	h, built := integrationService(t)
	ctx := context.Background()

	result, err := built.EnableIntegration(ctx, "codex")
	if err != nil {
		t.Fatalf("EnableIntegration() error = %v", err)
	}
	if !strings.HasPrefix(result.Token, "rlo_ak_") {
		t.Fatalf("token = %q, want a client key", result.Token)
	}
	if !result.Integration.Enabled || result.Integration.State != "on" {
		t.Fatalf("integration = %+v, want it on", result.Integration)
	}
	if result.Integration.Key == nil || result.Integration.Key.Hint == "" {
		t.Fatalf("integration key = %+v, want the hint of the key it owns", result.Integration.Key)
	}
	configPath := filepath.Join(home, ".codex", "config.toml")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read the codex config: %v", err)
	}
	if !strings.Contains(string(config), "model_providers.relo") {
		t.Fatalf("codex config = %q, want Relo's provider", config)
	}

	keys, err := h.keys.AccessKeys(ctx)
	if err != nil {
		t.Fatalf("AccessKeys() error = %v", err)
	}
	var owned appaccess.AccessKey
	for _, key := range keys {
		if key.Owner == "codex" {
			owned = key
		}
	}
	if owned.ID == "" {
		t.Fatalf("keys = %+v, want one owned by the codex integration", keys)
	}

	// An operator may not change a key an integration owns.
	if _, err := h.keys.UpdateAccessKey(ctx, owned.ID, appaccess.AccessKeyUpdate{Name: "renamed"}); !errors.Is(err, appaccess.ErrOwned) {
		t.Fatalf("UpdateAccessKey() error = %v, want %v", err, appaccess.ErrOwned)
	}
	if _, err := h.keys.RotateAccessKey(ctx, owned.ID); !errors.Is(err, appaccess.ErrOwned) {
		t.Fatalf("RotateAccessKey() error = %v, want %v", err, appaccess.ErrOwned)
	}
	if err := h.keys.RevokeAccessKey(ctx, owned.ID); !errors.Is(err, appaccess.ErrOwned) {
		t.Fatalf("RevokeAccessKey() error = %v, want %v", err, appaccess.ErrOwned)
	}

	if err := built.DisableIntegration(ctx, "codex"); err != nil {
		t.Fatalf("DisableIntegration() error = %v", err)
	}
	view, err := built.Integration(ctx, "codex")
	if err != nil {
		t.Fatalf("Integration() error = %v", err)
	}
	if view.Enabled || view.State != "off" {
		t.Fatalf("integration after disable = %+v, want it off", view)
	}
	if view.Key != nil && view.Key.Status != "revoked" {
		t.Fatalf("key after disable = %+v, want it revoked", view.Key)
	}
	// Relo created that file, so removing the contribution removes the file:
	// the operator had no Codex configuration before the integration.
	config, err = os.ReadFile(configPath)
	if err == nil && strings.Contains(string(config), "relo") {
		t.Fatalf("codex config after disable = %q, want Relo gone", config)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read the codex config after disable: %v", err)
	}
}

// TestIntegrationsListsEveryAgent keeps the console's list complete.
func TestIntegrationsListsEveryAgent(t *testing.T) {
	_, built := integrationService(t)
	views, err := built.Integrations(context.Background())
	if err != nil {
		t.Fatalf("Integrations() error = %v", err)
	}
	ids := make([]string, 0, len(views))
	for _, view := range views {
		ids = append(ids, view.ID)
		if view.State != "off" {
			t.Fatalf("%s state = %q, want off before anything is done", view.ID, view.State)
		}
	}
	for _, want := range []string{"codex", "claude-code", "cursor", "grok-build", "claude-desktop"} {
		found := false
		for _, id := range ids {
			if id == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("integrations = %v, want %s", ids, want)
		}
	}
}

// TestRefreshAccountModelsStoresTheRoster covers the per-account read: the
// account's own list is stored, and the connection's list takes it in.
func TestRefreshAccountModelsStoresTheRoster(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	roster := listingServer(t, http.StatusOK, "{\"models\":["+
		"{\"slug\":\"gpt-5.5\",\"display_name\":\"GPT-5.5\",\"supported_in_api\":true,\"visibility\":\"list\"},"+
		"{\"slug\":\"gpt-reserve\",\"display_name\":\"GPT-Reserve\",\"supported_in_api\":true,\"visibility\":\"hide\"}]}")
	seedRow(t, h, sqlite.ProviderRow{
		ID: "openai-codex", TemplateID: "openai-codex", Origin: string(catalog.OriginSignIn),
		Label: "ChatGPT (Codex sign-in)", Auth: string(catalog.AuthOAuth),
		APIFormat: string(catalog.FormatOpenAIResp), BaseURL: roster.URL,
		ModelsSource: "listing", ModelsFormat: string(catalog.ModelsCodex),
		ModelsDevProviderID: "openai", Enabled: true, Rank: 100,
	})
	repo := sqlite.NewCredentialRepo(h.db)
	if err := repo.Insert(ctx, sqlite.CredentialRow{
		ID: "cred-work", ProviderID: "openai-codex", Kind: "oauth", Label: "work", Status: account.StatusActive,
	}); err != nil {
		t.Fatalf("insert the credential: %v", err)
	}
	h.pools.Adopt(account.PoolEntry{
		ID: "cred-work", ProviderID: "openai-codex", Kind: "oauth",
		Label: "work", Status: account.StatusActive,
	})
	manager := pageService(t, h, catalogServer(t, catalogFixture()).URL)

	models, err := manager.accounts.RefreshAccountModels(ctx, "cred-work")
	if err != nil {
		t.Fatalf("RefreshAccountModels() error = %v", err)
	}
	if !models.Known {
		t.Fatalf("models = %+v, want the roster marked known", models)
	}
	if len(models.Models) != 1 || models.Models[0] != "gpt-5.5" {
		t.Fatalf("models = %+v, want the account's own entitlement", models.Models)
	}
	if !h.pools.RosterKnown("cred-work") {
		t.Fatal("the pool does not know the roster it just read")
	}

	// A connection that publishes one list for every account has nothing per
	// account to read, and says so rather than sending a pointless request.
	seedRow(t, h, sqlite.ProviderRow{
		ID: "shared", Origin: string(catalog.OriginCustom), Label: "Shared",
		Auth: string(catalog.AuthAPIKey), APIFormat: string(catalog.FormatOpenAIChat),
		BaseURL: roster.URL, ModelsSource: "listing", ModelsFormat: string(catalog.ModelsOpenAI),
		Enabled: true, Rank: 100,
	})
	if err := repo.Insert(ctx, sqlite.CredentialRow{
		ID: "cred-shared", ProviderID: "shared", Kind: "api_key", Label: "key", Status: account.StatusActive,
	}); err != nil {
		t.Fatalf("insert the shared credential: %v", err)
	}
	if _, err := manager.accounts.RefreshAccountModels(ctx, "cred-shared"); !errors.Is(err, appaccount.ErrSharedRoster) {
		t.Fatalf("RefreshAccountModels() error = %v, want %v", err, appaccount.ErrSharedRoster)
	}
}

// TestAccountModelsReportsAnUnknownRoster keeps the console honest about what
// it has not read yet.
func TestAccountModelsReportsAnUnknownRoster(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.seedProvider("openai", "OpenAI", string(catalog.AuthAPIKey))
	account := h.addAccount("openai", "work", "sk-test")
	models, err := h.accounts.AccountModels(ctx, account.ID)
	if err != nil {
		t.Fatalf("AccountModels() error = %v", err)
	}
	if models.Known {
		t.Fatalf("models = %+v, want an unread roster reported as unknown", models)
	}
	if models.Models == nil {
		t.Fatal("models = nil, want an empty list rather than a null one")
	}
}

// TestPerAccountRosterOnlyForAccountSpecificDialects keeps a shared listing from
// being read once per key.
func TestPerAccountRosterOnlyForAccountSpecificDialects(t *testing.T) {
	if !catalog.PerAccountRoster(catalog.ModelsCodex) || !catalog.PerAccountRoster(catalog.ModelsAntigravity) {
		t.Fatal("a dialect whose roster narrows per account is not read per account")
	}
	if catalog.PerAccountRoster(catalog.ModelsOpenAI) || catalog.PerAccountRoster(catalog.ModelsNone) {
		t.Fatal("a shared roster is read per account, which sends one request per key")
	}
	if !formats.New().Supports(catalog.FormatOpenAIChat) {
		t.Fatal("the wire formats this test leans on are missing")
	}
}

// TestRefreshCodexCatalogLeavesAnUnchangedCatalog keeps a background refresh
// from bumping the catalog's mtime when Relo would write the same list, so
// the card does not ask for a Codex restart it does not need.
func TestRefreshCodexCatalogLeavesAnUnchangedCatalog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	_, built := integrationService(t)
	ctx := context.Background()
	if _, err := built.EnableIntegration(ctx, "codex"); err != nil {
		t.Fatalf("EnableIntegration() error = %v", err)
	}
	path := codingclients.CodexCatalogPath(filepath.Join(home, ".codex", "config.toml"))
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatalf("age the catalog: %v", err)
	}
	built.RefreshCodexCatalog(ctx)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat the catalog: %v", err)
	}
	if info.ModTime().After(past.Add(time.Second)) {
		t.Fatal("an unchanged catalog was rewritten")
	}
}

// TestCodexRestartNeededWhenTheDaemonIsOlderThanTheCatalog is the picker
// warning: a managed app-server that started before Relo wrote the catalog
// still holds the old list.
func TestCodexRestartNeededWhenTheDaemonIsOlderThanTheCatalog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows Signal(0) cannot prove a pid is alive")
	}
	home := t.TempDir()
	codexHome := filepath.Join(home, ".codex")
	t.Setenv("CODEX_HOME", codexHome)
	_, built := integrationService(t)
	ctx := context.Background()
	if _, err := built.EnableIntegration(ctx, "codex"); err != nil {
		t.Fatalf("EnableIntegration() error = %v", err)
	}
	writeCodexDaemonPid(t, codexHome, os.Getpid(), time.Now().Add(-time.Hour).Unix())
	view, err := built.Integration(ctx, "codex")
	if err != nil {
		t.Fatalf("Integration() error = %v", err)
	}
	if !view.RestartNeeded {
		t.Fatalf("integration = %+v, want restart_needed", view)
	}
}

// TestRestartIntegrationOnlyRestartsCodex keeps the console from offering a
// restart on a client Relo has no process for.
func TestRestartIntegrationOnlyRestartsCodex(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	_, built := integrationService(t)
	ctx := context.Background()
	if _, err := built.RestartIntegration(ctx, "missing"); !errors.Is(err, appintegration.ErrUnknownIntegration) {
		t.Fatalf("RestartIntegration(missing) error = %v, want %v", err, appintegration.ErrUnknownIntegration)
	}
	if _, err := built.RestartIntegration(ctx, "claude-code"); !errors.Is(err, appintegration.ErrRestartUnsupported) {
		t.Fatalf("RestartIntegration(claude-code) error = %v, want %v", err, appintegration.ErrRestartUnsupported)
	}
	view, err := built.RestartIntegration(ctx, "codex")
	if err != nil {
		t.Fatalf("RestartIntegration(codex) error = %v", err)
	}
	if view.RestartNeeded {
		t.Fatalf("integration = %+v, want no restart without a daemon", view)
	}
}

// TestRestartIntegrationRestartsTheOpenCodeService stores the integration key
// in OpenCode's service config and then restarts that server.
func TestRestartIntegrationRestartsTheOpenCodeService(t *testing.T) {
	h, built := integrationService(t)
	keyFile := filepath.Join(h.home, "integrations", "opencode", "key")
	if err := os.MkdirAll(filepath.Dir(keyFile), 0o700); err != nil {
		t.Fatalf("create the key directory: %v", err)
	}
	if err := os.WriteFile(keyFile, []byte("test-key\n"), 0o600); err != nil {
		t.Fatalf("write the key: %v", err)
	}
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + marker + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatalf("write the stub launcher: %v", err)
	}
	t.Setenv("PATH", binDir)
	if _, err := built.RestartIntegration(context.Background(), "opencode"); err != nil {
		t.Fatalf("RestartIntegration(opencode) error = %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read the stub marker: %v", err)
	}
	want := "service set env RELO_OPENCODE_API_KEY test-key\nservice restart\n"
	if string(got) != want {
		t.Fatalf("launcher args = %q, want %q", got, want)
	}
}

func writeCodexDaemonPid(t *testing.T, home string, pid int, startSeconds int64) {
	t.Helper()
	dir := filepath.Join(home, "app-server-daemon")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create the daemon directory: %v", err)
	}
	encoded, err := json.Marshal(map[string]any{
		"pid": pid, "processIdentity": map[string]any{"startSeconds": startSeconds},
	})
	if err != nil {
		t.Fatalf("encode the pid file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "daemon.pid"), encoded, 0o600); err != nil {
		t.Fatalf("write the pid file: %v", err)
	}
}
