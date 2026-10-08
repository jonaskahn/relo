// OpenCode Free declares the signed-out Zen connection and its model policy.
package templates

import (
	"github.com/jonaskahn/relo/internal/adapters/modelsdev"
	"github.com/jonaskahn/relo/internal/catalog"
)

var freeTemplates = []Template{
	{
		ID: catalog.OpenCodeFreeTemplate, Label: "Opencode Free", Kind: KindKey,
		Origin: catalog.OriginTemplate, Auth: catalog.AuthNone, KeyHeader: catalog.KeyHeaderNone,
		DefaultFormat: catalog.FormatOpenAIChat, DefaultBaseURL: "https://opencode.ai/zen/v1",
		ModelsSource: "listing", ModelsFormat: catalog.ModelsOpenAI, ModelsDevProviderID: "opencode",
		AvailableFormats: []FormatOption{{
			Format: catalog.FormatOpenAIChat, DefaultBaseURL: "https://opencode.ai/zen/v1",
			KeyHeader: catalog.KeyHeaderNone, ModelsFormat: catalog.ModelsOpenAI,
			Label: "OpenAI Chat Completions",
		}},
	},
}

func withModelFilter(t Template, idx *modelsdev.Index) Template {
	if t.ID != catalog.OpenCodeFreeTemplate {
		return t
	}
	t.FilterModels = freeModel
	if idx == nil || idx.Providers == nil {
		return t
	}
	models := idx.Providers["opencode"].Models
	t.FilterModels = func(id string) bool {
		model, found := models[id]
		return found && freePrice(model.Prices.Input) && freePrice(model.Prices.ExtInput)
	}
	return t
}

func freeModel(string) bool { return false }

func freePrice(rate *int64) bool {
	return rate == nil || *rate <= 0
}
