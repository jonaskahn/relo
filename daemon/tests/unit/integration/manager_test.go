package integration_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/application/integration"
	"github.com/jonaskahn/relo/internal/platform"
)

// store is an in-memory integration store, which is what the manager writes
// through rather than a database.
type store struct {
	records map[string]integration.Record
	files   map[string][]integration.FileRecord
}

func newStore() *store {
	return &store{records: map[string]integration.Record{}, files: map[string][]integration.FileRecord{}}
}

func (s *store) Integration(_ context.Context, id string) (integration.Record, bool, error) {
	record, found := s.records[id]
	return record, found, nil
}

func (s *store) SaveIntegration(_ context.Context, record integration.Record) error {
	s.records[record.ID] = record
	return nil
}

func (s *store) Files(_ context.Context, id string) ([]integration.FileRecord, error) {
	return s.files[id], nil
}

func (s *store) SaveFile(_ context.Context, record integration.FileRecord) error {
	files := s.files[record.IntegrationID]
	for index, file := range files {
		if file.Kind == record.Kind {
			files[index] = record
			s.files[record.IntegrationID] = files
			return nil
		}
	}
	s.files[record.IntegrationID] = append(files, record)
	return nil
}

func (s *store) DeleteFiles(_ context.Context, id string) error {
	delete(s.files, id)
	return nil
}

func (s *store) AppendOp(_ context.Context, op integration.OpRecord) error {
	return nil
}

// harness is one manager over a temporary home and state directory.
type harness struct {
	manager *integration.Manager
	paths   codingclients.Paths
	files   integration.ClientFiles
	store   *store
}

func newHarness(t *testing.T, dataPlane func(string) string) *harness {
	t.Helper()
	return newHarnessAuth(t, dataPlane, integration.ClaudeAuthProxy)
}

