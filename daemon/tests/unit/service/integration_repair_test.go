package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestRepairPreservesStoredKey keeps a repair from invalidating the secret a
// running agent already holds: the stored key and its hint survive, and no
// replacement is revealed. A missing key file is the only case that mints.
func TestRepairPreservesStoredKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	h, built := integrationService(t)
	ctx := context.Background()

	enabled, err := built.EnableIntegration(ctx, "codex")
	if err != nil {
		t.Fatalf("EnableIntegration() error = %v", err)
	}
	before, err := built.IntegrationToken(ctx, "codex")
	if err != nil {
		t.Fatalf("IntegrationToken() error = %v", err)
	}
	if before != enabled.Token {
		t.Fatalf("token = %q, want the minted key", before)
	}
	beforeHint := ""
	if enabled.Integration.Key != nil {
		beforeHint = enabled.Integration.Key.Hint
	}

	repaired, err := built.RepairIntegration(ctx, "codex")
	if err != nil {
		t.Fatalf("RepairIntegration() error = %v", err)
	}
	if repaired.Token != "" {
		t.Fatalf("repaired token is set, want no replacement revealed")
	}
	after, err := built.IntegrationToken(ctx, "codex")
	if err != nil {
		t.Fatalf("IntegrationToken() error = %v", err)
	}
	if after != before {
		t.Fatalf("stored key changed by repair")
	}
	if repaired.Integration.Key == nil || repaired.Integration.Key.Hint != beforeHint {
		t.Fatalf("key hint = %+v, want %q preserved", repaired.Integration.Key, beforeHint)
	}

	if err := os.Remove(filepath.Join(h.home, "integrations", "codex", "key")); err != nil {
		t.Fatalf("remove the key file: %v", err)
	}
	healed, err := built.RepairIntegration(ctx, "codex")
	if err != nil {
		t.Fatalf("RepairIntegration() without a key error = %v", err)
	}
	if healed.Token == "" {
		t.Fatalf("healed token is empty, want the replacement revealed once")
	}
	stored, err := built.IntegrationToken(ctx, "codex")
	if err != nil {
		t.Fatalf("IntegrationToken() error = %v", err)
	}
	if stored != healed.Token {
		t.Fatalf("stored key does not match the revealed replacement")
	}
}
