// Catalog naming: slugs and client-facing model names.
package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strconv"
	"strings"
)

// Every identifier Relo shows a coding agent is namespaced, so a client reads
// one name while Relo decides how that name is served, and a route is never
// confused with a model a connection happens to serve under the same spelling.
//
// A name a client asks for is a slug: the characters a coding agent can carry
// in a picker without escaping, joined by hyphens. Because a hyphen is both
// the separator and a character an upstream identifier may hold, a public name
// is never split to recover its parts. It is resolved through the exact map a
// snapshot builds, so `relo-openai-gpt-4o` names one entry and nothing else.
const (
	// ClientPrefix namespaces a connection's model: relo-<provider>-<model>.
	ClientPrefix = "relo-"
	// RoutePrefix namespaces a route: reloc-<route>.
	RoutePrefix = "reloc-"
	// ClaudeSubscriptionTemplate is the template of a Claude.ai sign-in
	// connection. Only models served through such a connection take the
	// native and beta 1M rules; a Claude model on any other connection is
	// published with the generated 1M suffix like every other model there.
	ClaudeSubscriptionTemplate = "claude"
	// ClaudePrefix is the spelling Claude Code's model picker keeps. The picker
	// drops every id that does not start with "claude" or "anthropic", so the
	// Anthropic surface lists each public name with this prefix and resolves
	// it back off.
	ClaudePrefix = "claude-"
	// ClaudeAliasSeparator splits the provider from the model in a Claude Code
	// id. A single hyphen cannot, because a model id may itself start with
	// claude- and still has to round-trip unchanged.
	ClaudeAliasSeparator = "--"
	// ContextSuffix marks an identifier a Claude Code client reads as offering
	// a million-token window.
	ContextSuffix = "[1m]"
	// MillionContext is the window an entry needs before that marker is added.
	MillionContext int64 = 1_000_000
	// DefaultContext is the window a prompt stays in the base rate at,
	// published beside a separate 1M entry.
	DefaultContext int64 = 200_000
	// MillionSlugSuffix distinguishes the 1M entry on clients that do not use
	// Claude Code's bracketed context marker.
	MillionSlugSuffix = "-1m"
	// SlugSeparator joins the parts of one public identifier.
	SlugSeparator = "-"
	// LabelSeparator separates the parts of the display name a client's own
	// picker shows beside the identifier.
	LabelSeparator = " | "
	// RouteLabelPrefix names the product a route's display name belongs to.
	RouteLabelPrefix = "Relo"
	// contextPrecision is the token count a published window is rounded to, so
	// the size a client is offered is one an operator reads as a round thousand
	// rather than a figure no preset offers.
	contextPrecision int64 = 1_000
)

// Slug renders one source identifier in the form a coding agent reads. Lower
// case ASCII letters, digits, dot, and underscore are kept; an existing hyphen
// is kept and runs of anything else collapse to a single hyphen; leading and
// trailing hyphens are trimmed. An identifier that is already a slug is
// returned unchanged, so a stable upstream id keeps its spelling.
func Slug(raw string) string {
	var builder strings.Builder
	builder.Grow(len(raw))
	pendingSeparator := false
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_':
			if pendingSeparator && builder.Len() > 0 {
				builder.WriteString(SlugSeparator)
			}
			pendingSeparator = false
			builder.WriteRune(r)
		default:
			// A hyphen and every character a client cannot carry collapse to
			// one separator between the parts that remain.
			pendingSeparator = true
		}
	}
	return builder.String()
}

// ClientModelID names one connection's model the way a client asks for it.
func ClientModelID(providerID, modelID string) string {
	return ClientPrefix + Slug(providerID) + SlugSeparator + Slug(modelID)
}

// ClientModelName is the label Claude Code and Claude Desktop show for one
// connection's model: the model, then the connection. A million-token window,
// the same size that earns the [1m] identifier suffix, is marked in front of
// the connection.
func ClientModelName(modelLabel, providerLabel string, window *int64) string {
	label := strings.TrimSpace(modelLabel)
	if window != nil && *window >= MillionContext {
		if label == "" {
			label = "1M"
		} else {
			label += " 1M"
		}
	}
	provider := strings.TrimSpace(providerLabel)
	if provider == "" {
		return label
	}
	if label == "" {
		return provider
	}
	return label + " on " + provider
}

// StripClaudePrefix drops the vendor word a Claude.ai connection's own models
// carry, so the picker reads Opus rather than Claude Opus. Models on any
// other connection keep their names untouched.
func StripClaudePrefix(label string) string {
	trimmed := strings.TrimSpace(label)
	if trimmed == "Claude" {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(trimmed, "Claude "))
}

// ConfigModelName is the label a client's own configuration file shows: the
// model, then the provider. A route names Relo as that provider.
func ConfigModelName(modelLabel, providerLabel string, window *int64) string {
	return ClientModelName(modelLabel, providerLabel, window)
}

