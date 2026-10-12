package server_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDisableOffersAForcedRemovalWhenASnapshotIsGone is the machine whose
// snapshot directory no longer exists: the plain removal stops and names the
// reason, and the forced one the operator confirms completes.
func TestDisableOffersAForcedRemovalWhenASnapshotIsGone(t *testing.T) {
	h := newHarness(t)
	paths := h.integrationPaths()
	// Relo only snapshots a file the operator already had, so the config is
	// seeded before the enable for the snapshot this test loses to exist.
	if err := os.MkdirAll(filepath.Dir(paths.CodexConfig()), 0o755); err != nil {
		t.Fatalf("create the codex directory: %v", err)
	}
	if err := os.WriteFile(paths.CodexConfig(), []byte("approval_policy = \"never\"\n"), 0o644); err != nil {
		t.Fatalf("seed the codex config: %v", err)
	}
	if code := h.managementJSON(t, http.MethodPost, "/api/v1/integrations/codex/enable", `{"overwrite":true}`, nil); code != http.StatusOK {
		t.Fatalf("enable = %d, want the integration wired", code)
	}
	if err := os.RemoveAll(paths.SnapshotDir()); err != nil {
		t.Fatalf("remove the snapshots: %v", err)
	}

	response := h.management(http.MethodPost, "/api/v1/integrations/codex/disable", adminToken, nil)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "restore_blocked") {
		t.Fatalf("disable = %d %s, want the blocked refusal", response.Code, response.Body.String())
	}

	forced := h.management(http.MethodPost, "/api/v1/integrations/codex/disable", adminToken, strings.NewReader(`{"force":true}`))
	if forced.Code != http.StatusOK || !strings.Contains(forced.Body.String(), "disabled") {
		t.Fatalf("forced disable = %d %s, want the integration removed", forced.Code, forced.Body.String())
	}
	if got, err := os.ReadFile(paths.CodexConfig()); err == nil && strings.Contains(string(got), "relo") {
		t.Fatalf("codex config = %s, want Relo gone", got)
	}
}
