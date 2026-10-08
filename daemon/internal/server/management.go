// Management route registration and the collaborator availability guards.
package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/activity"
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
)

const (
	// apiPrefix is the subtree every management route lives under.
	apiPrefix = "/api/v1/"
	// maxManagementBody bounds one management request body.
	maxManagementBody = 1 << 20
)

// Log-query refusals name the malformed filters the console maps to its own
// wording: a timestamp that is not milliseconds and a limit that is not a number.
var (
	ErrLogSinceRequired = errors.New("since_ms must be a millisecond timestamp")
	ErrLogUntilRequired = errors.New("until_ms must be a millisecond timestamp")
	ErrLogLimitRequired = errors.New("limit must be a number")
)

func (s *Server) registerManagementAPI(mux *http.ServeMux) {
	s.registerAccountRoutes(mux)
	s.registerAccessKeyRoutes(mux)
	s.registerConnectionRoutes(mux)
	s.registerCatalogRoutes(mux)
	s.registerRouteRoutes(mux)
	s.registerActivityRoutes(mux)
	s.registerSettingsRoutes(mux)
	s.registerIntegrationRoutes(mux)
	s.registerDaemonRoutes(mux)
	s.registerAuthRoutes(mux)
	s.registerOAuthRoutes(mux)
	mux.HandleFunc(apiPrefix, s.handleNotFound)
}

func (s *Server) registerAccountRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiPrefix+"accounts", s.handleListAccounts)
	mux.HandleFunc("POST "+apiPrefix+"accounts", s.handleCreateAccount)
	mux.HandleFunc("GET "+apiPrefix+"accounts/{id}", s.handleGetAccount)
	mux.HandleFunc("PATCH "+apiPrefix+"accounts/{id}", s.handlePatchAccount)
	mux.HandleFunc("DELETE "+apiPrefix+"accounts/{id}", s.handleDeleteAccount)
	mux.HandleFunc("GET "+apiPrefix+"accounts/{id}/models", s.handleGetAccountModels)
	mux.HandleFunc("POST "+apiPrefix+"accounts/{id}/models/refresh", s.handleRefreshAccountModels)
	mux.HandleFunc("GET "+apiPrefix+"accounts/{id}/models/context", s.handleGetAccountContext)
	mux.HandleFunc("POST "+apiPrefix+"accounts/{id}/models/context", s.handleSetAccountContext)
}

func (s *Server) registerAccessKeyRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiPrefix+"clients/keys", s.handleListAccessKeys)
	mux.HandleFunc("POST "+apiPrefix+"clients/keys", s.handleCreateAccessKey)
	mux.HandleFunc("POST "+apiPrefix+"clients/keys/deletions", s.handleDeleteExpiredAccessKeys)
	mux.HandleFunc("PATCH "+apiPrefix+"clients/keys/{id}", s.handleUpdateAccessKey)
	mux.HandleFunc("POST "+apiPrefix+"clients/keys/{id}/rotate", s.handleRotateAccessKey)
	mux.HandleFunc("DELETE "+apiPrefix+"clients/keys/{id}", s.handleDeleteAccessKey)
}

func (s *Server) registerConnectionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiPrefix+"connections", s.handleListProviders)
	mux.HandleFunc("GET "+apiPrefix+"connections/{id}/template-settings", s.handleGetTemplateSettings)
	mux.HandleFunc("PATCH "+apiPrefix+"connections/{id}/template-settings", s.handlePatchTemplateSettings)
	mux.HandleFunc("GET "+apiPrefix+"connections/{id}", s.handleGetProvider)
	mux.HandleFunc("PATCH "+apiPrefix+"connections/{id}", s.handlePatchProvider)
	mux.HandleFunc("DELETE "+apiPrefix+"connections/{id}", s.handleDeleteProvider)
	mux.HandleFunc("POST "+apiPrefix+"connections/probe", s.handleProbeProvider)
	mux.HandleFunc("POST "+apiPrefix+"connections/probes/{id}/commit", s.handleCommitProbe)
	mux.HandleFunc("DELETE "+apiPrefix+"connections/probes/{id}", s.handleDiscardProbe)
	mux.HandleFunc("POST "+apiPrefix+"connections/{id}/models/refresh", s.handleRefreshModels)
	mux.HandleFunc("POST "+apiPrefix+"connections/{id}/quota/refresh", s.handleRefreshQuota)
	mux.HandleFunc("POST "+apiPrefix+"connections/{id}/models/enabled", s.handleToggleModels)
	mux.HandleFunc("POST "+apiPrefix+"connections/{id}/models", s.handleAddModel)
	mux.HandleFunc("POST "+apiPrefix+"connections/{id}/models/clone", s.handleCloneModel)
	mux.HandleFunc("POST "+apiPrefix+"connections/{id}/models/context", s.handleSetModelsContext)
	mux.HandleFunc("POST "+apiPrefix+"connections/{id}/models/capabilities", s.handleSetModelsCapabilities)
}

