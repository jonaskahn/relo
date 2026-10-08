package cli_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/jonaskahn/relo/internal/adapters/secrets"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	"github.com/jonaskahn/relo/internal/cli"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestExecute(t *testing.T) {
	t.Run("Execute prints a failure and returns it", func(t *testing.T) {
		if err := withArgs(t, "nonsense"); err == nil {
			t.Fatal("Execute() error = nil, want an unknown-command failure")
		}
	})

	t.Run("Execute reports success for a valid command", func(t *testing.T) {
		if err := withArgs(t, "version"); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})
}

func TestStatusFailures(t *testing.T) {
	t.Run("a malformed config stops the report", func(t *testing.T) {
		home := testkit.TempHome(t)
		if err := os.WriteFile(config.ConfigPath(home), []byte("not = = toml"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if _, err := cli.Status(context.Background(), home); err == nil {
			t.Fatal("Status() error = nil, want a parse failure")
		}
	})

	t.Run("an unexpected config path stops the report", func(t *testing.T) {
		home := filepath.Join(string(filepath.Separator), "dev", "null", "relo")
		if _, err := cli.Status(context.Background(), home); err == nil {
			t.Fatal("Status() error = nil, want a stat failure")
		}
	})

	t.Run("an empty admin token counts as not running", func(t *testing.T) {
		home := testkit.TempHome(t)
		path := server.TokenPath(home, server.AdminTokenFile)
		if err := os.WriteFile(path, []byte("   "), 0o600); err != nil {
			t.Fatalf("write token: %v", err)
		}
		if _, err := cli.Status(context.Background(), home); !errors.Is(err, cli.ErrNotRunning) {
			t.Fatalf("Status() error = %v, want %v", err, cli.ErrNotRunning)
		}
	})

	t.Run("an undecodable report is reported", func(t *testing.T) {
		home := testkit.TempHome(t)
		management := fakeManagementStatus(t, home, http.StatusOK, "{not json")
		writePort(t, home, portOf(t, management.URL))
		if _, err := cli.Status(context.Background(), home); err == nil {
			t.Fatal("Status() error = nil, want a decode failure")
		}
	})

	t.Run("short uptimes are formatted readably", func(t *testing.T) {
		home := testkit.TempHome(t)
		payload := statusPayload()
		payload["uptime_seconds"] = 90
		management := fakeManagement(t, home, payload)
		t.Setenv(config.ReloHomeEnv, home)
		writePort(t, home, portOf(t, management.URL))
		stdout, _, err := run(t, nil, "daemon", "status")
		if err != nil {
			t.Fatalf("status error = %v", err)
		}
		if !strings.Contains(stdout, "1m") {
			t.Fatalf("stdout = %q, want a minute-granularity uptime", stdout)
		}

		payload["uptime_seconds"] = 5
		management = fakeManagement(t, home, payload)
		writePort(t, home, portOf(t, management.URL))
		stdout, _, err = run(t, nil, "daemon", "status")
		if err != nil {
			t.Fatalf("status error = %v", err)
		}
		if !strings.Contains(stdout, "5s") {
			t.Fatalf("stdout = %q, want a second-granularity uptime", stdout)
		}
	})
}

func TestServeFailures(t *testing.T) {
	t.Run("serve reports a corrupt database", func(t *testing.T) {
		home := testkit.TempHome(t)
		useFreePorts(t, home)
		if err := os.WriteFile(config.DatabasePath(home), []byte("not a database"), 0o600); err != nil {
			t.Fatalf("write a corrupt database: %v", err)
		}
		if err := platform.Serve(context.Background(), platform.Options{Home: home}); err == nil {
			t.Fatal("Serve() error = nil, want a database failure")
		}
	})

	t.Run("serve reports a database that cannot migrate", func(t *testing.T) {
		home := testkit.TempHome(t)
		useFreePorts(t, home)
		logger, _ := testkit.TestLogger(t)
		db, err := sqlite.OpenDB(config.DatabasePath(home), logger)
		if err != nil {
			t.Fatalf("OpenDB() error = %v", err)
		}
		if _, err := db.SQL().Exec("CREATE TABLE schema_migrations (version TEXT)"); err != nil {
			t.Fatalf("create a foreign migration table: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		if err := platform.Serve(context.Background(), platform.Options{Home: home}); err == nil {
			t.Fatal("Serve() error = nil, want a migration failure")
		}
	})

	t.Run("serve reports a broken master key", func(t *testing.T) {
		home := testkit.TempHome(t)
		useFreePorts(t, home)
		keyring.MockInitWithError(errors.New("no keychain in tests"))
		t.Cleanup(keyring.MockInit)
		corruptSecretKey(t, home)
		err := platform.Serve(context.Background(), platform.Options{Home: home})
		if !errors.Is(err, secrets.ErrInvalidKey) {
			t.Fatalf("Serve() error = %v, want %v", err, secrets.ErrInvalidKey)
		}
	})

	t.Run("serve reports a token path it cannot write", func(t *testing.T) {
		home := testkit.TempHome(t)
		useFreePorts(t, home)
		if err := os.Mkdir(server.TokenPath(home, server.AdminTokenFile), 0o700); err != nil {
			t.Fatalf("create a directory in place of the token: %v", err)
		}
		if err := platform.Serve(context.Background(), platform.Options{Home: home}); err == nil {
			t.Fatal("Serve() error = nil, want a token failure")
		}
	})

	t.Run("serve reports a malformed config", func(t *testing.T) {
		home := testkit.TempHome(t)
		if err := os.WriteFile(config.ConfigPath(home), []byte("not = = toml"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if err := platform.Serve(context.Background(), platform.Options{Home: home}); err == nil {
			t.Fatal("Serve() error = nil, want a parse failure")
		}
	})

	t.Run("serve reports a port the configuration cannot use", func(t *testing.T) {
		home := testkit.TempHome(t)
		writePort(t, home, "70000")
		err := platform.Serve(context.Background(), platform.Options{Home: home})
		if !errors.Is(err, config.ErrInvalidPort) {
			t.Fatalf("Serve() error = %v, want %v", err, config.ErrInvalidPort)
		}
	})

	t.Run("serve loads the credentials already stored", func(t *testing.T) {
		home := preparedHome(t)
		keyring.MockInitWithError(errors.New("no keychain in tests"))
		t.Cleanup(keyring.MockInit)
		ctx := context.Background()
		session, err := platform.OpenManagement(ctx, home, nil)
		if err != nil {
			t.Fatalf("OpenManagement() error = %v", err)
		}
		if _, err := session.Accounts().AddAccount(ctx, appaccount.NewAccount{
			ProviderID: "openai", Kind: "api_key", Label: "default", SecretValue: "sk-test",
		}); err != nil {
			t.Fatalf("AddAccount() error = %v", err)
		}
		if err := session.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		stopped, cancel := context.WithCancel(context.Background())
		cancel()
		if err := platform.Serve(stopped, platform.Options{Home: home, Port: freePort(t)}); err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	})

	t.Run("the serve command exits when its context is cancelled", func(t *testing.T) {
		home := testkit.TempHome(t)
		useFreePorts(t, home)
		keyring.MockInitWithError(errors.New("no keychain in tests"))
		t.Cleanup(keyring.MockInit)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		root, err := cli.NewRootCommand("en")
		if err != nil {
			t.Fatalf("NewRootCommand() error = %v", err)
		}
		root.SetContext(ctx)
		root.SetOut(&strings.Builder{})
		root.SetErr(&strings.Builder{})
		root.SetArgs([]string{"--home", home, "daemon", "run"})
		if err := root.Execute(); err != nil {
			t.Fatalf("serve error = %v", err)
		}
	})
}

// withArgs runs Execute with a temporary process argument list.
func withArgs(t *testing.T, args ...string) error {
	t.Helper()
	original := os.Args
	os.Args = append([]string{"relo"}, args...)
	t.Cleanup(func() { os.Args = original })
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer func() { _ = devNull.Close() }()
	originalStderr := os.Stderr
	os.Stderr = devNull
	t.Cleanup(func() { os.Stderr = originalStderr })
	return cli.Execute()
}