// ClientRouteID names one route the way a client asks for it.
func ClientRouteID(routeID string) string {
	return RoutePrefix + Slug(routeID)
}

// ClientRouteName is the label a managed client's configuration shows for one
// route. It names Relo rather than a connection, because a route is Relo's own
// choice of where to send a request. Claude's picker uses ClientModelName, so
// the route reads as the group by Relo.
func ClientRouteName(groupLabel string) string {
	return RouteLabelPrefix + LabelSeparator + groupLabel
}

var nativeMillionContext = map[string]bool{
	"claude-sonnet-5":   true,
	"claude-sonnet-5-5": true,
	"claude-haiku-5-5":  true,
	"claude-opus-4-7":   true,
	"claude-opus-4-8":   true,
	"claude-opus-5":     true,
	"claude-opus-5-5":   true,
	"claude-fable-5":    true,
	"claude-fable-5-1":  true,
	"claude-mythos-5":   true,
	"claude-mythos-5-1": true,
}

const releaseDateLength = 9

// NativeMillionContext reports whether one model already carries a million-token
// window of its own, which makes a separate long-context entry meaningless for
// it: the window is not something to opt into. A model the operator has narrowed
// below a million tokens is never native, however it is named.
func NativeMillionContext(modelID string, window *int64) bool {
	if window == nil || *window < MillionContext {
		return false
	}
	return nativeMillionContext[nativeModelKey(modelID)]
}

// ProviderMillionMarker reports whether an upstream identifier already names
// itself a million-token model: a router that publishes kimi-k3 and kimi-k3[1M]
// has said which of the two is the large one, and adding a twin beside it
// would only give the same window a second name.
func ProviderMillionMarker(modelID string, window *int64) bool {
	return window != nil && *window >= MillionContext && HasContextMarker(modelID)
}

// HasContextMarker reports whether an upstream identifier carries Claude Code's
// million-token marker itself. A router sells its large variant under a name like
// kimi-k3[1M], and providers are inconsistent about the case, so the marker is
// read whichever way it is spelled.
func HasContextMarker(modelID string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(modelID)),
		strings.ToLower(ContextSuffix))
}

// StripContextMarker removes a million-token marker an identifier already
// carries, so the marker Relo publishes is applied once rather than twice.
func StripContextMarker(modelID string) string {
	trimmed := strings.TrimSpace(modelID)
	if !HasContextMarker(trimmed) {
		return trimmed
	}
	return trimmed[:len(trimmed)-len(ContextSuffix)]
}

func nativeModelKey(modelID string) string {
	name := strings.TrimSpace(modelID)
	cut := len(name) - releaseDateLength
	if cut > 0 && name[cut] == '-' && isDigits(name[cut+1:]) {
		return name[:cut]
	}
	return name
}

func isDigits(text string) bool {
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// StripContextSuffix takes the million-token marker a Claude Code client may
// spell off one requested name, so both spellings of a name reach the same
// entry.
func StripContextSuffix(requested string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(requested), ContextSuffix))
}

// MillionAlias returns the non-Anthropic spelling of a 1M model entry.
func MillionAlias(publicID string) string {
	return strings.TrimSpace(publicID) + MillionSlugSuffix
}

// StripMillionAlias removes the non-Anthropic million-token marker one entry
// already carries, so the same entry can be spelled the way Claude Code reads
// without carrying the marker twice.
func StripMillionAlias(publicID string) string {
	return strings.TrimSuffix(strings.TrimSpace(publicID), MillionSlugSuffix)
}

// HasMillionContextSuffix reports the explicit Claude Code 1M spelling.
func HasMillionContextSuffix(requested string) bool {
	return strings.HasSuffix(strings.TrimSpace(requested), ContextSuffix)
}

// AnthropicClientID is the identifier the Anthropic model list publishes for
// a route, and for a connection whose provider id cannot be split back out.
// The claude- prefix is what Claude Code's picker keeps, and the million-token
// marker, when the window earns one, stays at the end.
func AnthropicClientID(publicID string, window *int64) string {
	return ClaudePrefix + publicID + ClientContextSuffix(window)
}

// AnthropicModelAlias is the identifier Claude Code lists for one connection's
// model. The provider and the model sit on either side of a double hyphen, so
// a model that already starts with claude- is not given a second picker prefix.
// A provider id that itself contains that separator keeps the public spelling,
// which is never split.
func AnthropicModelAlias(providerID, modelID string, window *int64) string {
	provider := strings.TrimSpace(providerID)
	model := strings.TrimSpace(modelID)
	if provider == "" || model == "" || strings.Contains(provider, ClaudeAliasSeparator) {
		return AnthropicClientID(ClientModelID(providerID, modelID), window)
	}
	return ClaudePrefix + ClientPrefix + provider + ClaudeAliasSeparator + model + ClientContextSuffix(window)
}

