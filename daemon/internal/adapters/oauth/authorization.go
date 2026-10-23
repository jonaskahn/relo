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
	// FlowDeepSeekAccount imports the grant an installed DeepSeek Harness
	// Desktop holds, which is a platform account rather than an API key.
	FlowDeepSeekAccount = "deepseek-account"
)

// Credential extra keys a flow stores and a request reads back, so a token
// that carries a project or an endpoint reaches the request that needs it.
const (
	// ExtraProjectID is the billing project a Cloud Code credential serves.
	ExtraProjectID = projectExtraKey
	// ExtraAPIBaseURL is the endpoint a token minted for a specific host
	// answers on.
	ExtraAPIBaseURL = apiBaseExtraKey
	// ExtraOrgID is the subscription workspace a Claude token draws its
	// limits from. One login email can hold several workspaces.
	ExtraOrgID = orgExtraKey
	// ExtraOrgName is the display name of the workspace ExtraOrgID names.
	ExtraOrgName = orgNameExtraKey
)

// Authorization is how one stored credential authenticates a request: the
// token, the headers that go beside it, and any endpoint the credential
// itself selects.
type Authorization struct {
	Token   string
	Headers map[string]string
	BaseURL string
	Project string
	// KeyHeader overrides where the token travels, for a sign-in whose grant
	// belongs in a vendor header rather than the bearer slot. Empty keeps the
	// bearer the sign-in flows have always used.
	KeyHeader string
	AccountID string
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
