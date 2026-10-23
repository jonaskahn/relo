// The wire SKU each logical model collapses onto per effort. Gemini models
// and the Claude 5.5 generation collapse; an earlier Claude generation passes
// through unchanged and never consults this table.
package antigravity

import "github.com/jonaskahn/relo/internal/catalog"

// The vendor exposes every thinking effort as its own backend SKU, so a
// logical model never reaches the wire unchanged. MaxOutput is the ceiling
// that SKU accepts; the endpoint refuses a larger one with a 400.
const (
	maxOutputGemini = 65536
	maxOutputPro    = 65535
	maxOutputClaude = 64000

	// OpenThinkingBudget is the budget the endpoint accepts for a model that
	// chooses its own thinking length. Discovery publishes it per SKU, and it is
	// the fallback for a SKU whose budget Relo has not read yet.
	OpenThinkingBudget = -1
)

// A row the vendor has not given an enum leaves ModelEnum empty, so the label
// stays off the request rather than filled with a guess. A (model, band) pair
// the vendor does not name is not a row to infer either: Collapse reports it
// absent and the logical name goes on the wire untouched. Claude models have
// no rows: they always go on the wire untouched.
var variants = map[string]map[string]Variant{
	"gemini-3.5-flash": {
		catalog.AntigravityBandLow:    {WireModel: "gemini-3.5-flash-extra-low", ModelEnum: "MODEL_PLACEHOLDER_M187", MaxOutput: maxOutputGemini},
		catalog.AntigravityBandMedium: {WireModel: "gemini-3.5-flash-low", ModelEnum: "MODEL_PLACEHOLDER_M20", MaxOutput: maxOutputGemini},
		catalog.AntigravityBandHigh:   {WireModel: "gemini-3-flash-agent", ModelEnum: "MODEL_PLACEHOLDER_M132", MaxOutput: maxOutputGemini},
	},
	"gemini-3.6-flash": {
		catalog.AntigravityBandLow:    {WireModel: "gemini-3.6-flash-low", MaxOutput: maxOutputGemini},
		catalog.AntigravityBandMedium: {WireModel: "gemini-3.6-flash-medium", MaxOutput: maxOutputGemini},
		catalog.AntigravityBandHigh:   {WireModel: "gemini-3.6-flash-high", MaxOutput: maxOutputGemini},
	},
	"gemini-3.7-flash": {
		catalog.AntigravityBandLow:    {WireModel: "gemini-3.7-flash-low", MaxOutput: maxOutputGemini},
		catalog.AntigravityBandMedium: {WireModel: "gemini-3.7-flash-medium", MaxOutput: maxOutputGemini},
		catalog.AntigravityBandHigh:   {WireModel: "gemini-3.7-flash-high", MaxOutput: maxOutputGemini},
	},
	"gemini-3.8-flash": {
		catalog.AntigravityBandLow:    {WireModel: "gemini-3.8-flash-low", MaxOutput: maxOutputGemini},
		catalog.AntigravityBandMedium: {WireModel: "gemini-3.8-flash-medium", MaxOutput: maxOutputGemini},
		catalog.AntigravityBandHigh:   {WireModel: "gemini-3.8-flash-high", MaxOutput: maxOutputGemini},
	},
	"gemini-3.1-pro": {
		catalog.AntigravityBandLow:  {WireModel: "gemini-3.1-pro-low", ModelEnum: "MODEL_PLACEHOLDER_M36", MaxOutput: maxOutputPro},
		catalog.AntigravityBandHigh: {WireModel: "gemini-pro-agent", ModelEnum: "MODEL_PLACEHOLDER_M16", MaxOutput: maxOutputPro},
	},
	"claude-sonnet-5-5": {
		catalog.AntigravityBandLow:    {WireModel: "claude-sonnet-5-5-low", MaxOutput: maxOutputClaude},
		catalog.AntigravityBandMedium: {WireModel: "claude-sonnet-5-5-medium", MaxOutput: maxOutputClaude},
		catalog.AntigravityBandHigh:   {WireModel: "claude-sonnet-5-5-high", MaxOutput: maxOutputClaude},
	},
	"claude-opus-5-5": {
		catalog.AntigravityBandLow:    {WireModel: "claude-opus-5-5-low", MaxOutput: maxOutputClaude},
		catalog.AntigravityBandMedium: {WireModel: "claude-opus-5-5-medium", MaxOutput: maxOutputClaude},
		catalog.AntigravityBandHigh:   {WireModel: "claude-opus-5-5-high", MaxOutput: maxOutputClaude},
	},
}

// Variant is the wire identity one logical model collapses onto at one
// thinking effort.
type Variant struct {
	WireModel string
	ModelEnum string
	MaxOutput int
	// ThinkingBudget is the budget the vendor publishes for this SKU. Nil
	// leaves the request open-ended rather than guessing at a budget the
	// endpoint may not serve.
	ThinkingBudget *int
}

// ClaudeVariant reports the wire identity one Claude model sends when the
// table names no band for it: its own id under the vendor's output ceiling.
func ClaudeVariant(model string) Variant {
	return Variant{WireModel: model, MaxOutput: maxOutputClaude}
}

// Collapse reports the wire identity a logical model collapses onto at one
// canonical effort, with false for a pair the table does not name.
func Collapse(logical, effort string) (Variant, bool) {
	variant, found := variants[logical][catalog.AntigravityBand(effort)]
	return variant, found
}

// Variants reports every wire identity one logical model collapses onto,
// keyed by band.
func Variants(logical string) map[string]Variant {
	rows, found := variants[logical]
	if !found {
		return nil
	}
	duplicated := make(map[string]Variant, len(rows))
	for band, variant := range rows {
		duplicated[band] = variant
	}
	return duplicated
}

// Logical reports the logical model one wire SKU belongs to.
func Logical(wireID string) (string, bool) {
	for logical, rows := range variants {
		for _, variant := range rows {
			if variant.WireModel == wireID {
				return logical, true
			}
		}
	}
	return "", false
}
