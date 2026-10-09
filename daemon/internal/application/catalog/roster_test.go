package catalog

import (
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
)

// listingTemplate is an API key row: a connection whose provider publishes a
// model list of its own.
func listingTemplate() catalog.Template {
	return catalog.Template{
		ID:                  "example",
		Kind:                catalog.KindKey,
		ModelsSource:        "listing",
		ModelsFormat:        catalog.ModelsOpenAI,
		ModelsDevProviderID: "example",
	}
}

// typedTemplate is a connection that publishes no model list, so its models
// are the ids an operator types.
func typedTemplate() catalog.Template {
	return catalog.Template{
		ID:                  "azure-openai",
		Kind:                catalog.KindCloud,
		ModelsSource:        "manual",
		ModelsFormat:        catalog.ModelsNone,
		ModelsDevProviderID: "azure",
	}
}

func sourcesOf(rows []rosterEntry) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ID+"="+row.Source)
	}
	return out
}

// TestBuildRosterKeepsWhatTheProviderLists is the rule the roster exists for:
// a model the provider's own list names is a model the connection serves, and
// it arrives with the provider's own name.
func TestBuildRosterKeepsWhatTheProviderLists(t *testing.T) {
	template := listingTemplate()

	rows := buildRoster(template, rosterModeFor(template),
		[]catalog.Listed{{ID: "live-1", Name: "Live One"}, {ID: "live-2", Name: "Live Two"}}, nil)

	want := []string{"live-1=listing", "live-2=listing"}
	got := sourcesOf(rows)
	if len(got) != len(want) {
		t.Fatalf("roster = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("roster = %v, want %v", got, want)
		}
	}
	if rows[0].Name != "Live One" {
		t.Fatalf("listed name = %q, want the provider's own", rows[0].Name)
	}
}

// TestBuildRosterLeavesATypedIdOffAConnectionThatLists covers the half of the
// rule that used to be wrong: an id nothing listed is not a model the
// connection serves, however the operator came by it.
func TestBuildRosterLeavesATypedIdOffAConnectionThatLists(t *testing.T) {
	template := listingTemplate()

	rows := buildRoster(template, rosterModeFor(template), nil,
		[]catalog.Listed{{ID: "typed-by-hand", Name: "Typed By Hand"}})

	if len(rows) != 0 {
		t.Fatalf("roster = %v, want no id the provider did not list", sourcesOf(rows))
	}
}

// TestBuildRosterKeepsOneRowPerId covers a provider that lists the same id
// twice, which is one model rather than two.
func TestBuildRosterKeepsOneRowPerId(t *testing.T) {
	template := listingTemplate()

	rows := buildRoster(template, rosterModeFor(template),
		[]catalog.Listed{{ID: "shared", Name: "Provider Name"}, {ID: "shared", Name: "Provider Name"}}, nil)

	if len(rows) != 1 || rows[0].Source != SourceListing {
		t.Fatalf("roster = %v, want one row per id", sourcesOf(rows))
	}
	if rows[0].Name != "Provider Name" {
		t.Fatalf("name = %q, want the provider's own", rows[0].Name)
	}
}

// TestBuildRosterAppliesTheTemplateFilterToBothSources covers the filter a
// template declares, which narrows a listing as much as it narrows what an
// operator may type.
func TestBuildRosterAppliesTheTemplateFilterToBothSources(t *testing.T) {
	listing := listingTemplate()
	listing.FilterModels = func(id string) bool { return len(id) > 4 }
	listed := buildRoster(listing, rosterModeFor(listing),
		[]catalog.Listed{{ID: "a"}, {ID: "gpt-6.1"}}, nil)
	if got := sourcesOf(listed); len(got) != 1 || got[0] != "gpt-6.1=listing" {
		t.Fatalf("listed roster = %v, want the id the filter keeps", got)
	}

	typed := typedTemplate()
	typed.FilterModels = func(id string) bool { return len(id) > 4 }
	typedRows := buildRoster(typed, rosterModeFor(typed), nil,
		[]catalog.Listed{{ID: "ab"}, {ID: "my-deployment"}})
	if got := sourcesOf(typedRows); len(got) != 1 || got[0] != "my-deployment=manual" {
		t.Fatalf("typed roster = %v, want the id the filter keeps", got)
	}
}

