// Desktop activity source tests: spend classification without a database.
package cli

import (
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/catalog"
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
		{"a coding-plan template", sqlite.ProviderRow{TemplateID: "alibaba-coding-plan-cn"}, true},
		{"a coding-plan models.dev id", sqlite.ProviderRow{ModelsDevProviderID: "zai-coding-plan"}, true},
		{"a step-plan id", sqlite.ProviderRow{ID: "stepfun-step-plan"}, true},
		{"a coding host", sqlite.ProviderRow{BaseURL: "https://coding-intl.dashscope.aliyuncs.com/v1"}, true},
		// The curated templates are sold as subscriptions with nothing in their
		// identifier or host saying so, which is what planTemplates is for.
		{"the OpenDesign plan template", sqlite.ProviderRow{TemplateID: "open-design"}, true},
		{"the OpenCode Go plan template", sqlite.ProviderRow{TemplateID: "opencode-go"}, true},
		{"a sign-in account", sqlite.ProviderRow{Origin: "signin"}, true},
		{"an OpenAI key", sqlite.ProviderRow{TemplateID: "openai", BaseURL: "https://api.openai.com/v1"}, false},
		// The pay-as-you-go twin of a Coding Plan stays metered: same vendor,
		// different product and host.
		{"the Alibaba pay-as-you-go template", sqlite.ProviderRow{TemplateID: "alibaba", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"}, false},
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
		{"a coding plan", sqlite.ProviderRow{TemplateID: "alibaba-coding-plan-cn"}, false},
		{"the OpenDesign plan template", sqlite.ProviderRow{TemplateID: "open-design"}, false},
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

// TestConnectionKindResolvesTheFallback covers the branches connectionKind
// takes when no curated template names the kind: a custom origin reads as
// local, and everything else reads as a key.
func TestConnectionKindResolvesTheFallback(t *testing.T) {
	if got := connectionKind(sqlite.ProviderRow{Origin: string(catalog.OriginCustom)}); got != catalog.KindLocal {
		t.Fatalf("connectionKind(custom) = %v, want local", got)
	}
	if got := connectionKind(sqlite.ProviderRow{Origin: "signin"}); got != catalog.KindKey {
		t.Fatalf("connectionKind(signin) = %v, want key", got)
	}
}

// TestPlanPricedConnectionRejectsAnUnparseableURL covers the error branch:
// a malformed base URL never reads as a plan.
func TestPlanPricedConnectionRejectsAnUnparseableURL(t *testing.T) {
	row := sqlite.ProviderRow{BaseURL: "://not-a-url"}
	if planPricedConnection(row) {
		t.Fatalf("planPricedConnection(%q) = true, want false", row.BaseURL)
	}
}

// TestEveryPlanTemplateIsAPlan keeps the curated list and the rule that reads
// it in step: a template named in planTemplates must classify as a plan, so a
// template added to one and not the other fails here rather than in the tray.
func TestEveryPlanTemplateIsAPlan(t *testing.T) {
	if len(planTemplates) == 0 {
		t.Fatal("planTemplates names no template")
	}
	for _, id := range planTemplates {
		row := sqlite.ProviderRow{TemplateID: id}
		if !planConnection(row) {
			t.Errorf("planConnection(%q) = false, want a plan", id)
		}
		if paidConnection(row) {
			t.Errorf("paidConnection(%q) = true, want no spend", id)
		}
	}
}