func (s *Server) registerCatalogRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiPrefix+"templates", s.handleListProviderTemplates)
	mux.HandleFunc("GET "+apiPrefix+"modelsdev", s.handleGetModelsDevState)
	mux.HandleFunc("POST "+apiPrefix+"modelsdev/refresh", s.handleRefreshModelsDev)
	mux.HandleFunc("GET "+apiPrefix+"modelsdev/models", s.handleSearchModelsDev)
	mux.HandleFunc("POST "+apiPrefix+"catalog/refresh", s.handleRefreshCatalog)
	mux.HandleFunc("GET "+apiPrefix+"models", s.handleListCatalogModels)
	mux.HandleFunc("GET "+apiPrefix+"models/{provider}/{model...}", s.handleGetModel)
	mux.HandleFunc("PATCH "+apiPrefix+"models/{provider}/{model...}", s.handlePatchModel)
	mux.HandleFunc("DELETE "+apiPrefix+"models/{provider}/{model...}", s.handleDeleteModel)
}

func (s *Server) registerRouteRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiPrefix+"routes", s.handleListGroups)
	mux.HandleFunc("GET "+apiPrefix+"routes/preview", s.handleRoutePreview)
	mux.HandleFunc("GET "+apiPrefix+"routes/{id}", s.handleGetGroup)
	mux.HandleFunc("PUT "+apiPrefix+"routes/{id}", s.handlePutGroup)
	mux.HandleFunc("DELETE "+apiPrefix+"routes/{id}", s.handleDeleteGroup)
}

func (s *Server) registerActivityRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiPrefix+"activity/usage", s.handleUsage)
	mux.HandleFunc("GET "+apiPrefix+"activity/usage/summary", s.handleUsageSummary)
	mux.HandleFunc("GET "+apiPrefix+"activity/quota", s.handleQuotas)
	mux.HandleFunc("GET "+apiPrefix+"activity/requests", s.handleLogs)
	mux.HandleFunc("GET "+apiPrefix+"activity/requests/{id}/attempts", s.handleAttempts)
	mux.HandleFunc("GET "+apiPrefix+"activity/requests/{id}/captures", s.handleCaptures)
	mux.HandleFunc("GET "+apiPrefix+"activity/requests/{id}/captures/{capture}/body", s.handleCaptureBody)
	mux.HandleFunc("GET "+apiPrefix+"logs/daemon", s.handleDaemonLogs)
	mux.HandleFunc("GET "+apiPrefix+"logs/startups", s.handleStartupList)
	mux.HandleFunc("GET "+apiPrefix+"logs/startups/{name}", s.handleStartupTranscript)
}

func (s *Server) registerSettingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiPrefix+"ui/usage-metrics", s.handleUsageMetrics)
	mux.HandleFunc("PUT "+apiPrefix+"ui/usage-metrics", s.handleSaveUsageMetrics)
	mux.HandleFunc("GET "+apiPrefix+"settings", s.handleGetSettings)
	mux.HandleFunc("PUT "+apiPrefix+"settings/retention", s.handlePutRetention)
	mux.HandleFunc("PATCH "+apiPrefix+"settings/access", s.handlePatchAccess)
	mux.HandleFunc("PATCH "+apiPrefix+"settings/network", s.handlePatchNetwork)
	mux.HandleFunc("PATCH "+apiPrefix+"settings/providers", s.handlePatchProviders)
	mux.HandleFunc("PATCH "+apiPrefix+"settings/system", s.handlePatchSystem)
	mux.HandleFunc("PATCH "+apiPrefix+"settings/language", s.handlePatchLanguage)
	mux.HandleFunc("PATCH "+apiPrefix+"settings/appearance", s.handlePatchAppearance)
	mux.HandleFunc("PUT "+apiPrefix+"settings/retention/preview", s.handlePreviewRetention)
	mux.HandleFunc("POST "+apiPrefix+"settings/retention/run", s.handleRunRetention)
	mux.HandleFunc("POST "+apiPrefix+"settings/retention/sweep", s.handleStartSweep)
	mux.HandleFunc("GET "+apiPrefix+"settings/retention/sweep", s.handleGetSweep)
	mux.HandleFunc("GET "+apiPrefix+"doctor", s.handleDoctor)
}

