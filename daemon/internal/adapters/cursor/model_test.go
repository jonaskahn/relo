package cursor

import (
	"testing"
)

func TestEffortMapping(t *testing.T) {
	cases := []struct {
		params []Param
		want   string
	}{
		{[]Param{{ID: "effort", Value: "high"}}, "high"},
		{[]Param{{ID: "reasoning_effort", Value: "Low"}}, "low"},
		{[]Param{{ID: "thinking", Value: "false"}}, SelectOff},
		{[]Param{{ID: "thinking", Value: "true"}}, SelectToggle},
		{[]Param{{ID: "thinking_effort", Value: "extra-high"}}, "xhigh"},
		{[]Param{{ID: "effort", Value: "nonsense"}}, ""},
		{[]Param{{ID: "context", Value: "1m"}}, ""},
		{nil, ""},
	}
	for _, tc := range cases {
		if got := EffortOf(tc.params); got != tc.want {
			t.Errorf("EffortOf(%v) = %q, want %q", tc.params, got, tc.want)
		}
	}
}

func TestGroupingSeparatesLanes(t *testing.T) {
	variants := []Variant{
		{BaseID: "sonnet", BaseName: "Sonnet", WireID: "sonnet-low", Params: []Param{{ID: "effort", Value: "low"}}},
		{BaseID: "sonnet", BaseName: "Sonnet", WireID: "sonnet-high", Params: []Param{{ID: "effort", Value: "high"}}, DefaultNonMax: true},
		{BaseID: "sonnet", BaseName: "Sonnet", WireID: "sonnet-max", Params: []Param{{ID: "effort", Value: "high"}}, MaxMode: true},
		{BaseID: "sonnet", BaseName: "Sonnet", WireID: "sonnet-fast", Params: []Param{{ID: "fast", Value: "true"}, {ID: "effort", Value: "high"}}},
	}
	groups := GroupVariants(variants)
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3 lanes", len(groups))
	}
	standard := findGroup(t, groups, "sonnet")
	if len(standard.Members) != 2 {
		t.Fatalf("standard members = %d, want low and high", len(standard.Members))
	}
	if standard.Default != "high" {
		t.Fatalf("default = %q, want the flagged high member", standard.Default)
	}
	selected, ok := standard.Select("low")
	if !ok || selected.DetailsID != "sonnet-low" {
		t.Fatalf("Select(low) = %+v %v", selected, ok)
	}
	if _, ok := standard.Select("max"); ok {
		t.Fatal("Select(max) succeeded without a max member")
	}
}

func TestGroupingIgnoresOrder(t *testing.T) {
	members := []Variant{
		{BaseID: "opus", BaseName: "Opus", WireID: "opus-a", Params: []Param{{ID: "effort", Value: "low"}, {ID: "context", Value: "1m"}}},
		{BaseID: "opus", BaseName: "Opus", WireID: "opus-b", Params: []Param{{ID: "context", Value: "1m"}, {ID: "effort", Value: "high"}}},
	}
	forward := GroupVariants(members)
	backward := GroupVariants([]Variant{members[1], members[0]})
	if len(forward) != 1 || len(backward) != 1 || forward[0].ID != backward[0].ID {
		t.Fatalf("forward = %+v backward = %+v, want one stable lane", forward, backward)
	}
	if len(forward[0].Members) != 2 {
		t.Fatalf("members = %+v, want both efforts", forward[0].Members)
	}
}

func TestOffNeedsAMapping(t *testing.T) {
	groups := GroupVariants([]Variant{
		{BaseID: "sonnet", BaseName: "Sonnet", WireID: "sonnet-on", Params: []Param{{ID: "thinking", Value: "true"}}},
	})
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	if _, ok := groups[0].Select(SelectOff); ok {
		t.Fatal("Select(off) succeeded without an off mapping")
	}
	toggle, ok := groups[0].Select(SelectToggle)
	if !ok || toggle.DetailsID != "sonnet-on" {
		t.Fatalf("Select(toggle) = %+v %v", toggle, ok)
	}
}

func findGroup(t *testing.T, groups []Group, id string) Group {
	t.Helper()
	for _, group := range groups {
		if group.ID == id {
			return group
		}
	}
	t.Fatalf("no group %q in %+v", id, groups)
	return Group{}
}
