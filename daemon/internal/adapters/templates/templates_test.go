package templates

import (
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/antigravity"
	"github.com/jonaskahn/relo/internal/adapters/modelsdev"
	"github.com/jonaskahn/relo/internal/catalog"
)

func TestTemplates(t *testing.T) {
	all := All(nil)
	if len(all) == 0 {
		t.Fatal("expected curated templates, got none")
	}

	// Verify azure-openai
	azure, ok := Get("azure-openai", nil)
	if !ok {
		t.Fatal("azure-openai template not found")
	}
	if azure.KeyHeader != "api-key" {
		t.Errorf("expected azure key header api-key, got %s", azure.KeyHeader)
	}

	// Verify bedrock
	bedrock, ok := Get("amazon-bedrock", nil)
	if !ok {
		t.Fatal("amazon-bedrock template not found")
	}
	if bedrock.DefaultFormat != "bedrock-converse" {
		t.Errorf("expected bedrock-converse, got %s", bedrock.DefaultFormat)
	}

	// The ChatGPT sign-in draws its roster from the vendor's own endpoint,
	// which is what tells Relo which models that account may use.
	codex, ok := Get("openai-codex", nil)
	if !ok {
		t.Fatal("openai-codex template not found")
	}
	if codex.ModelsFormat != "codex" {
		t.Errorf("codex models format = %q, want codex", codex.ModelsFormat)
	}
	if codex.ModelsSource != "listing" {
		t.Errorf("codex models source = %q, want listing", codex.ModelsSource)
	}
	// The ChatGPT Codex backend refuses a one-shot request, so the relay has
	// to ask for the stream and reassemble it.
	if !codex.RequiresStream {
		t.Error("codex template must declare requires_stream")
	}
	if !codex.RefusesMaxOutputTokens {
		t.Error("codex template must declare refuses_max_output_tokens")
	}

	// Cloud Code Assist gates its models on the client family the credential
	// was minted for, so the sign-in template carries the same fingerprint the
	// login and the quota probe send.
	antigravityTemplate, ok := Get("google-antigravity", nil)
	if !ok {
		t.Fatal("google-antigravity template not found")
	}
	if got := antigravityTemplate.Headers["User-Agent"]; got != antigravity.UserAgent() {
		t.Errorf("antigravity user agent = %q, want the hub fingerprint %q", got, antigravity.UserAgent())
	}

	// Dynamic modelsdev
	idx := &modelsdev.Index{
		Providers: map[string]modelsdev.Provider{
			"groq": {
				ID:   "groq",
				Name: "Groq",
				NPM:  "@ai-sdk/groq",
				API:  "https://api.groq.com/openai/v1",
			},
		},
	}
	groq, ok := Get("groq", idx)
	if !ok {
		t.Fatal("groq dynamic template not found")
	}
	if groq.DefaultFormat != "openai-chat" {
		t.Errorf("expected openai-chat, got %s", groq.DefaultFormat)
	}

	// A row Relo cannot reach yet stays listed and says why in the copy the
	// console shows beside the disabled row.
	for _, id := range []string{"devin"} {
		held, ok := Get(id, nil)
		if !ok {
			t.Fatalf("template %s not found", id)
		}
		if held.UnsupportedReason != "No transport yet" {
			t.Errorf("%s unsupported reason = %q, want No transport yet", id, held.UnsupportedReason)
		}
		if held.Kind != KindSignIn {
			t.Errorf("%s kind = %q, want it listed under an account sign-in", id, held.Kind)
		}
	}

	// The Cursor subscription speaks the agent protocol over a duplex
	// stream, with an account-scoped roster of grouped configurations.
	cursorTemplate, ok := Get("cursor", nil)
	if !ok {
		t.Fatal("cursor template not found")
	}
	if cursorTemplate.UnsupportedReason != "" {
		t.Errorf("cursor unsupported reason = %q, want it routable", cursorTemplate.UnsupportedReason)
	}
	if cursorTemplate.DefaultFormat != "cursor-agent" {
		t.Errorf("cursor default format = %q, want cursor-agent", cursorTemplate.DefaultFormat)
	}
	if cursorTemplate.ModelsFormat != "cursor" {
		t.Errorf("cursor models format = %q, want cursor", cursorTemplate.ModelsFormat)
	}
	if !cursorTemplate.RequiresStream {
		t.Error("cursor template must declare requires_stream")
	}

	// A gateway publishes one model list across several protocols, and each
	// model declares the transport it answers on. The template has to name that
	// wire per model, or the connection would send every model on its own.
	gateways := &modelsdev.Index{
		Providers: map[string]modelsdev.Provider{
			"opencode-go": {
				ID: "opencode-go", Name: "OpenCode Go", NPM: "@ai-sdk/openai-compatible",
				API: "https://opencode.ai/inference/go/v1",
				Models: map[string]modelsdev.Model{
					"deepseek-v4.1-flash": {ID: "deepseek-v4.1-flash"},
					"gpt-6-luna":          {ID: "gpt-6-luna", Via: modelsdev.TransportVia{NPM: "@ai-sdk/openai"}},
					"claude-haiku-5-5":    {ID: "claude-haiku-5-5", Via: modelsdev.TransportVia{NPM: "@ai-sdk/anthropic"}},
					"azure-model":         {ID: "azure-model", Via: modelsdev.TransportVia{NPM: "@ai-sdk/amazon-bedrock/mantle"}},
				},
			},
		},
	}
	gateway, ok := Get("opencode-go", gateways)
	if !ok {
		t.Fatal("opencode-go dynamic template not found")
	}
	if gateway.DefaultFormat != catalog.FormatOpenAIChat {
		t.Errorf("opencode-go default format = %q, want the connection's own", gateway.DefaultFormat)
	}
	if gateway.FormatForModel == nil {
		t.Fatal("opencode-go names no per-model wire")
	}
	wantFormats := map[string]catalog.APIFormat{
		"gpt-6-luna":          catalog.FormatOpenAIResp,
		"claude-haiku-5-5":    catalog.FormatAnthropic,
		"deepseek-v4.1-flash": "",
		"azure-model":         "",
		"unknown-model":       "",
	}
	for id, want := range wantFormats {
		if got := gateway.FormatForModel(id); got != want {
			t.Errorf("FormatForModel(%q) = %q, want %q", id, got, want)
		}
	}

	// A provider whose models declare no transport of their own gets no hook,
	// so nothing about an ordinary connection changes.
	if groq.FormatForModel != nil {
		t.Error("groq names a per-model wire, want the connection's own to stand")
	}

	// A transport Relo has no codec for is still a transport the vendor
	// declares, so the model answers on Chat Completions rather than on
	// whatever the connection itself speaks.
	openAIGateway := &modelsdev.Index{
		Providers: map[string]modelsdev.Provider{
			"openai-gateway": {
				ID: "openai-gateway", Name: "OpenAI Gateway", NPM: "@ai-sdk/openai",
				API: "https://gateway.example.com/v1",
				Models: map[string]modelsdev.Model{
					"chat-model": {ID: "chat-model", Via: modelsdev.TransportVia{NPM: "@ai-sdk/unknown-sdk"}},
				},
			},
		},
	}
	gateway, ok = Get("openai-gateway", openAIGateway)
	if !ok {
		t.Fatal("openai-gateway dynamic template not found")
	}
	if gateway.DefaultFormat != catalog.FormatOpenAIResp {
		t.Errorf("openai-gateway default format = %q, want the transport's own", gateway.DefaultFormat)
	}
	if got := gateway.FormatForModel("chat-model"); got != catalog.FormatOpenAIChat {
		t.Errorf("FormatForModel(chat-model) = %q, want %q", got, catalog.FormatOpenAIChat)
	}

	// A package Relo cannot speak names the transport it would need.
	unsupported := &modelsdev.Index{
		Providers: map[string]modelsdev.Provider{
			"gitlab": {
				ID:   "gitlab",
				Name: "GitLab",
				NPM:  "gitlab-ai-provider",
				API:  "https://cloud.gitlab.com",
			},
		},
	}
	gitlab, ok := Get("gitlab", unsupported)
	if !ok {
		t.Fatal("gitlab dynamic template not found")
	}
	want := "Not supported yet: needs the gitlab-ai-provider transport"
	if gitlab.UnsupportedReason != want {
		t.Errorf("gitlab unsupported reason = %q, want %q", gitlab.UnsupportedReason, want)
	}
	if gitlab.Kind != KindKey {
		t.Errorf("gitlab kind = %q, want it listed under an API key", gitlab.Kind)
	}

	wantSignIn := map[string]string{
		"openai-codex":     "ChatGPT",
		"claude":           "Claude.ai",
		"grok":             "Grok",
		"kimi":             "Kimi",
		"orcarouter-oauth": "OrcaRouter",
		"qwen":             "Qwen",
	}
	for id, label := range wantSignIn {
		held, ok := Get(id, nil)
		if !ok {
			t.Fatalf("template %s not found", id)
		}
		if held.Label != label {
			t.Errorf("%s label = %q, want %q", id, held.Label, label)
		}
	}

	twins := &modelsdev.Index{
		Providers: map[string]modelsdev.Provider{
			"anthropic":      {ID: "anthropic", Name: "Anthropic"},
			"xai":            {ID: "xai", Name: "xAI"},
			"orcarouter":     {ID: "orcarouter", Name: "OrcaRouter"},
			"github-copilot": {ID: "github-copilot", Name: "GitHub Copilot"},
			"openai":         {ID: "openai", Name: "OpenAI"},
		},
	}
	wantAPI := map[string]string{
		"anthropic":      "Claude API",
		"xai":            "Grok API",
		"orcarouter":     "OrcaRouter API",
		"github-copilot": "GitHub Copilot API",
		"openai":         "OpenAI",
	}
	for id, label := range wantAPI {
		held, ok := Get(id, twins)
		if !ok {
			t.Fatalf("template %s not found", id)
		}
		if held.Label != label {
			t.Errorf("%s label = %q, want %q", id, held.Label, label)
		}
	}
}

