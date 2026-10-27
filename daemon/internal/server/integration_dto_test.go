package server

import (
	"encoding/json"
	"strings"
	"testing"

	appintegration "github.com/jonaskahn/relo/internal/application/integration"
)

// TestIntegrationReasonsReachTheConsole checks the shape the Agent Harness
// reads: the code names the cause, and the detail behind it rides alongside.
func TestIntegrationReasonsReachTheConsole(t *testing.T) {
	view := appintegration.IntegrationView{
		ID: "codex", Enabled: true, State: appintegration.StateDrifted,
		Reasons: []appintegration.Reason{
			{Code: appintegration.ReasonFileChanged, Path: "/home/op/.codex/relo-key.sh"},
			{Code: appintegration.ReasonModelsChanged, Added: 3, Removed: 1},
		},
	}
	encoded, err := json.Marshal(toIntegrationViewResponse(view))
	if err != nil {
		t.Fatalf("marshal the view: %v", err)
	}
	var document struct {
		Reasons []map[string]any `json:"reasons"`
	}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("unmarshal the view: %v", err)
	}
	if len(document.Reasons) != 2 {
		t.Fatalf("reasons = %+v, want two", document.Reasons)
	}
	file := document.Reasons[0]
	if file["code"] != appintegration.ReasonFileChanged || file["path"] != "/home/op/.codex/relo-key.sh" {
		t.Fatalf("first reason = %+v, want the changed file", file)
	}
	models := document.Reasons[1]
	if models["code"] != appintegration.ReasonModelsChanged || models["added"] != float64(3) || models["removed"] != float64(1) {
		t.Fatalf("second reason = %+v, want the model counts", models)
	}
}

// TestIntegrationWithoutReasonsOmitsTheField keeps a healthy agent payload
// from carrying an empty array the console would have to special-case.
func TestIntegrationWithoutReasonsOmitsTheField(t *testing.T) {
	encoded, err := json.Marshal(toIntegrationViewResponse(appintegration.IntegrationView{ID: "codex", Enabled: true}))
	if err != nil {
		t.Fatalf("marshal the view: %v", err)
	}
	if strings.Contains(string(encoded), "reasons") {
		t.Fatalf("encoded view = %s, want no reasons field", encoded)
	}
}
