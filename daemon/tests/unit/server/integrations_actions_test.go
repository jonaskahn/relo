package server_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegrationActionRoutes holds the paths the Integrations page posts to
// against the routes the daemon mounts. The page's words differ from the
// daemon's — an operator "removes" what the API calls "disable" — and a path
// that names no route is answered with "no matching route", which is what the
// page would show as a failed action.
func TestIntegrationActionRoutes(t *testing.T) {
	// An inherited client home is what a Codex session exports into go test.
	// The harness has to replace it before any action can reach the live daemon.
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	h := newHarness(t)

	for _, action := range []string{"enable", "disable", "rotate", "repair", "restart", "reload", "restore", "verify"} {
		t.Run(action, func(t *testing.T) {
			response := h.management(http.MethodPost, "/api/v1/integrations/codex/"+action, adminToken, nil)
			if response.Code == http.StatusNotFound && strings.Contains(response.Body.String(), "no matching route") {
				t.Fatalf("%s matched no route: %s", action, response.Body.String())
			}
		})
	}

	t.Run("removing an integration is the disable route", func(t *testing.T) {
		// The path the page used to post is the one shape that must not be
		// reachable: nothing mounts an action called remove.
		response := h.management(http.MethodPost, "/api/v1/integrations/codex/remove", adminToken, nil)
		if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "no matching route") {
			t.Fatalf("remove = %d %s, want the unmatched-route refusal", response.Code, response.Body.String())
		}
	})
}

// stubOpenCodeLauncher puts a launcher on PATH that runs the given script
// after it records its arguments, and gives OpenCode a config directory.
func stubOpenCodeLauncher(t *testing.T, script string) {
	t.Helper()
	configHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(configHome, "opencode"), 0o755); err != nil {
		t.Fatalf("create the opencode config directory: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)
	binDir := t.TempDir()
	body := "#!/bin/sh\n" + script
	if err := os.WriteFile(filepath.Join(binDir, "opencode"), []byte(body), 0o755); err != nil {
		t.Fatalf("write the stub launcher: %v", err)
	}
	t.Setenv("PATH", binDir)
}

func TestReloadIntegrationRoute(t *testing.T) {
	t.Run("reloads a running OpenCode service", func(t *testing.T) {
		stubOpenCodeLauncher(t, "exit 0\n")
		h := newHarness(t)
		var body struct {
			Integration struct {
				ID string `json:"id"`
			} `json:"integration"`
		}
		if code := h.managementJSON(t, http.MethodPost, "/api/v1/integrations/opencode/reload", "", &body); code != http.StatusOK {
			t.Fatalf("reload = %d, want 200", code)
		}
		if body.Integration.ID != "opencode" {
			t.Fatalf("integration = %+v, want opencode", body.Integration)
		}
	})

	t.Run("refuses a client with no service to reload", func(t *testing.T) {
		h := newHarness(t)
		if code := h.managementJSON(t, http.MethodPost, "/api/v1/integrations/codex/reload", "", nil); code != http.StatusBadRequest {
			t.Fatalf("reload(codex) = %d, want 400", code)
		}
	})

	t.Run("reports a launcher that fails", func(t *testing.T) {
		stubOpenCodeLauncher(t, "exit 1\n")
		h := newHarness(t)
		if code := h.managementJSON(t, http.MethodPost, "/api/v1/integrations/opencode/reload", "", nil); code < http.StatusBadRequest {
			t.Fatalf("reload with a failing launcher = %d, want an error", code)
		}
	})
}

// TestRepairIntegrationCarriesWarnings keeps a finished repair from being lost
// when the OpenCode service refuses to follow it: the response is a success
// that names what did not happen.
func TestRepairIntegrationCarriesWarnings(t *testing.T) {
	stubOpenCodeLauncher(t, "exit 1\n")
	h := newHarness(t)
	var body struct {
		Warnings []struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		} `json:"warnings"`
	}
	if code := h.managementJSON(t, http.MethodPost, "/api/v1/integrations/opencode/repair", "", &body); code != http.StatusOK {
		t.Fatalf("repair = %d, want 200", code)
	}
	if len(body.Warnings) != 1 || body.Warnings[0].Code != "restart_failed" || body.Warnings[0].Detail == "" {
		t.Fatalf("warnings = %+v, want the failed restart", body.Warnings)
	}
}
