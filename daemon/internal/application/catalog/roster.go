package catalog

import "github.com/jonaskahn/relo/internal/catalog"

// The sources a stored model row records. A roster draws its ids from the
// provider's own listing or from an operator who typed them, and from nothing
// else: modelsdev is what rows an older build added from the model catalog
// keep, and no code writes it now.
const (
	SourceListing   = "listing"
	SourceModelsDev = "modelsdev"
	SourceManual    = "manual"
)

const modelsSourceManual = "manual"

type rosterMode int

const (
	// modeManual records only the models an operator typed. Azure OpenAI
	// serves deployment names, and Vertex AI and Kiro publish no model list at
	// all, so no id a listing returns names a model the connection serves.
	modeManual rosterMode = iota
	modeListing
)

func rosterModeFor(t catalog.Template) rosterMode {
	if t.ModelsSource == modelsSourceManual && t.ModelsFormat == catalog.ModelsNone {
		return modeManual
	}
	return modeListing
}

type rosterEntry struct {
	catalog.Listed
	// Source is the source the id came from.
	Source string
	// Format is the upstream wire format this model answers on, empty when the
	// connection's own format already applies.
	Format catalog.APIFormat
}

func buildRoster(t catalog.Template, mode rosterMode, listed, typed []catalog.Listed) []rosterEntry {
	rows := make([]rosterEntry, 0, len(listed)+len(typed))
	seen := make(map[string]bool, len(listed)+len(typed))

	// A claimed id carries the format the template declares for it, and an
	// empty one so the connection's own format still applies.
	claim := func(id string) (catalog.APIFormat, bool) {
		if id == "" || seen[id] {
			return "", false
		}
		if t.FilterModels != nil && !t.FilterModels(id) {
			return "", false
		}
		seen[id] = true
		if t.FormatForModel == nil {
			return "", true
		}
		return t.FormatForModel(id), true
	}

	if mode != modeManual {
		return appendRosterRows(rows, listed, SourceListing, claim)
	}
	return appendRosterRows(rows, typed, SourceManual, claim)
}

func appendRosterRows(rows []rosterEntry, listed []catalog.Listed, source string, claim func(string) (catalog.APIFormat, bool)) []rosterEntry {
	for _, l := range listed {
		format, claimed := claim(l.ID)
		if !claimed {
			continue
		}
		rows = append(rows, rosterEntry{Listed: l, Source: source, Format: format})
	}
	return rows
}
