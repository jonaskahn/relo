package config_test

import (
	"github.com/jonaskahn/relo/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreeConnectionsDefaultsAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if config.ReadFreeConnections(path) {
		t.Fatal("missing file enabled free pools")
	}
	fresh, err := config.LoadConfig(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Connections.Free {
		t.Fatal("fresh home enabled free pools")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "free = false") {
		t.Fatal("fresh file omitted explicit opt-out")
	}
	source := "# keep me\n[server]\nport = 12345\n"
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	legacy, err := config.LoadConfig(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !legacy.Connections.Free {
		t.Fatal("upgrade lost free pools")
	}
	data, _ = os.ReadFile(path)
	if string(data) != source {
		t.Fatal("loading legacy file rewrote it")
	}
	for _, want := range []bool{false, true, false} {
		if err := config.UpdateConnectionsConfig(path, want); err != nil {
			t.Fatal(err)
		}
		if got := config.ReadFreeConnections(path); got != want {
			t.Fatalf("got %v want %v", got, want)
		}
		data, _ = os.ReadFile(path)
		if !strings.Contains(string(data), source) {
			t.Fatal("save lost unrelated settings or comments")
		}
	}
	if err := os.WriteFile(path, []byte("[connections]\nfree = 'yes'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if config.ReadFreeConnections(path) {
		t.Fatal("malformed config enabled pools")
	}
}
