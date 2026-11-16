package platform

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
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

// A daemon shutdown must keep the token and certificate the client's settings
// carry; only turning the integration off removes them.
func TestAShutdownKeepsTheTokenAndCertificateButRetireRemovesThem(t *testing.T) {
	home := t.TempDir()
	runtime := NewProxyRuntime(ProxyRuntimeOptions{Home: home})
	target := appintegration.ProxyTarget{Host: "api.anthropic.com", DataPlaneURL: "http://127.0.0.1:1"}
	first, err := runtime.Ensure(context.Background(), "claude-code", target)
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	runtime.Stop()
	if _, running := runtime.Status("claude-code"); running {
		t.Fatal("Stop() left the intercept running")
	}
	again := NewProxyRuntime(ProxyRuntimeOptions{Home: home})
	second, err := again.Ensure(context.Background(), "claude-code", target)
	if err != nil {
		t.Fatalf("Ensure() after a restart error = %v", err)
	}
	if strings.SplitN(first.URL, "@", 2)[0] != strings.SplitN(second.URL, "@", 2)[0] || first.CAPath != second.CAPath {
		t.Fatalf("restart gave %q / %q, want the same credential and certificate", second.URL, second.CAPath)
	}
	if err := again.Retire("claude-code"); err != nil {
		t.Fatalf("Retire() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "proxy-mode", "claude-code")); !os.IsNotExist(err) {
		t.Fatalf("Retire() left the intercept's directory behind: %v", err)
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

// A restart keeps the port a client's open session names, and a taken port
// falls back to a new one rather than refusing to start.
func TestARestartKeepsTheInterceptsPortUnlessSomethingTookIt(t *testing.T) {
	home := t.TempDir()
	target := appintegration.ProxyTarget{Host: "api.anthropic.com", DataPlaneURL: "http://127.0.0.1:1"}
	runtime := NewProxyRuntime(ProxyRuntimeOptions{Home: home})
	first, err := runtime.Ensure(context.Background(), "claude-code", target)
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	runtime.Stop()

	again := NewProxyRuntime(ProxyRuntimeOptions{Home: home})
	second, err := again.Ensure(context.Background(), "claude-code", target)
	if err != nil {
		t.Fatalf("Ensure() after a restart error = %v", err)
	}
	if first.URL != second.URL {
		t.Fatalf("restart gave %q, want the same address %q", second.URL, first.URL)
	}
	again.Stop()

	held, err := net.Listen("tcp", strings.SplitN(first.URL, "@", 2)[1])
	if err != nil {
		t.Skipf("the port was taken before the test could hold it: %v", err)
	}
	defer func() { _ = held.Close() }()
	crowded := NewProxyRuntime(ProxyRuntimeOptions{Home: home})
	third, err := crowded.Ensure(context.Background(), "claude-code", target)
	if err != nil {
		t.Fatalf("Ensure() with the port taken error = %v", err)
	}
	defer crowded.Stop()
	if third.URL == first.URL {
		t.Fatalf("a taken port was reused: %q", third.URL)
	}
}
