package cli_test

import (
	"strings"
	"testing"
)

// TestRunCommandSurface covers the command tree an operator and an installer
// reach for: the app is `daemon run`, and the desktop-only command a previous
// build carried is gone.
func TestRunCommandSurface(t *testing.T) {
	t.Run("daemon run help lists the port and startup log flags", func(t *testing.T) {
		stdout, _, err := run(t, nil, "daemon", "run", "--help")
		if err != nil {
			t.Fatalf("daemon run --help error = %v", err)
		}
		if !strings.Contains(stdout, "--port") {
			t.Fatalf("help = %q, want --port listed", stdout)
		}
	})

	t.Run("the desktop command is gone", func(t *testing.T) {
		// The tray lives in the daemon now, so a desktop subcommand that a
		// previous build carried must not answer at all.
		_, _, err := run(t, nil, "desktop")
		if err == nil {
			t.Fatal("desktop error = nil, want an unknown command")
		}
	})

	t.Run("the uninstall command is in every build, which is what npm runs", func(t *testing.T) {
		// packaging/npm/lib/uninstall.mjs runs `relo uninstall --prepare`
		// through the installed binary.
		stdout, _, err := run(t, nil, "uninstall", "--help")
		if err != nil {
			t.Fatalf("uninstall --help error = %v", err)
		}
		for _, want := range []string{"--keep-data", "--wipe"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("help = %q, want %s", stdout, want)
			}
		}
	})
}
