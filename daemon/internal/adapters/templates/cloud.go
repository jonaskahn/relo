// Package templates is the curated provider registry: the templates an
// operator adds a connection from and the sign-in metadata they carry.
package templates

import (
	"github.com/jonaskahn/relo/internal/catalog"
)

var cloudTemplates = []Template{
	{
		ID:                  "azure-openai",
		Label:               "Azure OpenAI",
		Kind:                KindCloud,
		Origin:              catalog.OriginTemplate,
		Auth:                catalog.AuthAPIKey,
		KeyHeader:           catalog.KeyHeaderApiKey,
		DefaultFormat:       catalog.FormatOpenAIResp,
		DefaultBaseURL:      "https://${resource}.openai.azure.com/openai/v1",
		ModelsSource:        "manual",
		ModelsFormat:        catalog.ModelsNone,
		ModelsDevProviderID: "azure",
		Variables: []Variable{
			{Name: "resource", Label: "Azure Resource Name", Placeholder: "my-azure-resource", Required: true},
		},
		AvailableFormats: []FormatOption{
			{
				Format:         catalog.FormatOpenAIResp,
				DefaultBaseURL: "https://${resource}.openai.azure.com/openai/v1",
				KeyHeader:      catalog.KeyHeaderApiKey,
				ModelsFormat:   catalog.ModelsNone,
				Label:          "Responses API (Codex format)",
			},
			{
				Format:         catalog.FormatOpenAIChat,
				DefaultBaseURL: "https://${resource}.openai.azure.com/openai/v1",
				KeyHeader:      catalog.KeyHeaderApiKey,
				ModelsFormat:   catalog.ModelsNone,
				Label:          "Chat Completions",
			},
		},
	},
	{
		ID:                  "amazon-bedrock",
		Label:               "Amazon Bedrock",
		Kind:                KindCloud,
		Origin:              catalog.OriginTemplate,
		Auth:                catalog.AuthAWS,
		KeyHeader:           catalog.KeyHeaderBearer,
		DefaultFormat:       catalog.FormatBedrockConverse,
		DefaultBaseURL:      "https://bedrock-runtime.${region}.amazonaws.com",
		ModelsSource:        "listing",
		ModelsFormat:        catalog.ModelsBedrock,
		ModelsDevProviderID: "amazon-bedrock",
		Variables: []Variable{
			{
				Name:        "region",
				Label:       "AWS Region",
				Placeholder: "us-east-1",
				Required:    true,
				Options:     []string{"us-east-1", "us-west-2", "eu-central-1", "ap-southeast-1"},
			},
		},
		AvailableFormats: []FormatOption{
			{
				Format:         catalog.FormatBedrockConverse,
				DefaultBaseURL: "https://bedrock-runtime.${region}.amazonaws.com",
				KeyHeader:      catalog.KeyHeaderBearer,
				ModelsFormat:   catalog.ModelsBedrock,
				Label:          "Bedrock Converse API",
			},
		},
	},
	{
		ID:             "google-vertex",
		Label:          "Google Vertex AI",
		Kind:           KindCloud,
		Origin:         catalog.OriginTemplate,
		Auth:           catalog.AuthGCP,
		KeyHeader:      catalog.KeyHeaderBearer,
		DefaultFormat:  catalog.FormatVertex,
		DefaultBaseURL: "https://${location}-aiplatform.googleapis.com/v1/projects/${project}/locations/${location}",
		// Vertex publishes no model list Relo reads, so its models are the ids
		// an operator types and the catalog only describes them.
		ModelsSource:        "manual",
		ModelsFormat:        catalog.ModelsNone,
		ModelsDevProviderID: "google-vertex",
		Variables: []Variable{
			{Name: "project", Label: "GCP Project ID", Placeholder: "my-gcp-project", Required: true},
			{
				Name:        "location",
				Label:       "Location",
				Placeholder: "us-central1",
				Required:    true,
				Options:     []string{"us-central1", "us-east4", "europe-west1", "asia-northeast1"},
			},
		},
		AvailableFormats: []FormatOption{
			{
				Format:         catalog.FormatVertex,
				DefaultBaseURL: "https://${location}-aiplatform.googleapis.com/v1/projects/${project}/locations/${location}",
				KeyHeader:      catalog.KeyHeaderBearer,
				ModelsFormat:   catalog.ModelsNone,
				Label:          "Gemini / GenerateContent",
			},
			{
				Format:         catalog.FormatVertexAnthropic,
				DefaultBaseURL: "https://${location}-aiplatform.googleapis.com/v1/projects/${project}/locations/${location}",
				KeyHeader:      catalog.KeyHeaderBearer,
				ModelsFormat:   catalog.ModelsNone,
				Label:          "Claude on Vertex",
			},
		},
	},
}
