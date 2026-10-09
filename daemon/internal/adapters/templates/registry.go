// Template registry: curated providers and sign-in metadata.
package templates

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/modelsdev"
	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/catalog"
)

var (
	curatedTemplates = buildCuratedTemplates()
)

// buildCuratedTemplates assembles the curated registry once, so no init
// function owns what the package serves.
func buildCuratedTemplates() []Template {
	list := []Template{}
	for _, signIn := range signInTemplates {
		list = append(list, signIn.Template)
	}
	list = append(list, cloudTemplates...)
	list = append(list, dualFormatTemplates...)
	list = append(list, freeTemplates...)
	list = append(list, kiloFreeTemplates...)
	list = append(list, localPresets...)
	return list
}

// All returns all available templates, combining curated and dynamic models.dev templates.
func All(idx *modelsdev.Index) []Template {
	seen := make(map[string]bool)
	list := make([]Template, 0, len(curatedTemplates)+200)

	for _, t := range curatedTemplates {
		seen[t.ID] = true
		t.Normalize()
		t.ModelsDevModels = modelsDevModelCount(idx, t.ModelsDevProviderID)
		list = append(list, t)
	}

	return append(list, dynamicTemplates(idx, seen)...)
}

func dynamicTemplates(idx *modelsdev.Index, seen map[string]bool) []Template {
	if idx == nil {
		return nil
	}
	var dyn []Template
	for pID, p := range idx.Providers {
		// A cloud provider already has a template that knows which
		// credentials it takes, so the row models.dev describes is not
		// offered a second time as one more API key.
		if seen[pID] || cloudModelsDevIDs[pID] {
			continue
		}
		t := fromModelsDevProvider(p)
		t.Normalize()
		t.ModelsDevModels = len(p.Models)
		dyn = append(dyn, t)
	}
	sort.Slice(dyn, func(i, j int) bool {
		return dyn[i].Label < dyn[j].Label
	})
	return dyn
}

// Get finds a template by ID.
func Get(id string, idx *modelsdev.Index) (Template, bool) {
	for _, t := range curatedTemplates {
		if t.ID == id {
			t.Normalize()
			t.ModelsDevModels = modelsDevModelCount(idx, t.ModelsDevProviderID)
			return t, true
		}
	}

	if idx != nil {
		if p, ok := idx.Providers[id]; ok {
			if cloudModelsDevIDs[id] {
				return Template{}, false
			}
			t := fromModelsDevProvider(p)
			t.Normalize()
			t.ModelsDevModels = len(p.Models)
			return t, true
		}
	}

	return Template{}, false
}

// Curated finds a hand-written template by ID, which is what a stored
// provider row remembers its connection policy from. It needs no saved
// models.dev copy, so a surface can name a provider's shape while the copy
// is missing.
func Curated(id string) (Template, bool) {
	for _, t := range curatedTemplates {
		if t.ID == id {
			t.Normalize()
			return t, true
		}
	}
	return Template{}, false
}

// Authorizer returns the authorizer function for a sign-in provider.
func Authorizer(providerID string) (func(oauth.OAuthCredential) oauth.Authorization, bool) {
	for _, t := range signInTemplates {
		if t.ID == providerID && t.authorize != nil {
			return t.authorize, true
		}
	}
	return nil, false
}

// SignInFlows returns the login flows for a sign-in provider.
func SignInFlows(providerID string) []string {
	for _, t := range signInTemplates {
		if t.ID == providerID {
			t.Normalize()
			return t.LoginFlows
		}
	}
	return nil
}

// ByFlow finds the sign-in template whose login declares a flow, which is
// how a started login knows which provider it is for.
func ByFlow(flow string) (Template, bool) {
	for _, t := range signInTemplates {
		for _, method := range t.LoginMethods {
			if method.Flow == flow {
				t.Normalize()
				return t.Template, true
			}
		}
	}
	return Template{}, false
}

// SignInTemplate reports the template a provider id belongs to when it is a
// sign-in, which is what tells a provider it can be signed into.
func SignInTemplate(providerID string) (Template, bool) {
	for _, t := range signInTemplates {
		if t.ID == providerID {
			t.Normalize()
			return t.Template, true
		}
	}
	return Template{}, false
}

var cloudModelsDevIDs = map[string]bool{
	"azure":                    true,
	"azure-cognitive-services": true,
	"amazon-bedrock":           true,
	"google-vertex":            true,
	"google-vertex-anthropic":  true,
}

