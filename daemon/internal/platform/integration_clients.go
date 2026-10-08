// Integration clients: the coding-client ports over the file adapter.
package platform

import (
	"context"
	"errors"
	"os"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	anthropiccodec "github.com/jonaskahn/relo/internal/adapters/wire/anthropic"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
)

var (
	_ appintegration.AgentRegistry  = AgentRegistry{}
	_ appintegration.AgentProcesses = AgentProcesses{}
	_ appintegration.PathResolver   = ClientPaths{}
	_ appintegration.ClientFiles    = ClientFiles{}
	_ appintegration.WireCodec      = WireCodec{}
)

// AgentRegistry lists the coding clients Relo knows.
type AgentRegistry struct{}

// NewAgentRegistry returns the registry over the client roster.
func NewAgentRegistry() AgentRegistry {
	return AgentRegistry{}
}

// Agents is every integration this build knows, in console order.
func (AgentRegistry) Agents() []appintegration.Agent {
	agents := codingclients.Agents()
	converted := make([]appintegration.Agent, 0, len(agents))
	for _, agent := range agents {
		converted = append(converted, clientAgent(agent))
	}
	return converted
}

// AgentFor returns the agent with the given identifier.
func (AgentRegistry) AgentFor(id string) (appintegration.Agent, bool) {
	agent, found := codingclients.AgentFor(id)
	if !found {
		return appintegration.Agent{}, false
	}
	return clientAgent(agent), true
}

// CatalogAgent reports whether the agent reads a model catalog Relo writes.
func (AgentRegistry) CatalogAgent(id string) bool {
	return codingclients.CatalogAgent(id)
}

func clientAgent(agent codingclients.Agent) appintegration.Agent {
	return appintegration.Agent{
		ID: agent.ID, Client: agent.Client, Protocol: agent.Protocol,
		Summary: agent.Summary, Label: agent.Label,
		ManagesFiles: agent.ManagesFiles, EnvKey: agent.EnvKey,
	}
}

func integrationAgent(agent appintegration.Agent) codingclients.Agent {
	return codingclients.Agent{
		ID: agent.ID, Client: agent.Client, Protocol: agent.Protocol,
		Summary: agent.Summary, Label: agent.Label,
		ManagesFiles: agent.ManagesFiles, EnvKey: agent.EnvKey,
	}
}

func clientModelRefs(models []appintegration.ModelRef) []codingclients.ModelRef {
	converted := make([]codingclients.ModelRef, 0, len(models))
	for _, ref := range models {
		converted = append(converted, codingclients.ModelRef{
			ID: ref.ID, Name: ref.Name, ContextWindow: ref.ContextWindow,
			MaxOutput: ref.MaxOutput, Tools: ref.Tools, Reasoning: ref.Reasoning,
			Vision: ref.Vision, Connection: ref.Connection, Upstream: ref.Upstream,
			ChatGPT: ref.ChatGPT, ReasoningEfforts: ref.ReasoningEfforts,
		})
	}
	return converted
}

// ClientPaths answers every location one integration reads or writes.
type ClientPaths struct {
	paths codingclients.Paths
}

// NewPathResolver returns the resolver over the machine's client paths.
func NewPathResolver(paths codingclients.Paths) ClientPaths {
	return ClientPaths{paths: paths}
}

// Home is the operator's home directory.
func (r ClientPaths) Home() string {
	return r.paths.Home
}

// KeyFile is where the agent's key lives.
func (r ClientPaths) KeyFile(id string) string {
	return r.paths.KeyFile(id)
}

// HelperFile is the key helper the client runs for its secret.
func (r ClientPaths) HelperFile(id string) string {
	return r.paths.HelperFile(id)
}

// BaseURL answers the data-plane address of one client protocol.
func (r ClientPaths) BaseURL(protocol string) string {
	return r.paths.BaseURL(protocol)
}

// CodexConfig is the Codex configuration file.
func (r ClientPaths) CodexConfig() string {
	return r.paths.CodexConfig()
}

// ClaudeSettings is the Claude Code settings file.
func (r ClientPaths) ClaudeSettings() string {
	return r.paths.ClaudeSettings()
}

