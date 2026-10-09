// Integration orchestration: views, keys, and results.
package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/access"
	appaccess "github.com/jonaskahn/relo/internal/application/access"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
)

var (
	// ErrUnknownIntegration reports an identifier no agent matches.
	ErrUnknownIntegration = errors.New("unknown integration")
	// ErrRestartUnsupported reports an agent Relo has no process to restart.
	ErrRestartUnsupported = errors.New("this client has no process Relo can restart")
	// ErrActionInProgress reports an action this service is already running
	// for one agent. A second run would race the first over the same files
	// and the same subprocess, so the caller waits for it to finish instead.
	ErrActionInProgress = errors.New("this action is already running for that agent")
)

// The bounds one proof that an integration key reaches the data plane runs
// under: a listing answers quickly, a test turn runs a whole model call, and
// neither reads more than a page of the answer.
const (
	integrationVerifyTimeout = 10 * time.Second
	integrationChatTimeout   = 60 * time.Second
	integrationBodyLimit     = 1 << 20
	// integrationChatMaxTokens is the ceiling a test turn asks for, which is
	// enough for an answer and short enough to stay cheap.
	integrationChatMaxTokens = 512
)

// IntegrationView is one integration as a console reads it. The token itself
// is never part of this shape: it is handed over once, when it is minted.
type IntegrationView struct {
	ID           string
	Client       string
	Summary      string
	Protocol     string
	BaseURL      string
	ManagesFiles bool
	Enabled      bool
	State        string
	LastError    string
	// Key is the client key the integration owns, when it has one.
	Key *IntegrationKey
	// Files are the client files Relo wrote, with what happened to each.
	Files []FileView
	// Preview is what setting this integration up would write, so an operator
	// reads it before anything on the machine changes. It is present while the
	// integration is off and the agent has files Relo manages.
	Preview []PlannedFile
	// Steps are what an operator does by hand for a client Relo does not
	// configure itself, filled in with this machine's own address, the hint of
	// the key the integration owns, and a model Relo publishes. They are empty
	// for a client Relo writes files for, whose wiring the console shows
	// instead.
	Steps       []string
	UpdatedAtMs int64
	// ModelsStale is true when the model list Relo would write now differs
	// from the one stored at setup, so the card offers Repair.
	ModelsStale bool
	// RestartNeeded is true when Codex's app-server started before Relo last
	// wrote the catalog, so the picker still holds the old list.
	RestartNeeded bool
	// EnvKey is the variable Relo publishes for this client, so the secret
	// stays in Relo's key file rather than in the client's configuration.
	EnvKey string
	// Context1M is the Codex card's 1M button. It is omitted for every other
	// agent. Highlighted means the 1M window is written on setup.
	Context1M *bool
}

// IntegrationKey is the client key one integration owns, as a console reads
// it: the hint that identifies it, never the key.
type IntegrationKey struct {
	ID           string
	Name         string
	Hint         string
	Status       string
	CreatedAtMs  int64
	LastUsedAtMs int64
}

// IntegrationResult is what one action returned, with the token when a new one
// was minted. The token is the only time the secret leaves Relo.
type IntegrationResult struct {
	Integration IntegrationView
	Token       string
}

// IntegrationVerify is the proof that one integration's key reaches the data
// plane: the address that answered, the status it answered with, and how many
// models it offered. Config is true when the check also confirmed the provider
// block in the agent's own file. Checks are the wiring behind the key, one
// entry per check the verification ran.
type IntegrationVerify struct {
	OK      bool
	URL     string
	Status  int
	Models  int
	Message string
	Config  bool
	Checks  []VerifyCheck
}

// VerifyCheck is one check a verification ran: the key reaching the data
// plane, the login environment exporting it, the settings files being on
// disk, and the files carrying Relo's block. Fixed is true when the check
// repaired what it found missing. Detail names the failure or what was
// written again.
type VerifyCheck struct {
	Name   string
	OK     bool
	Detail string
	Fixed  bool
}

// The checks a verification reports, in the order the console shows them.
const (
	CheckKey      = "key"
	CheckEnv      = "env"
	CheckSettings = "settings"
	CheckConfig   = "config"
)

// IntegrationModel is one model the data plane offers a client, in the shape
// the client's own protocol reports it.
type IntegrationModel struct {
	ID   string
	Name string
	// ContextWindow is the window the data plane advertises for this model, so
	// an operator picking a model for the tester sees the budget it carries.
	ContextWindow *int64
	// ProviderID and SourceModelID are the connection and the provider's own
	// identifier the listing reported beside the public name, so a surface
	// attributes an entry without decoding the name it was published under.
	ProviderID    string
	SourceModelID string
}

// IntegrationModels is what one integration's key reaches: the address that
// answered, the status it answered with, and the models the client's own
// protocol listed. The identifiers are the ones the client would send, so a
// test turn names a model the way the agent itself does.
type IntegrationModels struct {
	OK      bool
	URL     string
	Status  int
	Models  []IntegrationModel
	Message string
}

// IntegrationChatRequest is one test turn an operator sends through an
// integration's own key.
type IntegrationChatRequest struct {
	Model  string
	Prompt string
}

// IntegrationChat is what one test turn answered: the status the client's own
// surface returned, the text it read back, and how long the turn took.
type IntegrationChat struct {
	OK         bool
	Status     int
	Model      string
	Text       string
	Error      string
	DurationMs int64
	// Warnings are what Relo doubted about the model it sent to, read from the
	// answer's advisory header, so the tester shows the same note a real
	// client would see.
	Warnings []string
}

