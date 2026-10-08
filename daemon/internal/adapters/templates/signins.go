// Sign-in authorizations per template credential kind.
package templates

import (
	"github.com/jonaskahn/relo/internal/adapters/antigravity"
	"github.com/jonaskahn/relo/internal/adapters/codex"
	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/catalog"
)

// OpenAICodexTemplate is the ChatGPT subscription connection.
const OpenAICodexTemplate = "openai-codex"

func authorizeDefault(cred oauth.OAuthCredential) oauth.Authorization {
	return oauth.Authorization{Token: cred.AccessToken}
}

func authorizeCodex(cred oauth.OAuthCredential) oauth.Authorization {
	auth := authorizeDefault(cred)
	if cred.AccountID != "" {
		auth.Headers = map[string]string{"chatgpt-account-id": cred.AccountID}
	}
	return auth
}

func authorizeAntigravity(cred oauth.OAuthCredential) oauth.Authorization {
	auth := authorizeDefault(cred)
	auth.Project = cred.Extra[oauth.ExtraProjectID]
	return auth
}

func authorizeCopilot(cred oauth.OAuthCredential) oauth.Authorization {
	auth := authorizeDefault(cred)
	auth.BaseURL = cred.Extra[oauth.ExtraAPIBaseURL]
	auth.Headers = oauth.CopilotRequestHeaders()
	return auth
}

func authorizeKiro(cred oauth.OAuthCredential) oauth.Authorization {
	auth := authorizeDefault(cred)
	auth.Project = cred.Extra["profileArn"]
	return auth
}

// The Grok subscription token is minted for the Grok CLI's own proxy rather
// than the public xAI API, so a signed-in request goes to that proxy with the
// headers the CLI sends. An API-key xAI connection is a different credential
// kind and keeps the public endpoint.
const (
	grokOAuthBaseURL  = "https://cli-chat-proxy.grok.com/v1"
	grokClientVersion = "1.0.13"
)

func authorizeGrok(cred oauth.OAuthCredential) oauth.Authorization {
	auth := authorizeDefault(cred)
	auth.BaseURL = grokOAuthBaseURL
	auth.Headers = map[string]string{
		"User-Agent":               "grok/" + grokClientVersion,
		"x-grok-client-identifier": "cli",
		"x-grok-client-version":    grokClientVersion,
		"x-xai-token-auth":         "xai-grok-cli",
		"x-authenticateresponse":   "authenticate-response",
	}
	return auth
}

type signInTemplate struct {
	Template
	authorize func(oauth.OAuthCredential) oauth.Authorization
}