func modelsDevModelCount(idx *modelsdev.Index, providerID string) int {
	if idx == nil || providerID == "" {
		return 0
	}
	p, found := idx.Providers[providerID]
	if !found {
		return 0
	}
	return len(p.Models)
}
func fromModelsDevProvider(p modelsdev.Provider) Template {
	fmtChoice, modelsChoice, defaultBase, unsupported := resolveTransport(p.NPM, p.API)
	baseURL := p.API
	if baseURL == "" {
		baseURL = defaultBase
	}

	return Template{
		ID:                  p.ID,
		Label:               DisplayName(p.ID, p.Name),
		Kind:                KindKey,
		Origin:              catalog.OriginTemplate,
		Auth:                catalog.AuthAPIKey,
		KeyHeader:           catalog.KeyHeaderBearer,
		DefaultFormat:       fmtChoice,
		AvailableFormats:    formatOptions(fmtChoice, modelsChoice, baseURL, unsupported),
		DefaultBaseURL:      baseURL,
		ModelsSource:        "listing",
		ModelsFormat:        modelsChoice,
		ModelsDevProviderID: p.ID,
		DocURL:              p.Doc,
		KeyEnv:              p.Env,
		UnsupportedReason:   unsupported,
	}
}

func formatOptions(fmtChoice catalog.APIFormat, modelsChoice catalog.ModelsFormat, baseURL, unsupported string) []FormatOption {
	if unsupported != "" {
		return nil
	}
	return []FormatOption{
		{
			Format:         fmtChoice,
			DefaultBaseURL: baseURL,
			KeyHeader:      catalog.KeyHeaderBearer,
			ModelsFormat:   modelsChoice,
			Label:          string(fmtChoice),
		},
	}
}

var exoticTransports = map[string]bool{
	"@ai-sdk/amazon-bedrock":                 true,
	"@ai-sdk/amazon-bedrock/mantle":          true,
	"@ai-sdk/azure":                          true,
	"ai-gateway-provider":                    true,
	"gitlab-ai-provider":                     true,
	"@ai-sdk/google-vertex/anthropic":        true,
	"@qvac/ai-sdk-provider":                  true,
	"@saladtechnologies-oss/ai-sdk-provider": true,
	"@jerome-benoit/sap-ai-provider-v2":      true,
	"watsonx-ai-provider":                    true,
}

func resolveTransport(npm, api string) (catalog.APIFormat, catalog.ModelsFormat, string, string) {
	if exoticTransports[npm] {
		return catalog.FormatUnsupported, catalog.ModelsNone, "", fmt.Sprintf("Not supported yet: needs the %s transport", npm)
	}

	if transport, found := sdkTransports[npm]; found {
		return transport.format, transport.models, transport.baseURL, ""
	}
	if api == "" && !strings.Contains(npm, "openai") {
		return catalog.FormatUnsupported, catalog.ModelsNone, "", "models.dev publishes no base URL"
	}
	return catalog.FormatOpenAIChat, catalog.ModelsOpenAI, "", ""
}

type sdkTransport struct {
	format  catalog.APIFormat
	models  catalog.ModelsFormat
	baseURL string
}

var sdkTransports = map[string]sdkTransport{
	"@ai-sdk/openai":         {catalog.FormatOpenAIResp, catalog.ModelsOpenAI, "https://api.openai.com/v1"},
	"@ai-sdk/anthropic":      {catalog.FormatAnthropic, catalog.ModelsAnthropic, "https://api.anthropic.com/v1"},
	"@ai-sdk/google":         {catalog.FormatGemini, catalog.ModelsGemini, "https://generativelanguage.googleapis.com/v1beta"},
	"@ai-sdk/groq":           {catalog.FormatOpenAIChat, catalog.ModelsOpenAI, "https://api.groq.com/openai/v1"},
	"@ai-sdk/mistral":        {catalog.FormatOpenAIChat, catalog.ModelsOpenAI, "https://api.mistral.ai/v1"},
	"@ai-sdk/cerebras":       {catalog.FormatOpenAIChat, catalog.ModelsOpenAI, "https://api.cerebras.ai/v1"},
	"@ai-sdk/deepinfra":      {catalog.FormatOpenAIChat, catalog.ModelsOpenAI, "https://api.deepinfra.com/v1/openai"},
	"@ai-sdk/togetherai":     {catalog.FormatOpenAIChat, catalog.ModelsOpenAI, "https://api.together.xyz/v1"},
	"@ai-sdk/perplexity":     {catalog.FormatOpenAIChat, catalog.ModelsNone, "https://api.perplexity.ai"},
	"@ai-sdk/cohere":         {catalog.FormatOpenAIChat, catalog.ModelsOpenAI, "https://api.cohere.ai/compatibility/v1"},
	"@ai-sdk/xai":            {catalog.FormatOpenAIChat, catalog.ModelsOpenAI, "https://api.x.ai/v1"},
	"venice-ai-sdk-provider": {catalog.FormatOpenAIChat, catalog.ModelsOpenAI, "https://api.venice.ai/api/v1"},
	"@ai-sdk/gateway":        {catalog.FormatOpenAIChat, catalog.ModelsOpenAI, "https://ai-gateway.vercel.sh/v1"},
	"@ai-sdk/vercel":         {catalog.FormatOpenAIChat, catalog.ModelsNone, "https://api.v0.dev/v1"},
}