// Integrations returns every agent Relo knows, with the state of its wiring.
func (s *Service) Integrations(ctx context.Context) ([]IntegrationView, error) {
	manager, err := s.manager()
	if err != nil {
		return nil, err
	}
	views := make([]IntegrationView, 0, len(s.agents.Agents()))
	for _, agent := range s.agents.Agents() {
		view, err := s.describeIntegration(ctx, manager, agent)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// Integration returns one integration.
func (s *Service) Integration(ctx context.Context, id string) (IntegrationView, error) {
	agent, found := s.agents.AgentFor(id)
	if !found {
		return IntegrationView{}, fmt.Errorf("%s: %w", id, ErrUnknownIntegration)
	}
	manager, err := s.manager()
	if err != nil {
		return IntegrationView{}, err
	}
	return s.describeIntegration(ctx, manager, agent)
}

// EnableIntegration wires one agent to Relo: it mints the key the agent runs
// with, writes the files Relo manages for it, and the helper that starts the
// daemon when the agent asks for its key.
func (s *Service) EnableIntegration(ctx context.Context, id string) (IntegrationResult, error) {
	return s.enableIntegration(ctx, id, false)
}

// EnableIntegrationOverwrite wires one agent the way EnableIntegration does,
// and replaces a relo provider already in the client's file.
func (s *Service) EnableIntegrationOverwrite(ctx context.Context, id string) (IntegrationResult, error) {
	return s.enableIntegration(ctx, id, true)
}

func (s *Service) enableIntegration(ctx context.Context, id string, overwrite bool) (IntegrationResult, error) {
	agent, found := s.agents.AgentFor(id)
	if !found {
		return IntegrationResult{}, fmt.Errorf("%s: %w", id, ErrUnknownIntegration)
	}
	manager, err := s.manager()
	if err != nil {
		return IntegrationResult{}, err
	}
	token, err := s.mintIntegrationKey(ctx, agent)
	if err != nil {
		return IntegrationResult{}, err
	}
	write := manager.Enable
	if overwrite {
		write = manager.EnableOverwriting
	}
	if _, err := write(ctx, agent.ID, token); err != nil {
		return IntegrationResult{}, err
	}
	s.restartCodex(ctx, agent.ID)
	view, err := s.describeIntegration(ctx, manager, agent)
	if err != nil {
		return IntegrationResult{}, err
	}
	return IntegrationResult{Integration: view, Token: token}, nil
}

// DisableIntegration is the console's "remove": it puts the operator's own
// files back exactly as they were before Relo first wrote them, including a
// file Relo changed after setup, and only then retires the key the integration
// owned. A restoration that cannot complete keeps the key and records the
// error, so the action is worth retrying rather than half done.
func (s *Service) DisableIntegration(ctx context.Context, id string) error {
	agent, found := s.agents.AgentFor(id)
	if !found {
		return fmt.Errorf("%s: %w", id, ErrUnknownIntegration)
	}
	manager, err := s.manager()
	if err != nil {
		return err
	}
	if _, err := manager.Restore(ctx, agent.ID); err != nil {
		return err
	}
	s.restartCodex(ctx, agent.ID)
	return s.revokeIntegrationKey(ctx, agent)
}

// RotateIntegrationKey mints a new key for one integration and rewrites the
// files that carry it, so the next launch of the agent picks it up.
func (s *Service) RotateIntegrationKey(ctx context.Context, id string) (IntegrationResult, error) {
	agent, found := s.agents.AgentFor(id)
	if !found {
		return IntegrationResult{}, fmt.Errorf("%s: %w", id, ErrUnknownIntegration)
	}
	manager, err := s.manager()
	if err != nil {
		return IntegrationResult{}, err
	}
	token, err := s.mintIntegrationKey(ctx, agent)
	if err != nil {
		return IntegrationResult{}, err
	}
	if _, err := manager.Rotate(ctx, agent.ID, token); err != nil {
		return IntegrationResult{}, err
	}
	view, err := s.describeIntegration(ctx, manager, agent)
	if err != nil {
		return IntegrationResult{}, err
	}
	return IntegrationResult{Integration: view, Token: token}, nil
}

// SetCodexContext stores the Codex card's 1M button and rewrites a config
// Relo already owns so the window matches it.
func (s *Service) SetCodexContext(ctx context.Context, id string, enabled bool) (IntegrationView, error) {
	if id != "codex" {
		return IntegrationView{}, fmt.Errorf("%s: %w", id, ErrUnknownIntegration)
	}
	manager, err := s.manager()
	if err != nil {
		return IntegrationView{}, err
	}
	if _, err := manager.SetCodexContext(ctx, enabled); err != nil {
		return IntegrationView{}, err
	}
	agent, _ := s.agents.AgentFor(id)
	return s.describeIntegration(ctx, manager, agent)
}

// RepairIntegration rewrites everything Relo contributed, which is what a
// client update that replaced a file needs. An existing key is reused, so a
// repair never invalidates the secret a running agent already holds. A new
// key is minted only when none is stored.
func (s *Service) RepairIntegration(ctx context.Context, id string) (IntegrationResult, error) {
	release, err := s.begin("repair", id)
	if err != nil {
		return IntegrationResult{}, err
	}
	defer release()
	agent, found := s.agents.AgentFor(id)
	if !found {
		return IntegrationResult{}, fmt.Errorf("%s: %w", id, ErrUnknownIntegration)
	}
	manager, err := s.manager()
	if err != nil {
		return IntegrationResult{}, err
	}
	token, minted, err := s.repairToken(ctx, agent)
	if err != nil {
		return IntegrationResult{}, err
	}
	if _, err := manager.Repair(ctx, agent.ID, token); err != nil {
		return IntegrationResult{}, err
	}
	s.restartCodex(ctx, agent.ID)
	view, err := s.describeIntegration(ctx, manager, agent)
	if err != nil {
		return IntegrationResult{}, err
	}
	if !minted {
		token = ""
	}
	return IntegrationResult{Integration: view, Token: token}, nil
}

func (s *Service) repairToken(ctx context.Context, agent Agent) (string, bool, error) {
	token, err := s.storedIntegrationToken(agent.ID)
	if err != nil || token != "" {
		return token, false, err
	}
	minted, err := s.mintIntegrationKey(ctx, agent)
	if err != nil {
		return "", false, err
	}
	return minted, true, nil
}

func (s *Service) storedIntegrationToken(id string) (string, error) {
	token, err := s.files.ReadFileOrEmpty(s.paths.KeyFile(id))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(token), nil
}

// RestartIntegration restarts the process one client keeps after Relo writes
// its files. Codex rereads its catalog. OpenCode restarts its background
// server so the server picks up the key Relo published for it.
func (s *Service) RestartIntegration(ctx context.Context, id string) (IntegrationView, error) {
	release, err := s.begin("restart", id)
	if err != nil {
		return IntegrationView{}, err
	}
	defer release()
	agent, found := s.agents.AgentFor(id)
	if !found {
		return IntegrationView{}, fmt.Errorf("%s: %w", id, ErrUnknownIntegration)
	}
	switch agent.ID {
	case "codex":
		if err := s.processes.RestartCodexDaemon(ctx, filepath.Dir(s.paths.CodexConfig()), s.paths.Home()); err != nil {
			return IntegrationView{}, err
		}
	case ClientOpenCode:
		token, err := s.files.ReadFileOrEmpty(s.paths.KeyFile(agent.ID))
		if err != nil {
			return IntegrationView{}, err
		}
		if err := s.processes.RestartOpenCodeService(ctx, s.paths.Home(), token); err != nil {
			return IntegrationView{}, err
		}
	default:
		return IntegrationView{}, fmt.Errorf("%s: %w", id, ErrRestartUnsupported)
	}
	return s.Integration(ctx, id)
}

func (s *Service) restartCodex(ctx context.Context, id string) {
	if id != "codex" {
		return
	}
	if err := s.processes.RestartCodexDaemon(ctx, filepath.Dir(s.paths.CodexConfig()), s.paths.Home()); err != nil && s.logger != nil {
		s.logger.Warn("restart the Codex app-server", "error", err)
	}
}

func (s *Service) begin(action, id string) (func(), error) {
	key := id + "/" + action
	if _, busy := s.running.LoadOrStore(key, true); busy {
		return nil, fmt.Errorf("%s on %s: %w", action, id, ErrActionInProgress)
	}
	return func() { s.running.Delete(key) }, nil
}

// RestoreIntegration puts the operator's own files back exactly as they were
// before Relo first wrote them, and retires the key the integration owned.
func (s *Service) RestoreIntegration(ctx context.Context, id string) error {
	agent, found := s.agents.AgentFor(id)
	if !found {
		return fmt.Errorf("%s: %w", id, ErrUnknownIntegration)
	}
	manager, err := s.manager()
	if err != nil {
		return err
	}
	if _, err := manager.Restore(ctx, agent.ID); err != nil {
		return err
	}
	s.restartCodex(ctx, agent.ID)
	return s.revokeIntegrationKey(ctx, agent)
}

// VerifyIntegration proves that one integration's key reaches the data plane.
// An agent Relo manages files for also has its wiring checked: the login
// environment exporting the key, the settings files on disk, and the files
// carrying Relo's block. A missing file or env block is written again and
// reported as fixed; a file that is present but different fails the check,
// and the operator repairs it explicitly.
func (s *Service) VerifyIntegration(ctx context.Context, id string) (IntegrationVerify, error) {
	read, err := s.IntegrationModels(ctx, id)
	if err != nil {
		return IntegrationVerify{}, err
	}
	verify := IntegrationVerify{
		OK: read.OK, URL: read.URL, Status: read.Status,
		Models: len(read.Models), Message: read.Message,
		Checks: []VerifyCheck{{Name: CheckKey, OK: read.OK, Detail: read.Message}},
	}
	manager, err := s.manager()
	if err != nil {
		return IntegrationVerify{}, err
	}
	contributions, err := manager.VerifyContributions(ctx, id)
	if err != nil {
		return IntegrationVerify{}, err
	}
	verify.Checks = append(verify.Checks, contributions...)
	return s.foldContributionChecks(verify, contributions, id), nil
}

func (s *Service) foldContributionChecks(verify IntegrationVerify, contributions []VerifyCheck, id string) IntegrationVerify {
	for _, check := range contributions {
		if !check.OK {
			verify.OK = false
			verify.Message = check.Detail
		}
	}
	if verify.OK {
		if agent, found := s.agents.AgentFor(id); found && agent.ManagesFiles {
			verify.Config = true
			verify.Message = "the provider block is in place and the key reached the data plane"
		}
	}
	return verify
}

// IntegrationToken answers the secret one integration runs with, read from
// the key file Relo wrote, so the console can show it behind an explicit
// reveal rather than only once at setup. An integration with no key file has
// no token to answer.
func (s *Service) IntegrationToken(ctx context.Context, id string) (string, error) {
	agent, found := s.agents.AgentFor(id)
	if !found {
		return "", fmt.Errorf("%s: %w", id, ErrUnknownIntegration)
	}
	view, err := s.Integration(ctx, id)
	if err != nil {
		return "", err
	}
	if !view.Enabled {
		return "", fmt.Errorf("%s: %w", agent.ID, appaccess.ErrNotFound)
	}
	token, err := s.files.ReadFileOrEmpty(s.paths.KeyFile(agent.ID))
	if err != nil {
		return "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("%s: %w (the integration has no key file)", agent.ID, appaccess.ErrNotFound)
	}
	return token, nil
}

// IntegrationModels reads the model list one integration's key reaches. The
// call travels the port the client itself is pointed at and speaks the
// protocol that client speaks, so the identifiers an operator picks from are
// the ones the agent would send.
func (s *Service) IntegrationModels(ctx context.Context, id string) (IntegrationModels, error) {
	agent, baseURL, token, err := s.integrationClient(id)
	if err != nil {
		return IntegrationModels{}, err
	}
	url := baseURL + "/v1/models"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return IntegrationModels{}, err
	}
	s.integrationCredential(request, agent.Protocol, token)
	client := &http.Client{Timeout: integrationVerifyTimeout}
	response, err := client.Do(request)
	if err != nil {
		return IntegrationModels{URL: url, Models: []IntegrationModel{}, Message: err.Error()}, nil
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(response.Body, integrationBodyLimit))
	return describeModelsRead(url, response, body), nil
}

func describeModelsRead(url string, response *http.Response, body []byte) IntegrationModels {
	models := integrationModelsOf(body)
	read := IntegrationModels{
		OK:  response.StatusCode >= 200 && response.StatusCode < 300,
		URL: url, Status: response.StatusCode, Models: models,
	}
	switch {
	case read.OK:
		read.Message = "the key reached the data plane"
	case response.StatusCode == http.StatusUnauthorized:
		read.Message = "the data plane refused the key"
	default:
		read.Message = "the data plane answered " + response.Status
	}
	return read
}

// IntegrationChat sends one prompt through the surface the client speaks, with
// the key the client runs with. A listing proves the key is accepted; a turn
// proves the whole path an agent uses, from its own port to a model's answer.
func (s *Service) IntegrationChat(ctx context.Context, id string, request IntegrationChatRequest) (IntegrationChat, error) {
	model := strings.TrimSpace(request.Model)
	prompt := strings.TrimSpace(request.Prompt)
	switch {
	case model == "":
		return IntegrationChat{}, fmt.Errorf("a model: %w", ErrInvalidQuery)
	case prompt == "":
		return IntegrationChat{}, fmt.Errorf("a prompt: %w", ErrInvalidQuery)
	}
	agent, baseURL, token, err := s.integrationClient(id)
	if err != nil {
		return IntegrationChat{}, err
	}
	return s.sendIntegrationTurn(ctx, integrationTarget{agent: agent, baseURL: baseURL, token: token}, chatTurn{model: model, prompt: prompt})
}

type integrationTarget struct {
	agent   Agent
	baseURL string
	token   string
}

type chatTurn struct {
	model  string
	prompt string
}

func (s *Service) sendIntegrationTurn(ctx context.Context, target integrationTarget, turn chatTurn) (IntegrationChat, error) {
	agent, baseURL, token, model, prompt := target.agent, target.baseURL, target.token, turn.model, turn.prompt
	path, body, err := integrationTurn(agent.Protocol, model, prompt)
	if err != nil {
		return IntegrationChat{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, strings.NewReader(body))
	if err != nil {
		return IntegrationChat{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	s.integrationCredential(httpRequest, agent.Protocol, token)
	started := time.Now()
	response, err := (&http.Client{Timeout: integrationChatTimeout}).Do(httpRequest)
	if err != nil {
		return IntegrationChat{Model: model, Error: err.Error(), DurationMs: time.Since(started).Milliseconds()}, nil
	}
	defer func() { _ = response.Body.Close() }()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, integrationBodyLimit))
	return describeChatAnswer(agent, response, payload, model, started), nil
}

func describeChatAnswer(agent Agent, response *http.Response, payload []byte, model string, started time.Time) IntegrationChat {
	answer := IntegrationChat{Status: response.StatusCode, Model: model, DurationMs: time.Since(started).Milliseconds()}
	// The advisory notes a served turn accepted travel on the answer, so the
	// tester shows what a real client would have seen.
	answer.Warnings = integrationWarnings(response.Header)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		answer.Error = integrationRefusal(payload, response.Status)
		return answer
	}
	answer.OK = true
	answer.Text = integrationAnswer(agent.Protocol, payload)
	return answer
}

func (s *Service) integrationClient(id string) (Agent, string, string, error) {
	agent, found := s.agents.AgentFor(id)
	if !found {
		return Agent{}, "", "", fmt.Errorf("%s: %w", id, ErrUnknownIntegration)
	}
	paths := s.paths
	baseURL := paths.BaseURL(agent.Protocol)
	if baseURL == "" {
		return Agent{}, "", "", fmt.Errorf("%s: %w", agent.ID, ErrSurfaceDisabled)
	}
	token, err := s.files.ReadFileOrEmpty(paths.KeyFile(agent.ID))
	if err != nil {
		return Agent{}, "", "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return Agent{}, "", "", fmt.Errorf("%s: %w (the integration has no key file)", agent.ID, appaccess.ErrNotFound)
	}
	return agent, baseURL, token, nil
}

func (s *Service) integrationCredential(request *http.Request, protocol, token string) {
	if protocol == inference.ProtocolAnthropic {
		request.Header.Set("x-api-key", token)
		request.Header.Set(s.wire.VersionHeader(), s.wire.APIVersion())
		return
	}
	request.Header.Set("Authorization", "Bearer "+token)
}

func integrationModelsOf(body []byte) []IntegrationModel {
	var document struct {
		Models []listingRow `json:"models"`
		Data   []listingRow `json:"data"`
	}
	models := []IntegrationModel{}
	if err := json.Unmarshal(body, &document); err != nil {
		return models
	}
	for _, entry := range append(document.Models, document.Data...) {
		id := strings.TrimSpace(entry.Slug)
		if id == "" {
			id = strings.TrimSpace(entry.ID)
		}
		if id == "" {
			continue
		}
		name := strings.TrimSpace(entry.DisplayName)
		if name == "" {
			name = strings.TrimSpace(entry.Name)
		}
		models = append(models, IntegrationModel{
			ID: id, Name: name,
			ContextWindow: firstInt(entry.ContextLength, entry.ContextWindow, entry.MaxInputTokens),
			ProviderID:    entry.ProviderID, SourceModelID: entry.SourceModelID,
		})
	}
	return models
}

type listingRow struct {
	ID             string `json:"id"`
	Slug           string `json:"slug"`
	DisplayName    string `json:"display_name"`
	Name           string `json:"name"`
	ProviderID     string `json:"provider_id"`
	SourceModelID  string `json:"source_model_id"`
	ContextLength  *int64 `json:"context_length"`
	ContextWindow  *int64 `json:"context_window"`
	MaxInputTokens *int64 `json:"max_input_tokens"`
}

func firstInt(values ...*int64) *int64 {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

const integrationWarningHeader = "X-Relo-Warning"

func integrationWarnings(header http.Header) []string {
	raw := strings.TrimSpace(header.Get(integrationWarningHeader))
	if raw == "" {
		return nil
	}
	notes := make([]string, 0, 2)
	for _, note := range strings.Split(raw, "; ") {
		if trimmed := strings.TrimSpace(note); trimmed != "" {
			notes = append(notes, trimmed)
		}
	}
	return notes
}

func integrationTurn(protocol, model, prompt string) (string, string, error) {
	if protocol == inference.ProtocolAnthropic {
		body, err := json.Marshal(map[string]any{
			"model": model, "max_tokens": integrationChatMaxTokens,
			"messages": []map[string]string{{"role": "user", "content": prompt}},
		})
		return "/v1/messages", string(body), err
	}
	body, err := json.Marshal(map[string]any{
		"model": model, "max_tokens": integrationChatMaxTokens, "stream": false,
		"messages": []map[string]string{{"role": "user", "content": prompt}},
	})
	return "/v1/chat/completions", string(body), err
}

func integrationAnswer(protocol string, body []byte) string {
	if protocol == inference.ProtocolAnthropic {
		return anthropicAnswerText(body)
	}
	var document struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &document); err != nil || len(document.Choices) == 0 {
		return ""
	}
	return document.Choices[0].Message.Content
}

func anthropicAnswerText(body []byte) string {
	var document struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		return ""
	}
	var builder strings.Builder
	for _, block := range document.Content {
		if block.Type == "text" {
			builder.WriteString(block.Text)
		}
	}
	return builder.String()
}

