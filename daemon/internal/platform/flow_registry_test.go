package platform

import (
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/oauth"
)

// TestFlowRegistryResolvesTheConnectionTemplate covers a connection that was
// created from a template and kept its own id: the login flow is the template's,
// because that is the vendor Relo signs in to, while the connection is the
// provider whose credential is being renewed.
func TestFlowRegistryResolvesTheConnectionTemplate(t *testing.T) {
	registry := oauth.DefaultRegistry()
	resolve := func(providerID string) string {
		if providerID == "claude-work" {
			return oauth.FlowClaude
		}
		return providerID
	}

	t.Run("the template's flow serves the connection", func(t *testing.T) {
		flow, err := NewFlowRegistry(registry, resolve).Flow("claude-work")
		if err != nil {
			t.Fatalf("Flow() error = %v", err)
		}
		if flow.ProviderID() != oauth.FlowClaude {
			t.Fatalf("provider = %q, want the Claude flow", flow.ProviderID())
		}
	})

	t.Run("a provider the registry does not name is still refused", func(t *testing.T) {
		if _, err := NewFlowRegistry(registry, resolve).Flow("no-such-provider"); err == nil {
			t.Fatal("Flow() succeeded without a registered flow")
		}
	})

	t.Run("a template the registry does not name falls back to the connection", func(t *testing.T) {
		fallback := NewFlowRegistry(registry, func(string) string { return "no-such-template" })
		if _, err := fallback.Flow("cursor"); err != nil {
			t.Fatalf("Flow(cursor) error = %v, want the connection's own flow", err)
		}
	})
}