func TestTeamoRouter(t *testing.T) {
	held, ok := Get("teamorouter", nil)
	if !ok {
		t.Fatal("teamorouter template not found")
	}
	if held.Label != "TeamoRouter" {
		t.Errorf("label = %q, want TeamoRouter", held.Label)
	}
	if held.Kind != KindKey {
		t.Errorf("kind = %q, want key", held.Kind)
	}
	if held.DefaultFormat != catalog.FormatOpenAIChat {
		t.Errorf("default format = %q, want openai-chat", held.DefaultFormat)
	}
	if held.DefaultBaseURL != "https://api.teamorouter.cn/v1" {
		t.Errorf("default base url = %q, want the TeamoRouter v1 host", held.DefaultBaseURL)
	}
	if held.KeyHeader != catalog.KeyHeaderBearer {
		t.Errorf("key header = %q, want bearer", held.KeyHeader)
	}
	if held.ModelsFormat != catalog.ModelsOpenAI {
		t.Errorf("models format = %q, want openai", held.ModelsFormat)
	}
	if held.ModelsSource != "listing" {
		t.Errorf("models source = %q, want listing", held.ModelsSource)
	}
	if held.ModelsDevProviderID != "" {
		t.Errorf("models.dev id = %q, want none", held.ModelsDevProviderID)
	}
	if held.DocURL != "https://teamorouter.cn/docs" {
		t.Errorf("doc url = %q, want the TeamoRouter docs", held.DocURL)
	}
	if len(held.AvailableFormats) != 3 {
		t.Fatalf("available formats = %d, want the three TeamoRouter speaks", len(held.AvailableFormats))
	}
	want := []struct {
		format catalog.APIFormat
		header catalog.KeyHeader
		models catalog.ModelsFormat
		base   string
	}{
		{catalog.FormatOpenAIChat, catalog.KeyHeaderBearer, catalog.ModelsOpenAI, "https://api.teamorouter.cn/v1"},
		{catalog.FormatOpenAIResp, catalog.KeyHeaderBearer, catalog.ModelsOpenAI, "https://api.teamorouter.cn/v1"},
		{catalog.FormatAnthropic, catalog.KeyHeaderXApiKey, catalog.ModelsOpenAI, "https://api.teamorouter.cn/v1"},
	}
	for i, option := range want {
		got := held.AvailableFormats[i]
		if got.Format != option.format || got.KeyHeader != option.header || got.ModelsFormat != option.models || got.DefaultBaseURL != option.base {
			t.Errorf("format %d = %+v, want %+v", i, got, option)
		}
	}
}

