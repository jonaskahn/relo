// Client profiles: the known agents and their identifiers.
package access

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jonaskahn/relo/internal/inference"
)

// Setup placeholders are the values a client template carries until its
// setup fills them in: the token, the daemon address, and the model.
const (
	// CustomClient is the profile an operator picks when the client they are
	// wiring up is not one Relo writes instructions for.
	CustomClient = "custom"
	// TokenPlaceholder and its neighbours are what a template step carries
	// until a caller fills it in.
	TokenPlaceholder   = "{{token}}"
	BaseURLPlaceholder = "{{base_url}}"
	ModelPlaceholder   = "{{model}}"
	// DefaultModel is the model a step names when the caller picked none.
	DefaultModel = "gpt-4o"
)

// ErrProtocolDisabled reports a client that cannot be set up because the
// protocol it speaks is not served on any port.
var ErrProtocolDisabled = errors.New("the protocol this client speaks is not served")

// ClientProfile is one AI coding client Relo can hand a key to: what it is
// called, the protocol whose port it points at, the steps that point it at
// the data plane, and the command that proves the wiring works.
type ClientProfile struct {
	ID       string
	Name     string
	Summary  string
	Protocol string
	Steps    []string
	Verify   string
}

// ClientProfiles are the clients every build knows how to set up. The steps
// are data: a caller fills them in and prints them. The copy they return
// keeps callers from mutating the shipped profiles.
func ClientProfiles() []ClientProfile {
	out := make([]ClientProfile, len(clientProfiles))
	for i, profile := range clientProfiles {
		profile.Steps = append([]string(nil), profile.Steps...)
		out[i] = profile
	}
	return out
}

