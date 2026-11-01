package server

import (
	"encoding/json"
	"strings"
	"testing"

	appcatalog "github.com/jonaskahn/relo/internal/application/catalog"
	"github.com/jonaskahn/relo/internal/catalog"
)

func TestCatalogDTOsCarryTheDashboardURL(t *testing.T) {
	const dashboard = "https://open-design.ai/cloud/dashboard"
	provider, err := json.Marshal(toProviderResponse(appcatalog.Provider{DashboardURL: dashboard}))
	if err != nil {
		t.Fatalf("marshal the provider: %v", err)
	}
	if !strings.Contains(string(provider), `"dashboard_url":"`+dashboard+`"`) {
		t.Fatalf("provider response lost the dashboard URL: %s", provider)
	}
	template, err := json.Marshal(toTemplateResponse(catalog.Template{DashboardURL: dashboard}))
	if err != nil {
		t.Fatalf("marshal the template: %v", err)
	}
	if !strings.Contains(string(template), `"dashboard_url":"`+dashboard+`"`) {
		t.Fatalf("template response lost the dashboard URL: %s", template)
	}
}
