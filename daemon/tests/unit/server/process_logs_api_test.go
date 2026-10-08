package server_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	appstatus "github.com/jonaskahn/relo/internal/application/status"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

func processLogServer(t *testing.T, harness *harness, home string) *server.Server {
	t.Helper()
	status := appstatus.New(appstatus.Options{
		DB: harness.db,
		Retention: sqlite.NewRetention(harness.db, sqlite.RetentionOptions{
			Logger: harness.logger, Now: harness.clock.Now,
		}),
		Secrets: platform.NewSecretView(testkit.SecretStore(harness.secrets)), Entries: sqlite.NewCredentialStore(harness.db),
		Quotas: platform.NewQuotaStore(harness.db), Keys: harness.keys,
		Accounts: harness.accounts, Catalog: harness.catalog, Home: home,
	})
	return harness.newServerWithOptions(harness.cfg, nil, func(options *server.Options) {
		options.Status = status
	})
}

func writeLogFile(t *testing.T, home, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, "logs"), 0o700); err != nil {
		t.Fatalf("create the logs directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "logs", name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestDaemonLogsEndpoint(t *testing.T) {
	harness := newHarness(t)
	home := t.TempDir()
	writeLogFile(t, home, "daemon.log",
		`{"level":"info","msg":"http request","method":"GET","path":"/healthz","status":200,"duration_ms":1,"time":"2026-10-05T17:35:23.329568+07:00"}`+"\n"+
			`{"level":"error","msg":"upstream attempt failed","provider":"deepseek","status":429,"time":"2026-10-05T17:35:25.100000+07:00"}`+"\n")
	served := processLogServer(t, harness, home)

	response := harness.managementOn(served, http.MethodGet, "/api/v1/logs/daemon?hide_requests=1", adminToken, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", response.Code, response.Body.String())
	}
	body := struct {
		Items []struct {
			Level   string
			Message string
		}
		End string
	}{}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode the response: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Message != "upstream attempt failed" {
		t.Fatalf("items = %+v, want only the failed attempt", body.Items)
	}
	if body.End == "" {
		t.Fatalf("end = %q, want the file size", body.End)
	}

	bad := harness.managementOn(served, http.MethodGet, "/api/v1/logs/daemon?level=verbose", adminToken, nil)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", bad.Code, bad.Body.String())
	}
}

func TestStartupLogsEndpoint(t *testing.T) {
	harness := newHarness(t)
	home := t.TempDir()
	writeLogFile(t, home, "startup-20261005-113533.log",
		`{"addr":"127.0.0.1:10101","level":"info","msg":"relo serving","time":"2026-10-05T11:35:33.376019+07:00"}`+"\n"+
			"relo: context deadline exceeded\n")
	served := processLogServer(t, harness, home)

	list := harness.managementOn(served, http.MethodGet, "/api/v1/logs/startups", adminToken, nil)
	if list.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", list.Code, list.Body.String())
	}
	listed := struct {
		Items []struct {
			Name    string
			Outcome string
		}
	}{}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode the list: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].Outcome != "failed" {
		t.Fatalf("items = %+v, want the failed start", listed.Items)
	}

	detail := harness.managementOn(served, http.MethodGet, "/api/v1/logs/startups/startup-20261005-113533.log", adminToken, nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", detail.Code, detail.Body.String())
	}

	missing := harness.managementOn(served, http.MethodGet, "/api/v1/logs/startups/startup-20200101-000000.log", adminToken, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", missing.Code, missing.Body.String())
	}

	climb := harness.managementOn(served, http.MethodGet, "/api/v1/logs/startups/%2E%2E%2Fdaemon.log", adminToken, nil)
	if climb.Code != http.StatusBadRequest && climb.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 400 or 404 (%s)", climb.Code, climb.Body.String())
	}
}