var clientProfiles = []ClientProfile{
	{
		ID:       "codex",
		Name:     "Codex CLI",
		Summary:  "Point the OpenAI provider of the Codex CLI at Relo",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Add to ~/.codex/config.toml:",
			"",
			"[model_providers.relo]",
			"name = \"Relo\"",
			"base_url = \"{{base_url}}/v1\"",
			"env_key = \"RELO_CODEX_API_KEY\"",
			"",
			"Then export the key the data plane expects:",
			"",
			"export RELO_CODEX_API_KEY={{token}}",
		},
		Verify: "codex --model {{model}} \"say hello\"",
	},
	{
		ID:       "claude-desktop",
		Name:     "Claude Desktop",
		Summary:  "Route Claude Desktop through Relo",
		Protocol: inference.ProtocolAnthropic,
		Steps: []string{
			"Turn on the Claude Desktop integration. Relo writes a gateway profile into Claude Desktop's config library.",
			"That profile points Desktop at {{base_url}} and lists the models Relo publishes.",
		},
		Verify: "Open Claude Desktop, pick a Relo model, and send a message. It appears on the Logs page.",
	},
	{
		ID:       "claude-code",
		Name:     "Claude Code",
		Summary:  "Point Claude Code at the Anthropic-compatible surface",
		Protocol: inference.ProtocolAnthropic,
		Steps: []string{
			"export ANTHROPIC_BASE_URL={{base_url}}",
			"export ANTHROPIC_AUTH_TOKEN={{token}}",
			"export ANTHROPIC_MODEL={{model}}",
		},
		Verify: "claude -p \"say hello\"",
	},
	{
		ID:       "cursor",
		Name:     "Cursor",
		Summary:  "Use Relo as an OpenAI-compatible endpoint in Cursor",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Settings -> Models -> OpenAI API Key: {{token}}",
			"Settings -> Models -> Override OpenAI Base URL: {{base_url}}/v1",
		},
		Verify: "Ask Cursor one question with the Relo model {{model}} selected.",
	},
	{
		ID:       "grok-build",
		Name:     "Grok Build",
		Summary:  "Point Grok Build at the data plane",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"export GROK_API_BASE={{base_url}}/v1",
			"export GROK_API_KEY={{token}}",
		},
		Verify: "grok --model {{model}} \"say hello\"",
	},
	// The export clients. Each reads a provider block of its own shape, so the
	// steps name the file an operator edits, the address this machine serves,
	// and the model identifier Relo publishes.
	{
		ID:       "opencode",
		Name:     "OpenCode",
		Summary:  "Add a custom provider block to OpenCode's config",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Add to ~/.config/opencode/opencode.json under \"provider\":",
			"",
			"\"relo\": { \"name\": \"Relo\", \"options\": { \"baseURL\": \"{{base_url}}/v1\", \"apiKey\": \"{{token}}\" },",
			"  \"models\": { \"{{model}}\": {} } }",
		},
		Verify: "Select the Relo model in OpenCode and send one message.",
	},
	{
		ID:       "cline",
		Name:     "Cline",
		Summary:  "Add a custom provider to Cline's provider settings",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Add to ~/.cline/data/settings/providers.json under \"providers\":",
			"",
			"\"relo\": { \"settings\": { \"provider\": \"relo\", \"baseUrl\": \"{{base_url}}\",",
			"  \"protocol\": \"openai-responses\", \"client\": \"openai\", \"apiKey\": \"{{token}}\" } }",
			"",
			"Then add the matching entry to the sibling models.json catalog.",
		},
		Verify: "Ask Cline one question with the Relo model selected.",
	},
	{
		ID:       "pi",
		Name:     "Pi",
		Summary:  "Add a custom provider to Pi's model catalog",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Add to ~/.pi/agent/models.json under \"providers\":",
			"",
			"\"relo\": { \"baseUrl\": \"{{base_url}}\", \"api\": \"openai-completions\",",
			"  \"apiKey\": \"{{token}}\", \"models\": [{ \"id\": \"{{model}}\" }] }",
		},
		Verify: "Select the Relo model in Pi and send one message.",
	},
	{
		ID:       "omp",
		Name:     "Oh My Pi",
		Summary:  "Add a custom provider to Oh My Pi's model catalog",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Add to ~/.omp/agent/models.yml under \"providers\":",
			"",
			"relo:",
			"  baseUrl: {{base_url}}",
			"  api: openai-completions",
			"  apiKey: {{token}}",
			"  models:",
			"    - id: {{model}}",
		},
		Verify: "Select the Relo model in Oh My Pi and send one message.",
	},
	{
		ID:       "hermes",
		Name:     "Hermes",
		Summary:  "Add a custom provider block to Hermes's config",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Add to ~/.hermes/config.yaml under \"providers\":",
			"",
			"relo:",
			"  api: {{base_url}}/v1",
			"  api_key: {{token}}",
			"  api_mode: chat_completions",
			"  discover_models: false",
			"  models:",
			"    {{model}}: {}",
		},
		Verify: "Select the Relo model in Hermes and send one message.",
	},
	{
		ID:       "openclaw",
		Name:     "OpenClaw",
		Summary:  "Add a custom provider block to OpenClaw's config",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Add to ~/.openclaw/openclaw.json5:",
			"",
			"\"relo\": { \"baseUrl\": \"{{base_url}}/v1\", \"apiKey\": \"{{token}}\",",
			"  \"api\": \"openai-completions\", \"models\": [{ \"id\": \"{{model}}\", \"name\": \"Relo\" }] }",
		},
		Verify: "Select the Relo model in OpenClaw and send one message.",
	},
	{
		ID:       "kimi",
		Name:     "Kimi Code",
		Summary:  "Add a custom provider block to Kimi Code's config",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Add to ~/.kimi-code/config.toml:",
			"",
			"[providers.relo]",
			"base_url = \"{{base_url}}/v1\"",
			"api_key = \"{{token}}\"",
			"models = [\"{{model}}\"]",
		},
		Verify: "Select the Relo model in Kimi Code and send one message.",
	},
	{
		ID:       "gajae",
		Name:     "Gajae",
		Summary:  "Add a custom provider to Gajae's model catalog",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Add to ~/.gjc/agent/models.yml under \"providers\":",
			"",
			"relo:",
			"  baseUrl: {{base_url}}",
			"  api: openai-completions",
			"  models:",
			"    - id: {{model}}",
		},
		Verify: "Select the Relo model in Gajae and send one message.",
	},
	{
		ID:       "dsh",
		Name:     "DeepSeek Harness",
		Summary:  "Add a custom provider block to the DeepSeek Harness settings",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Add to ~/.dsh/settings.yaml under \"providers\":",
			"",
			"relo:",
			"  baseUrl: {{base_url}}",
			"  apiKey: {{token}}",
			"  models:",
			"    - id: {{model}}",
		},
		Verify: "Select the Relo model in the harness and send one message.",
	},
	{
		ID:       "mcode",
		Name:     "MiniMax Code",
		Summary:  "Add a custom provider block to MiniMax Code's config",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Add to ~/.minimax/config.yaml:",
			"",
			"relo:",
			"  baseURL: {{base_url}}",
			"  apiKey: {{token}}",
			"  models:",
			"    - id: {{model}}",
		},
		Verify: "Select the Relo model in MiniMax Code and send one message.",
	},
	{
		ID:       "zcode",
		Name:     "ZCode",
		Summary:  "Add a custom provider block to ZCode's config",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Add to ~/.zcode/v2/config.json under \"providers\":",
			"",
			"\"relo\": { \"baseURL\": \"{{base_url}}/v1\", \"apiKey\": \"{{token}}\",",
			"  \"models\": [\"{{model}}\"] }",
		},
		Verify: "Select the Relo model in ZCode and send one message.",
	},
	{
		ID:       "prime",
		Name:     "Prime",
		Summary:  "Add a custom provider to Prime's model catalog",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Add to ~/.prime/agent/models.json under \"providers\":",
			"",
			"\"relo\": { \"baseUrl\": \"{{base_url}}\", \"api\": \"openai-completions\",",
			"  \"apiKey\": \"{{token}}\", \"models\": [{ \"id\": \"{{model}}\" }] }",
		},
		Verify: "Select the Relo model in Prime and send one message.",
	},
	{
		ID:       "aside",
		Name:     "Aside",
		Summary:  "Add a custom provider to Aside's model catalog",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Add to ~/.aside/u/<account>/models.json under \"providers\":",
			"",
			"\"relo\": { \"baseUrl\": \"{{base_url}}\", \"api\": \"openai-completions\",",
			"  \"models\": [{ \"id\": \"{{model}}\" }] }",
		},
		Verify: "Select the Relo model in Aside and send one message.",
	},
	{
		ID:       "raycast",
		Name:     "Raycast AI",
		Summary:  "Add Relo as a custom provider in Raycast",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Add to ~/.config/raycast/ai/providers.yaml:",
			"",
			"providers:",
			"  - id: relo",
			"    name: Relo",
			"    base_url: {{base_url}}",
			"    models:",
			"      - id: {{model}}",
			"        name: Relo",
		},
		Verify: "Select the Relo model in Raycast AI and send one message.",
	},
	{
		ID:       "omo",
		Name:     "OMO",
		Summary:  "Add a custom provider to OMO's model catalog",
		Protocol: inference.ProtocolOpenAI,
		Steps: []string{
			"Add to ~/.omo/agent/models.json under \"providers\":",
			"",
			"\"relo\": { \"baseUrl\": \"{{base_url}}\", \"api\": \"openai-completions\",",
			"  \"models\": [{ \"id\": \"{{model}}\" }] }",
		},
		Verify: "Select the Relo model in OMO and send one message.",
	},
}

