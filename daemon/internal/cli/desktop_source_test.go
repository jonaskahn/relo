// Desktop activity source tests: spend classification without a database.
package cli

import (
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
)

// TestPlanConnectionsAreNotSpend covers the spend definition the console
// applies: pay-as-you-go connections only, with plan-covered and local ones
// outside it.
func TestPlanConnectionsAreNotSpend(t *testing.T) {
	tests := []struct {
		name string
		row  sqlite.ProviderRow
		plan bool
	}{
		{"an opener template id", sqlite.ProviderRow{TemplateID: "opencode-go"}, true},
		{"an opener models.dev id", sqlite.ProviderRow{ModelsDevProviderID: "opencode-go"}, true},
		{"the opener Go gateway", sqlite.ProviderRow{BaseURL: "https://opencode.ai/zen/go/v1"}, true},
		{"the Free template", sqlite.ProviderRow{TemplateID: "opencode-free"}, true},
		{"the Free models.dev id", sqlite.ProviderRow{ModelsDevProviderID: "opencode"}, true},
		{"the Free gateway", sqlite.ProviderRow{BaseURL: "https://opencode.ai/zen/v1"}, true},
		{"a token-plan id", sqlite.ProviderRow{ID: "xiaomi-token-plan-sgp"}, true},
		{"a token-plan host", sqlite.ProviderRow{BaseURL: "https://token-plan-ams.xiaomimimo.com/v1"}, true},
		{"a sign-in account", sqlite.ProviderRow{Origin: "signin"}, true},
		{"an OpenAI key", sqlite.ProviderRow{TemplateID: "openai", BaseURL: "https://api.openai.com/v1"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := planConnection(tt.row); got != tt.plan {
				t.Fatalf("planConnection(%+v) = %v, want %v", tt.row, got, tt.plan)
			}
		})
	}
}

// TestPaidConnectionsAreTheOnlySpend covers the spend definition the
// console applies: pay-as-you-go connections only, with plan-covered and
// local ones outside it.
func TestPaidConnectionsAreTheOnlySpend(t *testing.T) {
	tests := []struct {
		name string
		row  sqlite.ProviderRow
		paid bool
	}{
		{"a key generated from models.dev", sqlite.ProviderRow{ID: "openai"}, true},
		{"an endpoint the operator added by hand", sqlite.ProviderRow{Origin: "custom"}, false},
		{"a plan-covered connection", sqlite.ProviderRow{TemplateID: "opencode-go"}, false},
		{"a sign-in account", sqlite.ProviderRow{Origin: "signin"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := paidConnection(tt.row); got != tt.paid {
				t.Fatalf("paidConnection(%+v) = %v, want %v", tt.row, got, tt.paid)
			}
		})
	}
}
