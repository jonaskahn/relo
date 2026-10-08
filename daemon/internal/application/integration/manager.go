// Package integration orchestrates coding-client setup, repair, rotation,
// and restore: the workflows that point an agent at this daemon.
package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The states one integration reports. off is an integration nothing was done
// to, on is one whose contribution still matches what Relo writes, drifted is
// one an update or an editor changed, and error is one whose last write failed.
const (
	StateOff     = "off"
	StateOn      = "on"
	StateDrifted = "drifted"
	StateError   = "error"
)

// The kinds of file one integration owns.
const (
	KindKey         = "key"
	KindCodexConfig = "codex-config"
	KindCodexHelper = "codex-helper"
	// KindCodexLauncher is the record an earlier version kept for a wrapped
	// launcher. Nothing writes it now, and a stored one is never restored by
	// path because the path is the operator's own client.
	KindCodexLauncher  = "codex-launcher"
	KindClaudeSettings = "claude-settings"
	KindClaudeHelper   = "claude-helper"
	KindClaudeDesktop  = "claude-desktop"
)

var (
	// ErrUnknownAgent reports an identifier no integration matches.
	ErrUnknownAgent = errors.New("unknown integration")
	// ErrSurfaceDisabled reports an agent whose protocol is served on no port,
	// so there is no address to point it at.
	ErrSurfaceDisabled = errors.New("the surface this agent speaks is not served")
	// ErrUnsupportedFile reports a plan item no writer handles.
	ErrUnsupportedFile = errors.New("relo does not write")
)

var errForeignProviderReason = errors.New("the file already has a relo provider")

// foreignProvider reports a relo provider Relo did not write, in the same
// refusal shape the file port returns. It carries the same words, so a plan
// reads the same reason whether Relo or a file named it.
func foreignProvider() error {
	return NewFileRefusal(CodeForeignKey, errForeignProviderReason)
}

// Refusal codes a console translates. They travel with a preview that did not
// write anything.
const (
	CodeNotInstalled = "not_installed"
	CodeForeignKey   = "foreign_key"
	CodeRelativePath = "relative_path"
	CodeUnrecognised = "unrecognised"
)

// Record is one integration as it is stored.
type Record struct {
	ID           string
	Enabled      bool
	KeyID        string
	State        string
	LastError    string
	UpdatedAtMs  int64
	ModelsDigest string
	// Context1M is the Codex card's 1M button. A new row starts on.
	Context1M bool
}

// FileRecord is one file Relo wrote for an integration: where it is, the
// fingerprint of what Relo put there, and the copy of the operator's own
// file taken before the first write.
type FileRecord struct {
	IntegrationID string
	Kind          string
	Path          string
	Digest        string
	SnapshotPath  string
	UpdatedAtMs   int64
}

// OpRecord is one action Relo took, kept so the history of a machine's wiring
// is readable rather than inferred.
type OpRecord struct {
	ID            string
	IntegrationID string
	Action        string
	Detail        string
	CreatedAtMs   int64
}

// Store persists what one integration owns. The service implements it over the
// state database, so this package never opens one.
type Store interface {
	Integration(ctx context.Context, id string) (Record, bool, error)
	SaveIntegration(ctx context.Context, record Record) error
	Files(ctx context.Context, id string) ([]FileRecord, error)
	SaveFile(ctx context.Context, record FileRecord) error
	DeleteFiles(ctx context.Context, id string) error
	AppendOp(ctx context.Context, op OpRecord) error
}

// Options tune a manager.
type Options struct {
	Paths  PathResolver
	Agents AgentRegistry
	Files  ClientFiles
	Store  Store
	Logger *slog.Logger
	Now    func() time.Time
	// Models lists the catalog entries a managed client configuration names.
	// An empty list writes a provider with no models.
	Models func() []ModelRef
	// ClaudeModels lists what Claude Code's picker cache shows. When unset,
	// the cache uses Models.
	ClaudeModels func() []ModelRef
	// ClaudeAuth reports login or proxy. Nil detects it on this machine.
	ClaudeAuth func() ClaudeAuth
}

// Manager owns every write an integration makes.
type Manager struct {
	paths        PathResolver
	agents       AgentRegistry
	files        ClientFiles
	store        Store
	logger       *slog.Logger
	now          func() time.Time
	models       func() []ModelRef
	claudeModels func() []ModelRef
	claudeAuth   func() ClaudeAuth
}

// New returns a manager over the given paths and store.
func New(options Options) *Manager {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Manager{
		paths: options.Paths, agents: options.Agents, files: options.Files,
		store: options.Store, logger: logger, now: now,
		models: options.Models, claudeModels: options.ClaudeModels, claudeAuth: options.ClaudeAuth,
	}
}

// FileView is one file an integration owns as a console reads it.
type FileView struct {
	Kind     string
	Path     string
	Present  bool
	Drifted  bool
	Snapshot string
}

// View is one integration as a console reads it. KeyHint and the token itself
// belong to the caller, which owns the client keys.
type View struct {
	Agent       Agent
	Enabled     bool
	State       string
	LastError   string
	Files       []FileView
	KeyID       string
	UpdatedAtMs int64
	// ModelsStale is true when the model list Relo would write now differs
	// from the one stored at setup, so the card can offer Repair without
	// changing the stored state.
	ModelsStale bool
	// Steps are what an operator does by hand for an agent Relo hands a key
	// to rather than configuring.
	Steps []string
	// Context1M is whether Codex setup writes the 1M window. It stays true
	// until the operator turns that button off.
	Context1M bool
}

// Agents lists every integration this build knows.
func (m *Manager) Agents() []Agent {
	return m.agents.Agents()
}

