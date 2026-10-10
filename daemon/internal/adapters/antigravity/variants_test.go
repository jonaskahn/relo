package antigravity

import "testing"

// TestCollapse pins every row the vendor names, so a table edit that moves a
// model off its SKU fails here rather than against the endpoint.
func TestCollapse(t *testing.T) {
	cases := []struct {
		name      string
		logical   string
		effort    string
		wire      string
		enum      string
		maxOut    int
		collapsed bool
	}{
		{"3.5 flash low", "gemini-3.5-flash", "none", "gemini-3.5-flash-extra-low", "MODEL_PLACEHOLDER_M187", maxOutputGemini, true},
		{"3.5 flash minimal", "gemini-3.5-flash", "minimal", "gemini-3.5-flash-extra-low", "MODEL_PLACEHOLDER_M187", maxOutputGemini, true},
		{"3.5 flash low effort", "gemini-3.5-flash", "low", "gemini-3.5-flash-extra-low", "MODEL_PLACEHOLDER_M187", maxOutputGemini, true},
		{"3.5 flash medium", "gemini-3.5-flash", "medium", "gemini-3.5-flash-low", "MODEL_PLACEHOLDER_M20", maxOutputGemini, true},
		{"3.5 flash high", "gemini-3.5-flash", "high", "gemini-3-flash-agent", "MODEL_PLACEHOLDER_M132", maxOutputGemini, true},

		{"3.7 flash low", "gemini-3.7-flash", "low", "gemini-3.7-flash-low", "", maxOutputGemini, true},
		{"3.7 flash medium", "gemini-3.7-flash", "medium", "gemini-3.7-flash-medium", "", maxOutputGemini, true},
		{"3.7 flash high", "gemini-3.7-flash", "high", "gemini-3.7-flash-high", "", maxOutputGemini, true},

		{"pro low", "gemini-3.1-pro", "low", "gemini-3.1-pro-low", "MODEL_PLACEHOLDER_M36", maxOutputPro, true},
		{"pro off", "gemini-3.1-pro", "none", "gemini-3.1-pro-low", "MODEL_PLACEHOLDER_M36", maxOutputPro, true},
		{"pro high", "gemini-3.1-pro", "high", "gemini-pro-agent", "MODEL_PLACEHOLDER_M16", maxOutputPro, true},

		{"sonnet low", "claude-sonnet-4-6", "low", "claude-sonnet-4-6", "", maxOutputClaude, true},
		{"sonnet high", "claude-sonnet-4-6", "high", "claude-sonnet-4-6", "", maxOutputClaude, true},
		{"opus low", "claude-opus-4-6", "minimal", "claude-opus-4-6-thinking", "", maxOutputClaude, true},
		{"opus high", "claude-opus-4-6", "max", "claude-opus-4-6-thinking", "", maxOutputClaude, true},

		{"the ladder above the top band clamps down", "gemini-3.5-flash", "max", "gemini-3-flash-agent", "MODEL_PLACEHOLDER_M132", maxOutputGemini, true},

		{"a model the table omits stays itself", "gemini-2.0-flash", "high", "", "", 0, false},
		{"an effort the ladder omits takes the middle band", "gemini-3.5-flash", "extreme", "gemini-3.5-flash-low", "MODEL_PLACEHOLDER_M20", maxOutputGemini, true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			variant, collapsed := Collapse(test.logical, test.effort)
			if collapsed != test.collapsed {
				t.Fatalf("collapsed = %v, want %v", collapsed, test.collapsed)
			}
			if !test.collapsed {
				return
			}
			if variant.WireModel != test.wire {
				t.Fatalf("WireModel = %q, want %q", variant.WireModel, test.wire)
			}
			if variant.ModelEnum != test.enum {
				t.Fatalf("ModelEnum = %q, want %q", variant.ModelEnum, test.enum)
			}
			if variant.MaxOutput != test.maxOut {
				t.Fatalf("MaxOutput = %d, want %d", variant.MaxOutput, test.maxOut)
			}
		})
	}
}

// TestCollapseLeavesUnnamedBandsAlone covers the pair the vendor names no SKU
// for: a row is not invented for it.
func TestCollapseLeavesUnnamedBandsAlone(t *testing.T) {
	variant, collapsed := Collapse("gemini-3.1-pro", "medium")
	if collapsed {
		t.Fatalf("Collapse(gemini-3.1-pro, medium) = %+v, want no row", variant)
	}
}
