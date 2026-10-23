package catalog

import (
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
)

// TestMarkRefreshSuperseded covers the rows a collapsed model replaces: a
// listed row under one of its backend SKUs goes, while the collapsed model, a
// model an operator typed, and a clone all stay.
func TestMarkRefreshSuperseded(t *testing.T) {
	assembly := refreshAssembly{
		seen:    map[string]bool{"claude-sonnet-5-5": true},
		members: map[string]bool{"claude-sonnet-5-5-low": true, "claude-sonnet-5-5-medium": true},
	}
	existing := []catalog.Model{
		{ID: "claude-sonnet-5-5", Source: SourceListing},
		{ID: "claude-sonnet-5-5-low", Source: SourceListing},
		{ID: "claude-sonnet-5-5-medium", Source: SourceManual},
		{ID: "claude-sonnet-5-5-high", Source: SourceListing, ClonedFrom: "claude-sonnet-5-5"},
		{ID: "gemini-3.8-flash", Source: SourceListing},
	}
	superseded := markRefreshSuperseded(existing, assembly)
	if len(superseded) != 1 || superseded[0] != "claude-sonnet-5-5-low" {
		t.Fatalf("superseded = %v, want the listed backend row alone", superseded)
	}
}

// TestCollectMembersNamesTheBackendSkus pins where the superseded ids come
// from: the wire models a collapsed model's own request tuples resolve to.
func TestCollectMembersNamesTheBackendSkus(t *testing.T) {
	entry := rosterEntry{Listed: catalog.Listed{
		ID: "claude-sonnet-5-5",
		RequestConfigs: []catalog.ModelRequestConfig{
			catalog.EncodeAntigravityConfig(catalog.AntigravityVariant{
				LogicalID: "claude-sonnet-5-5", Selection: "low", WireModel: "claude-sonnet-5-5-low",
			}),
			catalog.EncodeAntigravityConfig(catalog.AntigravityVariant{
				LogicalID: "claude-sonnet-5-5", Selection: "medium", WireModel: "claude-sonnet-5-5-medium",
			}),
		},
	}}
	assembly := refreshAssembly{members: map[string]bool{}}
	assembly.collectMembers(entry)
	for _, wire := range []string{"claude-sonnet-5-5-low", "claude-sonnet-5-5-medium"} {
		if !assembly.members[wire] {
			t.Fatalf("members = %v, want %s named", assembly.members, wire)
		}
	}
	if len(assembly.members) != 2 {
		t.Fatalf("members = %v, want only the backend SKUs", assembly.members)
	}
}