// PlannedFile is one file an enable would write, with the fragment it would
// put there. A refusal is reported the same way, so an operator reads what
// would happen before anything on the machine changes.
type PlannedFile struct {
	Kind     string
	Path     string
	Fragment string
	Refused  bool
	Reason   string
	// Code is the machine-readable reason a console translates. It is empty
	// when the plan can be written.
	Code string
}

// Preview reports what enabling one integration would write.
func (m *Manager) Preview(ctx context.Context, id string) ([]PlannedFile, error) {
	agent, found := m.agents.AgentFor(id)
	if !found {
		return nil, fmt.Errorf("%s: %w", id, ErrUnknownAgent)
	}
	planned := []PlannedFile{{Kind: KindKey, Path: m.paths.KeyFile(agent.ID)}}
	if agent.ManagesFiles {
		switch agent.ID {
		case "codex":
			planned = append(planned,
				PlannedFile{Kind: KindCodexHelper, Path: m.paths.HelperFile(agent.ID)},
				m.previewCodex(ctx, agent))
		case "claude-code":
			planned = append(planned,
				PlannedFile{Kind: KindClaudeHelper, Path: m.paths.HelperFile(agent.ID)},
				m.previewClaudeSettings(agent))
		case "claude-desktop":
			planned = append(planned, m.previewClaudeDesktop(agent))
		case ClientOpenCode, ClientHermes, ClientOpenClaw:
			planned = append(planned, m.previewManaged(ctx, agent))
		default:
			if m.agents.CatalogAgent(agent.ID) {
				planned = append(planned, m.previewCatalog(ctx, agent)...)
			}
		}
	}
	return planned, nil
}

// previewCodex reports what the Codex configuration would become.

func (m *Manager) previewCodex(ctx context.Context, agent Agent) PlannedFile {
	path := m.paths.CodexConfig()
	existing, err := m.files.ReadFileOrEmpty(path)
	if err != nil {
		return PlannedFile{Kind: KindCodexConfig, Path: path, Refused: true, Reason: err.Error()}
	}
	helperFile := m.paths.HelperFile(agent.ID)
	if _, err := m.files.MergeCodexConfig(existing, m.paths.BaseURL(agent.Protocol), m.files.CodexCatalogPath(path), helperFile, m.codexWideContext(ctx)); err != nil {
		return PlannedFile{Kind: KindCodexConfig, Path: path, Refused: true, Reason: err.Error()}
	}
	return PlannedFile{Kind: KindCodexConfig, Path: path, Fragment: m.files.CodexBlock(m.paths.BaseURL(agent.Protocol), helperFile)}
}

// previewClaudeSettings reports what the Claude Code settings would gain.

func (m *Manager) previewClaudeSettings(agent Agent) PlannedFile {
	path := m.paths.ClaudeSettings()
	existing, err := m.files.ReadFileOrEmpty(path)
	if err != nil {
		return PlannedFile{Kind: KindClaudeSettings, Path: path, Refused: true, Reason: err.Error()}
	}
	installHelper := m.claudeMode() != ClaudeAuthLogin
	if _, err := m.files.MergeClaudeSettings(existing, m.paths.BaseURL(agent.Protocol), m.paths.HelperFile(agent.ID), installHelper); err != nil {
		return PlannedFile{Kind: KindClaudeSettings, Path: path, Refused: true, Reason: err.Error()}
	}
	fragment := "env.ANTHROPIC_BASE_URL = " + m.paths.BaseURL(agent.Protocol) +
		"\nenv." + ClaudeGatewayDiscoveryEnv + " = 1"
	if installHelper {
		fragment += "\napiKeyHelper = " + m.paths.HelperFile(agent.ID)
	}
	return PlannedFile{Kind: KindClaudeSettings, Path: path, Fragment: fragment}
}

func (m *Manager) previewClaudeDesktop(agent Agent) PlannedFile {
	library := m.paths.DesktopLibrary()
	profile := m.files.DesktopProfilePath(library)
	if err := m.paths.DesktopInstalled(); err != nil {
		return refusedPlan(PlannedFile{Kind: KindClaudeDesktop, Path: profile}, err)
	}
	base := m.paths.BaseURL(agent.Protocol)
	return PlannedFile{
		Kind: KindClaudeDesktop, Path: profile,
		Fragment: "inferenceGatewayBaseUrl = " + base + "\nmodelDiscoveryEnabled = false",
	}
}

func (m *Manager) claudeMode() ClaudeAuth {
	if m.claudeAuth != nil {
		return m.claudeAuth()
	}
	return m.paths.ClaudeAuth()
}

func (m *Manager) previewManaged(ctx context.Context, agent Agent) PlannedFile {
	target, err := m.paths.ConfigTarget(agent.ID)
	plan := PlannedFile{Kind: target.Kind, Path: target.Path}
	if err != nil {
		return refusedPlan(plan, err)
	}
	fragment, err := m.files.Fragment(agent.ID, m.paths.BaseURL(agent.Protocol), m.modelList())
	if err != nil {
		return refusedPlan(plan, err)
	}
	plan.Fragment = fragment
	if err := m.files.Installed(target.Dir); err != nil {
		return refusedPlan(plan, err)
	}
	existing, err := m.files.ReadRegular(target.Path)
	if err != nil {
		return refusedPlan(plan, err)
	}
	present, err := m.files.HasReloProvider(agent.ID, existing)
	if err != nil {
		return refusedPlan(plan, err)
	}
	if present && !m.owns(ctx, agent.ID, target.Kind) {
		return refusedPlan(plan, foreignProvider())
	}
	return plan
}

func refusedPlan(plan PlannedFile, err error) PlannedFile {
	plan.Refused = true
	plan.Reason = err.Error()
	plan.Code = RefusalCode(err)
	return plan
}

func (m *Manager) modelList() []ModelRef {
	if m.models == nil {
		return nil
	}
	return m.models()
}

