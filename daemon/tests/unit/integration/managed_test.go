package integration_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/application/integration"
	"github.com/jonaskahn/relo/internal/platform"
)

func TestEnableOpenCodeWritesTheProviderAndDisableTakesItBack(t *testing.T) {
	h := newHarness(t, dataPlane)
	config := filepath.Join(h.paths.Home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatalf("create the opencode directory: %v", err)
	}
	original := "{\"theme\":\"dark\"}\n"
	if err := os.WriteFile(config, []byte(original), 0o644); err != nil {
		t.Fatalf("seed the opencode config: %v", err)
	}
	window := int64(1000)
	h.manager = integration.New(integration.Options{
		Paths: platform.NewPathResolver(h.paths), Agents: platform.NewAgentRegistry(),
		Files: h.files, Store: h.store, Now: func() time.Time { return time.Unix(1_700_000_000, 0) },
		Models: func() []integration.ModelRef {
			return []integration.ModelRef{{ID: "relo-openai-gpt-4o", Name: "GPT-4o", ContextWindow: &window}}
		},
	})

	view, err := h.manager.Enable(context.Background(), "opencode", "rlo_ak_secret")
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if view.State != integration.StateOn {
		t.Fatalf("state = %q, want on", view.State)
	}
	merged := readFile(t, config)
	if strings.Contains(merged, "rlo_ak_secret") {
		t.Fatalf("config carries the secret:\n%s", merged)
	}
	if !strings.Contains(merged, "{env:RELO_OPENCODE_API_KEY}") || !strings.Contains(merged, "\"theme\":\"dark\"") {
		t.Fatalf("config = %s, want the provider and the operator's theme", merged)
	}
	if !strings.Contains(merged, "\"context\":1000") || strings.Contains(merged, "\"output\"") {
		t.Fatalf("config = %s, want only the known context window", merged)
	}

	if _, err := h.manager.Disable(context.Background(), "opencode"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if got := readFile(t, config); strings.Contains(got, "relo") || !strings.Contains(got, "\"theme\":\"dark\"") {
		t.Fatalf("config after disable = %s, want Relo gone and the theme kept", got)
	}

	if _, err := h.manager.Enable(context.Background(), "opencode", "rlo_ak_secret"); err != nil {
		t.Fatalf("Enable() again error = %v", err)
	}
	if _, err := h.manager.Restore(context.Background(), "opencode"); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if got := readFile(t, config); got != original {
		t.Fatalf("config after restore = %q, want the original %q", got, original)
	}
}

func TestEnableOpenCodeRefusesAMissingInstallAndAForeignProvider(t *testing.T) {
	h := newHarness(t, dataPlane)
	planned, err := h.manager.Preview(context.Background(), "opencode")
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	refusal := plannedFile(t, planned, codingclients.FileOpenCode)
	if !refusal.Refused || refusal.Code != integration.CodeNotInstalled {
		t.Fatalf("preview = %+v, want not installed", refusal)
	}
	if _, err := h.manager.Enable(context.Background(), "opencode", "rlo_ak_secret"); integration.RefusalCode(err) != integration.CodeNotInstalled {
		t.Fatalf("Enable() error = %v, want not installed", err)
	}
	if _, err := os.Stat(filepath.Join(h.paths.Home, ".config", "opencode")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a missing install was created: %v", err)
	}

	config := filepath.Join(h.paths.Home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatalf("create the opencode directory: %v", err)
	}
	if err := os.WriteFile(config, []byte("{\"theme\":\"dark\",\"provider\":{\"relo\":{\"name\":\"mine\"}}}\n"), 0o644); err != nil {
		t.Fatalf("seed a foreign provider: %v", err)
	}
	planned, err = h.manager.Preview(context.Background(), "opencode")
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	refusal = plannedFile(t, planned, codingclients.FileOpenCode)
	if !refusal.Refused || refusal.Code != integration.CodeForeignKey {
		t.Fatalf("preview = %+v, want a foreign provider", refusal)
	}
	if _, err := h.manager.Enable(context.Background(), "opencode", "rlo_ak_secret"); integration.RefusalCode(err) != integration.CodeForeignKey {
		t.Fatalf("Enable() error = %v, want a foreign provider", err)
	}
	if _, err := h.manager.EnableOverwriting(context.Background(), "opencode", "rlo_ak_secret"); err != nil {
		t.Fatalf("EnableOverwriting() error = %v", err)
	}
	merged := readFile(t, config)
	if !strings.Contains(merged, "\"theme\":\"dark\"") || !strings.Contains(merged, "{env:RELO_OPENCODE_API_KEY}") {
		t.Fatalf("config after overwrite = %s, want the theme kept and Relo's provider in place", merged)
	}
}

func TestEnableWritesAPerClientEnvBlock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("login-shell blocks are Unix")
	}
	h := newHarness(t, dataPlane)
	openCode := filepath.Join(h.paths.Home, ".config", "opencode", "opencode.json")
	hermes := filepath.Join(h.paths.Home, ".hermes", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(openCode), 0o755); err != nil {
		t.Fatalf("create the opencode directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(hermes), 0o755); err != nil {
		t.Fatalf("create the hermes directory: %v", err)
	}
	if _, err := h.manager.Enable(context.Background(), "opencode", "rlo_ak_open"); err != nil {
		t.Fatalf("Enable(opencode) error = %v", err)
	}
	if _, err := h.manager.Enable(context.Background(), "hermes", "rlo_ak_hermes"); err != nil {
		t.Fatalf("Enable(hermes) error = %v", err)
	}
	for _, name := range []string{".zshenv", ".zprofile", ".bashrc", ".bash_profile", ".profile"} {
		env := readFile(t, filepath.Join(h.paths.Home, name))
		if !strings.Contains(env, "RELO_OPENCODE_API_KEY") || !strings.Contains(env, "RELO_HERMES_API_KEY") {
			t.Fatalf("%s = %s, want both variables", name, env)
		}
		if strings.Contains(env, "rlo_ak_open") || strings.Contains(env, "rlo_ak_hermes") {
			t.Fatalf("%s carries a secret:\n%s", name, env)
		}
	}
	fish := readFile(t, filepath.Join(h.paths.Home, ".config", "fish", "conf.d", "relo.fish"))
	if !strings.Contains(fish, "RELO_OPENCODE_API_KEY") || !strings.Contains(fish, "RELO_HERMES_API_KEY") {
		t.Fatalf("relo.fish = %s, want both variables", fish)
	}
	if !strings.Contains(fish, "set -gx") || strings.Contains(fish, "rlo_ak_open") {
		t.Fatalf("relo.fish = %s, want fish syntax without a secret", fish)
	}
	if _, err := h.manager.Disable(context.Background(), "opencode"); err != nil {
		t.Fatalf("Disable(opencode) error = %v", err)
	}
	env := readFile(t, filepath.Join(h.paths.Home, ".zshenv"))
	if strings.Contains(env, "RELO_OPENCODE_API_KEY") || !strings.Contains(env, "RELO_HERMES_API_KEY") {
		t.Fatalf("zshenv after disable = %s, want Hermes kept", env)
	}
	if _, err := h.manager.Disable(context.Background(), "hermes"); err != nil {
		t.Fatalf("Disable(hermes) error = %v", err)
	}
	for _, name := range []string{".zshenv", ".zprofile", ".bashrc", ".bash_profile", ".profile", filepath.Join(".config", "fish", "conf.d", "relo.fish")} {
		if _, err := os.Stat(filepath.Join(h.paths.Home, name)); !os.IsNotExist(err) {
			t.Fatalf("%s after both gone = %v, want removed", name, err)
		}
	}
}

