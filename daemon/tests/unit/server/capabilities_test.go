package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCapabilityWritesReachTheCatalog(t *testing.T) {
	harness := newHarness(t)
	patched := harness.management(http.MethodPatch, "/api/v1/models/openai/gpt-4o", adminToken,
		strings.NewReader(`{"tools":true}`))
	if patched.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", patched.Code, patched.Body.String())
	}
	var model struct {
		Capabilities struct {
			Tools *bool `json:"tools"`
		} `json:"capabilities"`
		Override struct {
			Tools     *bool `json:"tools"`
			Reasoning *bool `json:"reasoning"`
		} `json:"capability_override"`
	}
	if err := json.Unmarshal(patched.Body.Bytes(), &model); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if model.Override.Tools == nil || !*model.Override.Tools || model.Capabilities.Tools == nil || !*model.Capabilities.Tools {
		t.Fatalf("tools = capabilities %#v override %#v", model.Capabilities, model.Override)
	}

	bulk := harness.management(http.MethodPost, "/api/v1/connections/openai/models/capabilities", adminToken,
		strings.NewReader(`{"model_ids":["gpt-4o"],"reasoning":false}`))
	if bulk.Code != http.StatusOK {
		t.Fatalf("bulk status = %d, body = %s", bulk.Code, bulk.Body.String())
	}
	got := harness.management(http.MethodGet, "/api/v1/models/openai/gpt-4o", adminToken, nil)
	if got.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", got.Code, got.Body.String())
	}
	if err := json.Unmarshal(got.Body.Bytes(), &model); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if model.Override.Tools == nil || !*model.Override.Tools || model.Override.Reasoning == nil || *model.Override.Reasoning {
		t.Fatalf("override = %#v, want tools on and reasoning off", model.Override)
	}
}
