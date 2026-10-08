// Package codingclients owns what the coding agents on this machine expect of
// Relo: the paths an agent reads, the configuration file Relo merges into,
// and the key helper that starts the daemon on demand. Every write snapshots
// first, every action has an undo, and a file Relo does not recognise is
// refused rather than rewritten.
package codingclients

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jonaskahn/relo/internal/inference"
)

// Agent is one coding client Relo knows how to wire.
type Agent struct {
	ID       string
	Client   string
	Protocol string
	Summary  string
	// Label names the client the way an operator reads it. It is empty for the
	// clients the console already labels, which fall back to their identifier.
	Label string
	// ManagesFiles reports whether Relo writes the client's own configuration
	// file, which is what separates an integration that sets itself up from one
	// that hands over a key and a set of steps.
	ManagesFiles bool
	// EnvKey is the environment variable Relo publishes for this client, so
	// the secret stays in Relo's key file rather than in that configuration.
	EnvKey string
}

// The client identifiers Relo writes a configuration document for. They match
// the reference project's export clients, so a machine already wired with one
// of them keeps the same name here.
const (
	ClientOpenCode = "opencode"
	ClientPi       = "pi"
	ClientOMP      = "omp"
	ClientHermes   = "hermes"
	ClientOpenClaw = "openclaw"
	ClientKimi     = "kimi"
	ClientGajae    = "gajae"
	ClientDSH      = "dsh"
	ClientMCode    = "mcode"
	ClientZCode    = "zcode"
	ClientPrime    = "prime"
	ClientAside    = "aside"
	ClientRaycast  = "raycast"
	ClientOMO      = "omo"
	ClientCline    = "cline"
)

// Agents is every integration this build knows, in the order a console lists
// them: the clients Relo configures itself first, then the clients it hands a
// key and a generated document to. The identifiers match the client profiles a
// key is issued against, so a key an integration owns is attributable in the
// log. The copy they return keeps callers from mutating the shipped roster.
func Agents() []Agent {
	return append([]Agent(nil), agents...)
}

var agents = []Agent{
	{ID: "codex", Client: "codex", Protocol: inference.ProtocolOpenAI, Label: "Codex CLI",
		Summary:      "Point the Codex CLI at Relo and start the daemon when it launches",
		ManagesFiles: true, EnvKey: EnvKeyName("codex")},
	{ID: "claude-code", Client: "claude-code", Protocol: inference.ProtocolAnthropic, Label: "Claude Code",
		Summary:      "Point Claude Code at the Anthropic surface Relo serves",
		ManagesFiles: true, EnvKey: EnvKeyName("claude-code")},
	{ID: "claude-desktop", Client: "claude-desktop", Protocol: inference.ProtocolAnthropic, Label: "Claude Desktop",
		Summary: "Point Claude Desktop at the Anthropic surface Relo serves", ManagesFiles: true, EnvKey: EnvKeyName("claude-desktop")},
	{ID: "cursor", Client: "cursor", Protocol: inference.ProtocolOpenAI, Label: "Cursor",
		Summary: "Use Relo as an OpenAI-compatible endpoint in Cursor", EnvKey: EnvKeyName("cursor")},
	{ID: "grok-build", Client: "grok-build", Protocol: inference.ProtocolOpenAI, Label: "Grok Build",
		Summary: "Point Grok Build at the data plane", EnvKey: EnvKeyName("grok-build")},

	// Every integration publishes its own Relo-named variable, so two never
	// share a secret. A client Relo writes a file for expands that variable.
	// Cursor and Grok Build still take a key in their setup steps.
	{ID: ClientOpenCode, Client: ClientOpenCode, Protocol: inference.ProtocolOpenAI, Label: "OpenCode",
		Summary: "Point OpenCode at Relo with a custom provider block", ManagesFiles: true, EnvKey: EnvKeyName(ClientOpenCode)},
	{ID: ClientCline, Client: ClientCline, Protocol: inference.ProtocolOpenAI, Label: "Cline",
		Summary: "Point Cline at Relo with custom provider settings and a model catalog", ManagesFiles: true, EnvKey: EnvKeyName(ClientCline)},
	{ID: ClientPi, Client: ClientPi, Protocol: inference.ProtocolOpenAI, Label: "Pi",
		Summary: "Point Pi at Relo with a custom provider catalog", ManagesFiles: true, EnvKey: EnvKeyName(ClientPi)},
	{ID: ClientOMP, Client: ClientOMP, Protocol: inference.ProtocolOpenAI, Label: "Oh My Pi",
		Summary: "Point Oh My Pi at Relo with a custom provider catalog", ManagesFiles: true, EnvKey: EnvKeyName(ClientOMP)},
	{ID: ClientHermes, Client: ClientHermes, Protocol: inference.ProtocolOpenAI, Label: "Hermes",
		Summary: "Point Hermes at Relo with a custom provider block", ManagesFiles: true, EnvKey: EnvKeyName(ClientHermes)},
	{ID: ClientOpenClaw, Client: ClientOpenClaw, Protocol: inference.ProtocolOpenAI, Label: "OpenClaw",
		Summary: "Point OpenClaw at Relo with a custom provider block", ManagesFiles: true, EnvKey: EnvKeyName(ClientOpenClaw)},
	{ID: ClientKimi, Client: ClientKimi, Protocol: inference.ProtocolOpenAI, Label: "Kimi Code",
		Summary: "Point Kimi Code at Relo with a custom provider block", ManagesFiles: true, EnvKey: EnvKeyName(ClientKimi)},
	{ID: ClientGajae, Client: ClientGajae, Protocol: inference.ProtocolOpenAI, Label: "Gajae",
		Summary: "Point Gajae at Relo with a custom provider catalog", ManagesFiles: true, EnvKey: EnvKeyName(ClientGajae)},
	{ID: ClientDSH, Client: ClientDSH, Protocol: inference.ProtocolOpenAI, Label: "DeepSeek Harness",
		Summary: "Point the DeepSeek Harness at Relo with a custom provider block", ManagesFiles: true, EnvKey: EnvKeyName(ClientDSH)},
	{ID: ClientMCode, Client: ClientMCode, Protocol: inference.ProtocolOpenAI, Label: "MiniMax Code",
		Summary: "Point MiniMax Code at Relo with a custom provider block", ManagesFiles: true, EnvKey: EnvKeyName(ClientMCode)},
	{ID: ClientZCode, Client: ClientZCode, Protocol: inference.ProtocolOpenAI, Label: "ZCode",
		Summary: "Point ZCode at Relo with a custom provider block", ManagesFiles: true, EnvKey: EnvKeyName(ClientZCode)},
	{ID: ClientPrime, Client: ClientPrime, Protocol: inference.ProtocolOpenAI, Label: "Prime",
		Summary: "Point Prime at Relo with a custom provider catalog", ManagesFiles: true, EnvKey: EnvKeyName(ClientPrime)},
	{ID: ClientAside, Client: ClientAside, Protocol: inference.ProtocolOpenAI, Label: "Aside",
		Summary: "Point Aside at Relo with a custom provider catalog", ManagesFiles: true, EnvKey: EnvKeyName(ClientAside)},
	{ID: ClientRaycast, Client: ClientRaycast, Protocol: inference.ProtocolOpenAI, Label: "Raycast",
		Summary: "Point Raycast AI at Relo with a custom provider entry", ManagesFiles: true, EnvKey: EnvKeyName(ClientRaycast)},
	{ID: ClientOMO, Client: ClientOMO, Protocol: inference.ProtocolOpenAI, Label: "OMO",
		Summary: "Point OMO at Relo with a custom provider catalog", ManagesFiles: true, EnvKey: EnvKeyName(ClientOMO)},
}

