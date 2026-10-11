package cursor

import (
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
)

func selectConfigs() []catalog.CursorVariant {
	return []catalog.CursorVariant{
		{GroupID: "sonnet", Selection: "low", Effort: "low", ModelID: "sonnet", DetailsID: "sonnet-low", Default: true},
		{GroupID: "sonnet", Selection: "high", Effort: "high", ModelID: "sonnet", DetailsID: "sonnet-high"},
		{GroupID: "sonnet", Selection: "off", ModelID: "sonnet", DetailsID: "sonnet-off"},
		{GroupID: "sonnet-max", Selection: "high", Effort: "high", ModelID: "sonnet", DetailsID: "sonnet-max", MaxMode: true},
	}
}

func TestSelectConfig(t *testing.T) {
	configs := selectConfigs()
	selected, ok := SelectConfig(configs, "sonnet", "")
	if !ok || selected.DetailsID != "sonnet-low" {
		t.Fatalf("Select() = %+v %v, want the default low member", selected, ok)
	}
	selected, ok = SelectConfig(configs, "sonnet", "high")
	if !ok || selected.DetailsID != "sonnet-high" {
		t.Fatalf("Select(high) = %+v %v", selected, ok)
	}
	selected, ok = SelectConfig(configs, "sonnet", "off")
	if !ok || selected.DetailsID != "sonnet-off" {
		t.Fatalf("Select(off) = %+v %v", selected, ok)
	}
	if _, ok := SelectConfig(configs, "sonnet", "max"); ok {
		t.Fatal("Select(max) succeeded without a max member in the lane")
	}
	if _, ok := SelectConfig(configs, "sonnet-max", "low"); ok {
		t.Fatal("Select(low) crossed into the max lane")
	}
	selected, ok = SelectConfig(configs, "sonnet-max", "high")
	if !ok || !selected.MaxMode {
		t.Fatalf("Select(max high) = %+v %v", selected, ok)
	}
	if _, ok := SelectConfig(configs, "missing", ""); ok {
		t.Fatal("Select() succeeded for an unknown group")
	}
}

func TestNormalizeSelection(t *testing.T) {
	for input, want := range map[string]string{"high": "high", "ultra": "max", "off": "off", "toggle": "toggle", "default": "default"} {
		got, ok := NormalizeSelection(input)
		if !ok || got != want {
			t.Errorf("NormalizeSelection(%q) = %q %v, want %q", input, got, ok, want)
		}
	}
	if _, ok := NormalizeSelection("nonsense"); ok {
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
