package catalog

import (
	"slices"
	"testing"
)

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

func TestCleanAntigravityGeminiName(t *testing.T) {
	cases := []struct {
		name string
		id   string
		want string
	}{
		{"Gemini 3.8 Flash (Medium)", "gemini-3.8-flash", "Gemini 3.8 Flash"},
		{"Gemini 3.8 Flash (High)", "gemini-3.8-flash", "Gemini 3.8 Flash"},
		{"Gemini 3.8 Flash (Low)", "gemini-3.8-flash", "Gemini 3.8 Flash"},
		{"Gemini 3.5 Flash (Extra Low)", "gemini-3.5-flash", "Gemini 3.5 Flash"},
		{"Gemini 3.8 Flash", "gemini-3.8-flash", "Gemini 3.8 Flash"},
		{"GPT-OSS 120B (Medium)", "gpt-oss-120b-medium", "GPT-OSS 120B (Medium)"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := CleanAntigravityGeminiName(test.id, test.name); got != test.want {
				t.Fatalf("CleanAntigravityGeminiName(%q, %q) = %q, want %q", test.id, test.name, got, test.want)
			}
		})
	}
}

// TestAntigravityBandCollectsTheLadder pins that the vendor's three bands
// answer for the whole canonical ladder: the steps below low share the low
// band, the steps above high share the high one, and an effort the ladder does
// not name takes the middle band every collapsed model publishes.
func TestAntigravityBandCollectsTheLadder(t *testing.T) {
	cases := map[string]string{
		"none": AntigravityBandLow, "minimal": AntigravityBandLow, "low": AntigravityBandLow,
		"medium": AntigravityBandMedium, "": AntigravityBandMedium, "nonsense": AntigravityBandMedium,
		"high": AntigravityBandHigh, "xhigh": AntigravityBandHigh, "max": AntigravityBandHigh,
	}
	for effort, want := range cases {
		if got := AntigravityBand(effort); got != want {
			t.Errorf("AntigravityBand(%q) = %q, want %q", effort, got, want)
		}
	}
}

// TestSelectAntigravityVariantFallsBackToTheSelection pins the row a vendor
// wrote before the band names existed: it names the SKU to select on, so the
// selection alone still resolves it.
func TestSelectAntigravityVariantFallsBackToTheSelection(t *testing.T) {
	configs := []AntigravityVariant{{
		LogicalID: "gemini-3.8-flash", Selection: "high", WireModel: "gemini-3.8-flash-high",
	}}
	selected, ok := SelectAntigravityVariant(configs, "gemini-3.8-flash", "max")
	if !ok || selected.WireModel != "gemini-3.8-flash-high" {
		t.Fatalf("selected = %+v, %v, want the SKU its selection names", selected, ok)
	}
}

func TestAntigravityBandsAreOfferedLowestFirst(t *testing.T) {
	want := []string{AntigravityBandLow, AntigravityBandMedium, AntigravityBandHigh}
	if got := AntigravityBands(); !slices.Equal(got, want) {
		t.Fatalf("AntigravityBands() = %v, want %v", got, want)
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
