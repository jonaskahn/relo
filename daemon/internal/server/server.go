// Package server exposes the HTTP listeners Relo runs: the management
// listener carrying the API and the dashboard operators use, and one data
// plane listener per client protocol, which is the address an AI client
// points at.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	appaccess "github.com/jonaskahn/relo/internal/application/access"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	appactivity "github.com/jonaskahn/relo/internal/application/activity"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
	approuting "github.com/jonaskahn/relo/internal/application/routing"
	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	appstatus "github.com/jonaskahn/relo/internal/application/status"
	apptemplates "github.com/jonaskahn/relo/internal/application/templatesettings"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/clock"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/i18n"
	"github.com/jonaskahn/relo/internal/routing"
	"github.com/jonaskahn/relo/internal/updates"
	"github.com/tiktoken-go/tokenizer"
)

// RecordSaveTimeout is the total allowance for final records after serving closes.
const RecordSaveTimeout = 500 * time.Millisecond

const readHeaderTimeout = 10 * time.Second

// Options configures a Server. The relay, pools, templates, redactor, and
// usage recorder are optional: without them the inference surfaces answer
// 501 and the management API still reports status.
type Options struct {
	Config        *config.Config
	SchemaVersion SchemaVersionSource
	Logger        *slog.Logger
	Clock         clock.Clock
	Version       string
	// Catalogs holds the messages this process speaks. A server built
	// without them loads the ones compiled into the binary.
	Catalogs *i18n.Catalogs
	// Language is the operator's language as this process resolved it at
	// startup, which answers whenever no stored setting exists.
	Language   string
	AdminToken string
	AccessKeys AccessKeySource
	// Keys is the client-key lifecycle the management API reads and writes.
	Keys *appaccess.Keys
	// Settings is the settings use cases the management API reads and writes.
	Settings *appsettings.Service
	// TemplateSettings is the Claude connection choices the settings tab writes.
	TemplateSettings *apptemplates.Service
	// Activity is the usage reads the management API answers.
	Activity *appactivity.Service
	// Accounts is the credential lifecycle the management API reads and writes.
	Accounts *appaccount.Service
	// Status is the health and doctor view the management API reports.
	Status *appstatus.Service
	// Routes saves and deletes the route groups the router reads.
	Routes *approuting.Service
	// CatalogAPI is the connection, model, and pricing use cases the
	// management API reads and writes.
	CatalogAPI *appcatalog.Service
	// Integrations is the coding-client setup use cases.
	Integrations *appintegration.Service
	// NativeAnthropicURL is where a claude.ai model id is forwarded. Empty
	// means https://api.anthropic.com.
	NativeAnthropicURL string
	// Catalog, Router, and Relay are what the data plane resolves a
	// request through. A server built without them answers 501 on inference.
	Catalog *catalog.Catalog
	Router  *routing.Router
	Relay   Relay
	// Templates resolves the curated connection shapes the relay branches
	// on. A server built without it treats every connection as untemplated.
	Templates Templates
	// CaptureRedactor bounds and scrubs the raw messages the log keeps. A
	// server built without one cannot serve inference.
	CaptureRedactor CaptureRedactor
	Pools           *account.Manager
	Secrets         SecretModeSource
	// Usage records completed requests in the usage log.
	Usage activity.RequestWriter
	// Captures stores the raw messages of a request: what the agent sent,
	// what Relo sent to a provider account, and what the provider answered.
	Captures activity.CaptureWriter
	// Quota stores the windows a live upstream response reported, and
	// QuotaFreshness reports which credentials a failed probe left reading
	// an older observation. QuotaRefresh reads that quota again for one
	// account or one connection.
	Quota          activity.Recorder
	QuotaFreshness activity.Freshness
	QuotaRefresh   activity.QuotaRefresh
	// RetryDelay, when set, replaces the drawn wait before each retry of a
	// retryable failure. A provider Retry-After longer than that wait still
	// wins.
	RetryDelay func(retry int) time.Duration
	// Formats resolves the codec of an upstream format, so a model whose
	// format this build has no codec for is refused before anything is sent.
	Formats  FormatRegistry
	Sessions *Sessions
	Events   *EventBus
	Login    LoginRunner
	// Shutdown, when set, is how a headless daemon ends itself: the stop
	// route calls it so `relo daemon stop` can end a daemon no terminal owns. A
	// process that runs the proxy itself, like the tray app, leaves it nil,
	// and the route reports that this process is not one to stop that way.
	Shutdown func()
	// InstanceID names the headless run this process is, which a stop request
	// has to repeat so a stale runtime file cannot end a newer daemon.
	InstanceID string
	// Restart, when set, is how a headless daemon cycles itself from the
	// console. The hook starts a detached helper so this process can finish
	// answering first.
	Restart func(force bool)
	// Updates is the feed this process checks for a newer build.
	Updates *updates.Checker
	// Callbacks routes a provider redirect to the login that started it. The
	// platform builds the broker once and hands it over; without one the
	// browser is left on the loopback page the flow renders for itself.
	Callbacks CallbackBroker
	// CallbackIcon is the artwork the callback page carries in its own
	// document. The platform hands over the shared brand icon.
	CallbackIcon string
	// CallbackPorts reports the loopback port one login flow listens on.
	// Without one every port reads as free.
	CallbackPorts CallbackPorts
	// Ports reports and clears the process holding a loopback port. A server
	// built without one treats every port as free, which is what a build with
	// no process table to ask does.
	Ports PortInspector
}

