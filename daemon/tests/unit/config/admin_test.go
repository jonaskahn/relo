package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/config"
)

func TestAdminAllowExternalRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	got, err := config.ReadAdminAllowExternal(path)
	if err != nil || got {
		t.Fatalf("ReadAdminAllowExternal() = %v, %v, want false, nil", got, err)
	}
	if err := config.UpdateAdminAllowExternal(path, true); err != nil {
		t.Fatal(err)
	}
	if got, err = config.ReadAdminAllowExternal(path); err != nil || !got {
		t.Fatalf("ReadAdminAllowExternal() = %v, %v, want true, nil", got, err)
	}
	if err := config.UpdateAdminAllowExternal(path, false); err != nil {
		t.Fatal(err)
	}
	if got, err = config.ReadAdminAllowExternal(path); err != nil || got {
		t.Fatalf("ReadAdminAllowExternal() = %v, %v, want false, nil", got, err)
	}
}

func TestUpdateAdminAllowExternalKeepsTheRestOfTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	source := "[server]\nbind = \"127.0.0.1\"\n\n[admin]\nlogin = false\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateAdminAllowExternal(path, true); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, kept := range []string{"[server]", "bind = \"127.0.0.1\"", "[admin]", "login = false"} {
		if !strings.Contains(text, kept) {
			t.Fatalf("update dropped %q:\n%s", kept, text)
		}
	}
	if count := strings.Count(text, "allow_external"); count != 1 {
		t.Fatalf("allow_external appears %d times, want once:\n%s", count, text)
	}

	// A second update rewrites the key rather than adding another.
	if err := config.UpdateAdminAllowExternal(path, false); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(content), "allow_external"); count != 1 {
		t.Fatalf("allow_external appears %d times after a rewrite, want once:\n%s", count, content)
	}
	if !strings.Contains(string(content), "allow_external = false") {
		t.Fatalf("rewrite did not store false:\n%s", content)
	}
}

func TestUpdateAdminAllowExternalRefusesAnUnreachableConsole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	source := "[server]\nbind = \"0.0.0.0\"\n\n[admin]\nlogin = false\nallow_external = true\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.ValidateAdminChange(path, false); err == nil {
		t.Fatal("turning external access off was accepted outside loopback")
	}
	if err := config.UpdateAdminAllowExternal(path, false); err == nil {
		t.Fatal("turning external access off was stored outside loopback")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "allow_external = true") {
		t.Fatalf("a refused update changed the file:\n%s", content)
	}

	// With the sign-in on, the same bind may keep external access off.
	source = "[server]\nbind = \"0.0.0.0\"\n\n[admin]\nlogin = true\nallow_external = true\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateAdminAllowExternal(path, false); err != nil {
		t.Fatalf("UpdateAdminAllowExternal() error = %v", err)
	}
}
