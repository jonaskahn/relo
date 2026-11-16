package claudebindings

import (
	"github.com/jonaskahn/relo/internal/catalog"
)

// Why a request was or was not bound.
const (
	ReasonBound    = "binding"
	ReasonDangling = "dangling"
	ReasonDisabled = "disabled"
)

// Resolution is what one request's model resolves to under the bindings.
type Resolution struct {
	// Matched reports that a binding applies and its target is reachable.
	Matched bool
	// Rewrite is the spelling of the target Relo's data plane resolves.
	Rewrite   string
	Direction string
	OnFailure string
	// Reason names why an unmatched request that had a binding was left alone.
	Reason string
}

// resolve applies one document to a model over a catalog snapshot. A binding
// whose target no longer exists is left alone, so the request goes to the
// vendor as it did before the binding was written.
func resolve(document Document, snapshot *catalog.Snapshot, model string) Resolution {
	binding, found := document.Lookup(model)
	if !found {
		return Resolution{}
	}
	if !binding.Enabled {
		return Resolution{Reason: ReasonDisabled}
	}
	if snapshot == nil {
		return Resolution{Reason: ReasonDangling}
	}
	spelling, reachable := spell(snapshot, binding.Target, catalog.HasMillionContextSuffix(model))
	if !reachable {
		return Resolution{Reason: ReasonDangling}
	}
	return Resolution{
		Matched: true, Rewrite: spelling,
		Direction: binding.Direction, OnFailure: binding.OnFailure, Reason: ReasonBound,
	}
}

// spell answers the name Relo's data plane resolves for a target, and whether
// the target can serve a request now. A request that carried the million-token
// marker keeps it, and the data plane reads it as it does on any Relo alias.
func spell(snapshot *catalog.Snapshot, target Target, million bool) (string, bool) {
	if million {
		window := int64(catalog.MillionContext)
		if name, ok := spellWith(snapshot, target, &window); ok {
			return name, true
		}
	}
	return spellWith(snapshot, target, nil)
}

func spellWith(snapshot *catalog.Snapshot, target Target, window *int64) (string, bool) {
	var name string
	switch target.Kind {
	case TargetRoute:
		group, found := snapshot.Group(target.GroupID)
		if !found || !group.Enabled || len(snapshot.GroupModels(group)) == 0 {
			return "", false
		}
		name = catalog.AnthropicClientID(catalog.ClientRouteID(group.ID), window)
	case TargetModel:
		model, found := snapshot.Model(target.ProviderID, target.ModelID)
		if !found || !snapshot.Eligible(model, catalog.Requirements{}) {
			return "", false
		}
		name = catalog.AnthropicModelAlias(model.ProviderID, model.ID, window)
	default:
		return "", false
	}
	if _, resolved := snapshot.ResolveClientID(name); !resolved {
		return "", false
	}
	return name, true
}