func (m *Manager) claudeModelList() []ModelRef {
	if m.claudeModels != nil {
		return m.claudeModels()
	}
	return m.modelList()
}

func (m *Manager) modelsFor(agent Agent) []ModelRef {
	switch agent.ID {
	case "claude-code", "claude-desktop":
		return m.claudeModelList()
	default:
		return m.modelList()
	}
}

func (m *Manager) modelsStale(agent Agent, record Record) bool {
	models := m.modelsFor(agent)
	if record.ModelsDigest != m.files.ModelsDigest(models) {
		return true
	}
	if agent.ID == "codex" {
		return !m.files.CodexCatalogLists(m.files.CodexCatalogPath(m.paths.CodexConfig()), models)
	}
	return false
}

func (m *Manager) rememberModels(ctx context.Context, agent Agent) error {
	record, found, err := m.store.Integration(ctx, agent.ID)
	if err != nil || !found {
		return err
	}
	record.ModelsDigest = m.files.ModelsDigest(m.modelsFor(agent))
	record.UpdatedAtMs = m.now().UnixMilli()
	return m.store.SaveIntegration(ctx, record)
}

func (m *Manager) removeCodexCache(agent Agent) error {
	if agent.ID != "codex" {
		return nil
	}
	return m.files.StripReloFromCodexCache(filepath.Dir(m.paths.CodexConfig()))
}

func (m *Manager) owns(ctx context.Context, id, kind string) bool {
	files, err := m.store.Files(ctx, id)
	if err != nil {
		return false
	}
	for _, file := range files {
		if file.Kind == kind {
			return true
		}
	}
	return false
}

// Inspect reports what one integration is right now: whether the files Relo
// wrote are still the ones in place, and which of them an editor or a client
// update replaced.
func (m *Manager) Inspect(ctx context.Context, id string) (View, error) {
	agent, found := m.agents.AgentFor(id)
	if !found {
		return View{}, fmt.Errorf("%s: %w", id, ErrUnknownAgent)
	}
	record, found, err := m.store.Integration(ctx, id)
	if err != nil {
		return View{}, err
	}
	files, err := m.store.Files(ctx, id)
	if err != nil {
		return View{}, err
	}
	view := View{
		Agent: agent, Enabled: record.Enabled, State: stateOrOff(record.State),
		LastError: record.LastError, KeyID: record.KeyID, UpdatedAtMs: record.UpdatedAtMs,
		ModelsStale: record.Enabled && m.modelsStale(agent, record),
		Files:       []FileView{},
		Context1M:   agent.ID == "codex" && (!found || record.Context1M),
	}
	return m.inspectFiles(agent, files, view), nil
}

func (m *Manager) inspectFiles(agent Agent, files []FileRecord, view View) View {
	for _, file := range files {
		if file.Kind == KindCodexLauncher {
			continue
		}
		entry := FileView{Kind: file.Kind, Path: file.Path, Snapshot: file.SnapshotPath}
		content, err := os.ReadFile(file.Path)
		switch {
		case err != nil:
			entry.Present = false
		default:
			entry.Present = true
			entry.Drifted = m.contributionDrifted(agent, file, string(content))
		}
		if entry.Drifted && view.State == StateOn {
			view.State = StateDrifted
		}
		view.Files = append(view.Files, entry)
	}
	return view
}

func (m *Manager) contributionDrifted(agent Agent, file FileRecord, content string) bool {
	switch file.Kind {
	case KindCodexConfig:
		return !m.files.CodexConfigPointsAt(content, m.paths.BaseURL(agent.Protocol), m.paths.HelperFile(agent.ID))
	case KindClaudeSettings:
		installHelper := m.claudeMode() != ClaudeAuthLogin
		return !m.files.ClaudeSettingsPointAt(content, m.paths.BaseURL(agent.Protocol), m.paths.HelperFile(agent.ID), installHelper)
	case FileOpenCode, FileHermes, FileOpenClaw:
		return !m.files.ProviderPointsAt(agent.ID, content, m.paths.BaseURL(agent.Protocol), m.modelList())
	default:
		if m.files.CatalogKind(file.Kind) {
			return !m.files.KindPointsAt(file.Kind, content, m.paths.BaseURL(agent.Protocol), m.modelList())
		}
		return m.files.Digest(content) != file.Digest
	}
}

// EnableOverwriting writes everything one agent needs even when the client's
// file already holds something else: the key file its helper reads, the
// client's own configuration where Relo manages one, and the helper that
// starts the daemon on demand. The rest of that file is kept.
func (m *Manager) EnableOverwriting(ctx context.Context, id, token string) (View, error) {
	return m.enable(ctx, id, token, true)
}

// Enable wires one agent to Relo, refusing where the client's file already
// has a provider Relo does not own; EnableOverwriting takes it over instead.
func (m *Manager) Enable(ctx context.Context, id, token string) (View, error) {
	return m.enable(ctx, id, token, false)
}

func (m *Manager) enable(ctx context.Context, id, token string, overwrite bool) (View, error) {
	agent, found := m.agents.AgentFor(id)
	if !found {
		return View{}, fmt.Errorf("%s: %w", id, ErrUnknownAgent)
	}
	if err := m.requireSurface(agent); err != nil {
		return View{}, err
	}
	record, err := m.ensureRecord(ctx, agent.ID)
	if err != nil {
		return View{}, err
	}
	if err := m.writeKeyFile(ctx, agent, token); err != nil {
		return View{}, err
	}
	if err := m.provisionAgentFiles(ctx, agent, token, overwrite); err != nil {
		return View{}, err
	}
	if err := m.publishAgentEnv(ctx, agent); err != nil {
		return View{}, err
	}
	record.ID, record.Enabled, record.State, record.LastError = id, true, StateOn, ""
	record.ModelsDigest = m.files.ModelsDigest(m.modelsFor(agent))
	record.UpdatedAtMs = m.now().UnixMilli()
	if err := m.store.SaveIntegration(ctx, record); err != nil {
		return View{}, err
	}
	m.appendOp(ctx, id, "enable", "")
	return m.Inspect(ctx, id)
}

