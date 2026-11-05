// Claude Desktop behavior mapping: which built-in model one published entry
// behaves as. The mapping is client-side metadata only; it grants no upstream
// capability.
package codingclients

import (
	"regexp"
	"strings"

	"github.com/jonaskahn/relo/internal/catalog"
)

// desktopSupportedBehaviors are the thinking behaviors the inspected Desktop
// definitions carry directly. Some canonical ids have no Desktop definition,
// so they resolve to a sibling below. Remove an entry from the fallback table
// only after inspecting a newer Desktop build and verifying its controls.
var desktopSupportedBehaviors = map[string]bool{
	"claude-haiku-4-5":  true,
	"claude-sonnet-4-5": true,
	"claude-sonnet-4-6": true,
	"claude-sonnet-5":   true,
	"claude-opus-4-6":   true,
	"claude-opus-4-7":   true,
	"claude-opus-4-8":   true,
	"claude-opus-5":     true,
}

var (
	claudeFamilyFirst   = regexp.MustCompile(`^claude-(sonnet|opus|haiku|fable|mythos)(?:-([0-9]+)(?:-([0-9]+))?)?$`)
	claudeVersionFirst  = regexp.MustCompile(`^claude-([0-9]+)(?:-([0-9]+))?-(sonnet|opus|haiku|fable|mythos)$`)
	claudeDateSuffix    = regexp.MustCompile(`-20[0-9]{6}$`)
	bedrockModelVersion = regexp.MustCompile(`-v[0-9]+(?::[0-9]+)?$`)
	bedrockRegionPrefix = regexp.MustCompile(`^(us|eu|apac|ca|sa|me|af)\.`)
	decimalVersion      = regexp.MustCompile(`([0-9])\.([0-9])`)
	familyLevelBehavior = regexp.MustCompile(`^claude-(fable|mythos)-[0-9]+(?:-[0-9]+)?$`)
	// desktopRefusedVendor mirrors the DS pattern of Claude Desktop 4.4212.0.
	// Re-check it against a newer Desktop build; if it drifts, fall back to
	// opaque ids only.
	desktopRefusedVendor  = regexp.MustCompile(`ark-code|astron|command-r|deepseek|doubao|gemini|gemma|glm|gpt|grok|hermes|hy3|kimi|lfm|\bling\b|llama|longcat|mimo|minimax|mistral|mixtral|moonshot|nemotron|openai|phi-|qianfan|qwen|tc-code|\bunic\b|yi-|stepfun|step-3|seed-|bytedance|hunyuan|granite|amazon\.nova|nova-|devstral|ministral|ernie|codex|arcee|trinity|abab|phi\d|\bk2\.|\bm2\.|jamba|arctic|solar|mercury|zamba|kat-coder|\bds-|dpsk`)
	desktopFamilyWord     = regexp.MustCompile(`sonnet|opus|haiku|fable|mythos`)
	desktopVersionSegment = regexp.MustCompile(`(^|-)[0-9]{1,2}(-|$)`)
)

// claudeTrailingQualifiers are the variant words a provider appends after a
// Claude version. They name a lane rather than a different model, so they are
// dropped before the family and version are read.
var claudeTrailingQualifiers = []string{
	"-extra-low", "-thinking", "-preview", "-latest", "-beta",
	"-fast", "-max", "-high", "-medium", "-low",
}

// desktopBehavesAs resolves one published model to the compatible Claude
// behavior its row carries. A route is classified from its own identifier;
// a concrete model is classified from its upstream identity, never from a
// connection name or a display label.
func desktopBehavesAs(model ModelRef) string {
	if desktopRoute(model) {
		if strings.Contains(strings.ToLower(model.ID), "flash") {
			return desktopCompatibleBehavior("claude-sonnet-5")
		}
		return desktopCompatibleBehavior("claude-sonnet-5-5")
	}
	identity := desktopSourceIdentity(model)
	if requested, ok := claudeBehavior(identity); ok {
		return desktopCompatibleBehavior(requested)
	}
	normalized := normalizeModelIdentity(identity)
	if isGPTIdentity(normalized) {
		return desktopCompatibleBehavior(gptBehavior(normalized))
	}
	if strings.Contains(normalized, "flash") {
		return desktopCompatibleBehavior("claude-sonnet-5")
	}
	return desktopCompatibleBehavior("claude-sonnet-5-5")
}

func desktopRoute(model ModelRef) bool {
	if model.Connection == catalog.RouteLabelPrefix {
		return true
	}
	id := strings.TrimSuffix(strings.TrimSpace(model.ID), catalog.ContextSuffix)
	if _, _, ok := catalog.AnthropicModelTarget(id); ok {
		return false
	}
	return strings.HasPrefix(id, catalog.RoutePrefix) ||
		strings.HasPrefix(id, catalog.ClaudePrefix+catalog.RoutePrefix)
}

// desktopSourceIdentity returns the model identity a row is classified from.
// The stored upstream id wins; a structured Claude alias is decoded next; a
// plain identifier is already the identity.
func desktopSourceIdentity(model ModelRef) string {
	if upstream := strings.TrimSpace(model.Upstream); upstream != "" {
		return upstream
	}
	if _, modelID, ok := catalog.AnthropicModelTarget(model.ID); ok {
		return modelID
	}
	return modelPortion(model.ID)
}

// modelPortion drops the synthetic Desktop alias and an undecoded Relo id.
// The alias is routing compatibility rather than model identity, and a Relo
// id would let the connection name supply a family or variant match. An
// identifier Relo cannot decode is opaque, so it takes the other-model rule.
func modelPortion(raw string) string {
	s := strings.TrimSpace(raw)
	lowered := strings.ToLower(s)
	if s == "" || strings.HasPrefix(lowered, catalog.DesktopAliasPrefix) ||
		strings.HasPrefix(lowered, catalog.LegacyDesktopAliasPrefix) ||
		strings.HasPrefix(lowered, catalog.ClientPrefix) {
		return ""
	}
	return s
}

