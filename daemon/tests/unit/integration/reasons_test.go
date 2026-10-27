package integration_test

import (
	"context"
	"os"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/application/integration"
)

// TestInspectNamesWhyAFileChanged is what turns a "Needs repair" pill into a
// sentence: the file Relo wrote was replaced, and Inspect says which one.
func TestInspectNamesWhyAFileChanged(t *testing.T) {
	h := newHarness(t, dataPlane)
	if _, err := h.manager.Enable(context.Background(), "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	helper := h.paths.HelperFile("codex")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\necho replaced\n"), 0o700); err != nil {
		t.Fatalf("replace the key helper: %v", err)
	}
	view, err := h.manager.Inspect(context.Background(), "codex")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	reason, found := reasonOf(view, integration.ReasonFileChanged)
	if !found {
		t.Fatalf("Inspect() reasons = %+v, want a file_changed", view.Reasons)
	}
	if reason.Path != helper {
		t.Fatalf("file_changed path = %q, want %q", reason.Path, helper)
	}
}

// TestInspectNamesAMissingFile covers the other file fact: the file Relo wrote
// is gone, so Repair has nothing to compare against.
func TestInspectNamesAMissingFile(t *testing.T) {
	h := newHarness(t, dataPlane)
	if _, err := h.manager.Enable(context.Background(), "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	helper := h.paths.HelperFile("codex")
	if err := os.Remove(helper); err != nil {
		t.Fatalf("remove the key helper: %v", err)
	}
	view, err := h.manager.Inspect(context.Background(), "codex")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	reason, found := reasonOf(view, integration.ReasonFileMissing)
	if !found || reason.Path != helper {
		t.Fatalf("Inspect() reasons = %+v, want file_missing for %s", view.Reasons, helper)
	}
}

// TestInspectCountsTheModelListChange is what tells an operator how many
// models moved, rather than only that something did.
func TestInspectCountsTheModelListChange(t *testing.T) {
	h := newHarness(t, dataPlane)
	models := []integration.ModelRef{{ID: "relo-a-one", Name: "One", Connection: "A"}}
	withModels(h, &models)
	ctx := context.Background()
	if _, err := h.manager.Enable(ctx, "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable(codex) error = %v", err)
	}
	if view, err := h.manager.Inspect(ctx, "codex"); err != nil || reasonOfFound(view, integration.ReasonModelsChanged) {
		t.Fatalf("Inspect(codex) reasons = %+v error = %v, want no model change", view.Reasons, err)
	}
	models = []integration.ModelRef{
		{ID: "relo-a-one", Name: "One", Connection: "A"},
		{ID: "relo-b-two", Name: "Two", Connection: "B"},
	}
	view, err := h.manager.Inspect(ctx, "codex")
	if err != nil {
		t.Fatalf("Inspect(codex) error = %v", err)
	}
	reason, found := reasonOf(view, integration.ReasonModelsChanged)
	if !found || reason.Added != 1 || reason.Removed != 0 {
		t.Fatalf("Inspect(codex) reasons = %+v, want one model added", view.Reasons)
	}
	if _, err := h.manager.Repair(ctx, "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Repair(codex) error = %v", err)
	}
	if view, err := h.manager.Inspect(ctx, "codex"); err != nil || reasonOfFound(view, integration.ReasonModelsChanged) {
		t.Fatalf("Inspect(codex) after repair reasons = %+v error = %v, want no model change", view.Reasons, err)
	}
}

// TestInspectStaysSilentOnAHealthyAgent keeps the reasons off a card an
// operator has nothing to do about.
func TestInspectStaysSilentOnAHealthyAgent(t *testing.T) {
	h := newHarness(t, dataPlane)
	models := []integration.ModelRef{{ID: "relo-a-one", Name: "One", Connection: "A"}}
	withModels(h, &models)
	ctx := context.Background()
	if _, err := h.manager.Enable(ctx, "codex", "rlo_ak_test"); err != nil {
		t.Fatalf("Enable(codex) error = %v", err)
	}
	view, err := h.manager.Inspect(ctx, "codex")
	if err != nil {
		t.Fatalf("Inspect(codex) error = %v", err)
	}
	if len(view.Reasons) != 0 {
		t.Fatalf("Inspect(codex) reasons = %+v, want none", view.Reasons)
	}
	if _, err := h.manager.Disable(ctx, "codex"); err != nil {
		t.Fatalf("Disable(codex) error = %v", err)
	}
	if err := os.Remove(codingclients.CodexCatalogPath(h.paths.CodexConfig())); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove the catalog: %v", err)
	}
	view, err = h.manager.Inspect(ctx, "codex")
	if err != nil {
		t.Fatalf("Inspect(codex) error = %v", err)
	}
	if len(view.Reasons) != 0 {
		t.Fatalf("Inspect(codex) after disable reasons = %+v, want none", view.Reasons)
	}
}

func reasonOf(view integration.View, code string) (integration.Reason, bool) {
	for _, reason := range view.Reasons {
		if reason.Code == code {
			return reason, true
		}
	}
	return integration.Reason{}, false
}

func reasonOfFound(view integration.View, code string) bool {
	_, found := reasonOf(view, code)
	return found
}
