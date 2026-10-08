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