func (m *Manager) provisionAgentFiles(ctx context.Context, agent Agent, token string, overwrite bool) error {
	if err := m.retireLauncher(agent); err != nil {
		m.recordFailure(ctx, agent, err)
		return err
	}
	if agent.ManagesFiles {
		if err := m.writeClientFiles(ctx, agent, token, overwrite); err != nil {
			m.recordFailure(ctx, agent, err)
			return err
		}
	}
	return nil
}

func (m *Manager) stripContributions(agent Agent, files []FileRecord) error {
	for _, file := range files {
		if err := m.removeContribution(agent, file); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) retireAgentRemnants(ctx context.Context, agent Agent) error {
	if err := m.retireLauncher(agent); err != nil {
		return err
	}
	if err := m.store.DeleteFiles(ctx, agent.ID); err != nil {
		return err
	}
	if err := m.removeClaudeCache(agent); err != nil {
		return err
	}
	if err := m.removeDesktopProfile(agent); err != nil {
		return err
	}
	return m.removeCodexCache(agent)
}

func (m *Manager) publishAgentEnv(ctx context.Context, agent Agent) error {
	if err := m.files.PublishEnv(agent); err != nil {
		m.recordFailure(ctx, agent, err)
		return err
	}
	return nil
}

// Disable removes everything Relo contributed and leaves the rest of the
// operator's files as they are. The snapshots stay on disk, so nothing Relo
// saw is lost by turning an integration off.
func (m *Manager) Disable(ctx context.Context, id string) (View, error) {
	agent, found := m.agents.AgentFor(id)
	if !found {
		return View{}, fmt.Errorf("%s: %w", id, ErrUnknownAgent)
	}
	files, err := m.store.Files(ctx, id)
	if err != nil {
		return View{}, err
	}
	if err := m.stripContributions(agent, files); err != nil {
		return View{}, err
	}
	if err := m.retireAgentRemnants(ctx, agent); err != nil {
		return View{}, err
	}
	if err := m.files.UnpublishEnv(agent); err != nil {
		return View{}, err
	}
	if err := m.recordDisable(ctx, id); err != nil {
		return View{}, err
	}
	m.appendOp(ctx, id, "disable", "")
	return m.Inspect(ctx, id)
}

func (m *Manager) recordDisable(ctx context.Context, id string) error {
	record, _, err := m.store.Integration(ctx, id)
	if err != nil {
		return err
	}
	record.ID, record.Enabled, record.State, record.LastError = id, false, StateOff, ""
	record.ModelsDigest = ""
	record.UpdatedAtMs = m.now().UnixMilli()
	return m.store.SaveIntegration(ctx, record)
}

// Rotate rewrites the files that carry the key, so a client that reads them
// picks the new one up on its next launch.
func (m *Manager) Rotate(ctx context.Context, id, token string) (View, error) {
	agent, found := m.agents.AgentFor(id)
	if !found {
		return View{}, fmt.Errorf("%s: %w", id, ErrUnknownAgent)
	}
	if _, err := m.ensureRecord(ctx, agent.ID); err != nil {
		return View{}, err
	}
	if err := m.writeKeyFile(ctx, agent, token); err != nil {
		return View{}, err
	}
	if agent.ID == "claude-code" {
		if err := m.writeKeyHelper(ctx, agent, KindClaudeHelper); err != nil {
			return View{}, err
		}
	}
	if agent.ID == "claude-desktop" {
		if err := m.writeClaudeDesktop(ctx, agent, token); err != nil {
			return View{}, err
		}
	}
	if err := m.files.PublishEnv(agent); err != nil {
		return View{}, err
	}
	if err := m.rememberModels(ctx, agent); err != nil {
		return View{}, err
	}
	m.appendOp(ctx, id, "rotate", "")
	return m.Inspect(ctx, id)
}

// Repair rewrites everything Relo contributed, which is what a client update
// that replaced a launcher or an editor that removed a block needs.
func (m *Manager) Repair(ctx context.Context, id, token string) (View, error) {
	view, err := m.Enable(ctx, id, token)
	if err != nil {
		return View{}, err
	}
	m.appendOp(ctx, id, "repair", "")
	return view, nil
}

// Restore puts the operator's own files back exactly as they were before Relo
// first wrote them, and removes everything Relo contributed.
func (m *Manager) Restore(ctx context.Context, id string) (View, error) {
	agent, found := m.agents.AgentFor(id)
	if !found {
		return View{}, fmt.Errorf("%s: %w", id, ErrUnknownAgent)
	}
	files, err := m.store.Files(ctx, id)
	if err != nil {
		return View{}, err
	}
	if err := m.retireLauncher(agent); err != nil {
		m.recordFailure(ctx, agent, err)
		return View{}, err
	}
	kept, err := m.stripCatalogFiles(ctx, agent, files)
	if err != nil {
		return View{}, err
	}
	return m.finishRestore(ctx, agent, id, kept)
}

func (m *Manager) finishRestore(ctx context.Context, agent Agent, id string, kept []FileRecord) (View, error) {
	// Every snapshot is read before anything is written, so a restore that
	// cannot complete leaves the operator's files as they were rather than
	// half rewritten. A failure keeps the stored file rows, which is what
	// makes the action worth retrying.
	staged, err := stageRestore(kept)
	if err != nil {
		m.recordFailure(ctx, agent, err)
		return View{}, err
	}
	if err := m.applyStagedRestore(ctx, agent, staged); err != nil {
		return View{}, err
	}
	if err := m.clearRestoredState(ctx, agent, id); err != nil {
		return View{}, err
	}
	return m.recordRestore(ctx, id)
}

func (m *Manager) applyStagedRestore(ctx context.Context, agent Agent, staged []stagedRestore) error {
	for _, file := range staged {
		if err := m.applyRestore(file); err != nil {
			m.recordFailure(ctx, agent, err)
			return err
		}
	}
	return nil
}

func (m *Manager) stripCatalogFiles(ctx context.Context, agent Agent, files []FileRecord) ([]FileRecord, error) {
	// The snapshot of a catalog file stays a backup. Restore takes the relo
	// block out of the live file and does not put the snapshot back, so an
	// edit made after setup survives, and the file is not deleted.
	kept := make([]FileRecord, 0, len(files))
	for _, file := range files {
		if !m.files.CatalogKind(file.Kind) {
			kept = append(kept, file)
			continue
		}
		if err := m.stripCatalog(file); err != nil {
			m.recordFailure(ctx, agent, err)
			return nil, err
		}
	}
	return kept, nil
}

func (m *Manager) clearRestoredState(ctx context.Context, agent Agent, id string) error {
	if err := m.store.DeleteFiles(ctx, id); err != nil {
		return err
	}
	if err := m.removeClaudeCache(agent); err != nil {
		return err
	}
	if err := m.removeDesktopProfile(agent); err != nil {
		return err
	}
	if err := m.removeCodexCache(agent); err != nil {
		return err
	}
	return m.files.UnpublishEnv(agent)
}

func (m *Manager) recordRestore(ctx context.Context, id string) (View, error) {
	record, _, err := m.store.Integration(ctx, id)
	if err != nil {
		return View{}, err
	}
	record.ID, record.Enabled, record.State, record.LastError = id, false, StateOff, ""
	record.ModelsDigest = ""
	record.UpdatedAtMs = m.now().UnixMilli()
	if err := m.store.SaveIntegration(ctx, record); err != nil {
		return View{}, err
	}
	m.appendOp(ctx, id, "restore", "")
	return m.Inspect(ctx, id)
}

// writeKeyFile stores the raw client key where the agent's launcher and helper
// ensureRecord returns one integration's stored row, creating it when the

func (m *Manager) ensureRecord(ctx context.Context, id string) (Record, error) {
	record, found, err := m.store.Integration(ctx, id)
	if err != nil {
		return Record{}, err
	}
	if found {
		return record, nil
	}
	record = Record{ID: id, State: StateOff, UpdatedAtMs: m.now().UnixMilli(), Context1M: true}
	if err := m.store.SaveIntegration(ctx, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (m *Manager) writeKeyFile(ctx context.Context, agent Agent, token string) error {
	path := m.paths.KeyFile(agent.ID)
	if err := m.files.WriteFileAtomic(path, []byte(token+"\n"), 0o600); err != nil {
		return err
	}
	return m.store.SaveFile(ctx, FileRecord{
		IntegrationID: agent.ID, Kind: KindKey, Path: path,
		Digest: m.files.Digest(token + "\n"), UpdatedAtMs: m.now().UnixMilli(),
	})
}

func (m *Manager) writeClientFiles(ctx context.Context, agent Agent, token string, overwrite bool) error {
	switch agent.ID {
	case "codex":
		if err := m.writeKeyHelper(ctx, agent, KindCodexHelper); err != nil {
			return err
		}
		return m.writeCodexConfig(ctx, agent)
	case "claude-code":
		return m.writeClaudeSettings(ctx, agent)
	case "claude-desktop":
		return m.writeClaudeDesktop(ctx, agent, token)
	case ClientOpenCode, ClientHermes, ClientOpenClaw:
		return m.writeManaged(ctx, agent, overwrite)
	default:
		if m.agents.CatalogAgent(agent.ID) {
			return m.writeCatalog(ctx, agent, overwrite)
		}
		return nil
	}
}

func (m *Manager) writeManaged(ctx context.Context, agent Agent, overwrite bool) error {
	target, err := m.paths.ConfigTarget(agent.ID)
	if err != nil {
		return err
	}
	if err := m.files.Installed(target.Dir); err != nil {
		return err
	}
	existing, err := m.files.ReadRegular(target.Path)
	if err != nil {
		return err
	}
	present, err := m.files.HasReloProvider(agent.ID, existing)
	if err != nil {
		return err
	}
	if present && !overwrite && !m.owns(ctx, agent.ID, target.Kind) {
		return foreignProvider()
	}
	merged, err := m.files.MergeProvider(agent.ID, existing, m.paths.BaseURL(agent.Protocol), m.modelList())
	if err != nil {
		return err
	}
	return m.writeContribution(ctx, agent, fileContribution{kind: target.Kind, path: target.Path, content: merged, mode: 0o644})
}

func (m *Manager) writeCatalog(ctx context.Context, agent Agent, overwrite bool) error {
	targets, err := m.paths.ConfigTargets(agent.ID)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := os.MkdirAll(target.Dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", target.Dir, err)
		}
		existing, err := m.files.ReadRegular(target.Path)
		if err != nil {
			return err
		}
		present, err := m.files.KindHasRelo(target.Kind, existing)
		if err != nil {
			return err
		}
		if present && !overwrite && !m.owns(ctx, agent.ID, target.Kind) {
			return foreignProvider()
		}
		merged, err := m.files.MergeKind(target.Kind, existing, m.paths.BaseURL(agent.Protocol), m.modelList())
		if err != nil {
			return err
		}
		if err := m.writeContribution(ctx, agent, fileContribution{kind: target.Kind, path: target.Path, content: merged, mode: 0o644}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) previewCatalog(ctx context.Context, agent Agent) []PlannedFile {
	targets, err := m.paths.ConfigTargets(agent.ID)
	if err != nil {
		path := filepath.Join(m.paths.Home(), ".aside", "u")
		return []PlannedFile{refusedPlan(PlannedFile{Kind: FileAside, Path: path}, err)}
	}
	planned := make([]PlannedFile, 0, len(targets))
	for _, target := range targets {
		planned = append(planned, m.previewCatalogTarget(ctx, agent, target))
	}
	return planned
}

func (m *Manager) previewCatalogTarget(ctx context.Context, agent Agent, target ConfigTarget) PlannedFile {
	fragment, ferr := m.files.MergeKind(target.Kind, "", m.paths.BaseURL(agent.Protocol), m.modelList())
	plan := PlannedFile{Kind: target.Kind, Path: target.Path, Fragment: fragment}
	if ferr != nil {
		return refusedPlan(plan, ferr)
	}
	existing, rerr := m.files.ReadRegular(target.Path)
	if rerr != nil {
		return refusedPlan(plan, rerr)
	}
	present, herr := m.files.KindHasRelo(target.Kind, existing)
	if herr != nil {
		return refusedPlan(plan, herr)
	}
	if present && !m.owns(ctx, agent.ID, target.Kind) {
		return refusedPlan(plan, foreignProvider())
	}
	return plan
}

func (m *Manager) writeCodexConfig(ctx context.Context, agent Agent) error {
	path := m.paths.CodexConfig()
	existing, err := m.files.ReadFileOrEmpty(path)
	if err != nil {
		return err
	}
	catalogPath := m.files.CodexCatalogPath(path)
	models := m.modelList()
	if err := m.files.WriteCodexCatalog(catalogPath, models); err != nil {
		return err
	}
	if err := m.files.SyncCodexCache(filepath.Dir(catalogPath), models); err != nil {
		return err
	}
	merged, err := m.files.MergeCodexConfig(existing, m.paths.BaseURL(agent.Protocol), catalogPath, m.paths.HelperFile(agent.ID), m.codexWideContext(ctx))
	if err != nil {
		return err
	}
	return m.writeContribution(ctx, agent, fileContribution{kind: KindCodexConfig, path: path, content: merged, mode: 0o644})
}

func (m *Manager) codexWideContext(ctx context.Context) bool {
	record, found, err := m.store.Integration(ctx, "codex")
	if err != nil || !found {
		return true
	}
	return record.Context1M
}

// SetCodexContext stores the 1M button. A Codex integration that is already
// set up has its configuration rewritten so the window matches the button.
func (m *Manager) SetCodexContext(ctx context.Context, enabled bool) (View, error) {
	agent, found := m.agents.AgentFor("codex")
	if !found {
		return View{}, fmt.Errorf("codex: %w", ErrUnknownAgent)
	}
	record, err := m.ensureRecord(ctx, agent.ID)
	if err != nil {
		return View{}, err
	}
	record.Context1M = enabled
	record.UpdatedAtMs = m.now().UnixMilli()
	if err := m.store.SaveIntegration(ctx, record); err != nil {
		return View{}, err
	}
	if record.Enabled {
		if err := m.writeCodexConfig(ctx, agent); err != nil {
			return View{}, err
		}
	}
	m.appendOp(ctx, agent.ID, "context", "")
	return m.Inspect(ctx, agent.ID)
}

func (m *Manager) writeClaudeSettings(ctx context.Context, agent Agent) error {
	if err := m.writeKeyHelper(ctx, agent, KindClaudeHelper); err != nil {
		return err
	}
	path := m.paths.ClaudeSettings()
	existing, err := m.files.ReadFileOrEmpty(path)
	if err != nil {
		return err
	}
	installHelper := m.claudeMode() != ClaudeAuthLogin
	merged, err := m.files.MergeClaudeSettings(existing, m.paths.BaseURL(agent.Protocol), m.paths.HelperFile(agent.ID), installHelper)
	if err != nil {
		return err
	}
	if err := m.writeContribution(ctx, agent, fileContribution{kind: KindClaudeSettings, path: path, content: merged, mode: 0o600}); err != nil {
		return err
	}
	return m.files.WriteGatewayCache(m.paths.ClaudeConfigDir(), m.paths.BaseURL(agent.Protocol), m.claudeModelList())
}

func (m *Manager) removeClaudeCache(agent Agent) error {
	if agent.ID != "claude-code" {
		return nil
	}
	return m.files.RemoveGatewayCache(m.paths.ClaudeConfigDir(), m.paths.BaseURL(agent.Protocol))
}

func (m *Manager) writeClaudeDesktop(ctx context.Context, agent Agent, token string) error {
	library := m.paths.DesktopLibrary()
	if err := m.paths.DesktopInstalled(); err != nil {
		return err
	}
	content, err := m.files.DesktopProfileContent(m.paths.BaseURL(agent.Protocol), token, m.claudeModelList())
	if err != nil {
		return err
	}
	path := m.files.DesktopProfilePath(library)
	if err := m.writeContribution(ctx, agent, fileContribution{kind: KindClaudeDesktop, path: path, content: string(content), mode: 0o600}); err != nil {
		return err
	}
	return m.files.SelectDesktopProfile(library)
}

func (m *Manager) removeDesktopProfile(agent Agent) error {
	if agent.ID != "claude-desktop" {
		return nil
	}
	return m.files.RemoveDesktopProfile(m.paths.DesktopLibrary())
}

func (m *Manager) writeKeyHelper(ctx context.Context, agent Agent, kind string) error {
	path := m.paths.HelperFile(agent.ID)
	script := m.files.KeyHelperScript(agent.ID)
	if err := m.files.WriteFileAtomic(path, []byte(script), 0o700); err != nil {
		return err
	}
	return m.store.SaveFile(ctx, FileRecord{
		IntegrationID: agent.ID, Kind: kind, Path: path,
		Digest: m.files.Digest(script), UpdatedAtMs: m.now().UnixMilli(),
	})
}

func (m *Manager) retireLauncher(agent Agent) error {
	if agent.ID != "codex" {
		return nil
	}
	return m.files.RetireLauncher(m.paths.LauncherState(agent.ID))
}

type fileContribution struct {
	kind    string
	path    string
	content string
	mode    os.FileMode
}

func (m *Manager) writeContribution(ctx context.Context, agent Agent, contrib fileContribution) error {
	files, err := m.store.Files(ctx, agent.ID)
	if err != nil {
		return err
	}
	snapshotPath := ""
	for _, file := range files {
		if file.Kind == contrib.kind {
			snapshotPath = file.SnapshotPath
			break
		}
	}
	if snapshotPath == "" {
		created, err := m.files.Snapshot(contrib.path, m.paths.SnapshotDir(), agent.ID+"-"+contrib.kind)
		if err != nil {
			return err
		}
		snapshotPath = created
	}
	if err := m.files.WriteFileAtomic(contrib.path, []byte(contrib.content), contrib.mode); err != nil {
		return err
	}
	return m.store.SaveFile(ctx, FileRecord{
		IntegrationID: agent.ID, Kind: contrib.kind, Path: contrib.path,
		Digest: m.files.Digest(contrib.content), SnapshotPath: snapshotPath, UpdatedAtMs: m.now().UnixMilli(),
	})
}

func (m *Manager) removeManaged(agent Agent, file FileRecord) error {
	if file.SnapshotPath == "" {
		if err := os.Remove(file.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", file.Path, err)
		}
		return nil
	}
	existing, err := m.files.ReadRegular(file.Path)
	if err != nil {
		return err
	}
	stripped, err := m.files.StripProvider(agent.ID, existing)
	if err != nil {
		return err
	}
	if strings.TrimSpace(stripped) == "" {
		if err := os.Remove(file.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", file.Path, err)
		}
		return nil
	}
	return m.files.WriteFileAtomic(file.Path, []byte(stripped), 0o644)
}

func (m *Manager) removeContribution(agent Agent, file FileRecord) error {
	switch file.Kind {
	case KindCodexConfig:
		return m.removeCodexContribution(file)
	case KindClaudeSettings:
		return m.removeClaudeContribution(agent, file)
	case KindCodexLauncher:
		return nil
	case FileOpenCode, FileHermes, FileOpenClaw:
		return m.removeManaged(agent, file)
	default:
		return m.removeGenericContribution(agent, file)
	}
}

func (m *Manager) removeCodexContribution(file FileRecord) error {
	existing, err := m.files.ReadFileOrEmpty(file.Path)
	if err != nil {
		return err
	}
	stripped := m.files.StripCodexConfig(existing)
	if err := os.Remove(m.files.CodexCatalogPath(file.Path)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if strings.TrimSpace(stripped) == "" {
		return os.Remove(file.Path)
	}
	return m.files.WriteFileAtomic(file.Path, []byte(stripped), 0o644)
}

func (m *Manager) removeClaudeContribution(agent Agent, file FileRecord) error {
	existing, err := m.files.ReadFileOrEmpty(file.Path)
	if err != nil {
		return err
	}
	if strings.TrimSpace(existing) == "" {
		return nil
	}
	stripped, err := m.files.StripClaudeSettings(existing, m.paths.BaseURL(agent.Protocol), m.paths.HelperFile(agent.ID))
	if err != nil {
		return err
	}
	return m.files.WriteFileAtomic(file.Path, []byte(stripped), 0o600)
}

func (m *Manager) removeGenericContribution(agent Agent, file FileRecord) error {
	if m.files.CatalogKind(file.Kind) {
		return m.stripCatalog(file)
	}
	if err := os.Remove(file.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", file.Path, err)
	}
	return nil
}

func (m *Manager) stripCatalog(file FileRecord) error {
	if _, err := os.Lstat(file.Path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("read %s: %w", file.Path, err)
	}
	existing, err := m.files.ReadRegular(file.Path)
	if err != nil {
		return err
	}
	stripped, err := m.files.StripKind(file.Kind, existing)
	if err != nil {
		return err
	}
	if strings.TrimSpace(stripped) == "" {
		stripped = m.files.KeptDocument(file.Kind)
	}
	return m.files.WriteFileAtomic(file.Path, []byte(stripped), 0o644)
}

// ConfigApplied reports whether every file Relo wrote for this agent still
// carries what Relo writes now. Agents Relo writes nothing for are applied.
// The message names the file that is missing or different.
func (m *Manager) ConfigApplied(ctx context.Context, id string) (bool, string) {
	agent, found := m.agents.AgentFor(id)
	if !found {
		return false, id + ": unknown integration"
	}
	if !agent.ManagesFiles {
		return true, ""
	}
	files, err := m.store.Files(ctx, id)
	if err != nil {
		return false, err.Error()
	}
	for _, file := range files {
		if file.Kind == KindKey || file.Kind == KindCodexLauncher {
			continue
		}
		content, err := os.ReadFile(file.Path)
		if err != nil {
			return false, "the settings file is not in " + file.Path
		}
		if m.contributionDrifted(agent, file, string(content)) {
			return false, "the provider block is not in " + file.Path
		}
	}
	return true, ""
}

// VerifyContributions checks what setup wrote for one agent: the login
// environment still exports the key, every settings file is on disk, and
// every file still carries Relo's block. A missing settings file is written
// again from what setup would write now, and a missing env block is
// published again; both are reported as fixed. A file that is present but
// different is reported, never rewritten: that repair stays explicit.
func (m *Manager) VerifyContributions(ctx context.Context, id string) ([]VerifyCheck, error) {
	agent, found := m.agents.AgentFor(id)
	if !found {
		return nil, fmt.Errorf("%s: %w", id, ErrUnknownAgent)
	}
	checks := make([]VerifyCheck, 0, 3)
	if strings.TrimSpace(agent.EnvKey) != "" {
		checks = append(checks, m.verifyEnv(agent))
	}
	if !agent.ManagesFiles {
		return checks, nil
	}
	if failed, generated := m.ensureMissingFiles(ctx, agent); failed != "" {
		checks = append(checks, VerifyCheck{Name: CheckSettings, Detail: failed})
	} else {
		checks = append(checks, VerifyCheck{
			Name: CheckSettings, OK: true,
			Fixed:  len(generated) > 0,
			Detail: strings.Join(generated, ", "),
		})
	}
	applied, message := m.ConfigApplied(ctx, id)
	checks = append(checks, VerifyCheck{Name: CheckConfig, OK: applied, Detail: message})
	return checks, nil
}

func (m *Manager) verifyEnv(agent Agent) VerifyCheck {
	if m.files.EnvPublished(agent) {
		return VerifyCheck{Name: CheckEnv, OK: true}
	}
	if err := m.files.PublishEnv(agent); err != nil {
		return VerifyCheck{Name: CheckEnv, Detail: err.Error()}
	}
	if !m.files.EnvPublished(agent) {
		return VerifyCheck{Name: CheckEnv, Detail: "the login environment does not carry " + agent.EnvKey}
	}
	return VerifyCheck{Name: CheckEnv, OK: true, Fixed: true}
}

func (m *Manager) ensureMissingFiles(ctx context.Context, agent Agent) (string, []string) {
	files, err := m.store.Files(ctx, agent.ID)
	if err != nil {
		return err.Error(), nil
	}
	var failed string
	generated := []string{}
	for _, file := range files {
		if file.Kind == KindKey || file.Kind == KindCodexLauncher {
			continue
		}
		if _, err := os.Stat(file.Path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			if failed == "" {
				failed = fmt.Errorf("read %s: %w", file.Path, err).Error()
			}
			continue
		}
		if err := m.regenerateMissing(ctx, agent, file); err != nil {
			if failed == "" {
				failed = err.Error()
			}
			continue
		}
		generated = append(generated, file.Path)
	}
	return failed, generated
}

func (m *Manager) regenerateMissing(ctx context.Context, agent Agent, file FileRecord) error {
	switch file.Kind {
	case KindCodexConfig:
		return m.writeCodexConfig(ctx, agent)
	case KindClaudeSettings:
		return m.writeClaudeSettings(ctx, agent)
	case KindClaudeDesktop:
		token, err := m.files.ReadFileOrEmpty(m.paths.KeyFile(agent.ID))
		if err != nil {
			return err
		}
		return m.writeClaudeDesktop(ctx, agent, strings.TrimSpace(token))
	case KindCodexHelper:
		return m.writeKeyHelper(ctx, agent, KindCodexHelper)
	case KindClaudeHelper:
		return m.writeKeyHelper(ctx, agent, KindClaudeHelper)
	case FileOpenCode, FileHermes, FileOpenClaw:
		return m.writeManaged(ctx, agent, false)
	default:
		if m.files.CatalogKind(file.Kind) {
			return m.writeCatalog(ctx, agent, false)
		}
		return fmt.Errorf("%w %s", ErrUnsupportedFile, file.Path)
	}
}

type stagedRestore struct {
	path    string
	content []byte
	remove  bool
	mode    os.FileMode
}

func stageRestore(files []FileRecord) ([]stagedRestore, error) {
	staged := make([]stagedRestore, 0, len(files))
	for _, file := range files {
		if file.Kind == KindCodexLauncher {
			continue
		}
		if file.SnapshotPath == "" {
			staged = append(staged, stagedRestore{path: file.Path, remove: true})
			continue
		}
		content, err := os.ReadFile(file.SnapshotPath)
		if err != nil {
			return nil, fmt.Errorf("read the snapshot %s: %w", file.SnapshotPath, err)
		}
		mode := os.FileMode(0o600)
		if info, err := os.Stat(file.Path); err == nil {
			mode = info.Mode().Perm()
		}
		staged = append(staged, stagedRestore{path: file.Path, content: content, mode: mode})
	}
	return staged, nil
}

func (m *Manager) applyRestore(file stagedRestore) error {
	if file.remove {
		if err := os.Remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", file.path, err)
		}
		return nil
	}
	return m.files.WriteFileAtomic(file.path, file.content, file.mode)
}

func (m *Manager) requireSurface(agent Agent) error {
	if m.paths.BaseURL(agent.Protocol) == "" {
		return fmt.Errorf("%s: %w (%s)", agent.ID, ErrSurfaceDisabled, agent.Protocol)
	}
	return nil
}

func (m *Manager) recordFailure(ctx context.Context, agent Agent, cause error) {
	record, _, err := m.store.Integration(ctx, agent.ID)
	if err != nil {
		return
	}
	record.ID, record.State, record.LastError = agent.ID, StateError, cause.Error()
	record.UpdatedAtMs = m.now().UnixMilli()
	if err := m.store.SaveIntegration(ctx, record); err != nil {
		m.logger.Warn("record a failed integration write", "integration", agent.ID, "error", err)
	}
}

func (m *Manager) appendOp(ctx context.Context, id, action, detail string) {
	op := OpRecord{
		ID: fmt.Sprintf("%s-%d", id, m.now().UnixNano()), IntegrationID: id,
		Action: action, Detail: detail, CreatedAtMs: m.now().UnixMilli(),
	}
	if err := m.store.AppendOp(ctx, op); err != nil {
		m.logger.Warn("record an integration action", "integration", id, "error", err)
	}
}

func stateOrOff(state string) string {
	if state == "" {
		return StateOff
	}
	return state
}