// TestBuildRosterRecordsTheFormatEachModelAnswersOn covers a provider that
// publishes one model list across several protocols: the wire format belongs to
// the model, and a template that says nothing about a model leaves the
// connection's own format in place.
func TestBuildRosterRecordsTheFormatEachModelAnswersOn(t *testing.T) {
	template := listingTemplate()
	template.FormatForModel = func(id string) catalog.APIFormat {
		if id == "reaches-via-responses" {
			return catalog.FormatOpenAIResp
		}
		return ""
	}

	rows := buildRoster(template, rosterModeFor(template),
		[]catalog.Listed{{ID: "reaches-via-responses"}, {ID: "exo-free"}}, nil)

	if len(rows) != 2 {
		t.Fatalf("roster = %v, want both listed ids", sourcesOf(rows))
	}
	if rows[0].ID != "reaches-via-responses" || rows[0].Format != catalog.FormatOpenAIResp {
		t.Fatalf("first row = %+v, want the Responses format the template declared", rows[0])
	}
	if rows[1].Format != "" {
		t.Fatalf("second row = %+v, want the connection's own format to stand", rows[1])
	}
}

// TestBuildRosterKeepsOnlyTypedModelsWhenTheProviderServesDeployments covers
// the connection a listing cannot describe: whatever a dialect answers with,
// the roster is what the operator typed.
func TestBuildRosterKeepsOnlyTypedModelsWhenTheProviderServesDeployments(t *testing.T) {
	template := typedTemplate()

	rows := buildRoster(template, rosterModeFor(template),
		[]catalog.Listed{{ID: "listed"}},
		[]catalog.Listed{{ID: "my-deployment", Name: "my-deployment"}})

	if len(rows) != 1 || rows[0].ID != "my-deployment" {
		t.Fatalf("roster = %v, want the typed deployment alone", sourcesOf(rows))
	}
	if rows[0].Source != SourceManual {
		t.Fatalf("source = %q, want the typed id tagged as the operator's own", rows[0].Source)
	}
}

func TestRosterModeFollowsTheTemplate(t *testing.T) {
	cases := []struct {
		name     string
		template catalog.Template
		want     rosterMode
	}{
		{"azure deployments", catalog.Template{ModelsSource: "manual", ModelsFormat: catalog.ModelsNone}, modeManual},
		{"vertex", catalog.Template{Kind: catalog.KindCloud, ModelsSource: "manual", ModelsFormat: catalog.ModelsNone}, modeManual},
		{"kiro", catalog.Template{Kind: catalog.KindSignIn, ModelsSource: "manual", ModelsFormat: catalog.ModelsNone}, modeManual},
		{"a row stored before the templates changed", catalog.Template{}, modeListing},
		{"an api key row", catalog.Template{Kind: catalog.KindKey, ModelsSource: "listing", ModelsFormat: catalog.ModelsOpenAI}, modeListing},
		{"bedrock", catalog.Template{ID: "amazon-bedrock", Kind: catalog.KindCloud, ModelsSource: "listing", ModelsFormat: catalog.ModelsBedrock}, modeListing},
		{"a local preset", catalog.Template{Kind: catalog.KindLocal, ModelsSource: "listing", ModelsFormat: catalog.ModelsOpenAI}, modeListing},
		{"an account sign-in that lists", catalog.Template{Kind: catalog.KindSignIn, ModelsSource: "listing", ModelsFormat: catalog.ModelsOpenAI}, modeListing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rosterModeFor(tc.template); got != tc.want {
				t.Fatalf("rosterModeFor() = %d, want %d", got, tc.want)
			}
		})
	}
}
