package service_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	appintegration "github.com/jonaskahn/relo/internal/application/integration"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/platform"
)

// errInjected is the refusal a test port hands back, so the assertion names a
// failure this test caused rather than anything the machine produced.
var errInjected = errors.New("injected refusal")

// failingFiles refuses to read the key file. A repair reads that file to find
// the key it reuses, so a refusal there is the one way the repair never
// reaches the files it would rewrite.
type failingFiles struct {
	appintegration.ClientFiles
}

func (f failingFiles) ReadFileOrEmpty(string) (string, error) {
	return "", errInjected
}

// failingStore refuses to list the files of one integration. A repair reads
// that list before it writes and again after, so arming it on the n-th read
// picks which of those reads refuses.
type failingStore struct {
	appintegration.Store
	armed bool
	reads int
	fail  int
}

func (s *failingStore) Files(ctx context.Context, id string) ([]appintegration.FileRecord, error) {
	if s.armed {
		s.reads++
		if s.reads == s.fail {
			return nil, errInjected
		}
	}
	return s.Store.Files(ctx, id)
}

// TestRepairIntegrationReportsTheReadThatRefusedIt covers a repair whose key
// file cannot be read: the key file holds the secret a repair reuses, so a
// refusal there returns the read's own error instead of a half-written client.
func TestRepairIntegrationReportsTheReadThatRefusedIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	h := newHarness(t)
	settings := &config.Config{}
	settings.Server.Bind = "127.0.0.1"
	settings.Server.Port = 10101
	built := integrationUseCasesOver(t, h, settings, t.TempDir(), failingFiles{}, nil)

	if _, err := built.RepairIntegration(context.Background(), "codex"); !errors.Is(err, errInjected) {
		t.Fatalf("RepairIntegration() error = %v, want the refusal to read the key", err)
	}
}

// TestRepairIntegrationReportsTheStoreThatRefusedIt walks every store read a
// codex repair makes. Each read is one step of the repair that can refuse, and
// every one of them has to carry its own error back rather than report a
// success the repair never finished.
func TestRepairIntegrationReportsTheStoreThatRefusedIt(t *testing.T) {
	for read := 1; read <= 5; read++ {
		t.Run(fmt.Sprintf("read %d", read), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
			h := newHarness(t)
			settings := &config.Config{}
			settings.Server.Bind = "127.0.0.1"
			settings.Server.Port = 10101
			store := &failingStore{Store: platform.NewIntegrationStore(h.db), fail: read}
			built := integrationUseCasesOver(t, h, settings, t.TempDir(), nil, store)
			ctx := context.Background()

			if _, err := built.EnableIntegration(ctx, "codex", ""); err != nil {
				t.Fatalf("EnableIntegration() error = %v", err)
			}
			store.armed = true

			if _, err := built.RepairIntegration(ctx, "codex"); !errors.Is(err, errInjected) {
				t.Fatalf("RepairIntegration() error = %v, want the refusal on read %d", err, read)
			}
		})
	}
}

// TestRepairIntegrationKeepsTheFilesWhenTheStoreRefuses proves the refusal
// leaves the machine alone rather than removing what an earlier setup wrote.
func TestRepairIntegrationKeepsTheFilesWhenTheStoreRefuses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	h := newHarness(t)
	settings := &config.Config{}
	settings.Server.Bind = "127.0.0.1"
	settings.Server.Port = 10101
	store := &failingStore{Store: platform.NewIntegrationStore(h.db), fail: 1}
	built := integrationUseCasesOver(t, h, settings, t.TempDir(), nil, store)
	ctx := context.Background()

	if _, err := built.EnableIntegration(ctx, "codex", ""); err != nil {
		t.Fatalf("EnableIntegration() error = %v", err)
	}
	configPath := filepath.Join(home, ".codex", "config.toml")
	written, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read the codex config: %v", err)
	}
	store.armed = true

	if _, err := built.RepairIntegration(ctx, "codex"); !errors.Is(err, errInjected) {
		t.Fatalf("RepairIntegration() error = %v, want the refusal", err)
	}
	kept, err := os.ReadFile(configPath)
	if err != nil || string(kept) != string(written) {
		t.Fatalf("the refused repair changed the config: %s, error %v", kept, err)
	}
}
