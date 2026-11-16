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

// ProxyTarget names what one intercept points at: the vendor host it
// terminates TLS for, Relo's own surface, and the key a routed request
// presents there.
type ProxyTarget struct {
	Host         string
	DataPlaneURL string
	Key          func() string
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

// ErrProxyUnavailable reports proxy mode asked for without a runtime that can
// run an intercept, which is a build or wiring mistake rather than an
// operator's.
var ErrProxyUnavailable = errors.New("proxy mode is not available in this build")

// ensureProxyMode starts the intercept this integration will be pointed at,
// so a port that cannot bind refuses before any file is written.
func (m *Manager) ensureProxyMode(agent Agent) error {
	_, err := m.proxyEnv(context.Background(), agent)
	return err
}

// proxyKey answers the client key a routed request presents to Relo, read from
// the key file instead of being held: the value a restart never has to mint
// again, and one an operator can see on disk beside the intercept's own.
func (m *Manager) proxyKey(agent Agent) func() string {
	path := m.paths.KeyFile(agent.ID)
	return func() string {
		content, err := m.files.ReadFileOrEmpty(path)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(content)
	}
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
	return m.proxy.Ensure(ctx, agent.ID, ProxyTarget{
		Host: ProxyHost, DataPlaneURL: baseURL, Key: m.proxyKey(agent),
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
	if m.proxy == nil {
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
	if requested == ModeProxy && agent.ID != "claude-code" {
		return View{}, fmt.Errorf("%s: proxy mode wires Claude Code only", id)
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
	if current != ModeProxy {
		return nil
	}
	if err := m.retireClaudeProxySettings(agent); err != nil {
		return err
	}
	if m.proxy == nil {
		return nil
	}
	return m.proxy.Retire(agent.ID)
}
