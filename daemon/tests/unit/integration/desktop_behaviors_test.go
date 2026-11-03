package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/application/integration"
)

// desktopHarness enables Claude Desktop over a temporary home and returns the
// profile path. The Desktop user-data directory is created, as an installed
// client would have it.
func desktopHarness(t *testing.T, models *[]integration.ModelRef) (*harness, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the Desktop directory is resolved from APPDATA on Windows")
	}
	h := newHarness(t, dataPlane)
	withModels(h, models)
	t.Setenv("CLAUDE_USER_DATA_DIR", "")
	support := filepath.Join(h.paths.Home, ".config")
	if runtime.GOOS == "darwin" {
		support = filepath.Join(h.paths.Home, "Library", "Application Support")
	}
	if err := os.MkdirAll(filepath.Join(support, "Claude"), 0o755); err != nil {
		t.Fatalf("create the Desktop directory: %v", err)
	}
	library := filepath.Join(support, "Claude-3p", "configLibrary")
	if _, err := h.manager.Enable(context.Background(), "claude-desktop", "rlo_ak_secret"); err != nil {
		t.Fatalf("Enable(claude-desktop) error = %v", err)
	}
	return h, codingclients.DesktopProfilePath(library)
}

func readDesktopModels(t *testing.T, path string) []map[string]any {
	t.Helper()
	var profile struct {
		Models []map[string]any `json:"inferenceModels"`
	}
	if err := json.Unmarshal([]byte(readFile(t, path)), &profile); err != nil {
		t.Fatalf("decode the profile: %v", err)
	}
	return profile.Models
}

// rewriteDesktopProfile replaces the managed profile with the operator's
// content and records it as Relo's own write, which is the shape an older
// build leaves behind.
func rewriteDesktopProfile(t *testing.T, h *harness, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write the profile: %v", err)
	}
	files, err := h.store.Files(context.Background(), "claude-desktop")
	if err != nil {
		t.Fatalf("read the stored files: %v", err)
	}
	for _, file := range files {
		if file.Kind != integration.KindClaudeDesktop {
			continue
		}
		file.Digest = h.files.Digest(content)
		if err := h.store.SaveFile(context.Background(), file); err != nil {
			t.Fatalf("save the stored file: %v", err)
		}
	}
}

// TestDesktopProfileCarriesBehavesAs pins the behavior mapping across the
// connection shapes Desktop publishes.
func TestDesktopProfileCarriesBehavesAs(t *testing.T) {
	models := []integration.ModelRef{
		{ID: "claude-relo-a--claude-sonnet-5-5", Name: "Sonnet 5.5", Upstream: "claude-sonnet-5-5", Connection: "Claude API"},
		{ID: "claude-relo-b--gpt-5.5-pro", Name: "GPT Pro", Upstream: "gpt-5.5-pro", Connection: "OpenAI"},
		{ID: "claude-relo-c--gpt-5.5-astra", Name: "GPT Astra", Upstream: "gpt-5.5-astra", Connection: "OpenAI"},
		{ID: "claude-relo-d--gpt-5.5-mini", Name: "GPT Mini", Upstream: "gpt-5.5-mini", Connection: "OpenAI"},
		{ID: "claude-relo-e--gemini-3.8-flash", Name: "Gemini Flash", Upstream: "gemini-3.8-flash", Connection: "Google"},
		{ID: "claude-reloc-combo", Name: "Combo", Connection: "Relo"},
	}
	_, path := desktopHarness(t, &models)
	rows := readDesktopModels(t, path)
	if len(rows) != len(models) {
		t.Fatalf("rows = %d, want %d", len(rows), len(models))
	}
	want := []string{
		"claude-sonnet-5", "claude-opus-4-6", "claude-fable-5",
		"claude-haiku-4-5", "claude-sonnet-5", "claude-sonnet-5",
	}
	for index, row := range rows {
		if got := row["behavesAs"]; got != want[index] {
			t.Fatalf("row %d behavesAs = %v, want %q", index, got, want[index])
		}
		if row["name"] == "" || row["labelOverride"] == "" {
			t.Fatalf("row %d lost its alias or label: %#v", index, row)
		}
	}
}