func integrationRefusal(body []byte, status string) string {
	var document struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &document); err == nil && strings.TrimSpace(document.Error.Message) != "" {
		return document.Error.Message
	}
	return "the data plane answered " + status
}

func (s *Service) describeIntegration(ctx context.Context, manager *Manager, agent Agent) (IntegrationView, error) {
	view, err := manager.Inspect(ctx, agent.ID)
	if err != nil {
		return IntegrationView{}, err
	}
	described := baseIntegrationView(s, agent, view)
	if agent.ID == "codex" {
		on := view.Context1M
		described.Context1M = &on
	}
	if described.Files == nil {
		described.Files = []FileView{}
	}
	described.Steps = s.integrationSteps(agent, described.Key)
	if !described.Enabled && agent.ManagesFiles {
		planned, err := manager.Preview(ctx, agent.ID)
		if err != nil {
			return IntegrationView{}, err
		}
		described.Preview = planned
	}
	return s.attachIntegrationKey(ctx, described, agent)
}

func baseIntegrationView(s *Service, agent Agent, view View) IntegrationView {
	return IntegrationView{
		ID: agent.ID, Client: agent.Client, Summary: agent.Summary,
		Protocol: agent.Protocol, BaseURL: s.paths.BaseURL(agent.Protocol),
		ManagesFiles: agent.ManagesFiles,
		Enabled:      view.Enabled, State: view.State, LastError: view.LastError,
		Files: view.Files, UpdatedAtMs: view.UpdatedAtMs, ModelsStale: view.ModelsStale,
		RestartNeeded: view.Enabled && agent.ID == "codex" && s.files.CodexNeedsRestart(
			filepath.Dir(s.paths.CodexConfig()), s.files.CodexCatalogPath(s.paths.CodexConfig()),
		),
		EnvKey: agent.EnvKey,
	}
}

