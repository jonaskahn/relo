package cursor

import "testing"

// TestToolDescriptionNamesTheClientTool covers the declaration's description:
// a canonical tool adds its name ahead of the client's own text, an empty
// description leaves the label alone, and an unnamed tool keeps the text.
func TestToolDescriptionNamesTheClientTool(t *testing.T) {
	cases := []struct {
		name        string
		canonical   string
		description string
		want        string
	}{
		{name: "both", canonical: "read", description: "read a file", want: "External client tool `read`. read a file"},
		{name: "no description", canonical: "read", want: "External client tool `read`."},
		{name: "no canonical name", description: "read a file", want: "read a file"},
		{name: "neither", want: ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := toolDescription(test.canonical, test.description); got != test.want {
				t.Fatalf("toolDescription() = %q, want %q", got, test.want)
			}
		})
	}
}