func (s *Server) registerIntegrationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiPrefix+"integrations", s.handleListIntegrations)
	mux.HandleFunc("GET "+apiPrefix+"integrations/{id}", s.handleGetIntegration)
	// The key file behind the manage modal's eye control. It is a GET so the
	// console reads the same secret the agent reads, without minting a new one.
	mux.HandleFunc("GET "+apiPrefix+"integrations/{id}/token", s.handleGetIntegrationToken)
	mux.HandleFunc("POST "+apiPrefix+"integrations/{id}/enable", s.handleEnableIntegration)
	mux.HandleFunc("POST "+apiPrefix+"integrations/{id}/disable", s.handleDisableIntegration)
	mux.HandleFunc("POST "+apiPrefix+"integrations/{id}/rotate", s.handleRotateIntegration)
	mux.HandleFunc("POST "+apiPrefix+"integrations/{id}/repair", s.handleRepairIntegration)
	mux.HandleFunc("POST "+apiPrefix+"integrations/{id}/context", s.handleCodexContext)
	mux.HandleFunc("POST "+apiPrefix+"integrations/{id}/restart", s.handleRestartIntegration)
	mux.HandleFunc("POST "+apiPrefix+"integrations/{id}/restore", s.handleRestoreIntegration)
	mux.HandleFunc("POST "+apiPrefix+"integrations/{id}/verify", s.handleVerifyIntegration)
	// The two calls a console makes to test a key: the models that client
	// shape sees, and one turn sent through it.
	mux.HandleFunc("GET "+apiPrefix+"integrations/{id}/models", s.handleIntegrationModels)
	mux.HandleFunc("POST "+apiPrefix+"integrations/{id}/chat", s.handleIntegrationChatTurn)
	// The chat tester runs one turn through the same relay a client uses,
	// recorded as internal traffic so an operator can tell a test from a real
	// request in the log.
	mux.HandleFunc("POST "+apiPrefix+"integrations/chat", s.handleIntegrationChat)
}

func (s *Server) registerDaemonRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiPrefix+"updates", s.handleUpdates)
	mux.HandleFunc("POST "+apiPrefix+"daemon/stop", s.handleDaemonStop)
	mux.HandleFunc("POST "+apiPrefix+"daemon/restart", s.handleDaemonRestart)
	mux.HandleFunc("POST "+apiPrefix+"daemon/force-restart", s.handleDaemonForceRestart)
	mux.HandleFunc("POST "+apiPrefix+"daemon/shutdown", s.handleDaemonShutdown)
}

func (s *Server) registerAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST "+authLoginPath, s.handleAPILogin)
	mux.HandleFunc("POST "+authLogoutPath, s.handleAPILogout)
	mux.HandleFunc("GET "+authSessionPath, s.handleAPISession)
}

func (s *Server) registerOAuthRoutes(mux *http.ServeMux) {
	// One pattern per method. A wildcard flow and a literal "operations"
	// segment both match /oauth/operations/port-status, and ServeMux panics
	// when it is asked to register that overlap.
	mux.HandleFunc("GET "+apiPrefix+"oauth/{flow}/{action}", s.handleOAuthGet)
	mux.HandleFunc("POST "+apiPrefix+"oauth/{flow}/{action}", s.handleOAuthPost)
}

func (s *Server) catalogAPIOrUnavailable(w http.ResponseWriter, r *http.Request) (*appcatalog.Service, bool) {
	if s.opts.CatalogAPI == nil {
		s.fail(w, r, refusal{
			Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: "api.service.unavailable",
		})
		return nil, false
	}
	return s.opts.CatalogAPI, true
}

func (s *Server) catalogAPI() (*appcatalog.Service, bool) {
	if s.opts.CatalogAPI == nil {
		return nil, false
	}
	return s.opts.CatalogAPI, true
}

func (s *Server) integrationsOrUnavailable(w http.ResponseWriter, r *http.Request) (*appintegration.Service, bool) {
	if s.opts.Integrations == nil {
		s.fail(w, r, refusal{
			Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: "api.service.unavailable",
		})
		return nil, false
	}
	return s.opts.Integrations, true
}

func (s *Server) keysOrUnavailable(w http.ResponseWriter, r *http.Request) (*appaccess.Keys, bool) {
	if s.opts.Keys == nil {
		s.fail(w, r, refusal{
			Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: "api.service.unavailable",
		})
		return nil, false
	}
	return s.opts.Keys, true
}