func (s *Service) attachIntegrationKey(ctx context.Context, described IntegrationView, agent Agent) (IntegrationView, error) {
	key, found, err := s.keys.OwnedKey(ctx, agent.ID)
	if err != nil {
		return IntegrationView{}, err
	}
	if found {
		described.Key = &IntegrationKey{
			ID: key.ID, Name: key.Name, Hint: key.Hint,
			Status:      key.Status,
			CreatedAtMs: key.CreatedAtMs, LastUsedAtMs: key.LastUsedAtMs,
		}
	}
	return described, nil
}

func (s *Service) integrationSteps(agent Agent, key *IntegrationKey) []string {
	if agent.ManagesFiles {
		return nil
	}
	profile, found := access.ClientProfileFor(agent.Client)
	if !found {
		return nil
	}
	token := access.TokenPlaceholder
	if key != nil && strings.TrimSpace(key.Hint) != "" {
		token = key.Hint
	}
	model := access.DefaultModel
	if snapshot, err := s.snapshot(); err == nil {
		if listed := snapshot.Listed(); len(listed) > 0 {
			model = catalog.ClientModelID(listed[0].ProviderID, listed[0].ID)
		}
	}
	return profile.Instructions(access.SetupFill{
		Token: token, BaseURL: s.paths.BaseURL(agent.Protocol), Model: model,
	})
}