// AgentFor returns the agent with the given identifier.
func AgentFor(id string) (Agent, bool) {
	for _, agent := range agents {
		if agent.ID == strings.TrimSpace(id) {
			return agent, true
		}
	}
	return Agent{}, false
}

// Paths is every location one integration reads or writes: the operator's
// home, Relo's state directory, and the executable Relo starts the daemon
// with. The client's own environment overrides are honoured, so a machine
// that keeps Codex or Claude Code somewhere else is wired where it actually
// lives.
type Paths struct {
	Home      string
	StateHome string
	// Relo is the executable a key helper starts the daemon with.
	Relo string
	// DataPlane answers the base URL of one client protocol.
	DataPlane func(protocol string) string
}

// CodexConfig returns the Codex configuration file.
func (p Paths) CodexConfig() string {
	if dir := strings.TrimSpace(os.Getenv("CODEX_HOME")); dir != "" {
		return filepath.Join(dir, "config.toml")
	}
	return filepath.Join(p.Home, ".codex", "config.toml")
}

// ClaudeConfigDir returns the directory Claude Code reads, honouring
// CLAUDE_CONFIG_DIR when the operator keeps it somewhere else.
func (p Paths) ClaudeConfigDir() string {
	if dir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); dir != "" {
		return dir
	}
	return filepath.Join(p.Home, ".claude")
}

// ClaudeSettings returns the Claude Code settings file.
func (p Paths) ClaudeSettings() string {
	return filepath.Join(p.ClaudeConfigDir(), "settings.json")
}

// AgentDir returns the directory Relo keeps one integration's own files in.
func (p Paths) AgentDir(id string) string {
	return filepath.Join(p.StateHome, "integrations", id)
}

// KeyFile returns the mode-0600 file holding the raw client key one
// integration runs with, which is what a key helper reads while the daemon is
// down.
func (p Paths) KeyFile(id string) string {
	return filepath.Join(p.AgentDir(id), "key")
}

// HelperFile returns the script a coding client reads its key from.
func (p Paths) HelperFile(id string) string {
	name := "key-helper"
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	return filepath.Join(p.AgentDir(id), name)
}

// LauncherState returns the file an earlier version recorded a wrapped
// launcher in.
func (p Paths) LauncherState(id string) string {
	return filepath.Join(p.AgentDir(id), "launcher.json")
}

// SnapshotDir returns the directory the pre-write copies of a client's own
// files are kept in.
func (p Paths) SnapshotDir() string {
	return filepath.Join(p.StateHome, "integrations", "snapshots")
}

// BaseURL answers the address one protocol is served on, and an empty string
// when nothing serves it.
func (p Paths) BaseURL(protocol string) string {
	if p.DataPlane == nil {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSpace(p.DataPlane(protocol)), "/")
}
