package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/config"
)

func TestAppearancePreservesOtherConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := "# keep this comment\n[server]\nport = 12345 # custom\n\n[ui]\nlanguage = \"de\"\naccent = \"red\" # keep this too\n\n[logging]\nlevel = \"debug\"\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	theme, accent := "dark", "blue"
	got, err := config.UpdateAppearance(path, config.AppearancePatch{
		Theme: &theme, Accent: &accent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != theme || got.Accent != accent {
		t.Fatalf("appearance = %+v", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"# keep this comment", "port = 12345 # custom", "language = \"de\"", "# keep this too", "level = \"debug\""} {
		if !strings.Contains(string(data), text) {
			t.Fatalf("lost %q in %s", text, data)
		}
	}
	loaded, err := config.LoadOffline(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Server.Port != 12345 || loaded.UI.Theme != theme || loaded.UI.Accent != accent {
		t.Fatalf("config = %+v", loaded)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}

func TestAppearanceDefaultsAndInvalidValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	got, err := config.ReadAppearance(path)
	if err != nil || got.Theme != "system" || got.Accent != "red" {
		t.Fatalf("default = %+v, %v", got, err)
	}
	bad := "magenta"
	if _, err := config.UpdateAppearance(path, config.AppearancePatch{Accent: &bad}); err == nil {
		t.Fatal("accepted invalid accent")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid write created config: %v", err)
	}
	bad = "sepia"
	if _, err := config.UpdateAppearance(path, config.AppearancePatch{Theme: &bad}); err == nil {
		t.Fatal("accepted invalid theme")
	}
}

func TestAppearanceAcceptsCurrentAndLegacyAccentNames(t *testing.T) {
	accents := []string{"red", "orange", "amber", "green", "teal", "cyan", "blue", "indigo", "purple", "rose", "emerald", "violet"}
	for _, accent := range accents {
		if err := config.ValidateAppearance("system", accent); err != nil {
			t.Errorf("accent %q: %v", accent, err)
		}
	}
}