// DesktopAliasPrefix starts every id Relo gives Claude Desktop. Desktop drops a
// gateway model whose name carries another vendor's word (gpt, grok, codex,
// deepseek, and so on), so Desktop is offered an opaque Claude-shaped id and
// the snapshot resolves it back to the model.
const DesktopAliasPrefix = "claude-opus-4-8-r"

// DesktopModelAlias is the id Claude Desktop lists for one Claude picker id.
// It is a hash of that id, so it stays the same for the same model and holds
// none of the words Desktop refuses. The million-token marker is not part of
// it. The context marker remains in the hash seed so the default and 1M
// entries stay distinct.
func DesktopModelAlias(pickerID string) string {
	seed := strings.TrimSpace(pickerID)
	for attempt := 0; ; attempt++ {
		sum := sha256.Sum256([]byte(seed + "\x00" + strconv.Itoa(attempt)))
		code := hex.EncodeToString(sum[:6])
		// Desktop's vendor list holds "abab", the one word hex digits can spell.
		if !strings.Contains(code, "abab") {
			return DesktopAliasPrefix + code
		}
	}
}

// AnthropicModelTarget reads a Claude Code model alias back into the
// connection and the model. The first double hyphen is the split, so a model
// id may contain another one. A name that is not this alias reports ok false.
func AnthropicModelTarget(name string) (providerID, modelID string, ok bool) {
	bare := StripContextSuffix(name)
	rest, found := strings.CutPrefix(bare, ClaudePrefix+ClientPrefix)
	if !found {
		return "", "", false
	}
	providerID, modelID, found = strings.Cut(rest, ClaudeAliasSeparator)
	if !found || providerID == "" || modelID == "" || strings.Contains(providerID, ClaudeAliasSeparator) {
		return "", "", false
	}
	return providerID, modelID, true
}

// StripClaudeAlias returns the public identifier a Claude Code picker name
// refers to. A name that is not one of Relo's aliases is returned unchanged.
func StripClaudeAlias(name string) string {
	rest, ok := strings.CutPrefix(name, ClaudePrefix)
	if !ok {
		return name
	}
	if strings.HasPrefix(rest, ClientPrefix) || strings.HasPrefix(rest, RoutePrefix) {
		return rest
	}
	return name
}

// ClaudeAlias reports whether a requested name is the Anthropic spelling of
// a public identifier Relo published.
func ClaudeAlias(name string) bool {
	bare := StripContextSuffix(name)
	return StripClaudeAlias(bare) != bare
}

// NativeClaudeID reports a model Claude Code considers its own. Relo did not
// publish it, so a login session forwards it to Anthropic with the caller's
// credential.
func NativeClaudeID(name string) bool {
	bare := StripContextSuffix(name)
	if ClaudeAlias(bare) {
		return false
	}
	lower := strings.ToLower(bare)
	return strings.HasPrefix(lower, "claude") || strings.HasPrefix(lower, "anthropic")
}

// ClientContextSuffix is the marker a client reads as a million-token window,
// empty for an entry that states no such size.
func ClientContextSuffix(window *int64) string {
	if window != nil && *window >= MillionContext {
		return ContextSuffix
	}
	return ""
}

// RoundContextWindow states the window a client is actually offered, as the
// nearest thousand an operator reads in K or M. A window is a count of tokens
// rather than an exact promise, so 262,144 is offered as 262,000 and 1,456,789
// as 1,457,000.
func RoundContextWindow(window *int64) *int64 {
	if window == nil {
		return nil
	}
	rounded := nearestThousand(*window)
	return &rounded
}

// ParseTokenCount reads a token count. A bare number is tokens. A K or M
// suffix is decimal, so 300k is 300,000 and 1.5M is 1,500,000. The count is
// not rounded; callers that publish a window round it afterwards.
func ParseTokenCount(raw string) (int64, bool) {
	text := strings.ToLower(strings.NewReplacer(" ", "", "_", "", ",", "").Replace(strings.TrimSpace(raw)))
	if text == "" {
		return 0, false
	}
	scale := 1.0
	switch {
	case strings.HasSuffix(text, "m"):
		scale = 1_000_000
		text = strings.TrimSuffix(text, "m")
	case strings.HasSuffix(text, "k"):
		scale = 1_000
		text = strings.TrimSuffix(text, "k")
	}
	amount, err := strconv.ParseFloat(text, 64)
	if err != nil || amount < 0 || math.IsInf(amount, 0) {
		return 0, false
	}
	value := int64(math.Round(amount * scale))
	if value < 0 {
		return 0, false
	}
	return value, true
}

func nearestThousand(value int64) int64 {
	if value < 0 {
		return -nearestThousand(-value)
	}
	return ((value + contextPrecision/2) / contextPrecision) * contextPrecision
}
