// Proxy mode: pointing Claude Code at the vendor through Relo's intercept
// instead of rewriting the address it talks to.
package integration

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ProxyHost is the vendor host proxy mode terminates TLS for. Every other
// host a client sends through the proxy is spliced to the network untouched.
const ProxyHost = "api.anthropic.com"

// ProxyOwner is the integration the shared intercept is kept under. Claude
// Code's settings file carries the proxy address, and a process Claude Desktop
// starts reads the same file, so one intercept serves both.
const ProxyOwner = "claude-code"

// ProxyTarget names what one intercept points at: the vendor host it
// terminates TLS for, Relo's own surface, and the key a routed request
// presents there.
type ProxyTarget struct {
	Host         string
	DataPlaneURL string
	// Key answers the key one client's routed request presents to Relo.
	Key func(client string) string
	// Admits reports whether a client is in proxy mode, which is what lets its
	// requests be routed. A client that is not has every request relayed.
	Admits func(client string) bool
}

// ProxyRuntime runs the intercept one integration is pointed at. The
// composition root backs it with the local proxy adapter, so the workflows
// never open a listener themselves.
type ProxyRuntime interface {
	// Ensure starts the intercept one integration is pointed at, binding its
	// listener before anything is written, and answers the environment that
	// reaches it. The target names the vendor host, Relo's own surface, and the
	// key a routed request presents there.
	Ensure(ctx context.Context, id string, target ProxyTarget) (ProxyEnv, error)
	// Status reports the intercept's loopback address, and whether it is up.
	Status(id string) (address string, running bool)
	// Retire stops the intercept one integration ran.
	Retire(id string) error
}

// ProxyDesktop is the other integration proxy mode wires: the processes Claude
// Desktop starts, which read Claude Code's settings file and so ride the same
// intercept.
const ProxyDesktop = "claude-desktop"

var (
	// ErrProxyOwnerNeeded reports Claude Desktop asked for proxy mode while
	// Claude Code is not in it. The intercept's address lives in Claude Code's
	// settings file, which Desktop's processes read, so it must be there first.
	ErrProxyOwnerNeeded = errors.New("proxy mode for Claude Desktop rides Claude Code's: put Claude Code in proxy mode first")
	// ErrProxyInUse reports Claude Code asked to leave proxy mode while Claude
	// Desktop is still wired through it.
	ErrProxyInUse = errors.New("this proxy still serves Claude Desktop: switch Claude Desktop back first")
)

// supportsProxy reports whether proxy mode wires this agent at all.
func supportsProxy(agent Agent) bool {
	return agent.ID == ProxyOwner || agent.ID == ProxyDesktop
}

// requireProxyOwner refuses a Desktop proxy request that has no intercept to
// ride. Every other agent owns its own wiring and needs nothing.
func (m *Manager) requireProxyOwner(ctx context.Context, agent Agent) error {
	if agent.ID != ProxyDesktop {
		return nil
	}
	record, found, err := m.store.Integration(ctx, ProxyOwner)
	if err != nil {
		return err
	}
	if mode, _ := NormalizeMode(record.Mode); !found || !record.Enabled || mode != ModeProxy {
		return fmt.Errorf("%s: %w", agent.ID, ErrProxyOwnerNeeded)
	}
	return nil
}

// requireNoProxyDependents refuses taking the intercept's wiring away from
// Claude Code while Desktop's processes still depend on it.
func (m *Manager) requireNoProxyDependents(ctx context.Context, agent Agent) error {
	if agent.ID != ProxyOwner {
		return nil
	}
	record, found, err := m.store.Integration(ctx, ProxyDesktop)
	if err != nil {
		return err
	}
	if mode, _ := NormalizeMode(record.Mode); found && record.Enabled && mode == ModeProxy {
		return fmt.Errorf("%s: %w", agent.ID, ErrProxyInUse)
	}
	return nil
}

// ErrProxyUnavailable reports proxy mode asked for without a runtime that can
// run an intercept, which is a build or wiring mistake rather than an
// operator's.
var ErrProxyUnavailable = errors.New("proxy mode is not available in this build")

// ensureProxyMode starts the intercept this integration will be pointed at,
// so a port that cannot bind refuses before any file is written.
func (m *Manager) ensureProxyMode(agent Agent) error {
	if err := m.requireProxyOwner(context.Background(), agent); err != nil {
		return err
	}
	_, err := m.proxyEnv(context.Background(), agent)
	return err
}

