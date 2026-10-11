// Grouped Cursor model configurations.
package cursor

import (
	"sort"
	"strings"

	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
)

// Selection kinds name how one configuration is chosen inside its group.
const (
	SelectDefault = catalog.CursorSelectDefault
	SelectOff     = catalog.CursorSelectOff
	SelectToggle  = catalog.CursorSelectToggle
)

// Reasoning parameter ids never participate in lane identity: they select
// the member inside the lane instead.
var reasoningParamIDs = map[string]bool{
	"effort": true, "reasoning": true, "reasoning_effort": true,
	"thinking": true, "thinking_effort": true,
}

// Param is one Cursor model parameter pair.
type Param struct {
	ID    string
	Value string
}

// Config is one exact runnable Cursor configuration: the request tuple the
// relay sends for one selection inside a group.
type Config struct {
	Selection string
	Effort    string
	ModelID   string
	DetailsID string
	MaxMode   bool
	Params    []Param
}

// Variant is one provider-declared model variant before grouping.
type Variant struct {
	BaseID        string
	BaseName      string
	WireID        string
	Params        []Param
	MaxMode       bool
	DefaultMax    bool
	DefaultNonMax bool
	Images        bool
	Reasoning     bool
	Context       int64
}

// Group is one selectable lane: configurations that differ only in
// reasoning effort or thinking on/off.
type Group struct {
	ID      string
	Name    string
	MaxMode bool
	Context int64
	Images  bool
	Members []Config
	Default string
}

// laneParams returns the parameters that identify a lane.
func laneParams(params []Param) []Param {
	out := make([]Param, 0, len(params))
	for _, param := range params {
		if reasoningParamIDs[param.ID] {
			continue
		}
		out = append(out, param)
	}
	return out
}

// EffortOf maps one variant's parameters onto a selection: off, a canonical
// effort, toggle-on, or empty when the variant states no reasoning choice.
func EffortOf(params []Param) string {
	thinking := ""
	for _, param := range params {
		if param.ID == "thinking" {
			thinking = strings.ToLower(param.Value)
		}
	}
	if thinking == "false" || thinking == "off" || thinking == "none" {
		return SelectOff
	}
	for _, param := range params {
		switch param.ID {
		case "reasoning", "reasoning_effort", "thinking_effort", "effort":
			return normalizeEffort(param.Value)
		}
	}
	if thinking == "true" || thinking == "on" {
		return SelectToggle
	}
	return ""
}

func normalizeEffort(value string) string {
	lowered := strings.ToLower(strings.TrimSpace(value))
	if lowered == "none" || lowered == "off" || lowered == "disabled" {
		return SelectOff
	}
	if lowered == "extra-high" {
		lowered = "xhigh"
	}
	name, ok := inference.NormalizeEffort(lowered)
	if !ok {
		return ""
	}
	return name
}

// GroupVariants collapses provider-declared variants into selectable lanes.
// Response order never affects the result: lanes sort by id, and members
// resolve deterministically by selection.
func GroupVariants(variants []Variant) []Group {
	lanes := map[string][]Variant{}
	for _, variant := range variants {
		lanes[laneKey(variant)] = append(lanes[laneKey(variant)], variant)
	}
	groups := make([]Group, 0, len(lanes))
	for _, members := range lanes {
		if group, ok := buildGroup(members); ok {
			groups = append(groups, group)
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	return groups
}

func laneKey(variant Variant) string {
	params := laneParams(variant.Params)
	sorted := append([]Param(nil), params...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].ID != sorted[j].ID {
			return sorted[i].ID < sorted[j].ID
		}
		return sorted[i].Value < sorted[j].Value
	})
	var key strings.Builder
	key.WriteString(variant.BaseID)
	key.WriteString("\x00")
	if variant.MaxMode {
		key.WriteString("max")
	}
	for _, param := range sorted {
		key.WriteString("\x00" + param.ID + "=" + param.Value)
	}
	return key.String()
}

