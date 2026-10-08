package oauth

// Flow identifiers Relo registers and the catalog names. A sign-in row holds
// these, so the registry and the catalog always agree on who can log in.
const (
	FlowChatGPT           = "openai-codex"
	FlowChatGPTDevice     = "openai-codex-device"
	FlowClaude            = "claude"
	FlowGoogleAntigravity = "google-antigravity"
	FlowGrok              = "grok"
	FlowCursor            = "cursor"
	FlowKiro              = "kiro"
	FlowDevin             = "devin"
	FlowMetaMuse          = "meta-muse"
	FlowKimi              = "kimi"
	FlowCopilot           = "copilot"
	FlowOrcaRouter        = "orcarouter-oauth"
	FlowNous              = "nous"
	FlowCommandCode       = "command-code"
	FlowQwen              = "qwen"
	FlowIFlow             = "iflow"
)

// Credential extra keys a flow stores and a request reads back, so a token
// that carries a project or an endpoint reaches the request that needs it.
const (
	// ExtraProjectID is the billing project a Cloud Code credential serves.
	ExtraProjectID = projectExtraKey
	// ExtraAPIBaseURL is the endpoint a token minted for a specific host
	// answers on.
	ExtraAPIBaseURL = apiBaseExtraKey
)

// Authorization is how one stored credential authenticates a request: the
// token, the headers that go beside it, and any endpoint the credential
// itself selects.
type Authorization struct {
	Token   string
	Headers map[string]string
	BaseURL string
	Project string
}

// CopilotRequestHeaders returns the editor headers the Copilot API requires,
// so a request identifies the client the token was minted for.
func CopilotRequestHeaders() map[string]string {
	return map[string]string{
		"Editor-Version":         editorVersion,
		"Editor-Plugin-Version":  editorPluginVersion,
		"Copilot-Integration-Id": copilotIntegrationID,
	}
}
