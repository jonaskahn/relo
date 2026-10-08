// Package modelsdev keeps the local copy of the models.dev dataset: it
// downloads the catalog, parses it, and saves it for offline reads.
package modelsdev

import "github.com/jonaskahn/relo/internal/catalog"

// MatchType describes how a model ID matched against models.dev.
type MatchType = catalog.ModelsDevMatch

// Match kinds name how a model met the index: exactly, after folding, by
// vendor alias, by operator hand, or not at all.
const (
	MatchExact      = catalog.ModelsDevMatchExact
	MatchNormalized = catalog.ModelsDevMatchNormalized
	MatchVendor     = catalog.ModelsDevMatchVendor
	MatchManual     = catalog.ModelsDevMatchManual
	MatchNone       = catalog.ModelsDevMatchNone
)
