package service_test

import (
	"context"
	"errors"
	"testing"

	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
)

// TestSetModelsContextWindowWritesEveryNamedModel is the connection-wide write
// an operator reaches for: one size for every model of a connection, and a
// clear that puts the connection back on what the layers state.
func TestSetModelsContextWindowWritesEveryNamedModel(t *testing.T) {
	manager := cloneService(t)
	ctx := context.Background()
	if _, err := manager.CloneModel(ctx, "openai", appcatalog.CloneRequest{
		SourceModelID: "model-1", ModelID: "model-2",
	}); err != nil {
		t.Fatalf("CloneModel() error = %v", err)
	}

	window := int64(1_000_000)
	if err := manager.SetModelsContextWindow(ctx, "openai", []string{"model-1", "model-2"}, &window); err != nil {
		t.Fatalf("SetModelsContextWindow() error = %v", err)
	}
	for _, id := range []string{"model-1", "model-2"} {
		model, err := manager.Model(ctx, "openai", id)
		if err != nil {
			t.Fatalf("Model(%s) error = %v", id, err)
		}
		if model.ContextWindow == nil || *model.ContextWindow != window {
			t.Fatalf("%s context = %v, want the written window", id, model.ContextWindow)
		}
		if model.ContextSource != "override" {
			t.Fatalf("%s context source = %q, want the override", id, model.ContextSource)
		}
	}

	// A name the connection does not hold is skipped rather than failing the
	// write, and the clear leaves the rest of the override layer alone.
	if err := manager.SetModelsContextWindow(ctx, "openai", []string{"model-1", "model-missing"}, nil); err != nil {
		t.Fatalf("SetModelsContextWindow(clear) error = %v", err)
	}
	cleared, err := manager.Model(ctx, "openai", "model-1")
	if err != nil {
		t.Fatalf("Model() error = %v", err)
	}
	if cleared.ContextLayers.Override != nil {
		t.Fatalf("override context = %v, want it cleared", cleared.ContextLayers.Override)
	}
	if cleared.Name != "Model 1" {
		t.Fatalf("name = %q, want the rest of the layer kept", cleared.Name)
	}
}

// TestSetModelsContextWindowRefusesAnOutOfRangeValue keeps a typo from
// reaching every model of a connection.
func TestSetModelsContextWindowRefusesAnOutOfRangeValue(t *testing.T) {
	manager := cloneService(t)
	ctx := context.Background()
	tooBig := int64(100_000_001)
	if err := manager.SetModelsContextWindow(ctx, "openai", []string{"model-1"}, &tooBig); !errors.Is(err, appcatalog.ErrInvalidCatalogRow) {
		t.Fatalf("SetModelsContextWindow(too big) error = %v, want a refusal", err)
	}
	model, err := manager.Model(ctx, "openai", "model-1")
	if err != nil {
		t.Fatalf("Model() error = %v", err)
	}
	if model.ContextLayers.Override != nil {
		t.Fatalf("override context = %v, want nothing written for a refused value", model.ContextLayers.Override)
	}
}