// ClientProfileFor returns the profile with the given identifier.
func ClientProfileFor(id string) (ClientProfile, bool) {
	for _, profile := range clientProfiles {
		if profile.ID == strings.TrimSpace(id) {
			return profile, true
		}
	}
	return ClientProfile{}, false
}

// ClientIDs lists the identifiers a profile exists for, in catalog order.
func ClientIDs() []string {
	ids := make([]string, 0, len(clientProfiles))
	for _, profile := range clientProfiles {
		ids = append(ids, profile.ID)
	}
	return ids
}

// KnownClientIDs renders the identifiers one message can name.
func KnownClientIDs() string {
	return strings.Join(ClientIDs(), ", ")
}

// Listener answers where one client protocol is served, which is all this
// package needs to hand a client an address. The caller passes the startup
// configuration, so this package reads no configuration type of its own.
type Listener interface {
	// DataPlaneAddr returns the host:port a protocol answers on, and an empty
	// string when no port serves it.
	DataPlaneAddr(protocol string) string
}

// ClientBaseURL is the address a client of one protocol points at. It reports
// ErrProtocolDisabled when no port serves the protocol, because an address on
// no port would only mislead the operator reading it.
func ClientBaseURL(protocol string, listener Listener) (string, error) {
	address := listener.DataPlaneAddr(protocol)
	if address == "" {
		return "", fmt.Errorf("server.data_plane.%s: %w", protocol, ErrProtocolDisabled)
	}
	return "http://" + address, nil
}

// SetupFill is the data one profile's steps are filled in with.
type SetupFill struct {
	Token   string
	BaseURL string
	Model   string
}

// Instructions returns the profile's steps filled in for one key.
func (c ClientProfile) Instructions(fill SetupFill) []string {
	steps := make([]string, 0, len(c.Steps))
	for _, step := range c.Steps {
		steps = append(steps, fill.Apply(step))
	}
	return steps
}

// Verification returns the command that proves the wiring works.
func (c ClientProfile) Verification(fill SetupFill) string {
	return fill.Apply(c.Verify)
}

// Apply replaces the placeholders one step carries. It is a data
// substitution, never a template engine: nothing here executes.
func (f SetupFill) Apply(line string) string {
	r := strings.NewReplacer(
		TokenPlaceholder, f.Token,
		BaseURLPlaceholder, f.BaseURL,
		ModelPlaceholder, f.Model,
	)
	return r.Replace(line)
}
