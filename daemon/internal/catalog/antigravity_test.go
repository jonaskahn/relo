package catalog

import "testing"

func TestMatchesPrefix(t *testing.T) {
	cases := []struct {
		name     string
		id       string
		prefixes []string
		want     bool
	}{
		{"empty keeps everything", "gpt-oss-120b-medium", nil, true},
		{"claude matches", "claude-sonnet-4-6", []string{"claude", "gemini"}, true},
		{"gemini matches case-insensitively", "GEMINI-3.8-flash", []string{"claude", "gemini"}, true},
		{"other family drops", "gpt-oss-120b-medium", []string{"claude", "gemini"}, false},
		{"tab helper drops", "tab_flash_lite_preview", []string{"claude", "gemini"}, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := MatchesPrefix(test.id, test.prefixes); got != test.want {
				t.Fatalf("MatchesPrefix(%q) = %v, want %v", test.id, got, test.want)
			}
		})
	}
}

func TestRetiredAntigravity(t *testing.T) {
	if !RetiredAntigravity("gemini-3.5-flash-low") {
		t.Fatal("RetiredAntigravity = false, want the superseded SKU retired")
	}
	if RetiredAntigravity("gemini-3.8-flash-low") {
		t.Fatal("RetiredAntigravity = true, want the live SKU kept")
	}
}

func TestSelectAntigravityVariant(t *testing.T) {
	configs := []AntigravityVariant{
		{LogicalID: "gemini-3.8-flash", Selection: "low", Effort: "low", WireModel: "gemini-3.8-flash-low"},
		{LogicalID: "gemini-3.8-flash", Selection: "medium", Effort: "medium", Default: true, WireModel: "gemini-3.8-flash-medium"},
		{LogicalID: "gemini-3.8-flash", Selection: "high", Effort: "high", WireModel: "gemini-3.8-flash-high"},
	}
	selected, ok := SelectAntigravityVariant(configs, "gemini-3.8-flash", "")
	if !ok || selected.WireModel != "gemini-3.8-flash-medium" {
		t.Fatalf("default = %+v, %v, want the medium SKU", selected, ok)
	}
	selected, ok = SelectAntigravityVariant(configs, "gemini-3.8-flash", "high")
	if !ok || selected.WireModel != "gemini-3.8-flash-high" {
		t.Fatalf("high = %+v, %v, want the high SKU", selected, ok)
	}
	for _, effort := range []string{"none", "minimal"} {
		if selected, ok := SelectAntigravityVariant(configs, "gemini-3.8-flash", effort); !ok || selected.WireModel != "gemini-3.8-flash-low" {
			t.Fatalf("%s = %+v, %v, want the low SKU", effort, selected, ok)
		}
	}
	for _, effort := range []string{"xhigh", "max"} {
		if selected, ok := SelectAntigravityVariant(configs, "gemini-3.8-flash", effort); !ok || selected.WireModel != "gemini-3.8-flash-high" {
			t.Fatalf("%s = %+v, %v, want the high SKU", effort, selected, ok)
		}
	}
	if _, ok := SelectAntigravityVariant(configs, "gemini-3.8-flash", "nonsense"); ok {
		t.Fatal("unknown effort resolved, want no selection")
	}
	if _, ok := SelectAntigravityVariant(configs, "missing", ""); ok {
		t.Fatal("missing group resolved, want no selection")
	}
}

func TestEncodeDecodeAntigravityConfig(t *testing.T) {
	variant := AntigravityVariant{
		LogicalID: "gemini-3.8-flash", Selection: "high", Effort: "high",
		WireModel: "gemini-3.8-flash-high", ModelEnum: "ENUM", MaxOutput: 65536,
		Context: 1048576, Reasoning: true,
	}
	decoded, ok := DecodeAntigravityConfig(EncodeAntigravityConfig(variant))
	if !ok || decoded.WireModel != variant.WireModel || decoded.LogicalID != variant.LogicalID {
		t.Fatalf("round trip = %+v, %v, want the stored tuple back", decoded, ok)
	}
	if _, ok := DecodeAntigravityConfig(ModelRequestConfig{APIFormat: FormatCursorAgent, WireConfig: "{}"}); ok {
		t.Fatal("foreign codec decoded, want no selection")
	}
}