var signInTemplates = []signInTemplate{
	{
		Template: Template{
			ID:             OpenAICodexTemplate,
			Label:          "ChatGPT",
			Kind:           KindSignIn,
			Origin:         catalog.OriginSignIn,
			Auth:           catalog.AuthOAuth,
			KeyHeader:      catalog.KeyHeaderBearer,
			DefaultFormat:  catalog.FormatOpenAIResp,
			DefaultBaseURL: "https://chatgpt.com/backend-api/codex",
			// The ChatGPT Codex backend refuses a request that does not stream,
			// and its dialect has no max-output ceiling parameter, so the relay
			// asks for the stream, reassembles it, and leaves the ceiling out.
			RequiresStream: true, RefusesMaxOutputTokens: true,
			ModelsSource:        "listing",
			ModelsFormat:        catalog.ModelsCodex,
			ModelsDevProviderID: "openai",
			LoginMethods: []LoginMethod{
				{Flow: oauth.FlowChatGPT, Kind: LoginBrowser},
				{Flow: oauth.FlowChatGPTDevice, Kind: LoginDevice},
			},
			Headers: map[string]string{
				"OpenAI-Beta": codex.OpenAIBeta,
				"originator":  codex.Originator,
				"version":     codex.Version,
			},
		},
		authorize: authorizeCodex,
	},
	{
		Template: Template{
			ID:                  "claude",
			Label:               "Claude.ai",
			Kind:                KindSignIn,
			Origin:              catalog.OriginSignIn,
			Auth:                catalog.AuthOAuth,
			KeyHeader:           catalog.KeyHeaderBearer,
			DefaultFormat:       catalog.FormatAnthropic,
			DefaultBaseURL:      "https://api.anthropic.com/v1",
			ModelsSource:        "listing",
			ModelsFormat:        catalog.ModelsAnthropic,
			ModelsDevProviderID: "anthropic",
			LoginMethods:        []LoginMethod{{Flow: oauth.FlowClaude, Kind: LoginBrowser}},
		},
		authorize: authorizeDefault,
	},
	{
		Template: Template{
			ID:             "google-antigravity",
			Label:          "Google Antigravity",
			Kind:           KindSignIn,
			Origin:         catalog.OriginSignIn,
			Auth:           catalog.AuthOAuth,
			KeyHeader:      catalog.KeyHeaderBearer,
			DefaultFormat:  catalog.FormatCloudCode,
			DefaultBaseURL: "https://daily-cloudcode-pa.googleapis.com",
			ModelsSource:   "listing",
			ModelsFormat:   catalog.ModelsAntigravity,
			LoginMethods:   []LoginMethod{{Flow: oauth.FlowGoogleAntigravity, Kind: LoginBrowser}},
			// Listing and sign-in stay on the IDE fingerprint. A chat request
			// replaces this header with the CLI fingerprint on the way out.
			Headers: map[string]string{"User-Agent": antigravity.UserAgent()},
		},
		authorize: authorizeAntigravity,
	},
	{
		Template: Template{
			ID:                  "copilot",
			Label:               "GitHub Copilot",
			Kind:                KindSignIn,
			Origin:              catalog.OriginSignIn,
			Auth:                catalog.AuthOAuth,
			KeyHeader:           catalog.KeyHeaderBearer,
			DefaultFormat:       catalog.FormatOpenAIChat,
			DefaultBaseURL:      "https://api.githubcopilot.com",
			ModelsSource:        "listing",
			ModelsFormat:        catalog.ModelsOpenAI,
			ModelsDevProviderID: "github-copilot",
			LoginMethods:        []LoginMethod{{Flow: oauth.FlowCopilot, Kind: LoginDevice}},
		},
		authorize: authorizeCopilot,
	},
	{
		Template: Template{
			ID:                  "grok",
			Label:               "Grok",
			Kind:                KindSignIn,
			Origin:              catalog.OriginSignIn,
			Auth:                catalog.AuthOAuth,
			KeyHeader:           catalog.KeyHeaderBearer,
			DefaultFormat:       catalog.FormatOpenAIChat,
			DefaultBaseURL:      "https://api.x.ai/v1",
			ModelsSource:        "listing",
			ModelsFormat:        catalog.ModelsOpenAI,
			ModelsDevProviderID: "xai",
			// Grok signs in on another device: the console shows a code and the page
			// the vendor publishes, and no loopback redirect is involved.
			LoginMethods: []LoginMethod{{Flow: oauth.FlowGrok, Kind: LoginDevice}},
		},
		authorize: authorizeGrok,
	},
	{
		Template: Template{
			ID:                  "kimi",
			Label:               "Kimi",
			Kind:                KindSignIn,
			Origin:              catalog.OriginSignIn,
			Auth:                catalog.AuthOAuth,
			KeyHeader:           catalog.KeyHeaderBearer,
			DefaultFormat:       catalog.FormatOpenAIChat,
			DefaultBaseURL:      "https://api.kimi.com/coding/v1",
			ModelsSource:        "listing",
			ModelsFormat:        catalog.ModelsOpenAI,
			ModelsDevProviderID: "kimi-code-plan-cn",
			LoginMethods:        []LoginMethod{{Flow: oauth.FlowKimi, Kind: LoginDevice}},
		},
		authorize: authorizeDefault,
	},
	{
		Template: Template{
			ID:             "nous",
			Label:          "Nous Portal",
			Kind:           KindSignIn,
			Origin:         catalog.OriginSignIn,
			Auth:           catalog.AuthOAuth,
			KeyHeader:      catalog.KeyHeaderBearer,
			DefaultFormat:  catalog.FormatOpenAIChat,
			DefaultBaseURL: "https://inference-api.nousresearch.com/v1",
			ModelsSource:   "listing",
			ModelsFormat:   catalog.ModelsOpenAI,
			LoginMethods:   []LoginMethod{{Flow: oauth.FlowNous, Kind: LoginDevice}},
		},
		authorize: authorizeDefault,
	},
	{
		Template: Template{
			ID:                  "orcarouter-oauth",
			Label:               "OrcaRouter",
			Kind:                KindSignIn,
			Origin:              catalog.OriginSignIn,
			Auth:                catalog.AuthOAuth,
			KeyHeader:           catalog.KeyHeaderBearer,
			DefaultFormat:       catalog.FormatOpenAIChat,
			DefaultBaseURL:      "https://api.orcarouter.ai/v1",
			ModelsSource:        "listing",
			ModelsFormat:        catalog.ModelsOpenAI,
			ModelsDevProviderID: "orcarouter",
			LoginMethods:        []LoginMethod{{Flow: oauth.FlowOrcaRouter, Kind: LoginBrowser}},
		},
		authorize: authorizeDefault,
	},
	{
		Template: Template{
			ID:                  "meta-muse",
			Label:               "Meta Muse Code",
			Kind:                KindSignIn,
			Origin:              catalog.OriginSignIn,
			Auth:                catalog.AuthOAuth,
			KeyHeader:           catalog.KeyHeaderBearer,
			DefaultFormat:       catalog.FormatOpenAIResp,
			DefaultBaseURL:      "https://api.meta.ai/v1",
			ModelsSource:        "listing",
			ModelsFormat:        catalog.ModelsOpenAI,
			ModelsDevProviderID: "meta",
			LoginMethods:        []LoginMethod{{Flow: oauth.FlowMetaMuse, Kind: LoginCLI}},
			Headers: map[string]string{
				"User-Agent":    "muse-build/1.3.0 (opencodex compatibility)",
				"x-api-version": "1.0.0",
			},
		},
		authorize: authorizeDefault,
	},
	{
		Template: Template{
			ID:             "command-code",
			Label:          "Command Code",
			Kind:           KindSignIn,
			Origin:         catalog.OriginSignIn,
			Auth:           catalog.AuthOAuth,
			KeyHeader:      catalog.KeyHeaderBearer,
			DefaultFormat:  catalog.FormatOpenAIChat,
			DefaultBaseURL: "https://api.commandcode.ai/provider/v1",
			ModelsSource:   "listing",
			ModelsFormat:   catalog.ModelsOpenAI,
			LoginMethods:   []LoginMethod{{Flow: oauth.FlowCommandCode, Kind: LoginCLI}},
		},
		authorize: authorizeDefault,
	},
	{
		Template: Template{
			ID:             "kiro",
			Label:          "Kiro",
			Kind:           KindSignIn,
			Origin:         catalog.OriginSignIn,
			Auth:           catalog.AuthOAuth,
			KeyHeader:      catalog.KeyHeaderBearer,
			DefaultFormat:  catalog.FormatKiro,
			DefaultBaseURL: "https://codewhisperer.us-east-1.amazonaws.com",
			ModelsSource:   "manual",
			// Kiro publishes no model list: the account arrives by sign-in, and the
			// models it serves are the ids an operator types.
			ModelsFormat: catalog.ModelsNone,
			LoginMethods: []LoginMethod{{Flow: oauth.FlowKiro, Kind: LoginCLI}},
		},
		authorize: authorizeKiro,
	},
	{
		Template: Template{
			ID:             "qwen",
			Label:          "Qwen",
			Kind:           KindSignIn,
			Origin:         catalog.OriginSignIn,
			Auth:           catalog.AuthOAuth,
			KeyHeader:      catalog.KeyHeaderBearer,
			DefaultFormat:  catalog.FormatOpenAIChat,
			DefaultBaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
			ModelsSource:   "listing",
			ModelsFormat:   catalog.ModelsOpenAI,
			LoginMethods:   []LoginMethod{{Flow: oauth.FlowQwen, Kind: LoginDevice}},
		},
		authorize: authorizeDefault,
	},
	{
		Template: Template{
			ID:             "iflow",
			Label:          "iFlow",
			Kind:           KindSignIn,
			Origin:         catalog.OriginSignIn,
			Auth:           catalog.AuthOAuth,
			KeyHeader:      catalog.KeyHeaderBearer,
			DefaultFormat:  catalog.FormatOpenAIChat,
			DefaultBaseURL: "https://apis.iflow.cn/v1",
			ModelsSource:   "listing",
			ModelsFormat:   catalog.ModelsOpenAI,
			LoginMethods:   []LoginMethod{{Flow: oauth.FlowIFlow, Kind: LoginBrowser}},
		},
		authorize: authorizeDefault,
	},
	{
		Template: Template{
			ID:                "cursor",
			Label:             "Cursor",
			Kind:              KindSignIn,
			Origin:            catalog.OriginSignIn,
			Auth:              catalog.AuthOAuth,
			LoginMethods:      []LoginMethod{{Flow: oauth.FlowCursor, Kind: LoginBrowser}},
			UnsupportedReason: "No transport yet",
		},
	},
	{
		Template: Template{
			ID:                "devin",
			Label:             "Devin",
			Kind:              KindSignIn,
			Origin:            catalog.OriginSignIn,
			Auth:              catalog.AuthOAuth,
			LoginMethods:      []LoginMethod{{Flow: oauth.FlowDevin, Kind: LoginBrowser}},
			UnsupportedReason: "No transport yet",
		},
	},
}
