package catalog

import (
	"slices"
	"testing"

	"github.com/jonaskahn/relo/internal/inference"
)

func selectConfigs() []CursorVariant {
	return []CursorVariant{
		{GroupID: "sonnet", Selection: "low", Effort: "low", ModelID: "sonnet", DetailsID: "sonnet-low", Default: true},
		{GroupID: "sonnet", Selection: "high", Effort: "high", ModelID: "sonnet", DetailsID: "sonnet-high"},
		{GroupID: "sonnet", Selection: "off", ModelID: "sonnet", DetailsID: "sonnet-off"},
		{GroupID: "sonnet-max", Selection: "high", Effort: "high", ModelID: "sonnet", DetailsID: "sonnet-max", MaxMode: true},
	}
}

func TestSelectConfig(t *testing.T) {
	configs := selectConfigs()
	selected, ok := SelectCursorVariant(configs, "sonnet", "")
	if !ok || selected.DetailsID != "sonnet-low" {
		t.Fatalf("Select() = %+v %v, want the default low member", selected, ok)
	}
	selected, ok = SelectCursorVariant(configs, "sonnet", "high")
	if !ok || selected.DetailsID != "sonnet-high" {
		t.Fatalf("Select(high) = %+v %v", selected, ok)
	}
	selected, ok = SelectCursorVariant(configs, "sonnet", "off")
	if !ok || selected.DetailsID != "sonnet-off" {
		t.Fatalf("Select(off) = %+v %v", selected, ok)
	}
	if _, ok := SelectCursorVariant(configs, "sonnet", "max"); ok {
		t.Fatal("Select(max) succeeded without a max member in the lane")
	}
	if _, ok := SelectCursorVariant(configs, "sonnet-max", "low"); ok {
		t.Fatal("Select(low) crossed into the max lane")
	}
	selected, ok = SelectCursorVariant(configs, "sonnet-max", "high")
	if !ok || !selected.MaxMode {
		t.Fatalf("Select(max high) = %+v %v", selected, ok)
	}
	if _, ok := SelectCursorVariant(configs, "missing", ""); ok {
		t.Fatal("Select() succeeded for an unknown group")
	}
}

func TestNormalizeSelection(t *testing.T) {
	for input, want := range map[string]string{"high": "high", "ultra": "max", "off": "off", "toggle": "toggle", "default": "default"} {
		got, ok := NormalizeCursorEffort(input)
		if !ok || got != want {
			t.Errorf("NormalizeCursorEffort(%q) = %q %v, want %q", input, got, ok, want)
		}
	}
	if _, ok := NormalizeCursorEffort("nonsense"); ok {
		t.Error("NormalizeSelection(nonsense) succeeded")
	}
}

func TestRequestEffort(t *testing.T) {
	if _, stated := RequestEffort(nil); stated {
		t.Fatal("RequestEffort(nil) reported a stated effort")
	}
	if effort, stated := RequestEffort(&inference.ReasoningConfig{Effort: "high"}); !stated || effort != "high" {
		t.Fatalf("RequestEffort() = %q %v", effort, stated)
	}
	if _, stated := RequestEffort(&inference.ReasoningConfig{}); stated {
		t.Fatal("RequestEffort() reported an unstated effort")
	}
}

func TestCursorConfigCoding(t *testing.T) {
	variant := CursorVariant{
		GroupID: "sonnet", Selection: "high", Effort: "high", Default: true,
		ModelID: "sonnet", DetailsID: "sonnet-high", MaxMode: true,
		Params:  []CursorParam{{ID: "effort", Value: "high"}},
		Context: 200000, Images: true, Reasoning: true,
	}
	stored := EncodeCursorConfig(variant)
	if stored.ModelID != "sonnet" || stored.Selection != "high" || !stored.Default {
		t.Fatalf("stored = %+v, want the shared identity kept", stored)
	}
	if stored.APIFormat != FormatCursorAgent {
		t.Fatalf("stored = %+v, want the owning codec named", stored)
	}
	decoded, ok := DecodeCursorConfig(stored)
	if !ok {
		t.Fatalf("stored = %+v, want the tuple back", stored)
	}
	if decoded.GroupID != variant.GroupID || decoded.Selection != variant.Selection ||
		decoded.Effort != variant.Effort || decoded.Default != variant.Default ||
		decoded.ModelID != variant.ModelID || decoded.DetailsID != variant.DetailsID ||
		decoded.MaxMode != variant.MaxMode || decoded.Context != variant.Context ||
		decoded.Images != variant.Images || decoded.Reasoning != variant.Reasoning ||
		!slices.Equal(decoded.Params, variant.Params) {
		t.Fatalf("decoded = %+v, want %+v", decoded, variant)
	}
	if _, ok := DecodeCursorConfig(ModelRequestConfig{APIFormat: FormatKiro, WireConfig: "{}"}); ok {
		t.Fatal("DecodeCursorConfig() read another codec's configuration")
	}
	if _, ok := DecodeCursorConfig(ModelRequestConfig{APIFormat: FormatCursorAgent, WireConfig: "{"}); ok {
		t.Fatal("DecodeCursorConfig() read a broken payload")
	}
	if _, ok := DecodeCursorConfig(ModelRequestConfig{APIFormat: FormatCursorAgent, WireConfig: "{}"}); ok {
		t.Fatal("DecodeCursorConfig() read a payload without the exact triple")
	}
}