// InboundRegistry resolves the codec one client surface speaks, so the
// transport never constructs a codec family directly. The registry also
// renders Messages refusals and recognizes Anthropic requests, which are
// the two remaining family details the transport needs.
type InboundRegistry interface {
	Inbound(surface string) (wire.InboundCodec, bool)
	FailureDocument(status int, message string) []byte
	IsAnthropicRequest(header http.Header) bool
}

// FormatRegistry resolves the codec one upstream format speaks, which is how
// an attempt is built without this package importing a codec family.
type FormatRegistry interface {
	Outbound(format catalog.APIFormat) (wire.CodecModule, bool)
	ModeFor(format catalog.APIFormat) string
	InboundRegistry
}

// ListenerManagement names the management listener in the address maps this
// package reports: the console, the management API, the login callbacks.
const ListenerManagement = "management"

// Addrs names every address one process bound: the management listener, and
// each data plane protocol under its identifier.
type Addrs struct {
	Management string
	DataPlane  map[string]string
}

// SetDashboard attaches the console this process serves. A server without
// one answers the dashboard paths with 501, so the management API stays
// usable on its own.
func (s *Server) SetDashboard(handler http.Handler) {
	s.dashboardMu.Lock()
	defer s.dashboardMu.Unlock()
	s.dashboard = handler
}

// OnReady registers a callback the server invokes once, with every address
// it bound, before it answers the first request. A caller that needs the
// bound address of a port-0 listener reads it here, because the configured
// address only names the port to ask for.
func (s *Server) OnReady(fn func(addrs Addrs)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onReady = fn
}

// Server holds the listener and everything its handlers need.
type Server struct {
	opts             Options
	startedAt        time.Time
	sessions         *Sessions
	oauth            *oauthOperations
	events           *EventBus
	gate             *AccessKeyGate
	nativeClient     *http.Client
	contextTokenizer tokenizer.Codec
	inflight         atomic.Int64
	// bookkeeping runs the advisory writes of relayed responses behind the
	// response instead of in front of it.
	bookkeeping *bookkeeping
	// sweep is the maintenance sweep the console can start and read.
	sweep sweepJob

	// dashboardMu guards the attached console, which is set after the server
	// exists. It is a lock of its own because the routes are built while the
	// listener lock is held, and that read must not wait on a write.
	dashboardMu sync.RWMutex
	dashboard   http.Handler

	mu                sync.RWMutex
	servers           []*http.Server
	addr              string
	dataPlane         map[string]string
	onReady           func(addrs Addrs)
	lifetime          context.Context
	cancel            context.CancelFunc
	persistence       context.Context
	cancelPersistence context.CancelFunc
	stopping          bool
	requests          sync.WaitGroup
	closeOnce         sync.Once
	shutdownOnce      sync.Once
	shutdownDone      chan struct{}
	shutdownErr       error
	closeErr          error
	stopDeadline      time.Time
}

