// Model discovery: reading the rosters providers publish.
package catalog

import (
	"context"
	"fmt"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
)

// DiscoverModels lists the models one provider publishes.
func (s *Service) DiscoverModels(ctx context.Context, host catalog.Provider) ([]catalog.Listed, error) {
	return s.discoverWith(ctx, host, "")
}

// DiscoverAccountModels lists the models one named account publishes, which
// is the account's own entitlement rather than the connection's union.
func (s *Service) DiscoverAccountModels(ctx context.Context, host catalog.Provider, credentialID string) ([]catalog.Listed, error) {
	return s.discoverWith(ctx, host, credentialID)
}

func (s *Service) discoverWith(ctx context.Context, host catalog.Provider, credentialID string) ([]catalog.Listed, error) {
	if s.discover == nil {
		return nil, catalog.ErrListUnsupported
	}
	authorization, baseURL, err := s.discoveryCredential(ctx, host, credentialID)
	if err != nil {
		return nil, err
	}
	if baseURL != "" {
		host.BaseURL = baseURL
	}
	listed, err := s.discover.List(s.withProxy(ctx, host.UseProxy), catalog.ListTarget{
		Format: host.ModelsFormat, BaseURL: host.BaseURL,
		Auth: authorization, Headers: host.Headers,
		OpenCodeFree: host.TemplateID == catalog.OpenCodeFreeTemplate,
	})
	if err != nil {
		return nil, fmt.Errorf("list the models of %s: %w", host.ID, err)
	}
	return listed, nil
}

func (s *Service) discoveryCredential(ctx context.Context, host catalog.Provider, credentialID string) (catalog.Authorization, string, error) {
	if s.credentials == nil {
		return catalog.Authorization{}, "", nil
	}
	request := catalog.AuthRequest{
		ProviderID: host.ID,
		Auth:       host.Auth,
		KeyHeader:  host.KeyHeader,
		Selection:  account.Selection{Surface: "discovery"},
	}
	if credentialID != "" {
		authorized, err := s.credentials.AuthorizeCredential(ctx, request, credentialID)
		if err != nil {
			return catalog.Authorization{}, "", err
		}
		return authorized, authorized.BaseURL, nil
	}
	authorized, err := s.credentials.Authorize(ctx, request)
	if err != nil {
		return catalog.Authorization{}, "", err
	}
	return authorized, authorized.BaseURL, nil
}

func (s *Service) withProxy(ctx context.Context, use bool) context.Context {
	if !use || s.proxyURL == nil {
		return ctx
	}
	return config.WithOutboundProxy(ctx, s.proxyURL())
}

func (s *Service) probeUsesProxy(ctx context.Context, templateID, providerID string) bool {
	if providerID != "" {
		row, err := s.store.GetProvider(ctx, providerID)
		if err == nil {
			return row.UseProxy
		}
	}
	return templateID == catalog.OpenCodeFreeTemplate
}

// SignInFlows returns the flows a provider's sign-in runs, in order.
func (s *Service) SignInFlows(host catalog.Provider) []string {
	if host.Auth == catalog.AuthOAuth {
		return host.LoginFlows
	}
	return nil
}
