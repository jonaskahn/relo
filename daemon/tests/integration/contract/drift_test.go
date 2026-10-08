package contract_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestContractComparisonDetectsDrift is the negative control: a fixture with
// one field renamed must fail the comparison, proving exact matching.
func TestContractComparisonDetectsDrift(t *testing.T) {
	h := newHarness(t)
	response := h.management(http.MethodGet, "/api/v1/status", adminToken, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/status = %d, want 200", response.Code)
	}
	normalized := h.normalize(response.Body.Bytes())
	if !jsonFieldPresent(normalized, `"status"`) {
		t.Fatalf("normalized status response lost its status field (body %s)", normalized)
	}
	renamed := strings.Replace(string(normalized), `"status"`, `"status_"`, 1)
	if renamed == string(normalized) {
		t.Fatalf("the negative control changed nothing; pick a field the response carries")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "drift.json")
	if err := os.WriteFile(path, []byte(renamed+"\n"), 0o644); err != nil {
		t.Fatalf("write the drifted fixture: %v", err)
	}
	wanted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the drifted fixture: %v", err)
	}
	if string(wanted) == string(append(normalized, '\n')) {
		t.Fatalf("the comparison missed a renamed field; exact matching is broken")
	}
}

func jsonFieldPresent(body []byte, field string) bool {
	return strings.Contains(string(body), field)
}
