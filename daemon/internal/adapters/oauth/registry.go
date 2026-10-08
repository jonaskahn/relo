// Flow registry: registering and resolving sign-in flows by provider.
package oauth

import (
	"fmt"
	"sort"
	"sync"
)

// Registry maps provider identifiers to the flows Relo ships. Every
// registration is explicit, so the composition root decides what a
// binary supports.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]FlowFactory
}

// NewRegistry returns an empty flow registry.
func NewRegistry() *Registry {
	return &Registry{factories: map[string]FlowFactory{}}
}

// RegisterFlow adds one provider flow, replacing an earlier one.
func (r *Registry) RegisterFlow(providerID string, factory FlowFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[providerID] = factory
}

// Flow builds the flow of one provider.
func (r *Registry) Flow(providerID string) (OAuthFlow, error) {
	r.mu.RLock()
	factory, found := r.factories[providerID]
	r.mu.RUnlock()
	if !found {
		return nil, fmt.Errorf("%s: %w", providerID, ErrFlowNotFound)
	}
	return factory(), nil
}

// ProviderIDs returns the providers this registry can log into, in
// alphabetical order.
func (r *Registry) ProviderIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	identifiers := make([]string, 0, len(r.factories))
	for identifier := range r.factories {
		identifiers = append(identifiers, identifier)
	}
	sort.Strings(identifiers)
	return identifiers
}

// CallbackPortFor returns the loopback port one flow's browser login needs,
// or zero when the flow has no listener of its own or this registry does not
// build it. A caller deciding whether a port has to be cleared first reads it
// here rather than hardcoding a provider's registered address.
func (r *Registry) CallbackPortFor(providerID string) int {
	flow, err := r.Flow(providerID)
	if err != nil {
		return 0
	}
	return flow.CallbackPort()
}

// DefaultRegistry returns the flows that need no local state: every
// provider Relo can log into over the network or an installed CLI.
func DefaultRegistry(options ...Option) *Registry {
	registry := NewRegistry()
	registry.RegisterFlow(providerChatGPT, func() OAuthFlow { return NewChatGPTFlow(options...) })
	registry.RegisterFlow(providerChatGPTDevice, func() OAuthFlow { return NewChatGPTDeviceFlow(options...) })
	registry.RegisterFlow(providerClaude, func() OAuthFlow { return NewAnthropicFlow(options...) })
	registry.RegisterFlow(providerGoogleAntigravity, func() OAuthFlow { return NewGoogleAntigravityFlow(options...) })
	registry.RegisterFlow(providerXAI, func() OAuthFlow { return NewXAIDeviceFlow(options...) })
	registry.RegisterFlow(providerCursor, func() OAuthFlow { return NewCursorFlow(options...) })
	registry.RegisterFlow(providerKiro, func() OAuthFlow { return NewKiroFlow(options...) })
	registry.RegisterFlow(providerDevin, func() OAuthFlow { return NewDevinFlow(options...) })
	registry.RegisterFlow(providerMetaMuse, func() OAuthFlow { return NewMetaMuseFlow(options...) })
	registry.RegisterFlow(providerKimi, func() OAuthFlow { return NewKimiFlow(options...) })
	registry.RegisterFlow(providerGitHubCopilot, func() OAuthFlow { return NewGitHubCopilotFlow(options...) })
	registry.RegisterFlow(providerOrcaRouter, func() OAuthFlow { return NewOrcaRouterFlow(options...) })
	registry.RegisterFlow(providerNous, func() OAuthFlow { return NewNousFlow(options...) })
	registry.RegisterFlow(providerCommandCode, func() OAuthFlow { return NewCommandCodeFlow(options...) })
	registry.RegisterFlow(providerQwen, func() OAuthFlow { return NewQwenFlow(options...) })
	registry.RegisterFlow(providerIFlow, func() OAuthFlow { return NewIFlowFlow(options...) })
	return registry
}
