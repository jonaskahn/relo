// Curated provider dashboard addresses a connection pane may open.
package templates

var dashboardURLs = map[string]string{
	"openai-codex":       "https://chatgpt.com/",
	"claude":             "https://platform.claude.com/settings/keys",
	"google-antigravity": "https://antigravity.google/",
	"copilot":            "https://github.com/settings/copilot",
	"grok":               "https://console.x.ai/",
	"kimi":               "https://platform.moonshot.ai/console/api-keys",
	"nous":               "https://portal.nousresearch.com/",
	"orcarouter-oauth":   "https://orcarouter.ai/dashboard",
	"meta-muse":          "https://www.meta.ai/",
	"kiro":               "https://kiro.dev/",
	"qwen":               "https://bailian.console.aliyun.com/",
	"iflow":              "https://iflow.cn/",
	"deepseek-account":   "https://platform.deepseek.com/usage",
	"cursor":             "https://cursor.com/dashboard",
	"devin":              "https://app.devin.ai/",
	"deepseek":           "https://platform.deepseek.com/usage",
	"minimax":            "https://platform.minimax.io/console/keys",
	"minimax-cn":         "https://platform.minimaxi.com/console/keys",
	"alibaba":            "https://bailian.console.aliyun.com/",
	"teamorouter":        "https://teamorouter.cn/",
	"open-design":        "https://open-design.ai/cloud/dashboard",
	"command-code":       "https://commandcode.ai/dashboard",
	"opencode-free":      "https://opencode.ai/auth",
	"kilo-free":          "https://app.kilo.ai/",
	"azure-openai":       "https://portal.azure.com/",
	"amazon-bedrock":     "https://console.aws.amazon.com/bedrock/",
	"google-vertex":      "https://console.cloud.google.com/vertex-ai",
	"openrouter":         "https://openrouter.ai/settings/keys",
	"groq":               "https://console.groq.com/keys",
	"mistral":            "https://console.mistral.ai/api-keys",
	"cohere":             "https://dashboard.cohere.com/api-keys",
	"fireworks-ai":       "https://fireworks.ai/account/api-keys",
	"huggingface":        "https://huggingface.co/settings/tokens",
	"opencode":           "https://opencode.ai/auth",
	"kilo":               "https://app.kilo.ai/",
}

func dashboardURLFor(templateID string) string {
	return dashboardURLs[templateID]
}