func (s *Service) mintIntegrationKey(ctx context.Context, agent Agent) (string, error) {
	_, found, err := s.keys.OwnedKey(ctx, agent.ID)
	if err != nil {
		return "", err
	}
	if found {
		issued, err := s.keys.RotateOwnedKey(ctx, agent.ID)
		if err != nil {
			return "", err
		}
		return issued.Token, nil
	}
	issued, err := s.keys.CreateAccessKey(ctx, appaccess.NewAccessKey{
		Name: integrationKeyName(agent.ID), Kind: appaccess.Agent,
		Client: agent.Client, Owner: agent.ID,
	})
	if err != nil {
		return "", err
	}
	return issued.Token, nil
}

func (s *Service) revokeIntegrationKey(ctx context.Context, agent Agent) error {
	return s.keys.RevokeOwnedKey(ctx, agent.ID)
}

func integrationKeyName(id string) string {
	return id + "-integration"
}

// ErrNoStore reports an integration session built without the state store
// integrations record their wiring in.
var ErrNoStore = errors.New("no integration store is configured")

// ErrInvalidQuery reports a test turn sent without a model or a prompt.
var ErrInvalidQuery = errors.New("invalid request")

// ServiceOptions configure the integration use cases.
type ServiceOptions struct {
	Paths     PathResolver
	Agents    AgentRegistry
	Files     ClientFiles
	Processes AgentProcesses
	Wire      WireCodec
	Store     Store
	Keys      *appaccess.Keys
	Catalog   *catalog.Catalog
	Logger    *slog.Logger
	Now       func() time.Time
}

