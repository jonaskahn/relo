// Reasoning effort vocabulary and model-aware resolution.
package inference

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUnknownEffort reports a value outside the reasoning effort vocabulary.
var ErrUnknownEffort = errors.New("unknown reasoning effort")

const effortVocabulary = "auto, none, minimal, low, medium, high, xhigh, max, ultra, on, or off"

// UnknownEffortError names the value a client sent that Relo does not
// recognize.
type UnknownEffortError struct {
	Value string
}

func (e *UnknownEffortError) Error() string {
	return fmt.Sprintf("unknown reasoning effort %q; use %s", e.Value, effortVocabulary)
}

// Unwrap makes errors.Is(err, ErrUnknownEffort) report every unknown value.
func (e *UnknownEffortError) Unwrap() error {
	return ErrUnknownEffort
}

// EffortKind names the meaning of one client reasoning value.
type EffortKind string

const (
	// EffortAuto leaves the provider's reasoning default alone.
	EffortAuto EffortKind = "auto"
	// EffortDefault asks for the model's default. A toggle-only model reads
	// it as thinking off; every model with a ladder reads it as omitted.
	EffortDefault EffortKind = "default"
	// EffortOff asks for thinking off.
	EffortOff EffortKind = "off"
	// EffortOn asks for thinking on without an intensity.
	EffortOn EffortKind = "on"
	// EffortLevel names one concrete effort on the canonical ladder.
	EffortLevel EffortKind = "level"
)

// EffortSelection is one parsed client reasoning value.
type EffortSelection struct {
	Kind  EffortKind
	Level string
	Raw   string
}

// EffortAction names what an attempt sends for its reasoning control.
type EffortAction string

const (
	// EffortOmit sends no new reasoning control.
	EffortOmit EffortAction = "omit"
	// EffortDisable turns thinking off.
	EffortDisable EffortAction = "disable"
	// EffortEnable turns thinking on without an intensity.
	EffortEnable EffortAction = "enable"
	// EffortSend sends one concrete level.
	EffortSend EffortAction = "send"
)

// EffortDecision is the model-aware result of one effort request.
type EffortDecision struct {
	Action    EffortAction
	Level     string
	Requested string
	Effective string
	Reason    string
	Coerced   bool
}

// ParseEffort validates a client reasoning value and normalizes its aliases.
// It owns the auto/default/on/off vocabulary; NormalizeEffort stays limited
// to concrete levels.
func ParseEffort(value string) (EffortSelection, error) {
	trimmed := strings.TrimSpace(value)
	lowered := strings.ToLower(trimmed)
	switch lowered {
	case "", "auto", "adaptive":
		return EffortSelection{Kind: EffortAuto, Raw: trimmed}, nil
	case "default":
		return EffortSelection{Kind: EffortDefault, Raw: trimmed}, nil
	case "on", "enabled", "enable":
		return EffortSelection{Kind: EffortOn, Raw: trimmed}, nil
	case "none", "off", "disabled", "disable":
		return EffortSelection{Kind: EffortOff, Raw: trimmed}, nil
	}
	if level, ok := NormalizeEffort(trimmed); ok {
		return EffortSelection{Kind: EffortLevel, Level: level, Raw: trimmed}, nil
	}
	return EffortSelection{}, &UnknownEffortError{Value: trimmed}
}

// ResolveEffort maps one parsed request onto the selected model's reasoning
// capability. supportsReasoning nil leaves the capability unstated.
func ResolveEffort(selection EffortSelection, ladder []string, toggle bool, supportsReasoning *bool) EffortDecision {
	decision := EffortDecision{Requested: selection.Raw}
	if selection.Kind == EffortLevel && decision.Requested == "" {
		decision.Requested = selection.Level
	}
	if supportsReasoning != nil && !*supportsReasoning {
		decision.Action = EffortOmit
		decision.Reason = "non-reasoning model"
		decision.Coerced = selection.Kind != EffortAuto
		return decision
	}
	if ToggleOnly(ladder, toggle) {
		return resolveToggleOnly(decision, selection.Kind)
	}
	switch selection.Kind {
	case EffortAuto:
		return omitEffort(decision, "auto")
	case EffortDefault:
		return omitEffort(decision, "provider default")
	case EffortOff:
		return resolveOff(decision, ladder, toggle)
	case EffortOn:
		return omitEffort(decision, "provider default")
	case EffortLevel:
		return resolveLevel(decision, selection.Level, ladder)
	}
	return omitEffort(decision, "unhandled selection")
}

