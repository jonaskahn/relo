package server_test

import (
	"net/http"
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

	for _, action := range []string{"enable", "disable", "rotate", "repair", "restart", "restore", "verify"} {
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