// Service is the integration setup use cases: what Relo knows about every
// coding client, and the actions an operator runs on one.
type Service struct {
	paths     PathResolver
	agents    AgentRegistry
	files     ClientFiles
	processes AgentProcesses
	wire      WireCodec
	store     Store
	keys      *appaccess.Keys
	catalog   *catalog.Catalog
	logger    *slog.Logger
	now       func() time.Time
	// running holds the action each agent has in flight, so a second run of
	// the same action waits rather than racing the first over the same files.
	running sync.Map
}

// NewService returns the integration use cases over the given ports.
func NewService(options ServiceOptions) *Service {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Service{
		paths: options.Paths, agents: options.Agents, files: options.Files,
		processes: options.Processes, wire: options.Wire,
		store: options.Store, keys: options.Keys,
		catalog: options.Catalog, logger: logger, now: now,
	}
}

// RefreshClaudeGateway rewrites the model cache Claude Code reads when the
// integration is on, so a catalog change reaches the picker without another
// enable. A machine that has not enabled Claude Code is left alone.
func (s *Service) RefreshClaudeGateway(ctx context.Context) {
	if s.store == nil || s.catalog == nil {
		return
	}
	record, found, err := s.store.Integration(ctx, "claude-code")
	if err != nil || !found || !record.Enabled {
		return
	}
	base := s.paths.BaseURL(inference.ProtocolAnthropic)
	if base == "" {
		return
	}
	models := s.claudePickerModels()
	if record.ModelsDigest == s.files.ModelsDigest(models) {
		return
	}
	if err := s.files.WriteGatewayCache(s.paths.ClaudeConfigDir(), base, models); err != nil {
		if s.logger != nil {
			s.logger.Warn("refresh the Claude Code model cache", "error", err)
		}
		return
	}
	s.rememberModelsDigest(ctx, "claude-code", models)
}

