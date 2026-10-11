package cursor

import (
	"testing"
)

func usableFixture(t *testing.T, models ...UsableModel) []byte {
	t.Helper()
	var out Writer
	for _, model := range models {
		var row Writer
		row.String(detailModelID, model.ID)
		row.String(detailDisplayName, model.Name)
		if model.Reasoning {
			row.Empty(detailThinking)
		}
		row.Bool(detailMaxMode, model.MaxMode)
		out.Message(usableModelsField, row.BytesOut())
	}
	return out.BytesOut()
}

func availableFixture(t *testing.T) []byte {
	t.Helper()
	var out Writer
	out.Message(2, richFixture(
		"sonnet", "Sonnet", []string{"sonnet-low", "sonnet-high"}, []paramFixture{
			{values: []Param{{ID: "effort", Value: "low"}}, slug: "sonnet-low"},
			{values: []Param{{ID: "effort", Value: "high"}}, slug: "sonnet-high", defNonMax: true},
		},
	))
	return out.BytesOut()
}

type paramFixture struct {
	values    []Param
	slug      string
	max       bool
	defMax    bool
	defNonMax bool
}

func richFixture(name, display string, slugs []string, variants []paramFixture) []byte {
	var row Writer
	row.String(richName, name)
	row.String(richClientName, display)
	row.Bool(richSupportsAgent, true)
	row.Bool(richThinking, true)
	row.Uint32(richContext, 200000)
	for _, slug := range slugs {
		row.String(richLegacySlugs, slug)
	}
	for _, variant := range variants {
		var encoded Writer
		for _, param := range variant.values {
			var pair Writer
			pair.String(paramID, param.ID)
			pair.String(paramValue, param.Value)
			encoded.Message(variantParams, pair.BytesOut())
		}
		encoded.Bool(variantMaxMode, variant.max)
		encoded.Bool(variantDefaultMax, variant.defMax)
		encoded.Bool(variantDefaultNonMax, variant.defNonMax)
		encoded.String(variantLegacySlug, variant.slug)
		row.Message(richVariants, encoded.BytesOut())
	}
	return row.BytesOut()
}

func TestParseUsableModels(t *testing.T) {
	body := usableFixture(t,
		UsableModel{ID: "sonnet", Name: "Sonnet", Reasoning: true},
		UsableModel{ID: "opus", Name: "Opus", MaxMode: true},
	)
	models, err := ParseUsableModels(Frame(body))
	if err != nil {
		t.Fatalf("ParseUsableModels() error = %v", err)
	}
	if len(models) != 2 || !models[0].Reasoning || !models[1].MaxMode {
		t.Fatalf("models = %+v", models)
	}
	raw, err := ParseUsableModels(body)
	if err != nil || len(raw) != 2 {
		t.Fatalf("ParseUsableModels(raw) = %+v %v", raw, err)
	}
}

func TestJoinFiltersEntitlement(t *testing.T) {
	usable, err := ParseUsableModels(usableFixture(t, UsableModel{ID: "sonnet-low", Name: "Sonnet"}))
	if err != nil {
		t.Fatalf("ParseUsableModels() error = %v", err)
	}
	rich, err := ParseAvailableModels(availableFixture(t))
	if err != nil {
		t.Fatalf("ParseAvailableModels() error = %v", err)
	}
	groups := GroupVariants(JoinModels(usable, true, rich))
	if len(groups) != 1 {
		t.Fatalf("groups = %+v, want only the entitled lane", groups)
	}
	if len(groups[0].Members) != 1 || groups[0].Members[0].DetailsID != "sonnet-low" {
		t.Fatalf("members = %+v", groups[0].Members)
	}
	fullUsable, err := ParseUsableModels(usableFixture(t,
		UsableModel{ID: "sonnet-low"}, UsableModel{ID: "sonnet-high"},
	))
	if err != nil {
		t.Fatalf("ParseUsableModels() error = %v", err)
	}
	full := GroupVariants(JoinModels(fullUsable, true, rich))
	if len(full) != 1 || len(full[0].Members) != 2 {
		t.Fatalf("full = %+v, want both efforts", full)
	}
}

