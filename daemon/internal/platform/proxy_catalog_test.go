package platform

import (
	"context"
	"testing"

	appintegration "github.com/jonaskahn/relo/internal/application/integration"
)

func TestProxyCatalogRowsCarryThePickerIDNameAndWindow(t *testing.T) {
	window := int64(400000)
	rows := proxyCatalogRows(func() []appintegration.ModelRef {
		return []appintegration.ModelRef{
			{ID: "claude-relo-openai--alpha", Name: "Alpha", ContextWindow: &window},
			{ID: "claude-reloc-cheap", Name: "Cheap"},
		}
	})()
	if len(rows) != 2 || rows[0].ID != "claude-relo-openai--alpha" || rows[0].ContextWindow != window ||
		rows[1].ID != "claude-reloc-cheap" || rows[1].ContextWindow != 0 || rows[0].Name == "" {
		t.Fatalf("rows = %+v, want one row per model with the window when known", rows)
	}
}

func TestTheRuntimeDropsTheClientsCatalogWhenAnInterceptStartsAndStops(t *testing.T) {
	dropped := 0
	runtime := NewProxyRuntime(ProxyRuntimeOptions{Home: t.TempDir(), Invalidate: func() { dropped++ }})
	target := appintegration.ProxyTarget{Host: "api.anthropic.com", DataPlaneURL: "http://127.0.0.1:1"}
	if _, err := runtime.Ensure(context.Background(), "claude-code", target); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if dropped != 1 {
		t.Fatalf("dropped after Ensure = %d, want 1", dropped)
	}
	if err := runtime.Retire("claude-code"); err != nil {
		t.Fatalf("Retire() error = %v", err)
	}
	if dropped != 2 {
		t.Fatalf("dropped after Retire = %d, want 2", dropped)
	}
}