func (s *Server) settingsOrUnavailable(w http.ResponseWriter, r *http.Request) (*appsettings.Service, bool) {
	if s.opts.Settings == nil {
		s.fail(w, r, refusal{
			Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: "api.service.unavailable",
		})
		return nil, false
	}
	return s.opts.Settings, true
}

func (s *Server) handleGetTemplateSettings(w http.ResponseWriter, r *http.Request) {
	service, ok := s.templateSettingsOrUnavailable(w, r)
	if !ok {
		return
	}
	stored, err := service.Read(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTemplateSettingResponse(stored))
}

func (s *Server) handlePatchTemplateSettings(w http.ResponseWriter, r *http.Request) {
	service, ok := s.templateSettingsOrUnavailable(w, r)
	if !ok {
		return
	}
	var patch templateSettingPatchRequest
	if !s.decodeJSON(w, r, &patch) {
		return
	}
	stored, err := service.Update(r.Context(), r.PathValue("id"), toTemplateSettingPatch(patch))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTemplateSettingResponse(stored))
}

func (s *Server) templateSettingsOrUnavailable(w http.ResponseWriter, r *http.Request) (*apptemplates.Service, bool) {
	if s.opts.TemplateSettings == nil {
		s.fail(w, r, refusal{
			Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: "api.service.unavailable",
		})
		return nil, false
	}
	return s.opts.TemplateSettings, true
}

func (s *Server) accountsOrUnavailable(w http.ResponseWriter, r *http.Request) (*appaccount.Service, bool) {
	if s.opts.Accounts == nil {
		s.fail(w, r, refusal{
			Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: "api.service.unavailable",
		})
		return nil, false
	}
	return s.opts.Accounts, true
}

func (s *Server) statusOrUnavailable(w http.ResponseWriter, r *http.Request) (*appstatus.Service, bool) {
	if s.opts.Status == nil {
		s.fail(w, r, refusal{
			Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: "api.service.unavailable",
		})
		return nil, false
	}
	return s.opts.Status, true
}

func (s *Server) routesOrUnavailable(w http.ResponseWriter, r *http.Request) (*approuting.Service, bool) {
	if s.opts.Routes == nil {
		s.fail(w, r, refusal{
			Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: "api.service.unavailable",
		})
		return nil, false
	}
	return s.opts.Routes, true
}

func (s *Server) activityOrUnavailable(w http.ResponseWriter, r *http.Request) (*appactivity.Service, bool) {
	if s.opts.Activity == nil {
		s.fail(w, r, refusal{
			Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: "api.service.unavailable",
		})
		return nil, false
	}
	return s.opts.Activity, true
}

func (s *Server) decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxManagementBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		s.fail(w, r, refusal{
			Status: http.StatusBadRequest, Code: "bad_request", Message: "api.body.invalid",
			Data: map[string]any{"Detail": err.Error()},
		})
		return false
	}
	if err := decoder.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		s.fail(w, r, refusal{
			Status: http.StatusBadRequest, Code: "bad_request", Message: "api.body.single_object",
		})
		return false
	}
	return true
}

func writeServiceError(w http.ResponseWriter, err error) {
	var portBusy *PortBusyError
	if errors.As(err, &portBusy) || errors.Is(err, ErrCallbackPortBusy) {
		writeError(w, http.StatusConflict, "callback_port_busy", err.Error())
		return
	}
	if errors.Is(err, appintegration.ErrActionInProgress) || errors.Is(err, activity.ErrRetentionBusy) {
		writeError(w, http.StatusConflict, "action_in_progress", err.Error())
		return
	}
	writeDomainError(w, err)
}

func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, appaccount.ErrAccountNotFound), errors.Is(err, appcatalog.ErrProviderNotFound),
		errors.Is(err, apptemplates.ErrNotFound),
		errors.Is(err, appaccess.ErrNotFound), errors.Is(err, appcatalog.ErrModelNotFound),
		errors.Is(err, appintegration.ErrUnknownIntegration), errors.Is(err, appstatus.ErrLogNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, appaccount.ErrNoSecretStore), errors.Is(err, catalog.ErrNoCatalog):
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", err.Error())
	case errors.Is(err, appcatalog.ErrCatalogConflict), errors.Is(err, appaccount.ErrDuplicateAccount),
		errors.Is(err, appcatalog.ErrListAuthoritative),
		appintegration.RefusalCode(err) == appintegration.CodeForeignKey:
		writeError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, catalog.ErrListRejected), errors.Is(err, catalog.ErrListUnsupported):
		writeError(w, http.StatusBadGateway, "upstream_unavailable", err.Error())
	case errors.Is(err, ErrNoLoginFlow):
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", err.Error())
	case isClientError(err):
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
	}
}

