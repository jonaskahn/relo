package catalog

import "testing"

// TestFormatModelLabelGathersQualifiers pins that a label keeps one group of
// qualifiers at its end, however they arrived: the window mark, the variant
// lane, and whatever the provider already parenthesised all land in the same
// group rather than stacking parentheses on the name.
func TestFormatModelLabelGathersQualifiers(t *testing.T) {
	million := int64(MillionContext)
	small := int64(DefaultContext)
	cases := []struct {
		name    string
		label   string
		variant string
		window  *int64
		want    string
	}{
		{name: "a plain name stands", label: "Opus 5.5", want: "Opus 5.5"},
		{name: "a million window is marked", label: "Opus 5.5", window: &million, want: "Opus 5.5 (1M)"},
		{name: "a window below a million is not marked", label: "Opus 5.5", window: &small, want: "Opus 5.5"},
		{name: "a lane joins the group", label: "Opus 5.5", variant: "Max", want: "Opus 5.5 (Max)"},
		{name: "a parenthesised lane joins the group", label: "Opus 5.5", variant: "(Max)", want: "Opus 5.5 (Max)"},
		{
			// The lane comes before the provider's own qualifier, so the group
			// reads in the order Relo knows them rather than the order they
			// arrived in.
			name:  "the provider's own qualifier joins the group",
			label: "Gemini 3.8 Flash (Medium)", variant: "Max", want: "Gemini 3.8 Flash (Max, Medium)",
		},
		{
			name:  "the same qualifier is not repeated",
			label: "Gemini 3.8 Flash (1M)", window: &million, want: "Gemini 3.8 Flash (1M)",
		},
		// A label that is nothing but a qualifier has no base to hang it on, so
		// the group is the whole label. A trailing bracket that opens nothing
		// is part of the name rather than an unterminated qualifier.
		{name: "a label that is only a qualifier", label: "(1M)", window: &million, want: "(1M)"},
		{name: "an unopened bracket is part of the name", label: "Opus 5.5)", window: &million, want: "Opus 5.5) (1M)"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := FormatModelLabel(test.label, test.window, test.variant); got != test.want {
				t.Fatalf("FormatModelLabel(%q, %v) = %q, want %q", test.label, test.window, got, test.want)
			}
		})
	}
}
