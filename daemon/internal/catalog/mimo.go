// MiMo family: the models one serving template answers and how it states
// thinking.
package catalog

import "strings"

// IsMimoReasoningText reports whether one id names a MiMo chat model the
// vendor serves through one template. The family is matched on the model id
// rather than the host, so every gateway that lists MiMo obeys the same
// thinking and tool-call rules. Speech and image variants are not chat
// models: they share the vendor name but answer a different template.
func IsMimoReasoningText(id string) bool {
	lowered := strings.ToLower(strings.TrimSpace(id))
	name := lowered[strings.LastIndex(lowered, "/")+1:]
	if !strings.HasPrefix(name, "mimo-v") || len(name) == len("mimo-v") {
		return false
	}
	if generation := name[len("mimo-v")]; generation < '0' || generation > '9' {
		return false
	}
	for _, variant := range []string{"tts", "asr", "voice", "clone", "design"} {
		if strings.Contains(name, variant) {
			return false
		}
	}
	return true
}

func applyMimoFamily(res *Model) {
	if !IsMimoReasoningText(res.ID) && !IsMimoReasoningText(res.UpstreamID) {
		return
	}
	if res.SupportsReasoning != nil && !*res.SupportsReasoning {
		return
	}
	if res.SupportsReasoning == nil {
		yes := true
		res.SupportsReasoning = &yes
	}
	if len(res.ReasoningEfforts) > 0 {
		return
	}
	res.ReasoningEfforts = []string{}
	res.ReasoningToggle = true
	res.ReasoningToggleKnown = true
}

func mimoToolCallDialect(res Model) string {
	if IsMimoReasoningText(res.ID) || IsMimoReasoningText(res.UpstreamID) {
		return ToolCallDialectText
	}
	return ""
}