// ClaudeConfigDir is Claude Code's configuration directory.
func (r ClientPaths) ClaudeConfigDir() string {
	return r.paths.ClaudeConfigDir()
}

// DesktopLibrary is the directory holding Claude Desktop's profile.
func (r ClientPaths) DesktopLibrary() string {
	return r.paths.DesktopLibrary()
}

// DesktopInstalled refuses when Claude Desktop is absent.
func (r ClientPaths) DesktopInstalled() error {
	return fileError(r.paths.DesktopInstalled())
}

// ConfigTarget resolves the configuration file for an agent Relo writes.
func (r ClientPaths) ConfigTarget(id string) (appintegration.ConfigTarget, error) {
	target, err := r.paths.ConfigTarget(id)
	converted := appintegration.ConfigTarget{Kind: target.Kind, Path: target.Path, Dir: target.Dir}
	if err != nil {
		return converted, fileError(err)
	}
	return converted, nil
}

// ConfigTargets resolves every catalog file for an agent Relo writes.
func (r ClientPaths) ConfigTargets(id string) ([]appintegration.ConfigTarget, error) {
	targets, err := r.paths.ConfigTargets(id)
	if err != nil {
		return nil, fileError(err)
	}
	converted := make([]appintegration.ConfigTarget, 0, len(targets))
	for _, target := range targets {
		converted = append(converted, appintegration.ConfigTarget{
			Kind: target.Kind, Path: target.Path, Dir: target.Dir,
		})
	}
	return converted, nil
}

// SnapshotDir holds the copies taken before the first write.
func (r ClientPaths) SnapshotDir() string {
	return r.paths.SnapshotDir()
}

// LauncherState is the record of an earlier wrapped launcher.
func (r ClientPaths) LauncherState(id string) string {
	return r.paths.LauncherState(id)
}

// ClaudeAuth reports whether this machine holds a claude.ai login.
func (r ClientPaths) ClaudeAuth() appintegration.ClaudeAuth {
	return appintegration.ClaudeAuth(r.paths.ClaudeAuth())
}

// ClientFiles reads and writes the client configuration files.
type ClientFiles struct {
	paths codingclients.Paths
}

// NewClientFiles returns the files over the machine's client paths.
func NewClientFiles(paths codingclients.Paths) ClientFiles {
	return ClientFiles{paths: paths}
}

// ReadFileOrEmpty reads a file that may not exist yet.
func (ClientFiles) ReadFileOrEmpty(path string) (string, error) {
	result, err := codingclients.ReadFileOrEmpty(path)
	return result, fileError(err)
}

// ReadRegular reads a file that must already be regular.
func (ClientFiles) ReadRegular(path string) (string, error) {
	result, err := codingclients.ReadRegular(path)
	return result, fileError(err)
}

// WriteFileAtomic replaces a file without a half-written middle.
func (ClientFiles) WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	return fileError(codingclients.WriteFileAtomic(path, data, mode))
}

// Installed refuses when an agent's configuration directory is absent.
func (ClientFiles) Installed(dir string) error {
	return fileError(codingclients.Installed(dir))
}

// MergeCodexConfig merges Relo's block into the Codex configuration.
func (ClientFiles) MergeCodexConfig(existing, baseURL, catalogPath, helperFile string, wideContext bool) (string, error) {
	result, err := codingclients.MergeCodexConfig(existing, baseURL, catalogPath, helperFile, wideContext)
	return result, fileError(err)
}

// StripCodexConfig takes Relo's block back out of the Codex configuration.
func (ClientFiles) StripCodexConfig(existing string) string {
	return codingclients.StripCodexConfig(existing)
}

// CodexConfigPointsAt reports whether the Codex configuration names Relo.
func (ClientFiles) CodexConfigPointsAt(content, baseURL, helperFile string) bool {
	return codingclients.CodexConfigPointsAt(content, baseURL, helperFile)
}

// CodexBlock renders Relo's Codex configuration block.
func (ClientFiles) CodexBlock(baseURL, helperFile string) string {
	return codingclients.CodexBlock(baseURL, helperFile)
}

