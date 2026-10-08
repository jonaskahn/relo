package service_test

import (
	"context"
	"errors"
	"testing"

	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
)

func cloneService(t *testing.T) *session {
	t.Helper()
	h := newHarness(t)
	return pageService(t, h, catalogServer(t, catalogFixture()).URL)
}

func TestCloneModelCopiesTheSource(t *testing.T) {
	manager := cloneService(t)
	source, err := manager.Model(context.Background(), "openai", "model-1")
	if err != nil {
		t.Fatalf("Model() error = %v", err)
	}

	cloned, err := manager.CloneModel(context.Background(), "openai", appcatalog.CloneRequest{
		SourceModelID: "model-1",
		ModelID:       "model-1-copy",
	})
	if err != nil {
		t.Fatalf("CloneModel() error = %v", err)
	}
	if cloned.ClonedFrom != "model-1" {
		t.Fatalf("cloned_from = %q, want the source", cloned.ClonedFrom)
	}
	if cloned.UpstreamModelID != "model-1" {
		t.Fatalf("upstream = %q, want the source's own id", cloned.UpstreamModelID)
	}
	if cloned.Source != appcatalog.SourceManual {
		t.Fatalf("source = %q, want a hand-added row so the delete path applies", cloned.Source)
	}
	if cloned.Name != source.Name || cloned.Category != source.Category {
		t.Fatalf("clone = %+v, want the source's own details", cloned)
	}
}

func TestCloneModelPointsAtAnotherUpstream(t *testing.T) {
	manager := cloneService(t)
	upstream := "model-2"
	window := int64(400000)

	cloned, err := manager.CloneModel(context.Background(), "openai", appcatalog.CloneRequest{
		SourceModelID:   "model-1",
		ModelID:         "model-2-preview",
		UpstreamModelID: &upstream,
		Override:        &appcatalog.ModelOverride{ContextWindow: &window},
	})
	if err != nil {
		t.Fatalf("CloneModel() error = %v", err)
	}
	if cloned.UpstreamModelID != upstream {
		t.Fatalf("upstream = %q, want the id the operator typed", cloned.UpstreamModelID)
	}
	if cloned.ContextWindow == nil || *cloned.ContextWindow != window {
		t.Fatalf("context = %v, want the override just set", cloned.ContextWindow)
	}
	if cloned.ContextSource != "override" {
		t.Fatalf("context source = %q, want the override", cloned.ContextSource)
	}
}

func TestCloneOfACloneKeepsTheRealUpstream(t *testing.T) {
	manager := cloneService(t)
	upstream := "model-2"
	if _, err := manager.CloneModel(context.Background(), "openai", appcatalog.CloneRequest{
		SourceModelID: "model-1", ModelID: "model-2-preview", UpstreamModelID: &upstream,
	}); err != nil {
		t.Fatalf("CloneModel() error = %v", err)
	}

	again, err := manager.CloneModel(context.Background(), "openai", appcatalog.CloneRequest{
		SourceModelID: "model-2-preview", ModelID: "model-2-preview-2",
	})
	if err != nil {
		t.Fatalf("CloneModel(clone) error = %v", err)
	}
	if again.UpstreamModelID != upstream {
		t.Fatalf("upstream = %q, want the real upstream rather than the clone's id", again.UpstreamModelID)
	}
}

func TestCloneModelRefusesADuplicateOrUnusableID(t *testing.T) {
	manager := cloneService(t)

	if _, err := manager.CloneModel(context.Background(), "openai", appcatalog.CloneRequest{
		SourceModelID: "model-1", ModelID: "model-1",
	}); !errors.Is(err, appcatalog.ErrCatalogConflict) {
		t.Fatalf("CloneModel(duplicate) error = %v, want a conflict", err)
	}
	if _, err := manager.CloneModel(context.Background(), "openai", appcatalog.CloneRequest{
		SourceModelID: "model-1", ModelID: "has space",
	}); !errors.Is(err, appcatalog.ErrInvalidCatalogRow) {
		t.Fatalf("CloneModel(space) error = %v, want a refusal", err)
	}
	if _, err := manager.CloneModel(context.Background(), "openai", appcatalog.CloneRequest{
		SourceModelID: "missing", ModelID: "some-copy",
	}); !errors.Is(err, appcatalog.ErrModelNotFound) {
		t.Fatalf("CloneModel(missing source) error = %v, want not found", err)
	}
}

func TestPatchModelContextWindowSetsAndClearsOneField(t *testing.T) {
	manager := cloneService(t)
	window := int64(500000)

	patched, err := manager.PatchModel(context.Background(), "openai", "model-1", appcatalog.ModelPatch{
		ContextWindow: appcatalog.OptionalInt{Set: true, Value: &window},
	})
	if err != nil {
		t.Fatalf("PatchModel() error = %v", err)
	}
	if patched.ContextWindow == nil || *patched.ContextWindow != window {
		t.Fatalf("context = %v, want the override", patched.ContextWindow)
	}
	if patched.ContextSource != "override" {
		t.Fatalf("context source = %q, want the override", patched.ContextSource)
	}
	// The rest of the layer the operator set stays put.
	if patched.Name != "Model 1" || patched.Category != "chat" {
		t.Fatalf("model = %+v, want the other override fields untouched", patched)
	}

	cleared, err := manager.PatchModel(context.Background(), "openai", "model-1", appcatalog.ModelPatch{
		ContextWindow: appcatalog.OptionalInt{Set: true},
	})
	if err != nil {
		t.Fatalf("PatchModel(clear) error = %v", err)
	}
	if cleared.ContextLayers.Override != nil {
		t.Fatalf("override context = %v, want it cleared", cleared.ContextLayers.Override)
	}
	if cleared.Name != "Model 1" {
		t.Fatalf("name = %q, want the rest of the override to survive a clear", cleared.Name)
	}
}