// New creates a Server with the listener configured but not started.
func New(opts Options) *Server {
	opts = withServerDefaults(opts)
	lifetime, cancel := context.WithCancel(context.Background())
	persistence, cancelPersistence := context.WithCancel(context.Background())
	built := &Server{
		opts: opts, startedAt: opts.Clock.Now(), sessions: opts.Sessions,
		oauth: newOAuthOperations(MaxSessions), events: opts.Events,
		gate:         NewAccessKeyGate(opts.AccessKeys, opts.Clock, opts.Logger),
		nativeClient: &http.Client{},
		dataPlane:    map[string]string{},
		bookkeeping:  newBookkeeping(),
		sweep:        sweepJob{state: sweepIdle},
		lifetime:     lifetime, cancel: cancel, persistence: persistence, cancelPersistence: cancelPersistence,
		shutdownDone: make(chan struct{}),
	}
	built.contextTokenizer, _ = tokenizer.Get(tokenizer.O200kBase)
	return built
}

func withServerDefaults(opts Options) Options {
	setServerInfra(&opts)
	setServerCatalogs(&opts)
	if opts.Language == "" {
		opts.Language = i18n.English
	}
	if opts.NativeAnthropicURL == "" {
		opts.NativeAnthropicURL = nativeAnthropicDefault
	}
	setServerUpdates(&opts)
	return opts
}

func setServerInfra(opts *Options) {
	if opts.Config == nil {
		defaults := config.DefaultConfig()
		opts.Config = &defaults
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if opts.Clock == nil {
		opts.Clock = clock.New()
	}
	if opts.Sessions == nil {
		opts.Sessions = NewSessions(opts.Clock)
	}
	if opts.Events == nil {
		opts.Events = NewEventBus(EventBusOptions{Subscribers: MaxSubscribers})
	}
	if opts.AccessKeys == nil && opts.Keys != nil {
		opts.AccessKeys = opts.Keys
	}
}

func setServerCatalogs(opts *Options) {
	if opts.Catalogs != nil {
		return
	}
	catalogs, err := i18n.Load()
	if err != nil {
		opts.Logger.Error("load the message catalogs", "error", err)
	}
	opts.Catalogs = catalogs
}

func setServerUpdates(opts *Options) {
	if opts.Updates != nil {
		return
	}
	opts.Updates = &updates.Checker{
		URL:      opts.Config.System.Updates.URL,
		Download: opts.Config.System.Updates.Download,
		Current:  opts.Version,
	}
}

func (s *Server) handleDashboardUnavailable(w http.ResponseWriter, r *http.Request) {
	s.fail(w, r, refusal{
		Status: http.StatusNotImplemented, Code: "not_implemented", Message: "api.dashboard.missing",
	})
}

// Start binds every listener and serves until ctx is cancelled or one of
// them fails. The caller closes the database.
func (s *Server) Start(ctx context.Context) error {
	if ctx.Err() != nil || s.lifetime.Err() != nil {
		return s.Shutdown(context.Background())
	}
	bound, err := s.bind()
	if err != nil {
		return err
	}
	return s.serve(ctx, bound)
}

// Addr returns the bound address, or an empty string before Start has bound
// it.
func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.addr
}

// DataPlaneAddrs returns the bound address of every data plane listener,
// keyed by the protocol it carries. A protocol this process does not serve
// is absent rather than empty.
func (s *Server) DataPlaneAddrs() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	addrs := make(map[string]string, len(s.dataPlane))
	maps.Copy(addrs, s.dataPlane)
	return addrs
}

