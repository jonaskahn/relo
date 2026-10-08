package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAppearanceAndLanguagePatchesAreIsolated(t *testing.T) {
	h := newHarness(t)
	retention := h.management(http.MethodPut, "/api/v1/settings/retention", adminToken,
		strings.NewReader(`{"usage_days":7,"max_events":42,"max_bytes":1024}`))
	if retention.Code != http.StatusOK {
		t.Fatalf("initial retention: %d %s", retention.Code, retention.Body.String())
	}
	chosen := h.management(http.MethodPatch, "/api/v1/settings/language", adminToken, strings.NewReader(`{"language":"de"}`))
	if chosen.Code != http.StatusOK {
		t.Fatalf("initial language: %d %s", chosen.Code, chosen.Body.String())
	}
	changed := h.management(http.MethodPatch, "/api/v1/settings/appearance", adminToken, strings.NewReader(`{"accent":"blue","theme":"dark","quota_display":"remaining"}`))
	if changed.Code != http.StatusOK {
		t.Fatalf("appearance: %d %s", changed.Code, changed.Body.String())
	}
	language := h.management(http.MethodPatch, "/api/v1/settings/language", adminToken, strings.NewReader(`{"language":"auto"}`))
	if language.Code != http.StatusOK {
		t.Fatalf("language: %d %s", language.Code, language.Body.String())
	}
	read := h.management(http.MethodGet, "/api/v1/settings", adminToken, nil)
	var settings struct {
		Language  string `json:"language"`
		Retention struct {
			UsageDays int `json:"usage_days"`
			MaxEvents int `json:"max_events"`
		} `json:"retention"`
		Appearance struct {
			Theme        string `json:"theme"`
			Accent       string `json:"accent"`
			QuotaDisplay string `json:"quota_display"`
		} `json:"appearance"`
	}
	if err := json.Unmarshal(read.Body.Bytes(), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Language != "auto" || settings.Retention.UsageDays != 7 || settings.Retention.MaxEvents != 42 ||
		settings.Appearance.Theme != "dark" || settings.Appearance.Accent != "blue" ||
		settings.Appearance.QuotaDisplay != "remaining" {
		t.Fatalf("settings = %+v", settings)
	}
	invalid := h.management(http.MethodPatch, "/api/v1/settings/appearance", adminToken, strings.NewReader(`{"accent":"magenta"}`))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid accent: %d %s", invalid.Code, invalid.Body.String())
	}
	badQuota := h.management(http.MethodPatch, "/api/v1/settings/appearance", adminToken, strings.NewReader(`{"quota_display":"left"}`))
	if badQuota.Code != http.StatusBadRequest {
		t.Fatalf("invalid quota display: %d %s", badQuota.Code, badQuota.Body.String())
	}
}