// proxyKey answers the client key a routed request presents to Relo, read from
// the key file instead of being held: the value a restart never has to mint
// again, and one an operator can see on disk beside the intercept's own. Each
// client presents its own key, so its traffic is attributed to it.
func (m *Manager) proxyKey(client string) string {
	if _, known := m.agents.AgentFor(client); !known {
		return ""
	}
	content, err := m.files.ReadFileOrEmpty(m.paths.KeyFile(client))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(content)
}

// proxyAdmits reports whether a client is enabled and wired through the
// intercept. The mode is read from the store, so a switch takes effect on the
// next request without restarting anything.
func (m *Manager) proxyAdmits(client string) bool {
	record, found, err := m.store.Integration(context.Background(), client)
	if err != nil || !found || !record.Enabled {
		return false
	}
	mode, _ := NormalizeMode(record.Mode)
	return mode == ModeProxy
}

// proxyEnv answers the environment proxy mode writes for one integration,
// starting the intercept when it is not already listening.
func (m *Manager) proxyEnv(ctx context.Context, agent Agent) (ProxyEnv, error) {
	if m.proxy == nil {
		return ProxyEnv{}, fmt.Errorf("%s: %w", agent.ID, ErrProxyUnavailable)
	}
	baseURL := m.paths.BaseURL(agent.Protocol)
	if baseURL == "" {
		return ProxyEnv{}, fmt.Errorf("%s: %w (%s)", agent.ID, ErrSurfaceDisabled, agent.Protocol)
	}
	return m.proxy.Ensure(ctx, ProxyOwner, ProxyTarget{
		Host: ProxyHost, DataPlaneURL: baseURL, Key: m.proxyKey, Admits: m.proxyAdmits,
	})
}

// proxyState classifies a client's settings against the intercept this build
// would write. A stopped intercept reads as stale rather than broken: the
// settings are right, and the process that answers them is not running.
func (m *Manager) proxyState(agent Agent, files []FileRecord) ProxyModeState {
	env, err := m.proxyEnv(context.Background(), agent)
	if err != nil {
		return ProxyStateUnreadable
	}
	stored, found := m.settingsRecord(agent, files)
	if !found {
		return ProxyStateAbsent
	}
	content, err := m.files.ReadFileOrEmpty(stored.Path)
	if err != nil {
		return ProxyStateUnreadable
	}
	return m.files.InspectClaudeProxySettings(content, env)
}

// claudeSettingsDrifted reports whether Claude Code's settings no longer carry
// the wiring of the mode the integration runs in. Proxy mode is judged by the
// intercept's address, never by the keys settings mode writes: a file wired
// through the intercept would otherwise always read as out of date, and a
// refresh would turn it back into a settings-mode file.
// ridingProxyState is the proxy state of an integration that has its own
// settings record, or, for Desktop, the state of the Claude Code wiring it
// rides: Desktop writes no file of its own.
func (m *Manager) ridingProxyState(ctx context.Context, agent Agent, files []FileRecord) ProxyModeState {
	if agent.ID != ProxyDesktop {
		return m.proxyState(agent, files)
	}
	owner, found := m.agents.AgentFor(ProxyOwner)
	if !found {
		return ProxyStateAbsent
	}
	ownerFiles, err := m.store.Files(ctx, ProxyOwner)
	if err != nil {
		return ProxyStateUnreadable
	}
	return m.proxyState(owner, ownerFiles)
}

func (m *Manager) claudeSettingsDrifted(agent Agent, content string) bool {
	if mode, err := m.Mode(context.Background(), agent.ID); err == nil && mode == ModeProxy {
		env, err := m.proxyEnv(context.Background(), agent)
		return err == nil && !m.files.ClaudeProxySettingsPointAt(content, env)
	}
	installHelper := m.claudeMode() != ClaudeAuthLogin
	return !m.files.ClaudeSettingsPointAt(content, m.paths.BaseURL(agent.Protocol), m.paths.HelperFile(agent.ID), installHelper)
}

func (m *Manager) settingsRecord(agent Agent, files []FileRecord) (FileRecord, bool) {
	for _, file := range files {
		if file.Kind == KindClaudeSettings {
			return file, true
		}
	}
	return FileRecord{}, false
}