// CodexCatalogPath answers the catalog beside a Codex configuration file.
func (ClientFiles) CodexCatalogPath(configPath string) string {
	return codingclients.CodexCatalogPath(configPath)
}

// WriteCodexCatalog writes the model catalog Codex reads.
func (ClientFiles) WriteCodexCatalog(path string, models []appintegration.ModelRef) error {
	return fileError(codingclients.WriteCodexCatalog(path, clientModelRefs(models)))
}

// SyncCodexCache refreshes the Codex models cache.
func (ClientFiles) SyncCodexCache(codexHome string, models []appintegration.ModelRef) error {
	return fileError(codingclients.SyncCodexCache(codexHome, clientModelRefs(models)))
}

// StripReloFromCodexCache removes Relo's entries from the Codex cache.
func (ClientFiles) StripReloFromCodexCache(codexHome string) error {
	return fileError(codingclients.StripReloFromCodexCache(codexHome))
}

// CodexCatalogLists reports whether the catalog file lists these models.
func (ClientFiles) CodexCatalogLists(path string, models []appintegration.ModelRef) bool {
	return codingclients.CodexCatalogLists(path, clientModelRefs(models))
}

// CodexNeedsRestart reports whether the Codex app-server predates the catalog.
func (ClientFiles) CodexNeedsRestart(codexHome, catalogPath string) bool {
	return codingclients.CodexNeedsRestart(codexHome, catalogPath)
}

// CodexCatalogRows renders the model rows the Codex catalog file carries.
func (ClientFiles) CodexCatalogRows(models []appintegration.ModelRef, codexHome string) []map[string]any {
	return codingclients.BuildCodexRows(clientModelRefs(models), codingclients.ReadCodexTemplates(codexHome))
}

// MergeClaudeSettings merges Relo's block into the Claude Code settings.
func (ClientFiles) MergeClaudeSettings(existing, baseURL, helperPath string, installHelper bool) (string, error) {
	result, err := codingclients.MergeClaudeSettings(existing, baseURL, helperPath, installHelper)
	return result, fileError(err)
}

// StripClaudeSettings takes Relo's block back out of the Claude settings.
func (ClientFiles) StripClaudeSettings(existing, baseURL, helperPath string) (string, error) {
	result, err := codingclients.StripClaudeSettings(existing, baseURL, helperPath)
	return result, fileError(err)
}

// ClaudeSettingsPointAt reports whether the settings name Relo.
func (ClientFiles) ClaudeSettingsPointAt(content, baseURL, helperPath string, installHelper bool) bool {
	return codingclients.ClaudeSettingsPointAt(content, baseURL, helperPath, installHelper)
}

// Fragment renders one managed client's provider fragment.
func (ClientFiles) Fragment(id, baseURL string, models []appintegration.ModelRef) (string, error) {
	result, err := codingclients.Fragment(id, baseURL, clientModelRefs(models))
	return result, fileError(err)
}

// MergeProvider merges Relo's provider into a managed client file.
func (ClientFiles) MergeProvider(id, existing, baseURL string, models []appintegration.ModelRef) (string, error) {
	result, err := codingclients.MergeProvider(id, existing, baseURL, clientModelRefs(models))
	return result, fileError(err)
}

// StripProvider takes Relo's provider back out of a managed client file.
func (ClientFiles) StripProvider(id, existing string) (string, error) {
	result, err := codingclients.StripProvider(id, existing)
	return result, fileError(err)
}

// ProviderPointsAt reports whether a managed client file names Relo.
func (ClientFiles) ProviderPointsAt(id, existing, baseURL string, models []appintegration.ModelRef) bool {
	return codingclients.ProviderPointsAt(id, existing, baseURL, clientModelRefs(models))
}

// HasReloProvider reports whether a file holds a Relo provider.
func (ClientFiles) HasReloProvider(id, existing string) (bool, error) {
	result, err := codingclients.HasReloProvider(id, existing)
	return result, fileError(err)
}

// MergeKind merges Relo's block into a catalog client file.
func (ClientFiles) MergeKind(kind, existing, baseURL string, models []appintegration.ModelRef) (string, error) {
	result, err := codingclients.MergeKind(kind, existing, baseURL, clientModelRefs(models))
	return result, fileError(err)
}

