// Kilo Free declares the keyless Kilo gateway connection and its model policy.
package templates

import (
	"github.com/jonaskahn/relo/internal/catalog"
)

const kiloGatewayURL = "https://api.kilo.ai/api/gateway"

var kiloFreeTemplates = []Template{
	{
		ID: catalog.KiloFreeTemplate, Label: "Kilo Free", Kind: KindKey,
		Origin: catalog.OriginTemplate, Auth: catalog.AuthNone, KeyHeader: catalog.KeyHeaderNone,
		DefaultFormat: catalog.FormatOpenAIChat, DefaultBaseURL: kiloGatewayURL,
		ModelsSource: "listing", ModelsFormat: catalog.ModelsOpenAI, ModelsDevProviderID: "kilo",
		FilterModels: kiloFreeModel,
		AvailableFormats: []FormatOption{{
			Format: catalog.FormatOpenAIChat, DefaultBaseURL: kiloGatewayURL,
			KeyHeader: catalog.KeyHeaderNone, ModelsFormat: catalog.ModelsOpenAI,
			Label: "OpenAI Chat Completions",
		}},
	},
}

// kiloFreeModel keeps what the gateway marks free. The flag is the only
// honest test, because the pool publishes free models under ids that state
// no price: inclusionai/ling-3.1-flash and stealth/glyph-cluster are free
// and a suffix test would drop them. A listing that never states the flag
// proves nothing, so it is not free access.
func kiloFreeModel(model catalog.Listed) bool {
	return model.IsFree != nil && *model.IsFree
}