func (s *Server) writeRetentionError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, activity.ErrInvalidRetention) {
		s.fail(w, r, refusal{
			Status: http.StatusBadRequest, Code: "bad_request",
			Message: "api.settings.retention.invalid",
			Data:    map[string]any{"Min": activity.MinUsageDays},
		})
		return
	}
	writeServiceError(w, err)
}

func isClientError(err error) bool {
	switch appintegration.RefusalCode(err) {
	case appintegration.CodeNotInstalled, appintegration.CodeRelativePath,
		appintegration.CodeUnrecognised:
		return true
	}
	sentinels := []error{
		appintegration.ErrInvalidQuery, appcatalog.ErrInvalidCatalogRow,
		appcatalog.ErrInvalidRouteMember,
		appaccount.ErrSharedRoster,
		appaccess.ErrOwned,
		appaccount.ErrUnknownProvider, appaccount.ErrEmptySecret, appaccount.ErrPriorityRange,
		appcatalog.ErrNotCustom,
		appaccess.ErrInvalid, appaccess.ErrNameTaken,
		appactivity.ErrInvalidQuery,
		appstatus.ErrInvalidLogQuery,
		activity.ErrInvalidFilter, activity.ErrInvalidCursor,
		account.ErrUnknownStrategy,
		appsettings.ErrInvalidWrite, activity.ErrInvalidRetention,
		apptemplates.ErrUnsupported,
		appcatalog.ErrUnknownModelsDevRef, ErrUnknownStatus,
		appintegration.ErrRestartUnsupported,
	}
	for _, sentinel := range sentinels {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}

func queryInt(r *http.Request, name string, fallback int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func queryInt64(r *http.Request, name string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func logsQuery(r *http.Request) (appactivity.LogQuery, error) {
	query := r.URL.Query()
	since, err := queryInt64(r, "since_ms", 0)
	if err != nil {
		return appactivity.LogQuery{}, ErrLogSinceRequired
	}
	until, err := queryInt64(r, "until_ms", 0)
	if err != nil {
		return appactivity.LogQuery{}, ErrLogUntilRequired
	}
	limit, err := queryInt(r, "limit", 0)
	if err != nil {
		return appactivity.LogQuery{}, ErrLogLimitRequired
	}
	return appactivity.LogQuery{
		SinceMs: since, UntilMs: until, Provider: query.Get("provider"),
		Model: query.Get("model"), Credential: query.Get("credential"),
		Account: query.Get("account"),
		Surface: query.Get("surface"), RequestID: query.Get("request_id"),
		// client names one access key, and client_app names the coding client a
		// key was issued for; the console offers one filter of each.
		Client: query.Get("client"), AccessClient: query.Get("client_app"),
		Origin: query.Get("origin"),
		// The status filter names codes and classes (4xx), which the service
		// reads so the console and the API accept the same spellings.
		Status: query.Get("status"), Cursor: query.Get("cursor"), Limit: limit,
	}, nil
}

func usageQuery(r *http.Request) (appactivity.UsageQuery, error) {
	query, err := logsQuery(r)
	if err != nil {
		return appactivity.UsageQuery{}, err
	}
	// The rollup takes one connection by name as before, and several with
	// repeated provider parameters, which is how the console totals its
	// paid connections in one read.
	providers := r.URL.Query()["provider"]
	filterProvider, providerList := usageProviders(query.Provider, providers)
	return appactivity.UsageQuery{
		SinceMs: query.SinceMs, UntilMs: query.UntilMs, Provider: filterProvider,
		Providers: providerList,
		Model:     query.Model, Credential: query.Credential, Account: query.Account, Surface: query.Surface,
		RouteProvider: r.URL.Query().Get("route_provider"), Client: query.Client,
		AccessClient: query.AccessClient, Origin: r.URL.Query().Get("origin"), Status: query.Status,
		GroupBy: r.URL.Query().Get("group_by"), Limit: query.Limit,
	}, nil
}

func usageProviders(single string, repeated []string) (string, []string) {
	if len(repeated) <= 1 {
		return single, nil
	}
	seen := make(map[string]bool, len(repeated))
	list := make([]string, 0, len(repeated))
	for _, provider := range repeated {
		if provider == "" || seen[provider] {
			continue
		}
		seen[provider] = true
		list = append(list, provider)
	}
	return "", list
}