func newHarnessAuth(t *testing.T, dataPlane func(string) string, mode integration.ClaudeAuth) *harness {
	t.Helper()
	home := t.TempDir()
	state := t.TempDir()
	codexHome := filepath.Join(home, ".codex")
	claudeHome := filepath.Join(home, ".claude")
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("CLAUDE_CONFIG_DIR", claudeHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HERMES_HOME", "")
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	paths := codingclients.Paths{
		Home: home, StateHome: state, Relo: "/usr/local/bin/relo",
		DataPlane: dataPlane,
	}
	built := newStore()
	manager := integration.New(integration.Options{
		Paths: platform.NewPathResolver(paths), Agents: platform.NewAgentRegistry(),
		Files: platform.NewClientFiles(paths),
		Store: built, Now: func() time.Time { return time.Unix(1_700_000_000, 0) },
		ClaudeAuth: func() integration.ClaudeAuth { return mode },
	})
	return &harness{manager: manager, paths: paths, files: platform.NewClientFiles(paths), store: built}
}

func dataPlane(protocol string) string {
	switch protocol {
	case "openai":
		return "http://127.0.0.1:10201"
	case "anthropic":
		return "http://127.0.0.1:10202"
	default:
		return ""
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

// TestEnableWiresCodexAndDisableTakesItBack is the whole promise of an
// integration: it writes the files, and turning it off leaves the machine as it
// was.
func TestEnableWiresCodexAndDisableTakesItBack(t *testing.T) {
	h := newHarness(t, dataPlane)
	configPath := h.paths.CodexConfig()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create the codex directory: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("approval_policy = \"never\"\n"), 0o644); err != nil {
		t.Fatalf("seed the codex config: %v", err)
	}

	view, err := h.manager.Enable(context.Background(), "codex", "rlo_ak_test")
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if view.State != integration.StateOn || !view.Enabled {
		t.Fatalf("Enable() state = %q enabled = %v, want on", view.State, view.Enabled)
	}
	config := readFile(t, configPath)
	keyPath := h.paths.KeyFile("codex")
	helperPath := h.paths.HelperFile("codex")
	if !strings.Contains(config, "model_providers.relo") || !strings.Contains(config, "approval_policy") {
		t.Fatalf("enabled config = %q, want Relo's provider beside the operator's own row", config)
	}
	if strings.Contains(config, "env_key") || !strings.Contains(config, helperPath) {
		t.Fatalf("enabled config = %q, want the key helper and no RELO_API_KEY lookup", config)
	}
	helper := readFile(t, helperPath)
	if !strings.Contains(helper, "daemon start") || !strings.Contains(helper, keyPath) {
		t.Fatalf("key helper = %q, want the daemon start and the key file", helper)
	}
	if got := readFile(t, keyPath); strings.TrimSpace(got) != "rlo_ak_test" {
		t.Fatalf("key file = %q, want the token", got)
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat the key file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode = %v, want 0600", info.Mode().Perm())
	}

	disabled, err := h.manager.Disable(context.Background(), "codex")
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if disabled.Enabled || disabled.State != integration.StateOff {
		t.Fatalf("Disable() state = %q enabled = %v, want off", disabled.State, disabled.Enabled)
	}
	config = readFile(t, configPath)
	if strings.Contains(config, "relo") {
		t.Fatalf("disabled config = %q, want Relo gone", config)
	}
	if !strings.Contains(config, "approval_policy") {
		t.Fatalf("disabled config = %q, want the operator's own row kept", config)
	}
	for _, path := range []string{keyPath, helperPath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s still exists after disable: %v", path, err)
		}
	}
}

// TestCodexContextButtonWritesTheWindow is the 1M control on the Codex card:
// setup writes the window while the button is on, turning it off takes those
// defaults back out, and turning it on puts them back once.
func TestCodexContextButtonWritesTheWindow(t *testing.T) {
	h := newHarness(t, dataPlane)
	configPath := h.paths.CodexConfig()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create the codex directory: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("model_context_window = 128000\n"), 0o644); err != nil {
		t.Fatalf("seed the codex config: %v", err)
	}
	ctx := context.Background()
	view, err := h.manager.Enable(ctx, "codex", "rlo_ak_test")
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if !view.Context1M {
		t.Fatal("Context1M = false, want the button on by default")
	}
	config := readFile(t, configPath)
	if strings.Count(config, "model_context_window") != 1 || !strings.Contains(config, "model_context_window = 128000") {
		t.Fatalf("enabled config replaced the operator's window:\n%s", config)
	}
	if !strings.Contains(config, "model_auto_compact_token_limit = 900000") {
		t.Fatalf("enabled config did not fill the missing compact limit:\n%s", config)
	}

	view, err = h.manager.SetCodexContext(ctx, false)
	if err != nil {
		t.Fatalf("SetCodexContext(false) error = %v", err)
	}
	if view.Context1M {
		t.Fatal("Context1M = true after the button was turned off")
	}
	config = readFile(t, configPath)
	if !strings.Contains(config, "model_context_window = 128000") || !strings.Contains(config, "model_providers.relo") {
		t.Fatalf("turning the button off changed more than Relo's default:\n%s", config)
	}
	if strings.Contains(config, "model_auto_compact_token_limit = 900000") {
		t.Fatalf("turning the button off left Relo's compact limit:\n%s", config)
	}

	if _, err := h.manager.SetCodexContext(ctx, true); err != nil {
		t.Fatalf("SetCodexContext(true) error = %v", err)
	}
	config = readFile(t, configPath)
	if strings.Count(config, "model_auto_compact_token_limit") != 1 || !strings.Contains(config, "model_auto_compact_token_limit = 900000") {
		t.Fatalf("turning the button on did not write the compact limit once:\n%s", config)
	}
	if strings.Count(config, "model_context_window") != 1 {
		t.Fatalf("turning the button on duplicated the window:\n%s", config)
	}
}

