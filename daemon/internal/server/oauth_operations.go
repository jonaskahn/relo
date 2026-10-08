// OAuth login operations: tracking the sign-ins currently in flight.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	appaccount "github.com/jonaskahn/relo/internal/application/account"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	"github.com/jonaskahn/relo/internal/catalog"
)

// Login operation states the dashboard polls.
const (
	LoginRunning = "running"
	loginTimeout = 10 * time.Minute
	// staleLogin is how long a login may sit unfinished before a later login
	// forgets it.
	staleLogin  = 24 * time.Hour
	LoginDone   = "done"
	LoginFailed = "failed"
)

// ErrUnknownStatus reports a status a caller asked an account to take.
var ErrUnknownStatus = errors.New("status must be active or paused")

// ErrNoLoginFlow reports a server this build has no provider login for.
var ErrNoLoginFlow = errors.New("no login flow is configured")

// LoginPrompt is what a login wants the operator to see: a URL to open, and
// the code to enter when the provider uses a device flow.
type LoginPrompt struct {
	URL          string
	DeviceCode   string
	Instructions string
	// Ticket names this login on the callback page the browser lands on. It
	// is empty for a flow that renders its own page, which is what a login
	// started with no callback page to report through does.
	Ticket string
}

// LoginRequest is one provider login the management API started.
type LoginRequest struct {
	ProviderID string
	// Flow is the login to run, which names a sign-in method rather than a
	// provider: a provider offering a browser login and a device login has
	// two flows and one provider id. An empty Flow runs the provider's own.
	Flow   string
	Label  string
	Prompt func(LoginPrompt) error
	// Callbacks is the broker this server serves the callback page from. A
	// flow registers the login it is about to run here, so the browser that
	// finishes it lands on a page this process can report on. It is the
	// server's own broker, never a second one: a login registered elsewhere
	// would send the browser to a page that never heard of it.
	Callbacks CallbackBroker
}

// LoginResult is a finished login: the credential the server stores for the
// operator, and the label it carries.
type LoginResult struct {
	Label  string
	Kind   string
	Secret string
}

// LoginRunner performs one provider login. The command line passes the
// implementation, because only the composition root knows the OAuth flows
// this binary ships.
type LoginRunner interface {
	Login(ctx context.Context, request LoginRequest) (LoginResult, error)
}

type loginOperation struct {
	mu        sync.Mutex
	state     string
	provider  string
	prompt    LoginPrompt
	accountID string
	err       string
	startedAt time.Time
}

type oauthOperations struct {
	mu    sync.Mutex
	max   int
	items map[string]*loginOperation
}

func newOAuthOperations(max int) *oauthOperations {
	return &oauthOperations{max: max, items: map[string]*loginOperation{}}
}

func (o *oauthOperations) start(providerID string, now time.Time) (string, *loginOperation, error) {
	id, err := operationID()
	if err != nil {
		return "", nil, err
	}
	operation := &loginOperation{state: LoginRunning, provider: providerID, startedAt: now}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.forgetStaleLocked(now)
	o.evictOldestLocked()
	o.items[id] = operation
	return id, operation, nil
}

func (o *oauthOperations) lookup(id string) (*loginOperation, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	operation, found := o.items[id]
	return operation, found
}

func (o *oauthOperations) forgetStaleLocked(now time.Time) {
	for id, operation := range o.items {
		if now.Sub(operation.startedAt) > staleLogin {
			delete(o.items, id)
		}
	}
}

func (o *oauthOperations) evictOldestLocked() {
	if len(o.items) < o.max {
		return
	}
	oldestID, oldest := "", time.Time{}
	for id, operation := range o.items {
		if oldest.IsZero() || operation.startedAt.Before(oldest) {
			oldestID, oldest = id, operation.startedAt
		}
	}
	delete(o.items, oldestID)
}

func (o *loginOperation) setPrompt(prompt LoginPrompt) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.prompt = prompt
	return nil
}

