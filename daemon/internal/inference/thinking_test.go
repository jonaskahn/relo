package inference

import "testing"

func TestToggleOnly(t *testing.T) {
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