func TestPatchModelMaxOutputSetsAndClearsOneField(t *testing.T) {
	manager := cloneService(t)
	window := int64(400000)
	if _, err := manager.PatchModel(context.Background(), "openai", "model-1", appcatalog.ModelPatch{
		ContextWindow: appcatalog.OptionalInt{Set: true, Value: &window},
	}); err != nil {
		t.Fatalf("PatchModel(context) error = %v", err)
	}

	output := int64(128000)
	patched, err := manager.PatchModel(context.Background(), "openai", "model-1", appcatalog.ModelPatch{
		MaxOutput: appcatalog.OptionalInt{Set: true, Value: &output},
	})
	if err != nil {
		t.Fatalf("PatchModel() error = %v", err)
	}
	if patched.MaxOutput == nil || *patched.MaxOutput != output {
		t.Fatalf("max output = %v, want the override", patched.MaxOutput)
	}
	if patched.MaxOutputSource != "override" {
		t.Fatalf("max output source = %q, want the override", patched.MaxOutputSource)
	}
	if patched.MaxOutputLayers.Override == nil || *patched.MaxOutputLayers.Override != output {
		t.Fatalf("max output layers = %+v, want the override layer", patched.MaxOutputLayers)
	}
	if patched.ContextWindow == nil || *patched.ContextWindow != window {
		t.Fatalf("context = %v, want the context override kept", patched.ContextWindow)
	}
	if patched.Name != "Model 1" {
		t.Fatalf("name = %q, want the rest of the layer kept", patched.Name)
	}

	cleared, err := manager.PatchModel(context.Background(), "openai", "model-1", appcatalog.ModelPatch{
		MaxOutput: appcatalog.OptionalInt{Set: true},
	})
	if err != nil {
		t.Fatalf("PatchModel(clear) error = %v", err)
	}
	if cleared.MaxOutputLayers.Override != nil {
		t.Fatalf("override max output = %v, want it cleared", cleared.MaxOutputLayers.Override)
	}
	if cleared.ContextLayers.Override == nil {
		t.Fatal("clearing max output also cleared the context override")
	}
}

func TestPatchModelRefusesAnOutOfRangeContextWindow(t *testing.T) {
	manager := cloneService(t)
	tooSmall := int64(0)
	if _, err := manager.PatchModel(context.Background(), "openai", "model-1", appcatalog.ModelPatch{
		ContextWindow: appcatalog.OptionalInt{Set: true, Value: &tooSmall},
	}); !errors.Is(err, appcatalog.ErrInvalidCatalogRow) {
		t.Fatalf("PatchModel(0) error = %v, want a refusal", err)
	}
	tooBig := int64(100_000_001)
	if _, err := manager.PatchModel(context.Background(), "openai", "model-1", appcatalog.ModelPatch{
		ContextWindow: appcatalog.OptionalInt{Set: true, Value: &tooBig},
	}); !errors.Is(err, appcatalog.ErrInvalidCatalogRow) {
		t.Fatalf("PatchModel(too big) error = %v, want a refusal", err)
	}
}

func TestPatchModelRefusesContextWindowTogetherWithAnOverride(t *testing.T) {
	manager := cloneService(t)
	window := int64(200000)
	if _, err := manager.PatchModel(context.Background(), "openai", "model-1", appcatalog.ModelPatch{
		ContextWindow: appcatalog.OptionalInt{Set: true, Value: &window},
		Override:      &appcatalog.ModelOverride{},
	}); !errors.Is(err, appcatalog.ErrInvalidCatalogRow) {
		t.Fatalf("PatchModel(both) error = %v, want a refusal", err)
	}
}

func TestPatchModelUpstreamIDOnlyChangesAClone(t *testing.T) {
	manager := cloneService(t)
	upstream := "model-9"
	if _, err := manager.PatchModel(context.Background(), "openai", "model-1", appcatalog.ModelPatch{
		UpstreamModelID: &upstream,
	}); !errors.Is(err, appcatalog.ErrNotCustom) {
		t.Fatalf("PatchModel(plain model) error = %v, want a refusal", err)
	}

	cloned, err := manager.CloneModel(context.Background(), "openai", appcatalog.CloneRequest{
		SourceModelID: "model-1", ModelID: "model-1-copy",
	})
	if err != nil {
		t.Fatalf("CloneModel() error = %v", err)
	}
	renamed, err := manager.PatchModel(context.Background(), "openai", cloned.ModelID, appcatalog.ModelPatch{
		UpstreamModelID: &upstream,
	})
	if err != nil {
		t.Fatalf("PatchModel(clone) error = %v", err)
	}
	if renamed.UpstreamModelID != upstream {
		t.Fatalf("upstream = %q, want the id just set", renamed.UpstreamModelID)
	}

	blank := ""
	if _, err := manager.PatchModel(context.Background(), "openai", cloned.ModelID, appcatalog.ModelPatch{
		UpstreamModelID: &blank,
	}); !errors.Is(err, appcatalog.ErrInvalidCatalogRow) {
		t.Fatalf("PatchModel(blank upstream) error = %v, want a refusal", err)
	}
}