func buildGroup(members []Variant) (Group, bool) {
	bySelection := map[string]Variant{}
	order := make([]string, 0, len(members))
	for _, member := range members {
		selection := EffortOf(member.Params)
		if selection == "" {
			selection = SelectDefault
		}
		previous, found := bySelection[selection]
		if !found {
			bySelection[selection] = member
			order = append(order, selection)
			continue
		}
		if variantRank(member) > variantRank(previous) {
			bySelection[selection] = member
		}
	}
	first := members[0]
	group := Group{
		ID:      laneID(first),
		Name:    first.BaseName,
		MaxMode: first.MaxMode,
		Images:  first.Images,
		Context: first.Context,
	}
	if group.ID == "" {
		group.ID = first.BaseID
	}
	if group.Name == "" {
		group.Name = group.ID
	}
	for _, selection := range order {
		member := bySelection[selection]
		effort := ""
		if selection != SelectDefault && selection != SelectOff && selection != SelectToggle {
			effort = selection
		}
		wire := member.WireID
		if wire == "" {
			wire = member.BaseID
		}
		group.Members = append(group.Members, Config{
			Selection: selection,
			Effort:    effort,
			ModelID:   member.BaseID,
			DetailsID: wire,
			MaxMode:   member.MaxMode,
			Params:    append([]Param(nil), member.Params...),
		})
		if member.Context > group.Context {
			group.Context = member.Context
		}
	}
	sort.Slice(group.Members, func(i, j int) bool { return group.Members[i].Selection < group.Members[j].Selection })
	group.Default = defaultSelection(bySelection)
	return group, len(group.Members) > 0
}

func variantRank(variant Variant) int {
	switch {
	case variant.DefaultNonMax || variant.DefaultMax:
		return 2
	default:
		return 1
	}
}

func defaultSelection(bySelection map[string]Variant) string {
	for selection, member := range bySelection {
		if member.DefaultNonMax || member.DefaultMax {
			return selection
		}
	}
	if _, found := bySelection[SelectOff]; found {
		return SelectOff
	}
	if _, found := bySelection[SelectDefault]; found {
		return SelectDefault
	}
	for selection := range bySelection {
		return selection
	}
	return SelectDefault
}

func laneID(variant Variant) string {
	parts := []string{variant.BaseID}
	params := laneParams(variant.Params)
	sorted := append([]Param(nil), params...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].ID != sorted[j].ID {
			return sorted[i].ID < sorted[j].ID
		}
		return sorted[i].Value < sorted[j].Value
	})
	for _, param := range sorted {
		id := sanitizeLanePart(param.ID)
		value := sanitizeLanePart(param.Value)
		if id == "" || value == "" || param.Value == "false" {
			continue
		}
		if param.Value == "true" {
			parts = append(parts, id)
		} else if param.ID == "context" {
			parts = append(parts, value)
		} else {
			parts = append(parts, id+"-"+value)
		}
	}
	if variant.MaxMode {
		parts = append(parts, "max")
	}
	return strings.Join(parts, "-")
}

func sanitizeLanePart(value string) string {
	lowered := strings.ToLower(value)
	var cleaned strings.Builder
	for _, char := range lowered {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			cleaned.WriteRune(char)
		default:
			if cleaned.Len() > 0 && !strings.HasSuffix(cleaned.String(), "-") {
				cleaned.WriteString("-")
			}
		}
	}
	return strings.Trim(cleaned.String(), "-")
}

// Select returns the configuration for one effort request: empty means the
// group default, "off" means thinking off, anything else is a canonical
// effort. It reports false when no member serves the request.
func (g Group) Select(effort string) (Config, bool) {
	want := effort
	if want == "" {
		want = g.Default
	}
	if want == "" {
		want = SelectDefault
	}
	for _, member := range g.Members {
		if member.Selection == want {
			return member, true
		}
	}
	return Config{}, false
}