// desktopRowName is the id one row lists in Claude Desktop. Desktop ignores
// behavesAs when a Claude name carries a version, so the exact picker id is
// published only when Desktop cannot read a family or version into it; every
// other row gets an opaque alias. The 1M marker stays in supports1m/prefer1m,
// and the snapshot resolves the stripped name back to that entry.
func desktopRowName(model ModelRef) string {
	picker := gatewayPickerID(model)
	name := catalog.StripContextMarker(picker)
	lowered := strings.ToLower(name)
	if strings.Contains(lowered, "claude") &&
		!desktopRefusedVendor.MatchString(lowered) &&
		!desktopFamilyWord.MatchString(lowered) &&
		!desktopVersionSegment.MatchString(lowered) {
		return name
	}
	return catalog.DesktopModelAlias(picker)
}

// claudeBehavior reads a recognizable Claude identity into its requested
// canonical behavior, preserving the family and version.
func claudeBehavior(raw string) (string, bool) {
	identity := normalizeClaudeIdentity(raw)
	if match := claudeVersionFirst.FindStringSubmatch(identity); match != nil {
		return canonicalClaudeBehavior(match[3], match[1], match[2]), true
	}
	if match := claudeFamilyFirst.FindStringSubmatch(identity); match != nil {
		return canonicalClaudeBehavior(match[1], match[2], match[3]), true
	}
	return "", false
}

func canonicalClaudeBehavior(family, major, minor string) string {
	canonical := "claude-" + family
	if major == "" {
		return canonical
	}
	canonical += "-" + major
	if minor != "" {
		canonical += "-" + minor
	}
	return canonical
}

// normalizeClaudeIdentity removes the provider wrappers around a Claude id
// and spells a decimal version the hyphenated way. It changes a working copy
// only, never the stored upstream id.
func normalizeClaudeIdentity(raw string) string {
	identity := normalizeModelIdentity(raw)
	for _, qualifier := range claudeTrailingQualifiers {
		identity = strings.TrimSuffix(identity, qualifier)
	}
	return identity
}

// normalizeModelIdentity lower-cases one identity and removes the namespaces,
// markers, and release suffixes providers wrap around it.
func normalizeModelIdentity(raw string) string {
	identity := strings.ToLower(strings.TrimSpace(raw))
	identity = strings.TrimSuffix(identity, strings.ToLower(catalog.ContextSuffix))
	if index := strings.LastIndex(identity, "/"); index >= 0 {
		identity = identity[index+1:]
	}
	if index := strings.Index(identity, "@"); index >= 0 {
		identity = identity[:index]
	}
	identity = bedrockRegionPrefix.ReplaceAllString(identity, "")
	identity = strings.TrimPrefix(identity, "anthropic.")
	identity = strings.TrimPrefix(identity, "openai.")
	identity = bedrockModelVersion.ReplaceAllString(identity, "")
	identity = claudeDateSuffix.ReplaceAllString(identity, "")
	return decimalVersion.ReplaceAllString(identity, "$1-$2")
}

func isGPTIdentity(identity string) bool {
	return identity == "gpt" || identity == "chatgpt" ||
		strings.HasPrefix(identity, "gpt-") || strings.HasPrefix(identity, "chatgpt-")
}

// gptBehavior applies the variant priority to whole tokens, so a word like
// professional never selects pro and pro wins over a later token.
func gptBehavior(identity string) string {
	tokens := identityTokens(identity)
	for _, variant := range []struct {
		names []string
		value string
	}{
		{names: []string{"pro"}, value: "claude-opus-4-6"},
		{names: []string{"astra"}, value: "claude-fable-5"},
		{names: []string{"sol"}, value: "claude-sonnet-5-5"},
		{names: []string{"terra"}, value: "claude-haiku-5-5"},
		{names: []string{"luna"}, value: "claude-haiku-4-5"},
		{names: []string{"mini", "nano"}, value: "claude-haiku-4-5"},
	} {
		if hasAnyToken(tokens, variant.names) {
			return variant.value
		}
	}
	return "claude-sonnet-4-6"
}

func identityTokens(identity string) []string {
	return strings.FieldsFunc(identity, func(char rune) bool {
		return (char < 'a' || char > 'z') && (char < '0' || char > '9')
	})
}

func hasAnyToken(tokens, names []string) bool {
	for _, token := range tokens {
		for _, name := range names {
			if token == name {
				return true
			}
		}
	}
	return false
}

// desktopCompatibleBehavior resolves a requested target through the shipped
// table. A target outside it falls back within its own Claude family, so a
// newer version keeps the family's behavior instead of borrowing another.
func desktopCompatibleBehavior(requested string) string {
	if desktopSupportedBehaviors[requested] {
		return requested
	}
	// Desktop keys its Fable and Mythos definitions by family, so every
	// numeric version of either is directly supported.
	if familyLevelBehavior.MatchString(requested) {
		return requested
	}
	family, _, _ := strings.Cut(strings.TrimPrefix(requested, "claude-"), "-")
	switch family {
	case "sonnet":
		return "claude-sonnet-5"
	case "haiku":
		return "claude-haiku-4-5"
	case "opus":
		return "claude-opus-5"
	case "fable":
		return "claude-fable-5"
	case "mythos":
		return "claude-mythos-5"
	default:
		return "claude-sonnet-5"
	}
}
