package templates

// DisplayName is the label a models.dev-derived template shows. A suffix
// is added only when that name would otherwise match a sign-in connection.
func DisplayName(id, name string) string {
	switch id {
	case "anthropic":
		return "Claude API"
	case "xai":
		return "Grok API"
	case "orcarouter":
		return "OrcaRouter API"
	case "github-copilot":
		if name == "" || name == "GitHub Copilot" {
			return "GitHub Copilot API"
		}
		return name
	}
	if name != "" {
		return name
	}
	return id
}

var retiredDefaults = map[string]string{
	"openai-codex":     "ChatGPT (Codex sign-in)",
	"claude":           "Claude (Pro/Max sign-in)",
	"grok":             "Grok (xAI sign-in)",
	"kimi":             "Kimi Code (sign-in)",
	"orcarouter-oauth": "OrcaRouter (sign-in)",
	"qwen":             "Qwen (Alibaba sign-in)",
	"anthropic":        "Anthropic",
	"xai":              "xAI",
	"orcarouter":       "OrcaRouter",
	"github-copilot":   "GitHub Copilot",
}

// RewrittenLabel returns the current default when stored is still the
// retired default for that template. A label the operator chose is left
// alone.
func RewrittenLabel(templateID, stored string) (string, bool) {
	retired, found := retiredDefaults[templateID]
	if !found || stored != retired {
		return "", false
	}
	if held, ok := Curated(templateID); ok {
		return held.Label, true
	}
	return DisplayName(templateID, stored), true
}