// StripKind takes Relo's block back out of a catalog client file.
func (ClientFiles) StripKind(kind, existing string) (string, error) {
	result, err := codingclients.StripKind(kind, existing)
	return result, fileError(err)
}

// KindHasRelo reports whether a catalog file holds Relo's block.
func (ClientFiles) KindHasRelo(kind, existing string) (bool, error) {
	result, err := codingclients.KindHasRelo(kind, existing)
	return result, fileError(err)
}

// KindPointsAt reports whether a catalog file names Relo.
func (ClientFiles) KindPointsAt(kind, existing, baseURL string, models []appintegration.ModelRef) bool {
	return codingclients.KindPointsAt(kind, existing, baseURL, clientModelRefs(models))
}

// CatalogKind reports whether a file kind is a model catalog Relo writes.
func (ClientFiles) CatalogKind(kind string) bool {
	return codingclients.CatalogKind(kind)
}

// KeptDocument is what a catalog file keeps once Relo's block is out.
func (ClientFiles) KeptDocument(kind string) string {
	return codingclients.KeptDocument(kind)
}

// MergeOpenCode merges Relo's provider into the OpenCode configuration.
func (ClientFiles) MergeOpenCode(existing, baseURL string, models []appintegration.ModelRef) (string, error) {
	result, err := codingclients.MergeOpenCode(existing, baseURL, clientModelRefs(models))
	return result, fileError(err)
}

// StripOpenCode takes Relo's provider back out of the OpenCode file.
func (ClientFiles) StripOpenCode(existing string) (string, error) {
	result, err := codingclients.StripOpenCode(existing)
	return result, fileError(err)
}

// MergeHermes merges Relo's provider into the Hermes configuration.
func (ClientFiles) MergeHermes(existing, baseURL string, models []appintegration.ModelRef) (string, error) {
	result, err := codingclients.MergeHermes(existing, baseURL, clientModelRefs(models))
	return result, fileError(err)
}

// StripHermes takes Relo's provider back out of the Hermes file.
func (ClientFiles) StripHermes(existing string) (string, error) {
	result, err := codingclients.StripHermes(existing)
	return result, fileError(err)
}

// MergeOpenClaw merges Relo's provider into the OpenClaw configuration.
func (ClientFiles) MergeOpenClaw(existing, baseURL string, models []appintegration.ModelRef) (string, error) {
	result, err := codingclients.MergeOpenClaw(existing, baseURL, clientModelRefs(models))
	return result, fileError(err)
}

// StripOpenClaw takes Relo's provider back out of the OpenClaw file.
func (ClientFiles) StripOpenClaw(existing string) (string, error) {
	result, err := codingclients.StripOpenClaw(existing)
	return result, fileError(err)
}

// WriteGatewayCache writes the model cache Claude Code's picker reads.
func (ClientFiles) WriteGatewayCache(configDir, baseURL string, models []appintegration.ModelRef) error {
	return fileError(codingclients.WriteGatewayCache(configDir, baseURL, clientModelRefs(models)))
}

// RemoveGatewayCache removes the model cache Claude Code's picker reads.
func (ClientFiles) RemoveGatewayCache(configDir, baseURL string) error {
	return fileError(codingclients.RemoveGatewayCache(configDir, baseURL))
}

// GatewayCachePath answers the picker cache file in a configuration dir.
func (ClientFiles) GatewayCachePath(configDir string) string {
	return codingclients.GatewayCachePath(configDir)
}

// DesktopProfileContent renders the Claude Desktop gateway profile.
func (ClientFiles) DesktopProfileContent(baseURL, apiKey string, models []appintegration.ModelRef) ([]byte, error) {
	result, err := codingclients.DesktopProfileContent(baseURL, apiKey, clientModelRefs(models))
	return result, fileError(err)
}

// DesktopProfilePath answers the gateway profile in a library directory.
func (ClientFiles) DesktopProfilePath(library string) string {
	return codingclients.DesktopProfilePath(library)
}

// SelectDesktopProfile points Claude Desktop at the gateway profile.
func (ClientFiles) SelectDesktopProfile(library string) error {
	return fileError(codingclients.SelectDesktopProfile(library))
}

