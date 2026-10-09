// OpenCode Free declares the signed-out Zen connection and its model policy.
package templates

import (
	"regexp"
	"strings"

	"github.com/jonaskahn/relo/internal/catalog"
)

var freeTemplates = []Template{
	{
		ID: catalog.OpenCodeFreeTemplate, Label: "Opencode Free", Kind: KindKey,
		Origin: catalog.OriginTemplate, Auth: catalog.AuthNone, KeyHeader: catalog.KeyHeaderNone,
		DefaultFormat: catalog.FormatOpenAIChat, DefaultBaseURL: "https://opencode.ai/zen/v1",
		ModelsSource: "listing", ModelsFormat: catalog.ModelsOpenAI, ModelsDevProviderID: "opencode",
		FilterModels: freeModel, FormatForModel: freeModelFormat, RequiresStream: true,
		AvailableFormats: []FormatOption{{
			Format: catalog.FormatOpenAIChat, DefaultBaseURL: "https://opencode.ai/zen/v1",
			KeyHeader: catalog.KeyHeaderNone, ModelsFormat: catalog.ModelsOpenAI,
			Label: "OpenAI Chat Completions",
		}},
	},
}

// freeLane names a free identifier the suffix rule cannot recognise, because
// the gateway publishes the model under a name that states no price.
var freeLane = map[string]bool{"big-pickle": true}

// freeModelSuffix matches the identifiers the gateway marks free by name. A
// missing price is not evidence of free access, so the rule reads the name.
var freeModelSuffix = regexp.MustCompile(`(?:^|[-_])free(?:$|[-_.])`)

// unsupportedFree matches the free identifiers the gateway answers on an API
// Relo does not speak: Jev is a structured-decision endpoint rather than a
// conversational one, and its answer is not a chat completion.
var unsupportedFree = regexp.MustCompile(`^jev-`)

func freeModel(id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	if unsupportedFree.MatchString(id) {
		return false
	}
	return freeLane[id] || freeModelSuffix.MatchString(id)
}

// responsesModel matches the free identifiers the gateway answers on the
// Responses wire rather than Chat Completions.
var responsesModel = regexp.MustCompile(`^muse[-_]?spark`)

// freeModelFormat names the protocol each free model answers on. The gateway
// publishes one model list across several protocols, so the format follows the
// model rather than the connection, and an unrecognised free model keeps the
// connection's own default.
func freeModelFormat(id string) catalog.APIFormat {
	if responsesModel.MatchString(strings.TrimSpace(id)) {
		return catalog.FormatOpenAIResp
	}
	return catalog.FormatOpenAIChat
}
