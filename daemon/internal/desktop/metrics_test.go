package desktop

import (
	"testing"
)

// row builds a rollup row with every counter the menu rows fold in.
func row(key string, requests, errors, input, output, cacheRead, cacheWrite, costMicros float64) UsageRow {
	return UsageRow{
		Key: key, Requests: requests, Errors: errors,
		InputTokens: input, OutputTokens: output,
		CacheReadTokens: cacheRead, CacheWriteTokens: cacheWrite,
		CostMicros: costMicros,
	}
}

func TestTotalsOf(t *testing.T) {
	rows := []UsageRow{
		row("a", 3, 1, 100, 50, 10, 5, 2000),
		row("b", 2, 0, 200, 100, 20, 10, 3000),
	}
	got := totalsOf(rows)
	if got.requests != 5 {
		t.Errorf("requests = %v, want 5", got.requests)
	}
	if got.errors != 1 {
		t.Errorf("errors = %v, want 1", got.errors)
	}
	// Tokens are input + output + cache reads + cache writes.
	if got.tokens != 495 {
		t.Errorf("tokens = %v, want 495", got.tokens)
	}
	if got.spend != 5000 {
		t.Errorf("spend = %v, want 5000", got.spend)
	}
}

func TestUsageTotalsValue(t *testing.T) {
	totals := usageTotals{requests: 7, errors: 2, tokens: 70, spend: 700}
	tests := []struct {
		metric metric
		want   float64
	}{
		{metricRequests, 7},
		{metricTokens, 70},
		{metricSpend, 700},
	}
	for _, tt := range tests {
		if got := totals.value(tt.metric); got != tt.want {
			t.Errorf("value(%v) = %v, want %v", tt.metric, got, tt.want)
		}
	}
}

func TestFormatMetric(t *testing.T) {
	tests := []struct {
		value  float64
		metric metric
		want   string
	}{
		{1234, metricRequests, "1,234"},
		{999, metricRequests, "999"},
		{-12, metricRequests, "-12"},
		{999, metricTokens, "999"},
		{1500, metricTokens, "1.5k"},
		{25000, metricTokens, "25k"},
		{2_500_000, metricTokens, "2.50M"},
		{25_000_000, metricTokens, "25.0M"},
		{1_234_000_000, metricTokens, "1.2B"},
		{12_340_000_000, metricTokens, "12.3B"},
		{1_000_000_000_000, metricTokens, "1.0T"},
		{1_234_000_000_000, metricTokens, "1.2T"},
		{1_234_000_000_000_000, metricTokens, "1.2Qa"},
		{1_234_000_000_000_000_000, metricTokens, "1.2Qi"},
		{0, metricSpend, "$0.00"},
		{50, metricSpend, "<$0.01"},
		{4_200_000, metricSpend, "$4.20"},
	}
	for _, tt := range tests {
		if got := formatMetric(tt.value, tt.metric); got != tt.want {
			t.Errorf("formatMetric(%v, %v) = %q, want %q", tt.value, tt.metric, got, tt.want)
		}
	}
}

// TestMetricsContractMatchesTheConsole is the metrics contract, executable:
// the same fixture the console's own contract test uses
// (console/src/lib/metric-contract.test.ts). Whichever side drifts, the two
// tests name the number that has to mean the same thing on every surface.
func TestMetricsContractMatchesTheConsole(t *testing.T) {
	rows := []UsageRow{
		{Key: "openai", Requests: 3, Errors: 1, InputTokens: 31, OutputTokens: 13,
			CacheReadTokens: 5, CacheWriteTokens: 7, CostMicros: 42},
		{Key: "anthropic", Requests: 1, Errors: 0, InputTokens: 2, OutputTokens: 3,
			CacheReadTokens: 0, CacheWriteTokens: 1, CostMicros: 0},
	}
	total := totalsOf(rows)
	if total.tokens != 62 {
		t.Errorf("tokens = %v, want 62", total.tokens)
	}
	if total.spend != 42 {
		t.Errorf("spend = %v, want 42", total.spend)
	}
	if total.requests != 4 {
		t.Errorf("requests = %v, want 4", total.requests)
	}
}
