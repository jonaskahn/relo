package platform

import (
	"context"
	"testing"

	wireformats "github.com/jonaskahn/relo/internal/adapters/wire/formats"
	"github.com/jonaskahn/relo/internal/catalog"
)

// stubCatalogReader serves one stored connection and nothing else, which is
// all a template lookup reads.
type stubCatalogReader struct {
	connections []catalog.ConnectionRecord
}

func (r stubCatalogReader) ListConnections(context.Context) ([]catalog.ConnectionRecord, error) {
	return r.connections, nil
}

func (stubCatalogReader) ListModels(context.Context) ([]catalog.ModelRecord, error) {
	return nil, nil
}

func (stubCatalogReader) ListModelFacts(context.Context) ([]catalog.FactsRecord, error) {
	return nil, nil
}

func (stubCatalogReader) ListRoutes(context.Context) ([]catalog.RouteRecord, error) {
	return nil, nil
}

// TestWiringTemplateID covers the resolver both the login flow registry and
// the credential source share: a connection created from a template signs in
// as that template, because that is the vendor whose credential it holds.
func TestWiringTemplateID(t *testing.T) {
	t.Run("a connection created from a template answers with it", func(t *testing.T) {
		w := wiringWithCatalog(t, "claude-work", "claude")
		if got := w.templateID("claude-work"); got != "claude" {
			t.Fatalf("templateID() = %q, want the template the connection was created from", got)
		}
	})

	t.Run("a connection created from itself answers with itself", func(t *testing.T) {
		w := wiringWithCatalog(t, "kimi", "kimi")
		if got := w.templateID("kimi"); got != "kimi" {
			t.Fatalf("templateID() = %q, want the connection itself", got)
		}
	})

	t.Run("a connection the catalog does not resolve is its own template", func(t *testing.T) {
		w := wiringWithCatalog(t, "claude", "claude")
		if got := w.templateID("no-such-connection"); got != "no-such-connection" {
			t.Fatalf("templateID() = %q, want the unresolved connection", got)
		}
	})

	t.Run("a catalog that never loaded is its own template", func(t *testing.T) {
		w := &wiring{models: catalog.New(stubCatalogReader{}, nil, wireformats.New())}
		if got := w.templateID("claude"); got != "claude" {
			t.Fatalf("templateID() = %q, want the connection itself", got)
		}
	})
}

func wiringWithCatalog(t *testing.T, id, templateID string) *wiring {
	t.Helper()
	models := catalog.New(stubCatalogReader{connections: []catalog.ConnectionRecord{{
		ID: id, TemplateID: templateID, Enabled: true,
	}}}, nil, wireformats.New())
	if err := models.Reload(context.Background()); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	return &wiring{models: models}
}
