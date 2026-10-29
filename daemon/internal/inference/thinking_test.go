package inference

import "testing"

func TestChooseThinking(t *testing.T) {
	deepseek := []string{"low", "high", "max"}
	xiaomi := []string{"low", "medium", "high"}
	glm := []string{"low", "high", "max"}
	withNone := []string{"none", "low", "medium", "high"}

	cases := []struct {
		name   string
		effort string
		ladder []string
		toggle bool
		level  string
		off    bool
	}{
		{name: "nil ladder passes the name", effort: "xhigh", level: "xhigh"},
		{name: "nil ladder keeps none", effort: "none", level: "none"},
		{name: "deepseek xhigh ceilings to max", effort: "xhigh", ladder: deepseek, level: "max"},
		{name: "deepseek medium ceilings to high", effort: "medium", ladder: deepseek, level: "high"},
		{name: "xiaomi xhigh ceilings to high", effort: "xhigh", ladder: xiaomi, level: "high"},
		{name: "exact level keeps its spelling", effort: "HIGH", ladder: deepseek, level: "high"},
		{name: "none stays none when listed", effort: "none", ladder: withNone, level: "none"},
		{name: "none uses minimal when that is the off level", effort: "none", ladder: []string{"minimal", "low", "high"}, level: "minimal"},
		{name: "none turns a toggle off", effort: "none", ladder: deepseek, toggle: true, off: true},
		{name: "none on a positive ladder uses the lowest", effort: "none", ladder: glm, level: "low"},
		{name: "a positive effort on an empty ladder sends nothing", effort: "high", ladder: []string{}},
		{name: "an unknown effort sends nothing", effort: "turbo", ladder: deepseek},
		{name: "ultra is max", effort: "ultra", ladder: deepseek, level: "max"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			level, off := ChooseThinking(tc.effort, tc.ladder, tc.toggle)
			if level != tc.level || off != tc.off {
				t.Fatalf("ChooseThinking() = %q, off %v, want %q, off %v", level, off, tc.level, tc.off)
			}
		})
	}
}

func TestToggleOnlyAcceptsEveryKnownEffort(t *testing.T) {
	if !ToggleOnly([]string{}, true) {
		t.Fatal("an empty ladder with a toggle is toggle-only")
	}
	for _, ladder := range [][]string{nil, {"low"}, {"none"}} {
		if ToggleOnly(ladder, true) {
			t.Fatalf("ToggleOnly(%v, true) = true, want false", ladder)
		}
	}
	if ToggleOnly([]string{}, false) {
		t.Fatal("an empty ladder without a toggle is not toggle-only")
	}

	for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"} {
		if !AcceptsEffort(effort, []string{}, true) {
			t.Fatalf("AcceptsEffort(%q) = false, want a toggle-only model to accept it", effort)
		}
	}
	if AcceptsEffort("turbo", []string{}, true) {
		t.Fatal("a toggle-only model accepted an unknown effort")
	}
	if AcceptsEffort("high", []string{}, false) {
		t.Fatal("an explicit empty non-toggle ladder accepted a positive effort")
	}
	if !AcceptsEffort("none", []string{}, false) {
		t.Fatal("an explicit empty non-toggle ladder rejected thinking off")
	}
	if !AcceptsEffort("high", nil, false) {
		t.Fatal("an unstated ladder rejected a positive effort")
	}
	if AcceptsEffort("max", []string{"low", "high"}, false) {
		t.Fatal("a stated ladder accepted an effort outside it")
	}
}

func TestClampBudget(t *testing.T) {
	min := int64(1024)
	max := int64(8000)
	if got := ClampBudget(16384, &min, &max); got != 8000 {
		t.Fatalf("ClampBudget() = %d, want the declared max", got)
	}
	if got := ClampBudget(100, &min, nil); got != 1024 {
		t.Fatalf("ClampBudget() = %d, want the declared min", got)
	}
}

func TestLowerEffort(t *testing.T) {
	cases := []struct {
		name   string
		effort string
		lower  string
		found  bool
	}{
		{name: "xhigh steps down to high", effort: "xhigh", lower: "high", found: true},
		{name: "minimal steps down to none", effort: "minimal", lower: "none", found: true},
		{name: "none has no step below", effort: "none"},
		{name: "an unknown effort has no step below", effort: "turbo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lower, found := LowerEffort(tc.effort)
			if lower != tc.lower || found != tc.found {
				t.Fatalf("LowerEffort(%q) = %q, %v, want %q, %v", tc.effort, lower, found, tc.lower, tc.found)
			}
		})
	}
}

func TestOrderEfforts(t *testing.T) {
	cases := []struct {
		name    string
		efforts []string
		want    []string
	}{
		{name: "a shuffled ladder reads lowest first", efforts: []string{"high", "low", "medium"}, want: []string{"low", "medium", "high"}},
		{name: "an unranked name follows the ranked ones", efforts: []string{"toggle", "medium"}, want: []string{"medium", "toggle"}},
		{name: "the input is left alone", efforts: []string{"high", "low"}, want: []string{"low", "high"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := OrderEfforts(tc.efforts)
			if len(got) != len(tc.want) {
				t.Fatalf("OrderEfforts(%v) = %v, want %v", tc.efforts, got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("OrderEfforts(%v) = %v, want %v", tc.efforts, got, tc.want)
				}
			}
		})
	}
	input := []string{"high", "low"}
	_ = OrderEfforts(input)
	if input[0] != "high" {
		t.Fatal("OrderEfforts reordered the caller's slice, want a copy")
	}
}
