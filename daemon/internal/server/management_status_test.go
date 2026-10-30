package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
	"github.com/jonaskahn/relo/internal/catalog"
)

func TestServiceStatusMapsRecoverableFailures(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "callback port owner", err: &PortBusyError{}, status: http.StatusConflict, code: "callback_port_busy"},
		{name: "callback port busy", err: ErrCallbackPortBusy, status: http.StatusConflict, code: "callback_port_busy"},
		{name: "integration action", err: appintegration.ErrActionInProgress, status: http.StatusConflict, code: "action_in_progress"},
		{name: "retention action", err: activity.ErrRetentionBusy, status: http.StatusConflict, code: "action_in_progress"},
		{name: "secret store", err: appaccount.ErrNoSecretStore, status: http.StatusServiceUnavailable, code: "service_unavailable"},
		{name: "catalog", err: catalog.ErrNoCatalog, status: http.StatusServiceUnavailable, code: "service_unavailable"},
		{name: "rejected list", err: catalog.ErrListRejected, status: http.StatusBadGateway, code: "upstream_unavailable"},
		{name: "unsupported list", err: catalog.ErrListUnsupported, status: http.StatusBadGateway, code: "upstream_unavailable"},
		{name: "free consent", err: appcatalog.ErrFreeConnectionsDisabled, status: http.StatusForbidden, code: "free_connections_disabled"},
		{name: "duplicate free connection", err: appcatalog.ErrFreeConnectionExists, status: http.StatusConflict, code: "free_connection_exists"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, code := serviceStatus(test.err)
			if status != test.status || code != test.code {
				t.Fatalf("serviceStatus() = %d, %q; want %d, %q", status, code, test.status, test.code)
			}
		})
	}
}

func TestFreeConsentSurvivesARequestThatRanOutOfMembers(t *testing.T) {
	server := New(Options{})
	for _, anthropic := range []bool{false, true} {
		driver := &sendDriver{
			s: server, w: httptest.NewRecorder(), r: httptest.NewRequest(http.MethodPost, "/", nil),
			anthropic: anthropic, outcome: &requestOutcome{},
		}
		driver.last = appcatalog.ErrFreeConnectionsDisabled
		driver.run()
		if recorder := driver.w.(*httptest.ResponseRecorder); recorder.Code != http.StatusForbidden {
			t.Fatalf("anthropic=%v: status = %d, want the consent refusal", anthropic, recorder.Code)
		}
	}
}
