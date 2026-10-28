package server

import (
	"net/http"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
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