func (o *loginOperation) succeed(accountID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.state, o.accountID = LoginDone, accountID
}

func (o *loginOperation) ticket() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.prompt.Ticket
}

func (o *loginOperation) fail(err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.state, o.err = LoginFailed, err.Error()
}

func operationID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate an operation id: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

type oauthStatusResponse struct {
	OperationID  string `json:"operation_id"`
	State        string `json:"state"`
	ProviderID   string `json:"provider_id"`
	URL          string `json:"url,omitempty"`
	DeviceCode   string `json:"device_code,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
	Error        string `json:"error,omitempty"`
}

type oauthPortStatus struct {
	Busy    bool   `json:"busy"`
	Port    int    `json:"port,omitempty"`
	Process string `json:"process,omitempty"`
	PID     int    `json:"pid,omitempty"`
}

func (s *Server) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	flow := r.PathValue("flow")
	id, err := s.StartLogin(r.Context(), flow, r.URL.Query().Get("label"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, oauthStatusResponse{
		OperationID: id, State: LoginRunning, ProviderID: s.providerIDForFlow(flow),
	})
}

// StartLogin begins one provider login in the background and returns the
// operation a caller polls. Both the management API and the dashboard start
// logins the same way, so neither can drift from the other.
func (s *Server) StartLogin(ctx context.Context, providerID, label string) (string, error) {
	catalogAPI, ok := s.catalogAPI()
	if !ok || s.opts.Accounts == nil {
		return "", ErrNoLoginFlow
	}
	flow := providerID
	// A provider that registered its redirect address with its vendor hands
	// the browser to one fixed port, and a listener already on it fails every
	// later login. The conflict is answered before a browser opens, so the
	// operator can clear the way instead of a failure after the fact.
	if conflict := s.portConflict(flow); conflict != nil {
		return "", conflict
	}
	providerID = s.providerIDForFlow(flow)
	if _, err := catalogAPI.Provider(ctx, providerID); err != nil {
		// A connection that is not there yet is created by the login itself,
		// once it has a credential to store: a browser that is opened and then
		// abandoned then leaves no empty connection behind. Starting one has to
		// refuse an unknown flow up front, before a browser opens on nothing.
		if _, found := s.templateByFlow(flow); !found {
			return "", fmt.Errorf("%s: %w", flow, ErrNoLoginFlow)
		}
	}
	id, operation, err := s.oauth.start(providerID, s.opts.Clock.Now())
	if err != nil {
		return "", err
	}
	go s.runLogin(catalogAPI, operation, LoginRequest{ProviderID: providerID, Flow: flow, Label: label})
	return id, nil
}

func (s *Server) providerIDForFlow(flow string) string {
	if t, found := s.templateByFlow(flow); found {
		return t.ID
	}
	return flow
}

func (s *Server) templateByFlow(flow string) (catalog.Template, bool) {
	if s.opts.Templates == nil {
		return catalog.Template{}, false
	}
	return s.opts.Templates.ByFlow(flow)
}

// LoginState is one provider login as a caller polls it. It carries the
// prompt an operator has to act on, and never the credential it produces.
type LoginState struct {
	State        string
	ProviderID   string
	URL          string
	DeviceCode   string
	Instructions string
	AccountID    string
	Error        string
}

// LoginStatus reports one started login, and whether the process still
// tracks it.
func (s *Server) LoginStatus(id string) (LoginState, bool) {
	if s.oauth == nil {
		return LoginState{}, false
	}
	operation, found := s.oauth.lookup(id)
	if !found {
		return LoginState{}, false
	}
	operation.mu.Lock()
	defer operation.mu.Unlock()
	return LoginState{
		State: operation.state, ProviderID: operation.provider,
		URL: operation.prompt.URL, DeviceCode: operation.prompt.DeviceCode,
		Instructions: operation.prompt.Instructions,
		AccountID:    operation.accountID, Error: operation.err,
	}, true
}

func (s *Server) runLogin(catalogAPI *appcatalog.Service, operation *loginOperation, request LoginRequest) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), loginTimeout)
	defer cancel()
	request.Callbacks = s.opts.Callbacks
	request.Prompt = operation.setPrompt
	result, err := s.opts.Login.Login(ctx, request)
	if err != nil {
		s.failLogin(operation, request, err)
		return
	}
	// The login wrote a credential, so the connection it belongs to is created
	// now, in the same step: nothing a browser leaves half-finished survives as
	// an empty connection.
	if _, err := catalogAPI.EnsureProvider(ctx, request.ProviderID, true); err != nil {
		s.failLogin(operation, request, err)
		return
	}
	account, err := s.opts.Accounts.AddAccount(ctx, appaccount.NewAccount{
		ProviderID: request.ProviderID, Kind: result.Kind, Label: result.Label, SecretValue: result.Secret,
	})
	if err != nil {
		s.failLogin(operation, request, err)
		return
	}
	s.probeAccountQuota(ctx, account.ID)
	operation.succeed(account.ID)
	s.opts.Callbacks.Complete(operation.ticket(), account.Label, nil)
}

func (s *Server) failLogin(operation *loginOperation, request LoginRequest, err error) {
	operation.fail(err)
	s.logLoginFailure(request.ProviderID, err)
	s.opts.Callbacks.Complete(operation.ticket(), "", err)
}

func (s *Server) logLoginFailure(providerID string, err error) {
	s.opts.Logger.Error("provider login failed", "provider", providerID, "error", err)
}

func (s *Server) handleOAuthGet(w http.ResponseWriter, r *http.Request) {
	flow := r.PathValue("flow")
	action := r.PathValue("action")
	if flow == "operations" {
		r.SetPathValue("id", action)
		s.handleOAuthStatus(w, r)
		return
	}
	if action == "port-status" {
		s.handleOAuthPortStatus(w, r)
		return
	}
	s.handleNotFound(w, r)
}

func (s *Server) handleOAuthPost(w http.ResponseWriter, r *http.Request) {
	switch r.PathValue("action") {
	case "start":
		s.handleOAuthStart(w, r)
	case "free-port":
		s.handleOAuthFreePort(w, r)
	default:
		s.handleNotFound(w, r)
	}
}

func (s *Server) handleOAuthStatus(w http.ResponseWriter, r *http.Request) {
	operation, found := s.oauth.lookup(r.PathValue("id"))
	if !found {
		s.fail(w, r, refusal{Status: http.StatusNotFound, Code: "not_found", Message: "api.oauth.unknown"})
		return
	}
	operation.mu.Lock()
	defer operation.mu.Unlock()
	writeJSON(w, http.StatusOK, oauthStatusResponse{
		OperationID: r.PathValue("id"), State: operation.state, ProviderID: operation.provider,
		URL: operation.prompt.URL, DeviceCode: operation.prompt.DeviceCode,
		Instructions: operation.prompt.Instructions,
		AccountID:    operation.accountID, Error: operation.err,
	})
}

func (s *Server) handleOAuthPortStatus(w http.ResponseWriter, r *http.Request) {
	flow := r.PathValue("flow")
	status := oauthPortStatus{}
	if conflict := s.portConflict(flow); conflict != nil {
		status = oauthPortStatus{
			Busy: true, Port: conflict.Port,
			Process: conflict.Owner.Name, PID: conflict.Owner.PID,
		}
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleOAuthFreePort(w http.ResponseWriter, r *http.Request) {
	flow := r.PathValue("flow")
	port := s.callbackPort(flow)
	if port <= 0 {
		s.fail(w, r, refusal{Status: http.StatusNotFound, Code: "not_found", Message: "api.oauth.unknown"})
		return
	}
	if err := s.freeCallbackPort(r.Context(), port); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