func resolveToggleOnly(decision EffortDecision, kind EffortKind) EffortDecision {
	switch kind {
	case EffortAuto:
		return omitEffort(decision, "auto")
	case EffortOff:
		return disableEffort(decision, "toggle-only model", false)
	case EffortDefault:
		return disableEffort(decision, "toggle-only model", true)
	case EffortOn:
		return enableEffort(decision, "toggle-only model", false)
	default:
		return enableEffort(decision, "toggle-only model", true)
	}
}

func resolveOff(decision EffortDecision, ladder []string, toggle bool) EffortDecision {
	level, off := offLevel(ladder, toggle)
	if off {
		return disableEffort(decision, "toggle off", true)
	}
	if level == "" {
		return omitEffort(decision, "no off rung")
	}
	return sendEffort(decision, level, "off rung", effortRank(level) != effortRank("none"))
}

func resolveLevel(decision EffortDecision, level string, ladder []string) EffortDecision {
	if ladder == nil {
		return sendEffort(decision, level, "unstated ladder", false)
	}
	if len(ladder) == 0 {
		return omitEffort(decision, "empty ladder")
	}
	chosen, matched, found := levelOnLadder(level, ladder)
	if !found {
		return omitEffort(decision, "no usable rung")
	}
	reason := "ladder"
	if matched {
		reason = "exact"
	}
	return sendEffort(decision, chosen, reason, !matched)
}

func sendEffort(decision EffortDecision, level, reason string, coerced bool) EffortDecision {
	decision.Action = EffortSend
	decision.Level = level
	decision.Effective = level
	decision.Reason = reason
	decision.Coerced = coerced
	return decision
}

// ResolveEffortString parses and resolves one string. A codec that receives
// an unknown value from an internal caller omits the control rather than
// panicking; client input is rejected at the relay entry point first.
func ResolveEffortString(value string, ladder []string, toggle bool, supportsReasoning *bool) EffortDecision {
	selection, err := ParseEffort(value)
	if err != nil {
		return EffortDecision{Action: EffortOmit, Requested: value, Reason: "unknown effort"}
	}
	return ResolveEffort(selection, ladder, toggle, supportsReasoning)
}

func omitEffort(decision EffortDecision, reason string) EffortDecision {
	decision.Action = EffortOmit
	decision.Effective = ""
	decision.Reason = reason
	return decision
}

func disableEffort(decision EffortDecision, reason string, coerced bool) EffortDecision {
	decision.Action = EffortDisable
	decision.Effective = "none"
	decision.Reason = reason
	decision.Coerced = coerced
	return decision
}

func enableEffort(decision EffortDecision, reason string, coerced bool) EffortDecision {
	decision.Action = EffortEnable
	decision.Effective = "on"
	decision.Reason = reason
	decision.Coerced = coerced
	return decision
}

func offLevel(ladder []string, toggle bool) (string, bool) {
	if ladder == nil {
		return "none", false
	}
	if match, found := listedEffort(ladder, "none"); found {
		return match, false
	}
	if match, found := listedEffort(ladder, "minimal"); found {
		return match, false
	}
	if lowest, found := extremeEffort(ladder, false); found {
		return lowest, false
	}
	if toggle {
		return "", true
	}
	return "", false
}

func levelOnLadder(level string, ladder []string) (string, bool, bool) {
	if match, found := listedEffort(ladder, level); found {
		return match, true, true
	}
	if match, found := ceilingEffort(ladder, effortRank(level)); found {
		return match, false, true
	}
	if highest, found := extremeEffort(ladder, true); found {
		return highest, false, true
	}
	return "", false, false
}
