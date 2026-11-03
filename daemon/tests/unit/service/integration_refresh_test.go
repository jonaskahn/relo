package service_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/platform"
)

// ageFile rewrites a file Relo wrote the way an older build would have, and
// stores the digest of that content, so the file reads as outdated and not
// as edited.
func ageFile(t *testing.T, h *harness, id, kind, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	aged := strings.Replace(string(content), "10101", "10999", 1)
	if aged == string(content) {
		t.Fatalf("%s holds no data plane address to age", path)
	}
	if err := os.WriteFile(path, []byte(aged), 0o644); err != nil {
		t.Fatalf("age %s: %v", path, err)
	}
	store := platform.NewIntegrationStore(h.db)
	files, err := store.Files(context.Background(), id)
	if err != nil {
		t.Fatalf("Files() error = %v", err)
	}
	for _, file := range files {
		if file.Kind == kind {
			file.Digest = platform.NewClientFiles(codingclients.Paths{}).Digest(aged)
			if err := store.SaveFile(context.Background(), file); err != nil {
				t.Fatalf("SaveFile() error = %v", err)
			}
		}
	}
}

// TestRefreshOutdatedFilesNeedsAStoredKey keeps the refresh from minting a
// key: without one the agent is skipped, and with one the file follows.
func TestRefreshOutdatedFilesNeedsAStoredKey(t *testing.T) {
	h := newHarness(t)
	settings := &config.Config{}
	settings.Server.Bind = "127.0.0.1"
	settings.Server.Port = 10101
	home := t.TempDir()
	built := integrationUseCases(t, h, settings, home)
	ctx := context.Background()
	if _, err := built.EnableIntegration(ctx, "pi", ""); err != nil {
		t.Fatalf("EnableIntegration(pi) error = %v", err)
	}
	path := filepath.Join(home, ".pi", "agent", "models.json")
	ageFile(t, h, "pi", codingclients.FilePi, path)
	aged, _ := os.ReadFile(path)
	keyFile := filepath.Join(h.home, "integrations", "pi", "key")
	key, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatalf("read the key: %v", err)
	}
	if err := os.WriteFile(keyFile, nil, 0o600); err != nil {
		t.Fatalf("empty the key: %v", err)
	}
	keysBefore, _ := h.keys.AccessKeys(ctx)

	built.RefreshOutdatedFiles(ctx)
	if now, _ := os.ReadFile(path); string(now) != string(aged) {
		t.Fatal("the refresh wrote a file for an agent with no key")
	}
	if keysAfter, _ := h.keys.AccessKeys(ctx); len(keysAfter) != len(keysBefore) {
		t.Fatal("the refresh minted a key")
	}

	if err := os.WriteFile(keyFile, key, 0o600); err != nil {
		t.Fatalf("restore the key: %v", err)
	}
	built.RefreshOutdatedFiles(ctx)
	if now, _ := os.ReadFile(path); !strings.Contains(string(now), "10101") {
		t.Fatalf("catalog = %s, want it refreshed", now)
	}
}

// TestRefreshOutdatedFilesNeverTouchesTheClientProcess keeps a silent refresh
// from restarting OpenCode: the launcher is never run.
func TestRefreshOutdatedFilesNeverTouchesTheClientProcess(t *testing.T) {
	h := newHarness(t)
	settings := &config.Config{}
	settings.Server.Bind = "127.0.0.1"
	settings.Server.Port = 10101
	home := t.TempDir()
	built := integrationUseCases(t, h, settings, home)
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	if err := os.MkdirAll(filepath.Join(configHome, "opencode"), 0o755); err != nil {
		t.Fatalf("create the opencode directory: %v", err)
	}
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + marker + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatalf("write the stub launcher: %v", err)
	}
	t.Setenv("PATH", binDir)
	ctx := context.Background()
	if _, err := built.EnableIntegration(ctx, "opencode", ""); err != nil {
		t.Fatalf("EnableIntegration(opencode) error = %v", err)
	}
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		t.Fatalf("clear the marker: %v", err)
	}
	path := filepath.Join(configHome, "opencode", "opencode.json")
	ageFile(t, h, "opencode", codingclients.FileOpenCode, path)

	built.RefreshOutdatedFiles(ctx)
	if now, _ := os.ReadFile(path); !strings.Contains(string(now), "10101") {
		t.Fatalf("config = %s, want it refreshed", now)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("launcher ran during a refresh (stat error = %v), want no process call", err)
	}
}