func TestEnablePublishesAnEnvKeyForAClientThatOnlyReadsTheEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("login-shell blocks are Unix")
	}
	h := newHarness(t, dataPlane)
	if _, err := h.manager.Enable(context.Background(), "cursor", "rlo_ak_cursor"); err != nil {
		t.Fatalf("Enable(cursor) error = %v", err)
	}
	if _, err := h.manager.Enable(context.Background(), "claude-code", "rlo_ak_claude"); err != nil {
		t.Fatalf("Enable(claude-code) error = %v", err)
	}
	env := readFile(t, filepath.Join(h.paths.Home, ".zshenv"))
	if !strings.Contains(env, "RELO_CURSOR_API_KEY") || !strings.Contains(env, "RELO_CLAUDE_CODE_API_KEY") {
		t.Fatalf("zshenv = %s, want both variables", env)
	}
	if !strings.Contains(env, h.paths.KeyFile("cursor")) || !strings.Contains(env, h.paths.KeyFile("claude-code")) {
		t.Fatalf("zshenv = %s, want both key files", env)
	}
	if strings.Contains(env, "rlo_ak_cursor") || strings.Contains(env, "rlo_ak_claude") {
		t.Fatalf("zshenv carries a secret:\n%s", env)
	}
	if _, err := h.manager.Disable(context.Background(), "cursor"); err != nil {
		t.Fatalf("Disable(cursor) error = %v", err)
	}
	env = readFile(t, filepath.Join(h.paths.Home, ".zshenv"))
	if strings.Contains(env, "RELO_CURSOR_API_KEY") || !strings.Contains(env, "RELO_CLAUDE_CODE_API_KEY") {
		t.Fatalf("zshenv after disable = %s, want Claude Code kept", env)
	}
	if _, err := h.manager.Disable(context.Background(), "claude-code"); err != nil {
		t.Fatalf("Disable(claude-code) error = %v", err)
	}
	for _, name := range []string{".zshenv", ".zprofile", ".bashrc", ".bash_profile", ".profile", filepath.Join(".config", "fish", "conf.d", "relo.fish")} {
		if _, err := os.Stat(filepath.Join(h.paths.Home, name)); !os.IsNotExist(err) {
			t.Fatalf("%s after both gone = %v, want removed", name, err)
		}
	}
}

