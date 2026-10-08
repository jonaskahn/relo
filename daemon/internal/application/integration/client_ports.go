// Client ports: the coding-client surface the setup workflows run on.
package integration

import (
	"context"
	"errors"
	"os"
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

// ModelRef is one model a client configuration names. A limit or a capability
// is carried only when the catalog already knows it. A nil capability is
// unknown, and a client config omits the field rather than inventing it.
type ModelRef struct {
	ID            string
	Name          string
	ContextWindow *int64
	MaxOutput     *int64
	Tools         *bool
	Reasoning     *bool
	Vision        *bool
	// Connection is the label of the connection serving the model, empty for
	// a route. Upstream is the provider's own model identifier, and ChatGPT
	// marks a model served by the ChatGPT sign-in, which Codex knows natively.
	Connection string
	Upstream   string
	ChatGPT    bool
	// ReasoningEfforts are the effort values this model accepts. Nil means the
	// model stated no ladder, so a client offers the shared list. An empty
	// slice means the model stated that it takes none.
	ReasoningEfforts []string
}

// ClaudeAuth is how Claude Code authenticates on one machine.
type ClaudeAuth string

// The machine kinds the Claude helper distinguishes: a claude.ai login hands
// the operator to the browser, while a proxy machine gets Relo's own key.
const (
	ClaudeAuthLogin ClaudeAuth = "login"
	ClaudeAuthProxy ClaudeAuth = "proxy"
)

// ConfigTarget is the file one agent reads and the directory that has to
// exist before Relo will create or edit that file.
type ConfigTarget struct {
	Kind string
	Path string
	Dir  string
}

// The client and file identifiers the workflows branch on. They repeat the
// adapter's roster so this package never imports it; a test pins them equal.
const (
	ClientOpenCode = "opencode"
	ClientHermes   = "hermes"
	ClientOpenClaw = "openclaw"

	FileOpenCode = "opencode-config"
	FileHermes   = "hermes-config"
	FileOpenClaw = "openclaw-config"
	FileAside    = "aside-models"
	FilePi       = "pi-models"
	FileOMP      = "omp-models"

	ClaudeGatewayDiscoveryEnv = "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"
)

// AgentRegistry lists the coding clients Relo knows and resolves one by id.
// The composition root backs it with the client roster.
type AgentRegistry interface {
	// Agents is every integration this build knows, in console order.
	Agents() []Agent
	// AgentFor returns the agent with the given identifier.
	AgentFor(id string) (Agent, bool)
	// CatalogAgent reports whether the agent reads a model catalog Relo writes.
	CatalogAgent(id string) bool
}

// PathResolver answers every location one integration reads or writes. The
// composition root backs it with the machine's paths, so the workflows never
// resolve an environment override themselves.
type PathResolver interface {
	// Home is the operator's home directory.
	Home() string
	// KeyFile is where the agent's key lives.
	KeyFile(id string) string
	// HelperFile is the key helper the client runs for its secret.
	HelperFile(id string) string
	// BaseURL answers the data-plane address of one client protocol, empty
	// when that protocol is served on no port.
	BaseURL(protocol string) string
	// CodexConfig is the Codex configuration file.
	CodexConfig() string
	// ClaudeSettings is the Claude Code settings file.
	ClaudeSettings() string
	// ClaudeConfigDir is Claude Code's configuration directory.
	ClaudeConfigDir() string
	// DesktopLibrary is the directory holding Claude Desktop's profile.
	DesktopLibrary() string
	// DesktopInstalled refuses when Claude Desktop is absent.
	DesktopInstalled() error
	// ConfigTarget resolves the configuration file for an agent Relo writes.
	ConfigTarget(id string) (ConfigTarget, error)
	// ConfigTargets resolves every catalog file for an agent Relo writes.
	ConfigTargets(id string) ([]ConfigTarget, error)
	// SnapshotDir holds the copies taken before the first write.
	SnapshotDir() string
	// LauncherState is the record of an earlier wrapped launcher.
	LauncherState(id string) string
	// ClaudeAuth reports whether this machine holds a claude.ai login.
	ClaudeAuth() ClaudeAuth
}

// ClientFiles reads and writes the client configuration files one integration
// owns: merging Relo's contribution in, stripping it out, and proving what a
// file holds. The composition root backs it with the file adapter, so the
// workflows never touch the filesystem layout themselves.
type ClientFiles interface {
	// ReadFileOrEmpty reads a file that may not exist yet.
	ReadFileOrEmpty(path string) (string, error)
	// ReadRegular reads a file that must already be regular.
	ReadRegular(path string) (string, error)
	// WriteFileAtomic replaces a file without a half-written middle.
	WriteFileAtomic(path string, data []byte, mode os.FileMode) error
	// Installed refuses when an agent's configuration directory is absent.
	Installed(dir string) error
	// MergeCodexConfig merges Relo's block into the Codex configuration.
	MergeCodexConfig(existing, baseURL, catalogPath, helperFile string, wideContext bool) (string, error)
	// StripCodexConfig takes Relo's block back out of the Codex configuration.
	StripCodexConfig(existing string) string
	// CodexConfigPointsAt reports whether the Codex configuration names Relo.
	CodexConfigPointsAt(content, baseURL, helperFile string) bool
	// CodexBlock renders Relo's Codex configuration block.
	CodexBlock(baseURL, helperFile string) string
	// CodexCatalogPath answers the catalog beside a Codex configuration file.
	CodexCatalogPath(configPath string) string
	// WriteCodexCatalog writes the model catalog Codex reads.
	WriteCodexCatalog(path string, models []ModelRef) error
	// SyncCodexCache refreshes the Codex models cache.
	SyncCodexCache(codexHome string, models []ModelRef) error
	// StripReloFromCodexCache removes Relo's entries from the Codex cache.
	StripReloFromCodexCache(codexHome string) error
	// CodexCatalogLists reports whether the catalog file lists these models.
	CodexCatalogLists(path string, models []ModelRef) bool
	// CodexNeedsRestart reports whether the Codex app-server predates the catalog.
	CodexNeedsRestart(codexHome, catalogPath string) bool
	// CodexCatalogRows renders the model rows the Codex catalog file carries.
	CodexCatalogRows(models []ModelRef, codexHome string) []map[string]any
	// MergeClaudeSettings merges Relo's block into the Claude Code settings.
	MergeClaudeSettings(existing, baseURL, helperPath string, installHelper bool) (string, error)
	// StripClaudeSettings takes Relo's block back out of the Claude settings.
	StripClaudeSettings(existing, baseURL, helperPath string) (string, error)
	// ClaudeSettingsPointAt reports whether the settings name Relo.
	ClaudeSettingsPointAt(content, baseURL, helperPath string, installHelper bool) bool
	// Fragment renders one managed client's provider fragment.
	Fragment(id, baseURL string, models []ModelRef) (string, error)
	// MergeProvider merges Relo's provider into a managed client file.
	MergeProvider(id, existing, baseURL string, models []ModelRef) (string, error)
	// StripProvider takes Relo's provider back out of a managed client file.
	StripProvider(id, existing string) (string, error)
	// ProviderPointsAt reports whether a managed client file names Relo.
	ProviderPointsAt(id, existing, baseURL string, models []ModelRef) bool
	// HasReloProvider reports whether a file holds a Relo provider.
	HasReloProvider(id, existing string) (bool, error)
	// MergeKind merges Relo's block into a catalog client file.
	MergeKind(kind, existing, baseURL string, models []ModelRef) (string, error)
	// StripKind takes Relo's block back out of a catalog client file.
	StripKind(kind, existing string) (string, error)
	// KindHasRelo reports whether a catalog file holds Relo's block.
	KindHasRelo(kind, existing string) (bool, error)
	// KindPointsAt reports whether a catalog file names Relo.
	KindPointsAt(kind, existing, baseURL string, models []ModelRef) bool
	// CatalogKind reports whether a file kind is a model catalog Relo writes.
	CatalogKind(kind string) bool
	// KeptDocument is what a catalog file keeps once Relo's block is out.
	KeptDocument(kind string) string
	// MergeOpenCode merges Relo's provider into the OpenCode configuration.
	MergeOpenCode(existing, baseURL string, models []ModelRef) (string, error)
	// StripOpenCode takes Relo's provider back out of the OpenCode file.
	StripOpenCode(existing string) (string, error)
	// MergeHermes merges Relo's provider into the Hermes configuration.
	MergeHermes(existing, baseURL string, models []ModelRef) (string, error)
	// StripHermes takes Relo's provider back out of the Hermes file.
	StripHermes(existing string) (string, error)
	// MergeOpenClaw merges Relo's provider into the OpenClaw configuration.
	MergeOpenClaw(existing, baseURL string, models []ModelRef) (string, error)
	// StripOpenClaw takes Relo's provider back out of the OpenClaw file.
	StripOpenClaw(existing string) (string, error)
	// WriteGatewayCache writes the model cache Claude Code's picker reads.
	WriteGatewayCache(configDir, baseURL string, models []ModelRef) error
	// RemoveGatewayCache removes the model cache Claude Code's picker reads.
	RemoveGatewayCache(configDir, baseURL string) error
	// GatewayCachePath answers the picker cache file in a configuration dir.
	GatewayCachePath(configDir string) string
	// DesktopProfileContent renders the Claude Desktop gateway profile.
	DesktopProfileContent(baseURL, apiKey string, models []ModelRef) ([]byte, error)
	// DesktopProfilePath answers the gateway profile in a library directory.
	DesktopProfilePath(library string) string
	// SelectDesktopProfile points Claude Desktop at the gateway profile.
	SelectDesktopProfile(library string) error
	// UpdateDesktopModels rewrites the model list in the gateway profile.
	UpdateDesktopModels(library, baseURL string, models []ModelRef) ([]byte, error)
	// RemoveDesktopProfile removes the gateway profile from a library dir.
	RemoveDesktopProfile(library string) error
	// KeyHelperScript renders the key helper for one agent.
	KeyHelperScript(agentID string) string
	// RetireLauncher retires the record of an earlier wrapped launcher.
	RetireLauncher(statePath string) error
	// PublishEnv publishes an agent's key to the login environment.
	PublishEnv(agent Agent) error
	// UnpublishEnv removes an agent's key from the login environment.
	UnpublishEnv(agent Agent) error
	// EnvPublished reports whether the login environment carries the key.
	EnvPublished(agent Agent) bool
	// Snapshot copies a file aside before the first write.
	Snapshot(source, dir, name string) (string, error)
	// Digest fingerprints file content.
	Digest(content string) string
	// ModelsDigest fingerprints the model list a file was written with.
	ModelsDigest(models []ModelRef) string
}

// FileRefusal carries a file-operation refusal across the port: the console
// code it translates to, with the original error behind it. It reports the
// original words, so a preview reads what actually refused the write.
type FileRefusal struct {
	// Code is the machine-readable reason a console translates: one of
	// CodeNotInstalled, CodeForeignKey, CodeRelativePath, CodeUnrecognised.
	Code string
	err  error
}

// RefusalCode names the refusal a file error carries for the console, empty
// when the error is not a file refusal the console translates.
func RefusalCode(err error) string {
	var refusal *FileRefusal
	if errors.As(err, &refusal) {
		return refusal.Code
	}
	return ""
}

// NewFileRefusal carries a file-operation refusal across the port with its
// console code attached. The composition root wraps the adapter's refusals;
// the workflows wrap the ones they report themselves.
func NewFileRefusal(code string, err error) *FileRefusal {
	return &FileRefusal{Code: code, err: err}
}

func (e *FileRefusal) Error() string {
	if e.err == nil {
		return e.Code
	}
	return e.err.Error()
}

// Unwrap reports the original file error, so errors.Is still matches what
// the operation returned.
func (e *FileRefusal) Unwrap() error {
	return e.err
}

// WireCodec names the Anthropic wire details the verification turn speaks.
// The composition root backs it with the codec family, so the turn never
// imports one.
type WireCodec interface {
	// VersionHeader is the header an Anthropic request carries.
	VersionHeader() string
	// APIVersion is the Anthropic API version a request declares.
	APIVersion() string
}

// AgentProcesses restarts the background processes one client keeps. The
// composition root backs it with the client daemons.
type AgentProcesses interface {
	// RestartCodexDaemon restarts the Codex app-server after Relo writes files.
	RestartCodexDaemon(ctx context.Context, codexHome, home string) error
	// RestartOpenCodeService restarts the OpenCode server with a fresh key.
	RestartOpenCodeService(ctx context.Context, home, apiKey string) error
}
