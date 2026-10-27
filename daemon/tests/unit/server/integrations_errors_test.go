package server_test

import (
	"net/http"
	"strings"
	"testing"

	appintegration "github.com/jonaskahn/relo/internal/application/integration"
	"github.com/jonaskahn/relo/internal/server"
)

func TestIntegrationListReportsAnUnavailableStore(t *testing.T) {
	h := newHarness(t)
	served := h.newServerWithOptions(h.cfg, nil, func(options *server.Options) {
		options.Integrations = appintegration.NewService(appintegration.ServiceOptions{})
	})

	response := h.managementOn(served, http.MethodGet, "/api/v1/integrations", adminToken, nil)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("list integrations = %d, want 500 (%s)", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "internal_error") {
		t.Fatalf("list integrations body = %s, want the internal error code", response.Body.String())
	}
	if !strings.Contains(h.logBuffer.String(), "integration request failed") {
		t.Fatalf("list integrations log = %q, want the failure logged", h.logBuffer.String())
	}
}

func TestIntegrationActionErrorsReachTheConsole(t *testing.T) {
	h := newHarness(t)
	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "enable", path: "/api/v1/integrations/missing/enable"},
		{name: "rotate", path: "/api/v1/integrations/missing/rotate"},
		{name: "restart", path: "/api/v1/integrations/missing/restart"},
		{name: "context", path: "/api/v1/integrations/missing/context", body: `{"enabled":true}`},
		{name: "repair", path: "/api/v1/integrations/missing/repair"},
		{name: "restore", path: "/api/v1/integrations/missing/restore"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := h.management(http.MethodPost, test.path, adminToken, strings.NewReader(test.body))
			if response.Code != http.StatusNotFound {
				t.Fatalf("%s = %d, want 404 (%s)", test.name, response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), "not_found") {
				t.Fatalf("%s body = %s, want the not_found code", test.name, response.Body.String())
			}
		})
	}
}
