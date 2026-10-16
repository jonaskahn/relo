package google

import (
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/antigravity"
)

// TestAssistGeminiThinkingUsesTheSkuBudget pins the thinking shape Cloud Code
// Assist accepts. The endpoint takes a token budget on this transport, so the
// budget comes from the wire SKU the attempt selected rather than from the
// effort name the caller used.
func TestAssistGeminiThinkingUsesTheSkuBudget(t *testing.T) {
	cases := []struct {
		name   string
		budget *int
		want   int
	}{
		{name: "low", budget: new(1000), want: 1000},
		{name: "medium", budget: new(4000), want: 4000},
		{name: "high thinks open-ended", budget: new(-1), want: -1},
		{name: "an unread budget stays open-ended", budget: nil, want: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := assistGeminiThinking(antigravity.Variant{ThinkingBudget: tc.budget})
			if got.ThinkingLevel != "" {
				t.Fatalf("thinkingLevel = %q, want none on this transport", got.ThinkingLevel)
			}
			if got.ThinkingBudget == nil {
				t.Fatal("thinkingBudget is unset, want the vendor budget")
			}
			if *got.ThinkingBudget != tc.want {
				t.Fatalf("thinkingBudget = %d, want %d", *got.ThinkingBudget, tc.want)
			}
			if !got.IncludeThoughts {
				t.Fatal("includeThoughts = false, want true")
			}
		})
	}
}