// RefreshCodexCatalog rewrites the model catalog Codex reads when the
// integration is on, so a catalog change reaches the picker without another
// enable. A machine that has not enabled Codex is left alone.
func (s *Service) RefreshCodexCatalog(ctx context.Context) {
	if s.store == nil || s.catalog == nil {
		return
	}
	record, found, err := s.store.Integration(ctx, "codex")
	if err != nil || !found || !record.Enabled {
		return
	}
	path := s.files.CodexCatalogPath(s.paths.CodexConfig())
	models := s.publishedModels()
	if record.ModelsDigest == s.files.ModelsDigest(models) && s.files.CodexCatalogLists(path, models) {
		return
	}
	if err := s.files.WriteCodexCatalog(path, models); err != nil {
		if s.logger != nil {
			s.logger.Warn("refresh the Codex model catalog", "error", err)
		}
		return
	}
	if err := s.files.SyncCodexCache(filepath.Dir(path), models); err != nil {
		if s.logger != nil {
			s.logger.Warn("refresh the Codex models cache", "error", err)
		}
		return
	}
	s.rememberModelsDigest(ctx, "codex", models)
}

// RefreshClaudeDesktop rewrites the model list in the gateway profile when
// the integration is on. The key already in the profile stays there.
func (s *Service) RefreshClaudeDesktop(ctx context.Context) {
	if s.store == nil || s.catalog == nil {
		return
	}
	record, found, err := s.store.Integration(ctx, "claude-desktop")
	if err != nil || !found || !record.Enabled {
		return
	}
	base := s.paths.BaseURL(inference.ProtocolAnthropic)
	if base == "" {
		return
	}
	models := s.claudePickerModels()
	if record.ModelsDigest == s.files.ModelsDigest(models) {
		return
	}
	content, err := s.files.UpdateDesktopModels(s.paths.DesktopLibrary(), base, models)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("refresh the Claude Desktop model list", "error", err)
		}
		return
	}
	if content == nil {
		return
	}
	s.rememberDesktopProfile(ctx, content)
	s.rememberModelsDigest(ctx, "claude-desktop", models)
}

// CodexCatalogRows is the catalog GET /v1/models returns: the same rows Relo
// writes to the Codex catalog file.
func (s *Service) CodexCatalogRows() []map[string]any {
	return s.files.CodexCatalogRows(s.publishedModels(), filepath.Dir(s.paths.CodexConfig()))
}

func (s *Service) rememberModelsDigest(ctx context.Context, id string, models []ModelRef) {
	if s.store == nil {
		return
	}
	record, found, err := s.store.Integration(ctx, id)
	if err != nil || !found {
		return
	}
	record.ModelsDigest = s.files.ModelsDigest(models)
	record.UpdatedAtMs = s.now().UnixMilli()
	_ = s.store.SaveIntegration(ctx, record)
}

func (s *Service) rememberDesktopProfile(ctx context.Context, content []byte) {
	files, err := s.store.Files(ctx, "claude-desktop")
	if err != nil {
		return
	}
	for _, file := range files {
		if file.Kind != KindClaudeDesktop {
			continue
		}
		file.Digest = s.files.Digest(string(content))
		file.UpdatedAtMs = s.now().UnixMilli()
		_ = s.store.SaveFile(ctx, file)
	}
}

func (s *Service) manager() (*Manager, error) {
	if s.store == nil {
		return nil, ErrNoStore
	}
	return New(Options{
		Paths: s.paths, Agents: s.agents, Files: s.files,
		Store: s.store, Logger: s.logger, Now: s.now,
		Models: s.publishedModels, ClaudeModels: s.claudePickerModels,
	}), nil
}

func (s *Service) publishedModels() []ModelRef {
	snap, err := s.snapshot()
	if err != nil {
		return nil
	}
	listed := snap.Listed()
	groups := snap.ListedGroups()
	models := make([]ModelRef, 0, len(listed)+len(groups))
	for _, model := range listed {
		ref := namedRef(snap, model)
		ref.Connection = connectionLabel(snap, model.ProviderID)
		ref.Upstream = model.ID
		ref.ChatGPT = servedByChatGPT(snap, model.ProviderID)
		models = appendPublishedVariants(models, snap, ref, model)
	}
	for _, group := range groups {
		window, output := routeBudget(snap, group)
		ref := routeModelRef(group, window, output)
		models = appendPublishedGroup(models, snap, ref, group)
	}
	return models
}

func (s *Service) claudePickerModels() []ModelRef {
	snap, err := s.snapshot()
	if err != nil {
		return nil
	}
	listed := snap.Listed()
	groups := snap.ListedGroups()
	models := make([]ModelRef, 0, len(listed)+len(groups))
	for _, model := range listed {
		models = append(models, claudeModelRefs(snap, model)...)
	}
	for _, group := range groups {
		models = append(models, claudeRouteRefs(snap, group)...)
	}
	return models
}

func claudeModelRefs(snap *catalog.Snapshot, model catalog.Model) []ModelRef {
	ref := namedRef(snap, model)
	ref.Connection = connectionLabel(snap, model.ProviderID)
	standard, paired := catalog.StandardContext(ref.ContextWindow, snap.OffersLongContext(model))
	if !paired {
		if snap.OffersSuffixed(model) {
			ref.ID = catalog.AnthropicModelAlias(model.ProviderID, model.ID, ref.ContextWindow)
			return []ModelRef{ref}
		}
		// The marker names the long-context beta variant of a 200K model, so it is
		// spelled only on the entry published as that variant. With no twin there is
		// nothing to select, and a model the provider already named kimi-k3[1M]
		// keeps that name rather than gaining a second marker.
		ref.ID = catalog.AnthropicModelAlias(model.ProviderID, model.ID, nil)
		return []ModelRef{ref}
	}
	full := ref
	ref.ContextWindow = standard
	ref.ID = catalog.AnthropicModelAlias(model.ProviderID, model.ID, ref.ContextWindow)
	full.ID = catalog.AnthropicModelAlias(model.ProviderID, model.ID, full.ContextWindow)
	return []ModelRef{ref, full}
}

