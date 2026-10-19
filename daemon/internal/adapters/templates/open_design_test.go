package templates

import (
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
)

func TestOpenDesignTemplateServesChatAndResponses(t *testing.T) {
	template, found := Get("open-design", nil)
	if !found {
		t.Fatal("open-design template not found")
	}

	if template.Label != "OpenDesign" {
		t.Errorf("label = %q, want OpenDesign", template.Label)
	}
	if template.Kind != KindKey || template.Auth != catalog.AuthAPIKey {
		t.Errorf("kind = %q, auth = %q, want an API key connection", template.Kind, template.Auth)
	}
	if template.KeyHeader != catalog.KeyHeaderBearer {
		t.Errorf("key header = %q, want bearer", template.KeyHeader)
	}
	if template.DefaultFormat != catalog.FormatOpenAIChat {
		t.Errorf("default format = %q, want openai-chat", template.DefaultFormat)
	}
	if template.DefaultBaseURL != "https://amr-link.open-design.ai/v1" {
		t.Errorf("base url = %q, want the OpenDesign endpoint", template.DefaultBaseURL)
	}
	if template.ModelsSource != "listing" || template.ModelsFormat != catalog.ModelsOpenAI {
		t.Errorf("models source = %q, format = %q, want an OpenAI listing",
			template.ModelsSource, template.ModelsFormat)
	}
	if template.FilterModels != nil || len(template.Prefixes) != 0 {
		t.Error("OpenDesign must keep every model the vendor lists")
	}

	for _, format := range []catalog.APIFormat{
		catalog.FormatOpenAIChat,
		catalog.FormatOpenAIResp,
	} {
		option, found := template.FormatOption(format)
		if !found {
			t.Errorf("format %q is missing", format)
			continue
		}
		if option.DefaultBaseURL != template.DefaultBaseURL {
			t.Errorf("%s base url = %q, want %q", format, option.DefaultBaseURL, template.DefaultBaseURL)
		}
		if option.KeyHeader != catalog.KeyHeaderBearer || option.ModelsFormat != catalog.ModelsOpenAI {
			t.Errorf("%s header = %q, listing = %q, want bearer and openai",
				format, option.KeyHeader, option.ModelsFormat)
		}
	}
}
