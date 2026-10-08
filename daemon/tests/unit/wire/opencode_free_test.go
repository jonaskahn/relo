package codec

import (
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

// The signed-out Zen gateway is the one OpenCode serves without an account.
// It keeps no subscription, so the two paths have to stay distinguishable.
func TestOpenCodeFreeIsNotOpenCodeGo(t *testing.T) {
	cases := []struct {
		baseURL string
		free    bool
		goPath  bool
	}{
		{baseURL: "https://opencode.ai/zen/v1", free: true},
		{baseURL: "https://opencode.ai/zen", free: true},
		{baseURL: "https://OPENCODE.AI/zen/v1", free: true},
		{baseURL: "https://opencode.ai/zen/go/v1", goPath: true},
		{baseURL: "https://opencode.ai/v1"},
		{baseURL: "https://api.opencode.ai/zen/v1"},
		{baseURL: "https://openrouter.ai/api/v1"},
		{baseURL: ""},
		{baseURL: "://nonsense"},
	}
	for _, entry := range cases {
		if got := wire.OpenCodeFree(entry.baseURL); got != entry.free {
			t.Errorf("OpenCodeFree(%q) = %t, want %t", entry.baseURL, got, entry.free)
		}
		if got := wire.OpenCodeGo(entry.baseURL); got != entry.goPath {
			t.Errorf("OpenCodeGo(%q) = %t, want %t", entry.baseURL, got, entry.goPath)
		}
	}
}
