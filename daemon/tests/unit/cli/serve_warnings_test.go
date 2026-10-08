package cli_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/jonaskahn/relo/internal/access"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestServeStartupWarnings covers what a daemon says about the data plane's
// reachability: an install with no client key serves nothing, and a state
// directory still carrying the legacy token file should say so.
func TestServeStartupWarnings(t *testing.T) {
	keyring.MockInitWithError(errors.New("no keychain in tests"))
	t.Cleanup(keyring.MockInit)

	start := func(t *testing.T, home string) string {
		t.Helper()
		port := freePort(t)
		logger, buffer := testkit.TestLogger(t)
		ctx, cancel := context.WithCancel(context.Background())
		serving := make(chan error, 1)
		go func() {
			serving <- platform.Serve(ctx, platform.Options{
				Home: home, Port: port, Logger: logger,
			})
		}()
		waitForHealthz(t, port)
		cancel()
		if err := <-serving; err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
		return buffer.String()
	}

	t.Run("an install with no client key says inference is refused", func(t *testing.T) {
		home := testkit.TempHome(t)
		useFreePorts(t, home)
		logged := start(t, home)
		if !strings.Contains(logged, "no active client key") {
			t.Fatalf("log = %q, want the warning about a keyless install", logged)
		}
		if !strings.Contains(logged, "/keys") {
			t.Fatalf("log = %q, want the console page that fixes it", logged)
		}
	})

	t.Run("a legacy token file is reported as ignored", func(t *testing.T) {
		home := preparedHome(t)
		legacy := filepath.Join(home, access.LegacyTokenFile)
		if err := os.WriteFile(legacy, []byte("legacy-token"), 0o600); err != nil {
			t.Fatalf("write the legacy token: %v", err)
		}
		logged := start(t, home)
		if !strings.Contains(logged, "legacy data-plane token file is ignored") {
			t.Fatalf("log = %q, want the warning about the legacy file", logged)
		}
		if !strings.Contains(logged, access.LegacyTokenFile) {
			t.Fatalf("log = %q, want the file named", logged)
		}
	})
}