func TestEnableOpenCodeRefusesARelativePath(t *testing.T) {
	h := newHarness(t, dataPlane)
	t.Setenv("XDG_CONFIG_HOME", "relative/config")
	planned, err := h.manager.Preview(context.Background(), "opencode")
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	refusal := plannedFile(t, planned, codingclients.FileOpenCode)
	if !refusal.Refused || refusal.Code != integration.CodeRelativePath {
		t.Fatalf("preview = %+v, want a relative path refused", refusal)
	}
	if _, err := h.manager.Enable(context.Background(), "opencode", "rlo_ak_secret"); integration.RefusalCode(err) != integration.CodeRelativePath {
		t.Fatalf("Enable() error = %v, want a relative path", err)
	}
}

func plannedFile(t *testing.T, planned []integration.PlannedFile, kind string) integration.PlannedFile {
	t.Helper()
	for _, file := range planned {
		if file.Kind == kind {
			return file
		}
	}
	t.Fatalf("planned files = %+v, want %s", planned, kind)
	return integration.PlannedFile{}
}

func TestEnableClaudeDesktopNeedsOnlyDesktopsOwnDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Desktop directory is resolved from APPDATA on Windows")
	}
	h := newHarness(t, dataPlane)
	t.Setenv("CLAUDE_USER_DATA_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	planned, err := h.manager.Preview(context.Background(), "claude-desktop")
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if refusal := plannedFile(t, planned, integration.KindClaudeDesktop); !refusal.Refused || refusal.Code != integration.CodeNotInstalled {
		t.Fatalf("preview = %+v, want not installed", refusal)
	}

	support := filepath.Join(h.paths.Home, ".config")
	if runtime.GOOS == "darwin" {
		support = filepath.Join(h.paths.Home, "Library", "Application Support")
	}
	if err := os.MkdirAll(filepath.Join(support, "Claude"), 0o755); err != nil {
		t.Fatalf("create the Desktop directory: %v", err)
	}
	planned, err = h.manager.Preview(context.Background(), "claude-desktop")
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if file := plannedFile(t, planned, integration.KindClaudeDesktop); file.Refused {
		t.Fatalf("preview = %+v, want a plan for an installed Desktop", file)
	}

	if _, err := h.manager.Enable(context.Background(), "claude-desktop", "rlo_ak_secret"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	library := filepath.Join(support, "Claude-3p", "configLibrary")
	profile := codingclients.DesktopProfilePath(library)
	if got := readFile(t, profile); !strings.Contains(got, "inferenceGatewayBaseUrl") {
		t.Fatalf("%s = %s, want the gateway profile", profile, got)
	}
	id := strings.TrimSuffix(filepath.Base(profile), ".json")
	if got := readFile(t, filepath.Join(library, "_meta.json")); !strings.Contains(got, "\"appliedId\": \""+id+"\"") {
		t.Fatalf("_meta.json = %s, want Relo's profile %s selected", got, id)
	}
}
