package inference

import (
	"errors"
	"strings"
	"testing"
)

func TestParseEffortAcceptsTheVocabulary(t *testing.T) {
	level := func(name string) EffortSelection {
		return EffortSelection{Kind: EffortLevel, Level: name}
	}
	cases := []struct {
		in   string
		want EffortSelection
	}{
		{"", EffortSelection{Kind: EffortAuto}},
		{"  auto ", EffortSelection{Kind: EffortAuto, Raw: "auto"}},
		{"default", EffortSelection{Kind: EffortDefault, Raw: "default"}},
		{"adaptive", EffortSelection{Kind: EffortAuto, Raw: "adaptive"}},
		{"none", EffortSelection{Kind: EffortOff, Raw: "none"}},
		{"off", EffortSelection{Kind: EffortOff, Raw: "off"}},
		{"disabled", EffortSelection{Kind: EffortOff, Raw: "disabled"}},
		{"disable", EffortSelection{Kind: EffortOff, Raw: "disable"}},
		{"on", EffortSelection{Kind: EffortOn, Raw: "on"}},
		{"enabled", EffortSelection{Kind: EffortOn, Raw: "enabled"}},
		{"enable", EffortSelection{Kind: EffortOn, Raw: "enable"}},
		{"minimal", level("minimal")},
		{"MIN", level("minimal")},
		{"\tlow\n", level("low")},
		{"medium", level("medium")},
		{"High", level("high")},
		{"xhigh", level("xhigh")},
		{"x-high", level("xhigh")},
		{"extra-high", level("xhigh")},
		{"extra_high", level("xhigh")},
		{"extra high", level("xhigh")},
		{"max", level("max")},
		{"maximum", level("max")},
		{"highest", level("max")},
		{"ultra", level("max")},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseEffort(tc.in)
			if err != nil {
				t.Fatalf("ParseEffort(%q) error = %v", tc.in, err)
			}
			if got.Kind != tc.want.Kind || got.Level != tc.want.Level {
				t.Fatalf("ParseEffort(%q) = %+v, want kind %q level %q", tc.in, got, tc.want.Kind, tc.want.Level)
			}
			if got.Raw != strings.TrimSpace(tc.in) {
				t.Fatalf("ParseEffort(%q) raw = %q, want the trimmed client value", tc.in, got.Raw)
			}
		})
	}
}

func TestParseEffortRejectsUnknownValues(t *testing.T) {
	_, err := ParseEffort(" invent ")
	if !errors.Is(err, ErrUnknownEffort) {
		t.Fatalf("ParseEffort() error = %v, want ErrUnknownEffort", err)
	}
	message := err.Error()
	for _, want := range []string{`unknown reasoning effort "invent"`, "auto", "xhigh", "on", "off"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error = %q, want %q", message, want)
		}
	}
}

func TestResolveEffortDoesNotRejectRecognizedValues(t *testing.T) {
	yes := true
	no := false
	cases := []struct {
		name     string
		request  string
		ladder   []string
		toggle   bool
		supports *bool
		action   EffortAction
		level    string
	}{
		{name: "auto keeps no new control", request: "auto", ladder: []string{"low", "high"}, supports: &yes, action: EffortOmit},
		{name: "empty keeps no new control", request: "", ladder: []string{"low", "high"}, supports: &yes, action: EffortOmit},
		{name: "default on a ladder omits the effort", request: "default", ladder: []string{"low", "high"}, supports: &yes, action: EffortOmit},
		{name: "default on an unknown ladder omits the effort", request: "default", supports: &yes, action: EffortOmit},
		{name: "default on a toggle turns thinking off", request: "default", ladder: []string{}, toggle: true, supports: &yes, action: EffortDisable},
		{name: "off on a toggle turns thinking off", request: "off", ladder: []string{}, toggle: true, supports: &yes, action: EffortDisable},
		{name: "off uses an exact off rung", request: "off", ladder: []string{"none", "low"}, supports: &yes, action: EffortSend, level: "none"},
		{name: "off uses the lowest compressible rung", request: "OFF", ladder: []string{"low", "high"}, supports: &yes, action: EffortSend, level: "low"},
		{name: "on enables a toggle", request: "on", ladder: []string{}, toggle: true, supports: &yes, action: EffortEnable},
		{name: "a level enables a toggle", request: "high", ladder: []string{}, toggle: true, supports: &yes, action: EffortEnable},
		{name: "on a ladder uses the provider default", request: "enabled", ladder: []string{"low", "high"}, supports: &yes, action: EffortOmit},
		{name: "an exact level keeps its rung", request: "low", ladder: []string{"low", "high"}, supports: &yes, action: EffortSend, level: "low"},
		{name: "an absent level rises to the nearest stronger rung", request: "medium", ladder: []string{"low", "high"}, supports: &yes, action: EffortSend, level: "high"},
		{name: "a level above the ladder uses the highest rung", request: "xhigh", ladder: []string{"low", "high"}, supports: &yes, action: EffortSend, level: "high"},
		{name: "a level below the ladder rises to the lowest rung", request: "minimal", ladder: []string{"low", "high"}, supports: &yes, action: EffortSend, level: "low"},
		{name: "an absent level rises in a shifted ladder", request: "low", ladder: []string{"medium", "high"}, supports: &yes, action: EffortSend, level: "medium"},
		{name: "an unstated ladder passes the canonical level", request: "ULTRA", supports: &yes, action: EffortSend, level: "max"},
		{name: "an explicit empty ladder omits the effort", request: "high", ladder: []string{}, supports: &yes, action: EffortOmit},
		{name: "a non-reasoning model drops the control", request: "high", ladder: []string{"low", "high"}, supports: &no, action: EffortOmit},
		{name: "a non-reasoning model also drops off", request: "none", ladder: []string{"low", "high"}, supports: &no, action: EffortOmit},
		{name: "an unstated capability keeps the ladder rule", request: "medium", ladder: []string{"low", "high"}, action: EffortSend, level: "high"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			selection, err := ParseEffort(tc.request)
			if err != nil {
				t.Fatalf("ParseEffort() error = %v", err)
			}
			got := ResolveEffort(selection, tc.ladder, tc.toggle, tc.supports)
			if got.Action != tc.action || got.Level != tc.level {
				t.Fatalf("ResolveEffort(%q) = %+v, want action %q level %q", tc.request, got, tc.action, tc.level)
			}
		})
	}
}

func TestResolveEffortMarksCoercion(t *testing.T) {
	decision := ResolveEffort(EffortSelection{Kind: EffortLevel, Level: "medium", Raw: "medium"}, []string{"low", "high"}, false, nil)
	if !decision.Coerced || decision.Effective != "high" {
		t.Fatalf("ResolveEffort() = %+v, want a coercion to high", decision)
	}
	decision = ResolveEffort(EffortSelection{Kind: EffortLevel, Level: "high", Raw: "high"}, []string{"low", "high"}, false, nil)
	if decision.Coerced {
		t.Fatalf("ResolveEffort() = %+v, want no coercion", decision)
	}
}