// TestRestorePutsTheOperatorsFileBack is the undo an operator reaches for after
// editing their own configuration: the snapshot is the file as it was.
func TestRestorePutsTheOperatorsFileBack(t *testing.T) {
	h := newHarness(t, dataPlane)
	configPath := h.paths.CodexConfig()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create the codex directory: %v", err)
	}
	original := "approval_policy = \"never\"\nmodel = \"gpt-5\"\n"
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatalf("seed the codex config: %v", err)
	}
	if _, err := h.manager.Enable(context.Background(), "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	// An operator edits the file while the integration is on.
	edited := readFile(t, configPath) + "\n[projects.\"/tmp\"]\ntrust_level = \"trusted\"\n"
	if err := os.WriteFile(configPath, []byte(edited), 0o644); err != nil {
		t.Fatalf("edit the codex config: %v", err)
	}
	if _, err := h.manager.Restore(context.Background(), "codex"); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if got := readFile(t, configPath); got != original {
		t.Fatalf("restored config = %q, want %q", got, original)
	}
}

// TestEnableRefusesASurfaceNothingServes reports the honest refusal rather than
// pointing a client at a port no listener answers on.
func TestEnableRefusesASurfaceNothingServes(t *testing.T) {
	h := newHarness(t, func(string) string { return "" })
	if _, err := h.manager.Enable(context.Background(), "codex", "rlo_ak_test"); !errors.Is(err, integration.ErrSurfaceDisabled) {
		t.Fatalf("Enable() error = %v, want %v", err, integration.ErrSurfaceDisabled)
	}
}