// Addrs returns every address this process bound.
func (s *Server) Addrs() Addrs {
	return Addrs{Management: s.Addr(), DataPlane: s.DataPlaneAddrs()}
}

func (s *Server) countInFlight(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.inflight.Add(1)
		defer s.inflight.Add(-1)
		next.ServeHTTP(w, r)
	})
}

// InFlight returns how many inference requests are being served right now.
func (s *Server) InFlight() int64 {
	return s.inflight.Load()
}

// Drained reports whether the inference surfaces are idle, which is what a
// compaction waits for.
func (s *Server) Drained() bool {
	return s.InFlight() == 0
}

// InvalidateAccessKey forgets one cached client key, so a revocation or
// rotation an operator just made takes effect on the next request.
func (s *Server) InvalidateAccessKey(id string) {
	s.gate.Invalidate(id)
}

type boundListener struct {
	name     string
	listener net.Listener
	handler  http.Handler
}

func (s *Server) bind() ([]boundListener, error) {
	requests := []struct {
		name    string
		addr    string
		handler http.Handler
	}{{name: ListenerManagement, addr: s.opts.Config.Server.Addr(), handler: s.Handler()}}
	for _, served := range s.servedProtocols() {
		requests = append(requests, struct {
			name    string
			addr    string
			handler http.Handler
		}{name: served.id, addr: s.opts.Config.Server.DataPlaneAddr(served.id),
			handler: LoggingMiddleware(s.opts.Logger, s.dataPlaneHandler(served.dataPlaneProtocol))})
	}
	bound := make([]boundListener, 0, len(requests))
	for _, request := range requests {
		listener, err := listen(request.addr)
		if err != nil {
			closeListeners(bound)
			return nil, fmt.Errorf("%s listener: %w", request.name, err)
		}
		bound = append(bound, boundListener{name: request.name, listener: listener, handler: request.handler})
	}
	return bound, nil
}

func closeListeners(bound []boundListener) {
	for _, item := range bound {
		_ = item.listener.Close()
	}
}

func (s *Server) serve(ctx context.Context, bound []boundListener) error {
	cancelled := context.AfterFunc(ctx, s.closeServing)
	defer cancelled()
	servers, onReady := s.publishListeners(bound)
	if len(servers) == 0 {
		return s.Shutdown(context.Background())
	}
	failures := make(chan error, len(bound))
	for index, item := range bound {
		go serveListener(servers[index], item.listener, failures)
	}
	if onReady != nil {
		onReady(s.Addrs())
	}
	select {
	case <-s.lifetime.Done():
		return s.Shutdown(context.Background())
	case err := <-failures:
		s.opts.Logger.Warn("daemon stopping", "cause", "listener failed", "error", err)
		_ = s.Shutdown(context.Background())
		return err
	}
}

func (s *Server) publishListeners(bound []boundListener) ([]*http.Server, func(Addrs)) {
	servers := make([]*http.Server, 0, len(bound))
	dataPlane := map[string]string{}
	management := ""
	for _, item := range bound {
		server := &http.Server{Handler: item.handler, ReadHeaderTimeout: readHeaderTimeout,
			BaseContext: func(net.Listener) context.Context { return s.lifetime }}
		servers = append(servers, server)
		if item.name == ListenerManagement {
			management = item.listener.Addr().String()
			continue
		}
		dataPlane[item.name] = item.listener.Addr().String()
	}
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		closeListeners(bound)
		return nil, nil
	}
	s.servers = servers
	s.addr = management
	s.dataPlane = dataPlane
	onReady := s.onReady
	s.mu.Unlock()
	return servers, onReady
}

func serveListener(server *http.Server, listener net.Listener, failures chan<- error) {
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		failures <- err
	}
}

func listen(addr string) (net.Listener, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	return listener, nil
}