func claudeRouteRefs(snap *catalog.Snapshot, group catalog.Group) []ModelRef {
	window, output := routeBudget(snap, group)
	ref := routeModelRef(group, window, output)
	standard, paired := catalog.StandardContext(ref.ContextWindow, snap.OffersLongContextGroup(group))
	if !paired {
		if snap.NativeMillionGroup(group) {
			ref.ID = catalog.AnthropicClientID(ref.ID, nil)
		}
		if snap.OffersSuffixedGroup(group) {
			ref.ID = catalog.AnthropicClientID(ref.ID, ref.ContextWindow)
		}
		return []ModelRef{ref}
	}
	full := ref
	ref.ContextWindow = standard
	ref.ID = catalog.AnthropicClientID(ref.ID, ref.ContextWindow)
	full.ID = catalog.AnthropicClientID(full.ID, full.ContextWindow)
	return []ModelRef{ref, full}
}

func appendPublishedVariants(models []ModelRef, snap *catalog.Snapshot, ref ModelRef, model catalog.Model) []ModelRef {
	if snap.OffersLongContext(model) {
		return appendModelRefVariants(models, ref, true)
	}
	if snap.OffersSuffixed(model) {
		return appendSuffixedVariant(models, ref)
	}
	return append(models, ref)
}

func appendPublishedGroup(models []ModelRef, snap *catalog.Snapshot, ref ModelRef, group catalog.Group) []ModelRef {
	if snap.OffersLongContextGroup(group) {
		return appendModelRefVariants(models, ref, true)
	}
	if snap.OffersSuffixedGroup(group) {
		return appendSuffixedVariant(models, ref)
	}
	return append(models, ref)
}

func appendModelRefVariants(models []ModelRef, ref ModelRef, enabled bool) []ModelRef {
	standard, paired := catalog.StandardContext(ref.ContextWindow, enabled)
	if !paired {
		return append(models, ref)
	}
	full := ref
	full.ID = catalog.MillionAlias(ref.ID)
	ref.ContextWindow = standard
	return append(models, ref, full)
}

func appendSuffixedVariant(models []ModelRef, ref ModelRef) []ModelRef {
	suffixed := ref
	suffixed.ID = catalog.MillionAlias(ref.ID)
	return append(models, suffixed)
}

func namedRef(snap *catalog.Snapshot, model catalog.Model) ModelRef {
	ref := modelRef(model)
	if snap.ClaudeSubscription(model.ProviderID) {
		ref.Name = catalog.StripClaudePrefix(ref.Name)
	}
	return ref
}

func routeModelRef(group catalog.Group, window, output *int64) ModelRef {
	label := strings.TrimSpace(group.Label)
	if label == "" {
		label = group.ID
	}
	return ModelRef{
		ID: catalog.ClientRouteID(group.ID), Name: label,
		Connection: catalog.RouteLabelPrefix, ContextWindow: window, MaxOutput: output,
	}
}

func connectionLabel(snap *catalog.Snapshot, providerID string) string {
	host, found := snap.Provider(providerID)
	if !found {
		return providerID
	}
	if label := strings.TrimSpace(host.Label); label != "" {
		return label
	}
	return providerID
}

const chatGPTTemplate = "openai-codex"

func servedByChatGPT(snap *catalog.Snapshot, providerID string) bool {
	host, found := snap.Provider(providerID)
	return found && host.TemplateID == chatGPTTemplate
}

func routeBudget(snap *catalog.Snapshot, group catalog.Group) (window, output *int64) {
	for _, model := range snap.GroupModels(group) {
		window = smallerLimit(window, model.ContextWindow)
		output = smallerLimit(output, model.MaxOutput)
	}
	return window, output
}

func smallerLimit(current, candidate *int64) *int64 {
	if candidate == nil {
		return current
	}
	if current != nil && *current <= *candidate {
		return current
	}
	value := *candidate
	return &value
}

func modelRef(model catalog.Model) ModelRef {
	ref := ModelRef{
		ID: catalog.ClientModelID(model.ProviderID, model.ID), Name: model.Name,
	}
	if ref.Name == "" {
		ref.Name = model.ID
	}
	if model.ContextWindow != nil {
		ref.ContextWindow = new(*model.ContextWindow)
	}
	if model.MaxOutput != nil {
		ref.MaxOutput = new(*model.MaxOutput)
	}
	if model.SupportsTools != nil {
		ref.Tools = new(*model.SupportsTools)
	}
	if model.SupportsReasoning != nil {
		ref.Reasoning = new(*model.SupportsReasoning)
	}
	if model.SupportsVision != nil {
		ref.Vision = new(*model.SupportsVision)
	}
	if model.ReasoningEfforts != nil {
		ref.ReasoningEfforts = append([]string(nil), model.ReasoningEfforts...)
	}
	return ref
}

func (s *Service) snapshot() (*catalog.Snapshot, error) {
	if s.catalog == nil {
		return nil, catalog.ErrNoCatalog
	}
	snap, found := s.catalog.Snapshot()
	if !found {
		return nil, catalog.ErrNoCatalog
	}
	return snap, nil
}