// legacyLauncher installs the wrapper an earlier Relo version left at the
// Codex launcher path, beside the client it moved aside.
func legacyLauncher(t *testing.T, h *harness) (codexPath, original string) {
	t.Helper()
	bin := filepath.Join(h.paths.Home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("create the bin directory: %v", err)
	}
	codexPath = filepath.Join(bin, "codex")
	original = "#!/bin/sh\necho real codex\n"
	if err := os.WriteFile(codexPath+".relo-backup", []byte(original), 0o755); err != nil {
		t.Fatalf("write the moved client: %v", err)
	}
	if err := os.WriteFile(codexPath, []byte("#!/bin/sh\n# relo codex launcher shim\nexec codex.relo-backup\n"), 0o755); err != nil {
		t.Fatalf("write the old shim: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return codexPath, original
}

// TestEnableNeverWrapsTheCodexLauncher is why `codex` cannot go missing: the
// client on PATH is left exactly as the operator has it.
func TestEnableNeverWrapsTheCodexLauncher(t *testing.T) {
	h := newHarness(t, dataPlane)
	bin := filepath.Join(h.paths.Home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("create the bin directory: %v", err)
	}
	codexPath := filepath.Join(bin, "codex")
	original := "#!/bin/sh\necho real codex\n"
	if err := os.WriteFile(codexPath, []byte(original), 0o755); err != nil {
		t.Fatalf("write the fake codex: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, err := h.manager.Enable(context.Background(), "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if got := readFile(t, codexPath); got != original {
		t.Fatalf("codex after enable = %q, want the client untouched", got)
	}
	if _, err := os.Stat(codexPath + ".relo-backup"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("enable left a backup beside the client: %v", err)
	}
}

// TestEnableRetiresAnOldLauncherWithoutItsState covers a machine where the
// state file is gone: the moved client still comes back.
func TestEnableRetiresAnOldLauncherWithoutItsState(t *testing.T) {
	h := newHarness(t, dataPlane)
	codexPath, original := legacyLauncher(t, h)

	if _, err := h.manager.Enable(context.Background(), "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if got := readFile(t, codexPath); got != original {
		t.Fatalf("codex after enable = %q, want the original client", got)
	}
	if _, err := os.Stat(codexPath + ".relo-backup"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the backup survived: %v", err)
	}
}

// TestRestoreKeepsTheClientWhenAnOldLauncherIsRecorded guards the failure that
// deleted the real client: a stored launcher row points at the operator's own
// binary, so a restore must not remove that path.
func TestRestoreKeepsTheClientWhenAnOldLauncherIsRecorded(t *testing.T) {
	h := newHarness(t, dataPlane)
	codexPath, original := legacyLauncher(t, h)
	ctx := context.Background()
	if _, err := h.manager.Enable(ctx, "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if err := h.store.SaveFile(ctx, integration.FileRecord{
		IntegrationID: "codex", Kind: integration.KindCodexLauncher, Path: codexPath,
	}); err != nil {
		t.Fatalf("record the old launcher: %v", err)
	}

	if _, err := h.manager.Restore(ctx, "codex"); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if got := readFile(t, codexPath); got != original {
		t.Fatalf("codex after restore = %q, want the original client", got)
	}
}

// TestInspectIgnoresEditsOutsideRelosContribution is why a client that rewrites
// its own file does not look broken: only Relo's block has to match.
func TestInspectIgnoresEditsOutsideRelosContribution(t *testing.T) {
	h := newHarness(t, dataPlane)
	configPath := h.paths.CodexConfig()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create the codex directory: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("approval_policy = \"never\"\n"), 0o644); err != nil {
		t.Fatalf("seed the codex config: %v", err)
	}
	if _, err := h.manager.Enable(context.Background(), "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	edited := strings.Replace(readFile(t, configPath), "approval_policy = \"never\"", "approval_policy = \"on-request\"", 1)
	if err := os.WriteFile(configPath, []byte(edited), 0o644); err != nil {
		t.Fatalf("edit the codex config: %v", err)
	}
	view, err := h.manager.Inspect(context.Background(), "codex")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if view.State != integration.StateOn {
		t.Fatalf("Inspect() state = %q, want on", view.State)
	}
	if file := fileOf(view, integration.KindCodexConfig); file.Drifted {
		t.Fatal("the codex config is drifted after an edit outside Relo's block")
	}

	settingsPath := h.paths.ClaudeSettings()
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("create the claude directory: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte("{\"model\":\"opus\"}\n"), 0o600); err != nil {
		t.Fatalf("seed the settings: %v", err)
	}
	if _, err := h.manager.Enable(context.Background(), "claude-code", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	settings := strings.Replace(readFile(t, settingsPath), "\"model\": \"opus\"", "\"model\": \"sonnet\"", 1)
	if err := os.WriteFile(settingsPath, []byte(settings), 0o600); err != nil {
		t.Fatalf("edit the settings: %v", err)
	}
	view, err = h.manager.Inspect(context.Background(), "claude-code")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if view.State != integration.StateOn {
		t.Fatalf("Inspect() state = %q, want on", view.State)
	}
	if file := fileOf(view, integration.KindClaudeSettings); file.Drifted {
		t.Fatal("the claude settings are drifted after an edit outside Relo's keys")
	}
}

// TestRepairRestoresARemovedContribution puts Relo's block back when an editor
// took it out, and leaves an intact file alone once that is done.
func TestRepairRestoresARemovedContribution(t *testing.T) {
	h := newHarness(t, dataPlane)
	configPath := h.paths.CodexConfig()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create the codex directory: %v", err)
	}
	if _, err := h.manager.Enable(context.Background(), "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte(codingclients.StripCodexConfig(readFile(t, configPath))), 0o644); err != nil {
		t.Fatalf("remove the codex block: %v", err)
	}
	view, err := h.manager.Inspect(context.Background(), "codex")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if view.State != integration.StateDrifted || !fileOf(view, integration.KindCodexConfig).Drifted {
		t.Fatalf("Inspect() state = %q drifted = %v, want drifted", view.State, fileOf(view, integration.KindCodexConfig).Drifted)
	}
	if _, err := h.manager.Repair(context.Background(), "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Repair() error = %v", err)
	}
	view, err = h.manager.Inspect(context.Background(), "codex")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if view.State != integration.StateOn || fileOf(view, integration.KindCodexConfig).Drifted {
		t.Fatalf("Inspect() after repair state = %q drifted = %v, want on", view.State, fileOf(view, integration.KindCodexConfig).Drifted)
	}

	settingsPath := h.paths.ClaudeSettings()
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("create the claude directory: %v", err)
	}
	if _, err := h.manager.Enable(context.Background(), "claude-code", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	stripped, err := codingclients.StripClaudeSettings(readFile(t, settingsPath), "http://127.0.0.1:10202", h.paths.HelperFile("claude-code"))
	if err != nil {
		t.Fatalf("StripClaudeSettings() error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(stripped), 0o600); err != nil {
		t.Fatalf("remove Relo's settings: %v", err)
	}
	view, err = h.manager.Inspect(context.Background(), "claude-code")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if view.State != integration.StateDrifted || !fileOf(view, integration.KindClaudeSettings).Drifted {
		t.Fatalf("Inspect() state = %q drifted = %v, want drifted", view.State, fileOf(view, integration.KindClaudeSettings).Drifted)
	}
	if _, err := h.manager.Repair(context.Background(), "claude-code", "rlo_ak_test"); err != nil {
		t.Fatalf("Repair() error = %v", err)
	}
	view, err = h.manager.Inspect(context.Background(), "claude-code")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if view.State != integration.StateOn || fileOf(view, integration.KindClaudeSettings).Drifted {
		t.Fatalf("Inspect() after repair state = %q drifted = %v, want on", view.State, fileOf(view, integration.KindClaudeSettings).Drifted)
	}
}

func fileOf(view integration.View, kind string) integration.FileView {
	for _, file := range view.Files {
		if file.Kind == kind {
			return file
		}
	}
	return integration.FileView{}
}

// TestInspectReportsDrift is what tells an operator an edit replaced the key
// helper Relo wrote, and Repair puts it back.
func TestInspectReportsDrift(t *testing.T) {
	h := newHarness(t, dataPlane)
	if _, err := h.manager.Enable(context.Background(), "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if view, err := h.manager.Inspect(context.Background(), "codex"); err != nil || view.State != integration.StateOn {
		t.Fatalf("Inspect() state = %q error = %v, want on", view.State, err)
	}
	if err := os.WriteFile(h.paths.HelperFile("codex"), []byte("#!/bin/sh\necho replaced\n"), 0o700); err != nil {
		t.Fatalf("replace the key helper: %v", err)
	}
	view, err := h.manager.Inspect(context.Background(), "codex")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if view.State != integration.StateDrifted {
		t.Fatalf("Inspect() state = %q, want drifted", view.State)
	}
	if _, err := h.manager.Repair(context.Background(), "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Repair() error = %v", err)
	}
	if view, err := h.manager.Inspect(context.Background(), "codex"); err != nil || view.State != integration.StateOn {
		t.Fatalf("Inspect() after repair state = %q error = %v, want on", view.State, err)
	}
}

// TestClaudeCodeWritesSettingsAndHelper covers the second managed agent: the
// settings point at Relo, and the key stays in Relo's own directory.
func TestClaudeCodeWritesSettingsAndHelper(t *testing.T) {
	h := newHarness(t, dataPlane)
	settingsPath := h.paths.ClaudeSettings()
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("create the claude directory: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte("{\"model\":\"opus\"}\n"), 0o600); err != nil {
		t.Fatalf("seed the settings: %v", err)
	}
	if _, err := h.manager.Enable(context.Background(), "claude-code", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	settings := readFile(t, settingsPath)
	if !strings.Contains(settings, "ANTHROPIC_BASE_URL") || !strings.Contains(settings, "apiKeyHelper") ||
		!strings.Contains(settings, codingclients.ClaudeGatewayDiscoveryEnv) {
		t.Fatalf("settings = %q, want Relo's surface, discovery, and helper", settings)
	}
	if !strings.Contains(settings, "\"model\": \"opus\"") {
		t.Fatalf("settings = %q, want the operator's own key kept", settings)
	}
	cache := readFile(t, codingclients.GatewayCachePath(h.paths.ClaudeConfigDir()))
	if !strings.Contains(cache, "http://127.0.0.1:10202") {
		t.Fatalf("cache = %q, want Relo's base URL", cache)
	}
	helper := h.paths.HelperFile("claude-code")
	if !strings.Contains(readFile(t, helper), h.paths.KeyFile("claude-code")) {
		t.Fatalf("helper = %q, want the key file it prints", readFile(t, helper))
	}
	info, err := os.Stat(helper)
	if err != nil {
		t.Fatalf("stat the helper: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("helper mode = %v, want 0700", info.Mode().Perm())
	}
	if _, err := h.manager.Disable(context.Background(), "claude-code"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if settings := readFile(t, settingsPath); strings.Contains(settings, "ANTHROPIC_BASE_URL") ||
		strings.Contains(settings, codingclients.ClaudeGatewayDiscoveryEnv) {
		t.Fatalf("settings after disable = %q, want Relo gone", settings)
	}
	if _, err := os.Stat(codingclients.GatewayCachePath(h.paths.ClaudeConfigDir())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("gateway cache after disable: %v, want it removed", err)
	}
}

// TestClaudeCodeLoginOmitsTheKeyHelper is the other detected mode: a claude.ai
// login gets the surface and the gateway switch, and not an auth source.
func TestClaudeCodeLoginOmitsTheKeyHelper(t *testing.T) {
	h := newHarnessAuth(t, dataPlane, integration.ClaudeAuthLogin)
	window := int64(128_000)
	h.manager = integration.New(integration.Options{
		Paths: platform.NewPathResolver(h.paths), Agents: platform.NewAgentRegistry(),
		Files: h.files, Store: h.store, Now: func() time.Time { return time.Unix(1_700_000_000, 0) },
		ClaudeAuth: func() integration.ClaudeAuth { return integration.ClaudeAuthLogin },
		Models: func() []integration.ModelRef {
			return []integration.ModelRef{{ID: "relo-openai-gpt-4o", Name: "OpenAI | GPT-4o", ContextWindow: &window}}
		},
	})
	if _, err := h.manager.Enable(context.Background(), "claude-code", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	settings := readFile(t, h.paths.ClaudeSettings())
	if strings.Contains(settings, "apiKeyHelper") || strings.Contains(settings, "ANTHROPIC_API_KEY") ||
		strings.Contains(settings, "ANTHROPIC_AUTH_TOKEN") {
		t.Fatalf("settings = %q, want no auth source", settings)
	}
	if !strings.Contains(settings, "ANTHROPIC_BASE_URL") || !strings.Contains(settings, codingclients.ClaudeGatewayDiscoveryEnv) {
		t.Fatalf("settings = %q, want the surface and gateway discovery", settings)
	}
	cache := readFile(t, codingclients.GatewayCachePath(h.paths.ClaudeConfigDir()))
	if !strings.Contains(cache, "claude-relo-openai-gpt-4o") || !strings.Contains(cache, "http://127.0.0.1:10202") {
		t.Fatalf("cache = %q, want the claude- spelling and Relo's base URL", cache)
	}
}

// TestClaudeCacheKeepsPickerNamesOffOtherClients pins the split: Claude Code's
// cache shows the picker label, and a client Relo configures beside it keeps
// the catalog name.
func TestClaudeCacheKeepsPickerNamesOffOtherClients(t *testing.T) {
	h := newHarness(t, dataPlane)
	config := filepath.Join(h.paths.Home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatalf("create the opencode directory: %v", err)
	}
	if err := os.WriteFile(config, []byte("{\"theme\":\"dark\"}\n"), 0o644); err != nil {
		t.Fatalf("seed the opencode config: %v", err)
	}
	million := int64(1_000_000)
	h.manager = integration.New(integration.Options{
		Paths: platform.NewPathResolver(h.paths), Agents: platform.NewAgentRegistry(),
		Files: h.files, Store: h.store, Now: func() time.Time { return time.Unix(1_700_000_000, 0) },
		ClaudeAuth: func() integration.ClaudeAuth { return integration.ClaudeAuthProxy },
		Models: func() []integration.ModelRef {
			return []integration.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o", Connection: "OpenAI"}}
		},
		ClaudeModels: func() []integration.ModelRef {
			return []integration.ModelRef{{
				ID: "claude-relo-openai--gpt-4o[1m]", Name: "GPT-4o", Connection: "OpenAI", ContextWindow: &million,
			}}
		},
	})
	if _, err := h.manager.Enable(context.Background(), "opencode", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable(opencode) error = %v", err)
	}
	merged := readFile(t, config)
	if !strings.Contains(merged, "\"name\":\"GPT-4o on OpenAI\"") {
		t.Fatalf("opencode config = %s, want the config label", merged)
	}
	if _, err := h.manager.Enable(context.Background(), "claude-code", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable(claude-code) error = %v", err)
	}
	cache := readFile(t, codingclients.GatewayCachePath(h.paths.ClaudeConfigDir()))
	for _, want := range []string{"GPT-4o 1M on OpenAI", "claude-relo-openai--gpt-4o[1m]"} {
		if !strings.Contains(cache, want) {
			t.Fatalf("cache = %s, want %s", cache, want)
		}
	}
	if strings.Contains(cache, "OpenAI | GPT-4o") {
		t.Fatalf("cache = %s, want the picker label", cache)
	}
}

func withModels(h *harness, models *[]integration.ModelRef) {
	h.manager = integration.New(integration.Options{
		Paths: platform.NewPathResolver(h.paths), Agents: platform.NewAgentRegistry(),
		Files: h.files,
		Store: h.store, Now: func() time.Time { return time.Unix(1_700_000_000, 0) },
		ClaudeAuth: func() integration.ClaudeAuth { return integration.ClaudeAuthProxy },
		Models:     func() []integration.ModelRef { return *models },
	})
}

// TestInspectReportsStaleModels is what offers Repair after a catalog change:
// Codex, OpenCode, and Pi all go stale when the list changes,
// and Repair clears it. A client that is not enabled stays quiet.
func TestInspectReportsStaleModels(t *testing.T) {
	h := newHarness(t, dataPlane)
	models := []integration.ModelRef{{ID: "relo-a-one", Name: "One", Connection: "A"}}
	withModels(h, &models)
	ctx := context.Background()

	if _, err := h.manager.Enable(ctx, "pi", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable(pi) error = %v", err)
	}
	view, err := h.manager.Inspect(ctx, "pi")
	if err != nil || view.ModelsStale {
		t.Fatalf("Inspect(pi) stale = %v error = %v, want current", view.ModelsStale, err)
	}

	models = []integration.ModelRef{{ID: "relo-b-two", Name: "Two", Connection: "B"}}
	view, err = h.manager.Inspect(ctx, "pi")
	if err != nil || !view.ModelsStale {
		t.Fatalf("Inspect(pi) after a catalog change stale = %v error = %v, want stale", view.ModelsStale, err)
	}
	if _, err := h.manager.Repair(ctx, "pi", "rlo_ak_test"); err != nil {
		t.Fatalf("Repair(pi) error = %v", err)
	}
	view, err = h.manager.Inspect(ctx, "pi")
	if err != nil || view.ModelsStale {
		t.Fatalf("Inspect(pi) after repair stale = %v error = %v, want current", view.ModelsStale, err)
	}

	config := filepath.Join(h.paths.Home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatalf("create the opencode directory: %v", err)
	}
	if err := os.WriteFile(config, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("seed the opencode config: %v", err)
	}
	if _, err := h.manager.Enable(ctx, "opencode", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable(opencode) error = %v", err)
	}
	models = []integration.ModelRef{{ID: "relo-c-three", Name: "Three", Connection: "C"}}
	view, err = h.manager.Inspect(ctx, "opencode")
	if err != nil || !view.ModelsStale {
		t.Fatalf("Inspect(opencode) stale = %v error = %v, want stale", view.ModelsStale, err)
	}

	if err := os.MkdirAll(filepath.Dir(h.paths.CodexConfig()), 0o755); err != nil {
		t.Fatalf("create the codex directory: %v", err)
	}
	if _, err := h.manager.Enable(ctx, "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable(codex) error = %v", err)
	}
	view, err = h.manager.Inspect(ctx, "codex")
	if err != nil || view.ModelsStale {
		t.Fatalf("Inspect(codex) stale = %v error = %v, want current", view.ModelsStale, err)
	}
	if err := os.Remove(codingclients.CodexCatalogPath(h.paths.CodexConfig())); err != nil {
		t.Fatalf("remove the catalog: %v", err)
	}
	view, err = h.manager.Inspect(ctx, "codex")
	if err != nil || !view.ModelsStale {
		t.Fatalf("Inspect(codex) without a catalog stale = %v error = %v, want stale", view.ModelsStale, err)
	}

	if _, err := h.manager.Disable(ctx, "pi"); err != nil {
		t.Fatalf("Disable(pi) error = %v", err)
	}
	models = []integration.ModelRef{{ID: "relo-d-four", Name: "Four"}}
	view, err = h.manager.Inspect(ctx, "pi")
	if err != nil || view.Enabled || view.ModelsStale {
		t.Fatalf("Inspect(pi) after disable stale = %v enabled = %v error = %v, want off and current", view.ModelsStale, view.Enabled, err)
	}
}

// TestEnableCodexListsAProviderAddedAfterClaude is the flow that hid models:
// Claude is set up, a provider is added, Claude is removed, then Codex is
// set up, and the new provider's models have to be in the catalog Codex reads.
func TestEnableCodexListsAProviderAddedAfterClaude(t *testing.T) {
	h := newHarness(t, dataPlane)
	models := []integration.ModelRef{{ID: "relo-old-one", Name: "Old"}}
	withModels(h, &models)
	ctx := context.Background()

	if err := os.MkdirAll(filepath.Dir(h.paths.ClaudeSettings()), 0o755); err != nil {
		t.Fatalf("create the claude directory: %v", err)
	}
	if err := os.WriteFile(h.paths.ClaudeSettings(), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("seed the settings: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(h.paths.CodexConfig()), 0o755); err != nil {
		t.Fatalf("create the codex directory: %v", err)
	}
	if err := os.WriteFile(h.paths.CodexConfig(), []byte("approval_policy = \"never\"\n"), 0o644); err != nil {
		t.Fatalf("seed the codex config: %v", err)
	}

	if _, err := h.manager.Enable(ctx, "claude-code", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable(claude-code) error = %v", err)
	}
	models = []integration.ModelRef{
		{ID: "relo-old-one", Name: "Old"},
		{ID: "relo-opencode-go-flash", Name: "Flash", Connection: "OpenCode Go"},
	}
	if _, err := h.manager.Restore(ctx, "claude-code"); err != nil {
		t.Fatalf("Restore(claude-code) error = %v", err)
	}
	if _, err := h.manager.Enable(ctx, "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable(codex) error = %v", err)
	}

	catalog := readFile(t, codingclients.CodexCatalogPath(h.paths.CodexConfig()))
	if !strings.Contains(catalog, "relo-opencode-go-flash") || !strings.Contains(catalog, "Routed via Relo") {
		t.Fatalf("catalog = %s, want the new provider's model", catalog)
	}
	cache := readFile(t, filepath.Join(filepath.Dir(h.paths.CodexConfig()), "models_cache.json"))
	if !strings.Contains(cache, "relo-opencode-go-flash") {
		t.Fatalf("cache = %s, want the new provider's model", cache)
	}
}