func TestJoinWithoutUsableSkipsFiltering(t *testing.T) {
	rich, err := ParseAvailableModels(availableFixture(t))
	if err != nil {
		t.Fatalf("ParseAvailableModels() error = %v", err)
	}
	groups := GroupVariants(JoinModels(nil, false, rich))
	if len(groups) != 1 || len(groups[0].Members) != 2 {
		t.Fatalf("groups = %+v, want unfiltered lanes", groups)
	}
}

func TestBlockedVariantsAreSkipped(t *testing.T) {
	var row Writer
	row.String(richName, "sonnet")
	row.String(richClientName, "Sonnet")
	var definition Writer
	definition.String(definitionID, "effort")
	var paramType Writer
	var enum Writer
	var blocked Writer
	blocked.String(valueName, "high")
	blocked.Bool(enumBlocked, true)
	enum.Message(1, blocked.BytesOut())
	paramType.Message(enumValues, enum.BytesOut())
	definition.Message(definitionType, paramType.BytesOut())
	row.Message(richDefinitions, definition.BytesOut())
	row.Message(richVariants, richFixtureVariant([]Param{{ID: "effort", Value: "high"}}, "sonnet-high"))
	row.Message(richVariants, richFixtureVariant([]Param{{ID: "effort", Value: "low"}}, "sonnet-low"))
	var out Writer
	out.Message(2, row.BytesOut())

	rich, err := ParseAvailableModels(out.BytesOut())
	if err != nil {
		t.Fatalf("ParseAvailableModels() error = %v", err)
	}
	usable, err := ParseUsableModels(usableFixture(t,
		UsableModel{ID: "sonnet-high"}, UsableModel{ID: "sonnet-low"},
	))
	if err != nil {
		t.Fatalf("ParseUsableModels() error = %v", err)
	}
	groups := GroupVariants(JoinModels(usable, true, rich))
	if len(groups) != 1 || len(groups[0].Members) != 1 || groups[0].Members[0].DetailsID != "sonnet-low" {
		t.Fatalf("groups = %+v, want only the allowed variant", groups)
	}
}

func richFixtureVariant(params []Param, slug string) []byte {
	var encoded Writer
	for _, param := range params {
		var pair Writer
		pair.String(paramID, param.ID)
		pair.String(paramValue, param.Value)
		encoded.Message(variantParams, pair.BytesOut())
	}
	encoded.String(variantLegacySlug, slug)
	return encoded.BytesOut()
}

func TestParseContextValue(t *testing.T) {
	cases := map[string]int64{"1m": 1000000, "200k": 200000, "128000": 128000, "": 0, "none": 0}
	for input, want := range cases {
		if got := ParseContextValue(input); got != want {
			t.Errorf("ParseContextValue(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestGroupsToListedKeepsTuples(t *testing.T) {
	rich, err := ParseAvailableModels(availableFixture(t))
	if err != nil {
		t.Fatalf("ParseAvailableModels() error = %v", err)
	}
	usable, err := ParseUsableModels(usableFixture(t,
		UsableModel{ID: "sonnet-low"}, UsableModel{ID: "sonnet-high"},
	))
	if err != nil {
		t.Fatalf("ParseUsableModels() error = %v", err)
	}
	listed := GroupsToListed(GroupVariants(JoinModels(usable, true, rich)))
	if len(listed) != 1 {
		t.Fatalf("listed = %+v", listed)
	}
	entry := listed[0]
	if entry.ID != "sonnet" || len(entry.Cursor) != 2 {
		t.Fatalf("entry = %+v", entry)
	}
	if len(entry.ReasoningEfforts) != 2 || entry.ReasoningToggle != nil {
		t.Fatalf("efforts = %v toggle = %v", entry.ReasoningEfforts, entry.ReasoningToggle)
	}
	if entry.SupportsReasoning == nil || !*entry.SupportsReasoning {
		t.Fatalf("entry = %+v, want reasoning stated", entry)
	}
	if entry.ContextWindow == nil || *entry.ContextWindow != 200000 {
		t.Fatalf("entry = %+v, want the declared context", entry)
	}
}
