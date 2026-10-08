package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/config"
)

func TestUpdateSystemConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	source := "[server]\nbind = \"127.0.0.1\"\n\n[ui]\ntheme = \"dark\"\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	autostart := false
	level := "debug"
	updatesURL := "https://example.test/appcast.xml"
	download := "https://example.test/releases"
	if err := config.UpdateSystemConfig(path, config.SystemConfigPatch{
		Autostart: &autostart, LogLevel: &level,
		UpdatesURL: &updatesURL, UpdatesDownload: &download,
	}); err != nil {
		t.Fatal(err)
	}
	file, err := config.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.System.Autostart {
		t.Fatal("autostart = true, want the stored false")
	}
	if file.System.Logging.Level != "debug" {
		t.Fatalf("log level = %q, want debug", file.System.Logging.Level)
	}
	if file.System.Updates.URL != updatesURL || file.System.Updates.Download != download {
		t.Fatalf("updates = %+v, want the stored values", file.System.Updates)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, kept := range []string{"[system]", "[system.logging]", "[system.updates]", "[server]", "theme = \"dark\""} {
		if !strings.Contains(text, kept) {
			t.Fatalf("update dropped %q:\n%s", kept, text)
		}
	}

	// A second update rewrites the keys rather than adding another.
	level = "warn"
	if err := config.UpdateSystemConfig(path, config.SystemConfigPatch{LogLevel: &level}); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text = string(content)
	if count := strings.Count(text, "autostart"); count != 1 {
		t.Fatalf("autostart appears %d times after a rewrite, want once:\n%s", count, text)
	}
	if count := strings.Count(text, "level"); count != 1 {
		t.Fatalf("level appears %d times after a rewrite, want once:\n%s", count, text)
	}
	if !strings.Contains(text, "level = \"warn\"") {
		t.Fatalf("rewrite did not store warn:\n%s", text)
	}
}

func TestUpdateSystemConfigRefusesInvalidValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	badLevel := "trace"
	if err := config.UpdateSystemConfig(path, config.SystemConfigPatch{LogLevel: &badLevel}); !errors.Is(err, config.ErrInvalidLogLevel) {
		t.Fatalf("UpdateSystemConfig() error = %v, want ErrInvalidLogLevel", err)
	}
	badURL := "not a url"
	if err := config.UpdateSystemConfig(path, config.SystemConfigPatch{UpdatesURL: &badURL}); !errors.Is(err, config.ErrInvalidURL) {
		t.Fatalf("UpdateSystemConfig() error = %v, want ErrInvalidURL", err)
	}
	if err := config.UpdateSystemConfig(path, config.SystemConfigPatch{UpdatesDownload: &badURL}); !errors.Is(err, config.ErrInvalidURL) {
		t.Fatalf("UpdateSystemConfig() error = %v, want ErrInvalidURL", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused update wrote the file")
	}

	// An empty update feed turns the check off and is accepted.
	empty := ""
	if err := config.UpdateSystemConfig(path, config.SystemConfigPatch{UpdatesURL: &empty}); err != nil {
		t.Fatalf("UpdateSystemConfig() error = %v", err)
	}
}

func TestUpdateServerConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	source := "[admin]\nlogin = true\n\n[upstream]\ntimeout_seconds = 180\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	bind := "127.0.0.1"
	port := 12000
	openAI := 12001
	anthropic := 12002
	gemini := 0
	if err := config.UpdateServerConfig(path, config.ServerConfigPatch{
		Bind: &bind, Port: &port, OpenAIPort: &openAI, AnthropicPort: &anthropic, GeminiPort: &gemini,
	}); err != nil {
		t.Fatal(err)
	}
	file, err := config.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.Server.Bind != bind || file.Server.Port != port {
		t.Fatalf("server = %+v, want %s:%d", file.Server, bind, port)
	}
	if file.Server.DataPlane.OpenAI != openAI || file.Server.DataPlane.Anthropic != anthropic || file.Server.DataPlane.Gemini != 0 {
		t.Fatalf("data plane = %+v, want the stored ports", file.Server.DataPlane)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, kept := range []string{"[server]", "[server.data_plane]", "[admin]", "login = true", "timeout_seconds = 180"} {
		if !strings.Contains(text, kept) {
			t.Fatalf("update dropped %q:\n%s", kept, text)
		}
	}
}

func TestUpdateServerConfigRefusesImpossibleListeners(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	source := "[admin]\nlogin = true\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	badBind := "not-an-ip"
	if err := config.UpdateServerConfig(path, config.ServerConfigPatch{Bind: &badBind}); !errors.Is(err, config.ErrInvalidBind) {
		t.Fatalf("UpdateServerConfig() error = %v, want ErrInvalidBind", err)
	}
	badPort := 70000
	if err := config.UpdateServerConfig(path, config.ServerConfigPatch{Port: &badPort}); !errors.Is(err, config.ErrInvalidPort) {
		t.Fatalf("UpdateServerConfig() error = %v, want ErrInvalidPort", err)
	}
	first, second := 12001, 12001
	if err := config.UpdateServerConfig(path, config.ServerConfigPatch{Port: &first, OpenAIPort: &second}); !errors.Is(err, config.ErrDuplicatePort) {
		t.Fatalf("UpdateServerConfig() error = %v, want ErrDuplicatePort", err)
	}

	// A non-loopback bind needs the sign-in or external access on.
	source = "[admin]\nlogin = false\nallow_external = false\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	external := "0.0.0.0"
	if err := config.UpdateServerConfig(path, config.ServerConfigPatch{Bind: &external}); !errors.Is(err, config.ErrLoginOffOutsideLoopback) {
		t.Fatalf("UpdateServerConfig() error = %v, want ErrLoginOffOutsideLoopback", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "0.0.0.0") {
		t.Fatalf("a refused update changed the file:\n%s", content)
	}
}

func TestUpdateCatalogURLRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[ui]\nlanguage = \"auto\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateCatalogURL(path, "https://models.example.test/api.json"); err != nil {
		t.Fatal(err)
	}
	file, err := config.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.Catalog.ModelsDevURL != "https://models.example.test/api.json" {
		t.Fatalf("modelsdev url = %q, want the stored value", file.Catalog.ModelsDevURL)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "[catalog]") || !strings.Contains(string(content), "language = \"auto\"") {
		t.Fatalf("update dropped the rest of the file:\n%s", content)
	}
	if err := config.UpdateCatalogURL(path, ""); !errors.Is(err, config.ErrInvalidURL) {
		t.Fatalf("UpdateCatalogURL() error = %v, want ErrInvalidURL", err)
	}
	if err := config.UpdateCatalogURL(path, "ftp://example.test"); !errors.Is(err, config.ErrInvalidURL) {
		t.Fatalf("UpdateCatalogURL() error = %v, want ErrInvalidURL", err)
	}
}