// TestDesktopRefreshAddsBehavesAsWithoutReplacingTheProfile is the upgrade
// path: an old managed profile gains the metadata, and the credential and
// unrelated settings stay exactly as they were.
func TestDesktopRefreshAddsBehavesAsWithoutReplacingTheProfile(t *testing.T) {
	models := []integration.ModelRef{{
		ID: "claude-relo-a--gpt-5.5-pro", Name: "GPT Pro", Upstream: "gpt-5.5-pro", Connection: "OpenAI",
	}}
	h, path := desktopHarness(t, &models)
	old := `{
  "inferenceGatewayApiKey": "rlo_ak_secret",
  "inferenceGatewayBaseUrl": "http://127.0.0.1:10202",
  "inferenceModels": [
    {
      "name": "` + desktopAlias(t, path) + `",
      "labelOverride": "GPT Pro",
      "anthropicFamilyTier": "opus",
      "isFamilyDefault": true
    }
  ],
  "modelDiscoveryEnabled": false,
  "theme": "dark"
}
`
	rewriteDesktopProfile(t, h, path, old)

	view, err := h.manager.Inspect(context.Background(), "claude-desktop")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if !view.ModelsStale {
		t.Fatalf("view = %+v, want the missing behavior metadata reported stale", view)
	}
	if reasonOfFound(view, integration.ReasonFileChanged) {
		t.Fatalf("reasons = %+v, want an outdated file rather than drift", view.Reasons)
	}

	if err := h.manager.RefreshOutdatedFiles(context.Background(), "claude-desktop", "rlo_ak_secret"); err != nil {
		t.Fatalf("RefreshOutdatedFiles() error = %v", err)
	}
	refreshed := readFile(t, path)
	for _, want := range []string{"rlo_ak_secret", "\"theme\": \"dark\"", "\"behavesAs\": \"claude-opus-4-6\""} {
		if !strings.Contains(refreshed, want) {
			t.Fatalf("profile = %s, want %s kept", refreshed, want)
		}
	}
	meta, err := os.ReadFile(filepath.Join(filepath.Dir(path), "_meta.json"))
	if err != nil || !strings.Contains(string(meta), "appliedId") {
		t.Fatalf("metadata = %s, %v, want the selection untouched", meta, err)
	}

	view, err = h.manager.Inspect(context.Background(), "claude-desktop")
	if err != nil || view.ModelsStale || len(view.Reasons) != 0 {
		t.Fatalf("view = %+v error = %v, want current after the refresh", view, err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat the profile: %v", err)
	}
	if err := h.manager.RefreshOutdatedFiles(context.Background(), "claude-desktop", "rlo_ak_secret"); err != nil {
		t.Fatalf("RefreshOutdatedFiles(again) error = %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat the profile again: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("a settled profile was rewritten")
	}
}

// TestDesktopRefreshLeavesAnEditedProfileAlone keeps an operator's edit: the
// profile is reported as drift and only Repair rewrites it.
func TestDesktopRefreshLeavesAnEditedProfileAlone(t *testing.T) {
	models := []integration.ModelRef{{
		ID: "claude-relo-a--gpt-5.5-pro", Name: "GPT Pro", Upstream: "gpt-5.5-pro", Connection: "OpenAI",
	}}
	h, path := desktopHarness(t, &models)
	edited := strings.Replace(readFile(t, path), "GPT Pro", "Operator's model", 1)
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatalf("edit the profile: %v", err)
	}
	view, err := h.manager.Inspect(context.Background(), "claude-desktop")
	if err != nil || !reasonOfFound(view, integration.ReasonFileChanged) {
		t.Fatalf("view = %+v error = %v, want file_changed", view, err)
	}
	if err := h.manager.RefreshOutdatedFiles(context.Background(), "claude-desktop", "rlo_ak_secret"); err != nil {
		t.Fatalf("RefreshOutdatedFiles() error = %v", err)
	}
	if readFile(t, path) != edited {
		t.Fatal("refresh rewrote an edited profile")
	}
}

// desktopAlias is the opaque name the profile row carries for its only model.
func desktopAlias(t *testing.T, path string) string {
	t.Helper()
	rows := readDesktopModels(t, path)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want one", len(rows))
	}
	name, _ := rows[0]["name"].(string)
	return name
}

// TestDesktopRefreshLeavesAMissingProfileForRepair keeps a deleted profile
// from being recreated silently: the missing file is its own reason.
func TestDesktopRefreshLeavesAMissingProfileForRepair(t *testing.T) {
	models := []integration.ModelRef{{
		ID: "claude-relo-a--gpt-5.5-pro", Name: "GPT Pro", Upstream: "gpt-5.5-pro", Connection: "OpenAI",
	}}
	h, path := desktopHarness(t, &models)
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove the profile: %v", err)
	}
	if err := h.manager.RefreshOutdatedFiles(context.Background(), "claude-desktop", "rlo_ak_secret"); err != nil {
		t.Fatalf("RefreshOutdatedFiles() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("profile = %v, want it left for Repair", err)
	}
	view, err := h.manager.Inspect(context.Background(), "claude-desktop")
	if err != nil || !reasonOfFound(view, integration.ReasonFileMissing) {
		t.Fatalf("view = %+v error = %v, want file_missing", view, err)
	}
}

// TestDesktopRefreshLeavesADisabledIntegrationAlone is the ownership rule:
// nothing is written while the integration is off.
func TestDesktopRefreshLeavesADisabledIntegrationAlone(t *testing.T) {
	models := []integration.ModelRef{{
		ID: "claude-relo-a--gpt-5.5-pro", Name: "GPT Pro", Upstream: "gpt-5.5-pro", Connection: "OpenAI",
	}}
	h, path := desktopHarness(t, &models)
	if _, err := h.manager.Disable(context.Background(), "claude-desktop"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if err := h.manager.RefreshOutdatedFiles(context.Background(), "claude-desktop", "rlo_ak_secret"); err != nil {
		t.Fatalf("RefreshOutdatedFiles() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("profile = %v, want no profile written for a disabled integration", err)
	}
}