// UpdateDesktopModels rewrites the model list in the gateway profile.
func (ClientFiles) UpdateDesktopModels(library, baseURL string, models []appintegration.ModelRef) ([]byte, error) {
	result, err := codingclients.UpdateDesktopModels(library, baseURL, clientModelRefs(models))
	return result, fileError(err)
}

// RemoveDesktopProfile removes the gateway profile from a library dir.
func (ClientFiles) RemoveDesktopProfile(library string) error {
	return fileError(codingclients.RemoveDesktopProfile(library))
}

// KeyHelperScript renders the key helper for one agent.
func (f ClientFiles) KeyHelperScript(agentID string) string {
	return codingclients.KeyHelperScript(f.paths, agentID)
}

// RetireLauncher retires the record of an earlier wrapped launcher.
func (ClientFiles) RetireLauncher(statePath string) error {
	return fileError(codingclients.RetireLauncher(statePath))
}

// PublishEnv publishes an agent's key to the login environment.
func (f ClientFiles) PublishEnv(agent appintegration.Agent) error {
	return fileError(codingclients.PublishEnv(f.paths, integrationAgent(agent)))
}

// UnpublishEnv removes an agent's key from the login environment.
func (f ClientFiles) UnpublishEnv(agent appintegration.Agent) error {
	return fileError(codingclients.UnpublishEnv(f.paths, integrationAgent(agent)))
}

// EnvPublished reports whether the login environment carries the key.
func (f ClientFiles) EnvPublished(agent appintegration.Agent) bool {
	return codingclients.EnvPublished(f.paths, integrationAgent(agent))
}

// Snapshot copies a file aside before the first write.
func (ClientFiles) Snapshot(source, dir, name string) (string, error) {
	result, err := codingclients.Snapshot(source, dir, name)
	return result, fileError(err)
}

// Digest fingerprints file content.
func (ClientFiles) Digest(content string) string {
	return codingclients.Digest(content)
}

// ModelsDigest fingerprints the model list a file was written with.
func (ClientFiles) ModelsDigest(models []appintegration.ModelRef) string {
	return codingclients.ModelsDigest(clientModelRefs(models))
}

// WireCodec names the Anthropic wire details the verification turn speaks.
type WireCodec struct{}

// NewWireCodec returns the codec details over the Anthropic family.
func NewWireCodec() WireCodec {
	return WireCodec{}
}

// VersionHeader is the header an Anthropic request carries.
func (WireCodec) VersionHeader() string {
	return anthropiccodec.VersionHeader
}

// APIVersion is the Anthropic API version a request declares.
func (WireCodec) APIVersion() string {
	return anthropiccodec.APIVersion
}

// AgentProcesses restarts the background processes one client keeps.
type AgentProcesses struct{}

// NewAgentProcesses returns the restarts over the client daemons.
func NewAgentProcesses() AgentProcesses {
	return AgentProcesses{}
}

// RestartCodexDaemon restarts the Codex app-server after Relo writes files.
func (AgentProcesses) RestartCodexDaemon(ctx context.Context, codexHome, home string) error {
	return fileError(codingclients.RestartCodexDaemon(ctx, codexHome, home))
}

// RestartOpenCodeService restarts the OpenCode server with a fresh key.
func (AgentProcesses) RestartOpenCodeService(ctx context.Context, home, apiKey string) error {
	return fileError(codingclients.RestartOpenCodeService(ctx, home, apiKey))
}

// fileError carries a file-operation refusal across the port with its console
// code attached, leaving every other error untouched.
func fileError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, codingclients.ErrNotInstalled):
		return appintegration.NewFileRefusal(appintegration.CodeNotInstalled, err)
	case errors.Is(err, codingclients.ErrRelativePath):
		return appintegration.NewFileRefusal(appintegration.CodeRelativePath, err)
	case errors.Is(err, codingclients.ErrForeignProvider):
		return appintegration.NewFileRefusal(appintegration.CodeForeignKey, err)
	case errors.Is(err, codingclients.ErrUnrecognisedFile):
		return appintegration.NewFileRefusal(appintegration.CodeUnrecognised, err)
	default:
		return err
	}
}