func TestRewrittenLabel(t *testing.T) {
	next, ok := RewrittenLabel("nvidia", "Nvidia")
	if !ok || next != "NVIDIA NIM" {
		t.Errorf("RewrittenLabel(nvidia) = %q, %v, want NVIDIA NIM, true", next, ok)
	}
	next, ok = RewrittenLabel("openai-codex", "ChatGPT (Codex sign-in)")
	if !ok || next != "ChatGPT" {
		t.Errorf("RewrittenLabel(codex) = %q, %v, want ChatGPT, true", next, ok)
	}
	next, ok = RewrittenLabel("anthropic", "Anthropic")
	if !ok || next != "Claude API" {
		t.Errorf("RewrittenLabel(anthropic) = %q, %v, want Claude API, true", next, ok)
	}
	if _, ok := RewrittenLabel("claude", "Work Claude"); ok {
		t.Error("RewrittenLabel left a name the operator chose alone")
	}
}

func TestCommandCodeKeyTemplate(t *testing.T) {
	held, ok := Get("command-code", nil)
	if !ok {
		t.Fatal("command-code template not found")
	}
	if held.Label != "Command Code" {
		t.Errorf("label = %q, want Command Code", held.Label)
	}
	if held.Kind != KindKey {
		t.Errorf("kind = %q, want key", held.Kind)
	}
	if held.Auth != catalog.AuthAPIKey {
		t.Errorf("auth = %q, want api_key", held.Auth)
	}
	if held.KeyHeader != catalog.KeyHeaderBearer {
		t.Errorf("key header = %q, want bearer", held.KeyHeader)
	}
	if held.DefaultFormat != catalog.FormatOpenAIChat {
		t.Errorf("default format = %q, want openai-chat", held.DefaultFormat)
	}
	if held.DefaultBaseURL != "https://api.commandcode.ai/provider/v1" {
		t.Errorf("base url = %q, want the Command Code provider host", held.DefaultBaseURL)
	}
	if held.ModelsSource != "listing" {
		t.Errorf("models source = %q, want listing", held.ModelsSource)
	}
	if held.ModelsFormat != catalog.ModelsOpenAI {
		t.Errorf("models format = %q, want openai", held.ModelsFormat)
	}
	if len(held.LoginMethods) != 0 {
		t.Errorf("login methods = %+v, want none", held.LoginMethods)
	}
	if held.DocURL != "https://commandcode.ai/docs/provider" {
		t.Errorf("doc url = %q, want the Command Code provider docs", held.DocURL)
	}
}
