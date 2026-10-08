// Listener composition: the management and data-plane handlers.
package server

import (
	"github.com/jonaskahn/relo/internal/inference"
	"net/http"
	"strconv"
	"strings"
)

const (
	healthPath          = "/healthz"
	statusPath          = "/api/v1/status"
	inferencePrefix     = "/v1"
	chatCompletionsPath = inferencePrefix + "/chat/completions"
	responsesPath       = inferencePrefix + "/responses"
	messagesPath        = inferencePrefix + "/messages"
	modelsPath          = inferencePrefix + "/models"
	// retiredAgentPrefix is the prefix the inference surfaces answered on
	// before every protocol got a port of its own. The management listener
	// still claims it, so a client left pointing at it is told where to go.
	retiredAgentPrefix = "/agent"
	// CallbackPrefix is the subtree a browser lands on after a provider
	// redirect. The management listener serves it in the open, because the
	// browser carries no session at that moment.
	CallbackPrefix = "/callback/"
	faviconPath    = "/favicon.ico"
)

var managementExemptPaths = []string{"/logo.svg", LoginPath, authLoginPath, authSessionPath}

var managementExemptPrefixes = []string{"/_app/", CallbackPrefix, inferencePrefix + "/", retiredAgentPrefix + "/"}

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type dataPlaneProtocol struct {
	id       string
	surfaces []inferenceSurface
}

func dataPlaneProtocols() []dataPlaneProtocol {
	return []dataPlaneProtocol{
		{id: inference.ProtocolOpenAI, surfaces: []inferenceSurface{chatCompletionsSurface(), responsesSurface()}},
		{id: inference.ProtocolAnthropic, surfaces: []inferenceSurface{messagesSurface()}},
		// The Gemini port is reserved: the codec that would answer it exists,
		// the inbound surface that would drive it does not yet.
		{id: inference.ProtocolGemini},
	}
}

type servedProtocol struct {
	dataPlaneProtocol
	port int
}

func (s *Server) servedProtocols() []servedProtocol {
	served := make([]servedProtocol, 0, len(dataPlaneProtocols()))
	for _, protocol := range dataPlaneProtocols() {
		port := s.opts.Config.Server.DataPlane.Port(protocol.id)
		if port <= 0 || len(protocol.surfaces) == 0 {
			continue
		}
		served = append(served, servedProtocol{dataPlaneProtocol: protocol, port: port})
	}
	return served
}

// Handler returns the handler the management listener serves: the login
// callbacks and the health probe in the open, and the management API and the
// dashboard behind the admin token or a session. The paths the inference
// surfaces left are claimed here so a stale client is told where they are.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// The health probe is served above the guard, in the open: it answers a
	// supervisor rather than an operator, and it reports nothing but liveness.
	mux.HandleFunc("GET "+healthPath, s.handleHealthz)
	mux.HandleFunc("GET "+faviconPath, serveFavicon)
	mux.Handle("/", s.managementHandler())
	return LoggingMiddleware(s.opts.Logger, mux)
}

// DataPlaneHandler returns the handler of one protocol's listener, and
// whether this build serves that protocol at all. The console, the admin
// token, and the dashboard session never authenticate here.
func (s *Server) DataPlaneHandler(protocol string) (http.Handler, bool) {
	for _, entry := range dataPlaneProtocols() {
		if entry.id == protocol && len(entry.surfaces) > 0 {
			return LoggingMiddleware(s.opts.Logger, s.dataPlaneHandler(entry)), true
		}
	}
	return nil, false
}

func (s *Server) dataPlaneHandler(served dataPlaneProtocol) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+healthPath, s.handleHealthz)
	// The browser asks every address it opens for the mark, carrying no client
	// key, so the icon is served above the guard on this port too.
	mux.HandleFunc("GET "+faviconPath, serveFavicon)
	guard := s.apiKeyGuard
	if served.id == inference.ProtocolAnthropic {
		guard = s.anthropicGuard
	}
	routes := s.dataPlaneRoutes(served)
	handler := s.countInFlight(guard(routes))
	if served.id == inference.ProtocolOpenAI {
		handler = openModelList(s.countInFlight(routes), handler)
	}
	mux.Handle("/", handler)
	return mux
}

func openModelList(open, guarded http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == modelsPath {
			open.ServeHTTP(w, r)
			return
		}
		guarded.ServeHTTP(w, r)
	})
}

func (s *Server) dataPlaneRoutes(served dataPlaneProtocol) *http.ServeMux {
	mux := http.NewServeMux()
	for _, surface := range served.surfaces {
		mux.HandleFunc("POST "+surface.path, s.handleInference(surface))
	}
	mux.HandleFunc("GET "+modelsPath, s.handleListModels(served.id))
	mux.HandleFunc("/", s.handleNotFound)
	return mux
}

func (s *Server) handleInferencePathRetired(w http.ResponseWriter, r *http.Request) {
	baseURL, found := s.dataPlaneBaseURL(r.URL.Path)
	if !found {
		s.handleNotFound(w, r)
		return
	}
	s.fail(w, r, refusal{
		Status: http.StatusNotFound, Code: "not_found", Message: "api.inference.retired",
		Data: map[string]any{"BaseURL": baseURL + inferencePrefix},
	})
}

func (s *Server) dataPlaneBaseURL(path string) (string, bool) {
	protocol, found := inferencePathProtocol(path)
	if !found {
		return "", false
	}
	port := s.opts.Config.Server.DataPlane.Port(protocol)
	if port <= 0 {
		return "", false
	}
	return "http://" + s.opts.Config.Server.Bind + ":" + strconv.Itoa(port), true
}

func inferencePathProtocol(path string) (string, bool) {
	trimmed := strings.TrimPrefix(path, retiredAgentPrefix)
	for _, protocol := range dataPlaneProtocols() {
		for _, surface := range protocol.surfaces {
			if surface.path == trimmed {
				return protocol.id, true
			}
		}
	}
	// The model listing is shared by every protocol; a client that asked the
	// management listener for it is sent to the OpenAI-compatible port, which
	// is where the clients that look it up point.
	if trimmed == modelsPath {
		return inference.ProtocolOpenAI, true
	}
	return "", false
}

func (s *Server) managementHandler() http.Handler {
	routes := s.managementRoutes()
	guarded := s.sessionGuard(routes)
	return Exempt(managementExemptPaths, managementExemptPrefixes, routes, guarded)
}

func (s *Server) managementRoutes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+statusPath, s.handleStatus)
	mux.HandleFunc(retiredAgentPrefix+"/", s.handleInferencePathRetired)
	mux.HandleFunc(inferencePrefix+"/", s.handleInferencePathRetired)
	s.registerCallbacks(mux)
	s.registerManagementAPI(mux)
	s.registerEvents(mux)
	s.registerDashboard(mux)
	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: s.opts.Version})
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	s.fail(w, r, refusal{Status: http.StatusNotFound, Code: "not_found", Message: "api.route.missing"})
}

func (s *Server) handleNotImplemented(w http.ResponseWriter, r *http.Request) {
	s.fail(w, r, refusal{
		Status: http.StatusNotImplemented, Code: "not_implemented", Message: "api.route.unavailable",
	})
}

func (s *Server) registerDashboard(mux *http.ServeMux) {
	mux.Handle("/", s.dashboardHandler())
}

func (s *Server) dashboardHandler() http.Handler {
	s.dashboardMu.RLock()
	handler := s.dashboard
	s.dashboardMu.RUnlock()
	if handler == nil {
		return http.HandlerFunc(s.handleDashboardUnavailable)
	}
	return handler
}