// writeClaudeProxySettings points Claude Code at the intercept: the settings
// carry the proxy and the certificate it presents, and the keys settings mode
// wrote are taken back out in the same write, so a client never reads a file
// that names both surfaces.
func (m *Manager) writeClaudeProxySettings(ctx context.Context, agent Agent) error {
	env, err := m.proxyEnv(ctx, agent)
	if err != nil {
		return err
	}
	path := m.paths.ClaudeSettings()
	existing, err := m.files.ReadFileOrEmpty(path)
	if err != nil {
		return err
	}
	stripped, err := m.files.StripClaudeSettings(existing, m.paths.BaseURL(agent.Protocol), m.paths.HelperFile(agent.ID))
	if err != nil {
		return err
	}
	merged, err := m.files.MergeClaudeProxySettings(stripped, env)
	if err != nil {
		return err
	}
	if err := m.writeContribution(ctx, agent, fileContribution{kind: KindClaudeSettings, path: path, content: merged, mode: 0o600}); err != nil {
		return err
	}
	// The picker cache belongs to settings mode: a client reached through the
	// proxy reads the vendor's own list, so a cache naming Relo's address
	// would point at a surface this client no longer uses.
	return m.files.RemoveGatewayCache(m.paths.ClaudeConfigDir(), m.paths.BaseURL(agent.Protocol))
}

// retireClaudeProxySettings takes the intercept's environment back out of
// Claude Code's settings, leaving a proxy the operator set alone.
func (m *Manager) retireClaudeProxySettings(agent Agent) error {
	if m.proxy == nil || agent.ID != ProxyOwner {
		return nil
	}
	env, err := m.proxyEnv(context.Background(), agent)
	if err != nil {
		return nil
	}
	path := m.paths.ClaudeSettings()
	existing, err := m.files.ReadFileOrEmpty(path)
	if err != nil {
		return err
	}
	if strings.TrimSpace(existing) == "" {
		return nil
	}
	stripped, err := m.files.StripClaudeProxySettings(existing, env)
	if err != nil {
		return err
	}
	if stripped == existing {
		return nil
	}
	return m.files.WriteFileAtomic(path, []byte(stripped), 0o600)
}

// SetMode switches one integration between the settings file and the
// intercept. The new mode is written first and the old one retired second, so
// a failure leaves the client connected the way it was.
func (m *Manager) SetMode(ctx context.Context, id, mode string) (View, error) {
	agent, found := m.agents.AgentFor(id)
	if !found {
		return View{}, fmt.Errorf("%s: %w", id, ErrUnknownAgent)
	}
	requested, err := NormalizeMode(mode)
	if err != nil {
		return View{}, err
	}
	record, err := m.storedRecord(ctx, id)
	if err != nil {
		return View{}, err
	}
	if err := m.guardModeSwitch(ctx, agent, record, requested); err != nil {
		return View{}, err
	}
	if err := m.switchMode(ctx, agent, record, requested); err != nil {
		m.recordFailure(ctx, agent, err)
		return View{}, err
	}
	record.Mode = requested
	record.State, record.LastError = StateOn, ""
	record.UpdatedAtMs = m.now().UnixMilli()
	if err := m.store.SaveIntegration(ctx, record); err != nil {
		return View{}, err
	}
	m.appendOp(ctx, id, "mode", requested)
	return m.Inspect(ctx, id)
}

// guardModeSwitch refuses a switch the two proxy clients cannot make
// independently: only they have a proxy mode, Desktop needs Claude Code's
// wiring to be there, and Claude Code cannot take it away from Desktop.
func (m *Manager) guardModeSwitch(ctx context.Context, agent Agent, record Record, to string) error {
	if to == ModeProxy {
		if !supportsProxy(agent) {
			return fmt.Errorf("%s: proxy mode wires Claude Code and Claude Desktop only", agent.ID)
		}
		return m.requireProxyOwner(ctx, agent)
	}
	if current, _ := NormalizeMode(record.Mode); current == ModeProxy {
		if err := m.retirePicker(ctx, agent); err != nil {
			return err
		}
		return m.requireNoProxyDependents(ctx, agent)
	}
	return nil
}

// storedRecord reads one enabled integration, which is the only state a mode
// switch has anything to switch.
func (m *Manager) storedRecord(ctx context.Context, id string) (Record, error) {
	record, found, err := m.store.Integration(ctx, id)
	if err != nil {
		return Record{}, err
	}
	if !found || !record.Enabled {
		return Record{}, fmt.Errorf("%s: %w", id, ErrUnknownAgent)
	}
	return record, nil
}

func (m *Manager) switchMode(ctx context.Context, agent Agent, record Record, to string) error {
	current, _ := NormalizeMode(record.Mode)
	if current == to {
		return nil
	}
	// The new mode is written first, so a failure leaves the client pointing
	// at the surface it already used.
	if err := m.writeClientFiles(ctx, agent, "", EnableOptions{Mode: to, Overwrite: true}); err != nil {
		return err
	}
	if current != ModeProxy || agent.ID != ProxyOwner {
		return nil
	}
	if err := m.retireClaudeProxySettings(agent); err != nil {
		return err
	}
	if m.proxy == nil {
		return nil
	}
	return m.proxy.Retire(ProxyOwner)
}
