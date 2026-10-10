// The wire SKU each logical Antigravity model collapses onto per effort.
package antigravity

import "slices"

// The vendor exposes every thinking effort as its own backend SKU, so a
// logical model never reaches the wire unchanged. MaxOutput is the ceiling
// that SKU accepts; the endpoint refuses a larger one with a 400.
const (
	maxOutputGemini = 65536
	maxOutputPro    = 65535
	maxOutputClaude = 64000
)

// The vendor bands are named low, medium, and high. The canonical effort
// ladder is longer, so each band lists the efforts answering for it and the
// ladder's top steps join the highest band.
const (
	bandLow    = "low"
	bandMedium = "medium"
	bandHigh   = "high"
)

var effortBands = []struct {
	band    string
	efforts []string
}{
	{bandLow, []string{"none", "minimal", "low"}},
	{bandMedium, []string{"medium"}},
	{bandHigh, []string{"high", "xhigh", "max"}},
}

// A row the vendor has not given an enum leaves ModelEnum empty, so the label
// stays off the request rather than filled with a guess. A (model, band) pair
// the vendor does not name is not a row to infer either: Collapse reports it
// absent and the logical name goes on the wire untouched.
var variants = map[string]map[string]Variant{
	"gemini-3.5-flash": {
		bandLow:    {WireModel: "gemini-3.5-flash-extra-low", ModelEnum: "MODEL_PLACEHOLDER_M187", MaxOutput: maxOutputGemini},
		bandMedium: {WireModel: "gemini-3.5-flash-low", ModelEnum: "MODEL_PLACEHOLDER_M20", MaxOutput: maxOutputGemini},
		bandHigh:   {WireModel: "gemini-3-flash-agent", ModelEnum: "MODEL_PLACEHOLDER_M132", MaxOutput: maxOutputGemini},
	},
	"gemini-3.7-flash": {
		bandLow:    {WireModel: "gemini-3.7-flash-low", MaxOutput: maxOutputGemini},
		bandMedium: {WireModel: "gemini-3.7-flash-medium", MaxOutput: maxOutputGemini},
		bandHigh:   {WireModel: "gemini-3.7-flash-high", MaxOutput: maxOutputGemini},
	},
	"gemini-3.1-pro": {
		bandLow:  {WireModel: "gemini-3.1-pro-low", ModelEnum: "MODEL_PLACEHOLDER_M36", MaxOutput: maxOutputPro},
		bandHigh: {WireModel: "gemini-pro-agent", ModelEnum: "MODEL_PLACEHOLDER_M16", MaxOutput: maxOutputPro},
	},
	"claude-sonnet-4-6": {
		bandLow:    {WireModel: "claude-sonnet-4-6", MaxOutput: maxOutputClaude},
		bandMedium: {WireModel: "claude-sonnet-4-6", MaxOutput: maxOutputClaude},
		bandHigh:   {WireModel: "claude-sonnet-4-6", MaxOutput: maxOutputClaude},
	},
	"claude-opus-4-6": {
		bandLow:    {WireModel: "claude-opus-4-6-thinking", MaxOutput: maxOutputClaude},
		bandMedium: {WireModel: "claude-opus-4-6-thinking", MaxOutput: maxOutputClaude},
		bandHigh:   {WireModel: "claude-opus-4-6-thinking", MaxOutput: maxOutputClaude},
	},
}

// Variant is the wire identity one logical model collapses onto at one
// thinking effort.
type Variant struct {
	WireModel string
	ModelEnum string
	MaxOutput int
}

// Collapse reports the wire identity a logical model collapses onto at one
// canonical effort, with false for a pair the table does not name.
func Collapse(logical, effort string) (Variant, bool) {
	variant, found := variants[logical][bandOf(effort)]
	return variant, found
}

func bandOf(effort string) string {
	for _, entry := range effortBands {
		if slices.Contains(entry.efforts, effort) {
			return entry.band
		}
	}
	return bandMedium
}
