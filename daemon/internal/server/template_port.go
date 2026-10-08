// Template lookup: the connection shapes the data plane branches on.
package server

import (
	"github.com/jonaskahn/relo/internal/catalog"
)

// Templates resolves the curated connection shapes the relay branches on:
// which upstream needs a stream, which refuses an output ceiling, and which
// login flow names which connection. The composition root backs it with the
// template registry, so this package never imports that adapter.
type Templates interface {
	// Curated finds a hand-written template by ID.
	Curated(id string) (catalog.Template, bool)
	// ByFlow finds the sign-in template whose login declares a flow.
	ByFlow(flow string) (catalog.Template, bool)
}

const codexTemplateID = "openai-codex"
