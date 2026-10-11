// Cursor account roster discovery: decoding the model-list RPCs.
package cursor

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jonaskahn/relo/internal/catalog"
)

// Field numbers verified against the pinned Cursor reference schemas.
const (
	usableModelsField = 1

	detailModelID      = 1
	detailThinking     = 2
	detailDisplayModel = 3
	detailDisplayName  = 4
	detailDisplayShort = 5
	detailAliases      = 6
	detailMaxMode      = 7

	richName          = 1
	richDefaultOn     = 2
	richChatOnly      = 4
	richSupportsAgent = 5
	richThinking      = 9
	richImages        = 10
	richAutoMax       = 12
	richAutoExtended  = 13
	richContext       = 15
	richContextMax    = 16
	richClientName    = 17
	richServerName    = 18
	richDefinitions   = 29
	richVariants      = 30
	richLegacySlugs   = 36
	richAliases       = 37
	richTagline       = 39
	richRetention     = 46

	definitionID   = 1
	definitionType = 4
	booleanValues  = 1
	enumValues     = 2
	valueName      = 1
	enumBlocked    = 4
	booleanBlocked = 6

	variantParams        = 1
	variantDisplayName   = 2
	variantMaxMode       = 3
	variantDefaultMax    = 4
	variantDefaultNonMax = 5
	variantString        = 9
	variantLegacySlug    = 11

	paramID    = 1
	paramValue = 2

	availableLongContext = 2
	availableParameters  = 5
	availableMarkdown    = 7
	availableEffortModes = 12
)

// UsableModel is one GetUsableModels row.
type UsableModel struct {
	ID        string
	Name      string
	MaxMode   bool
	Reasoning bool
}

// RichVariant is one AvailableModels variant before grouping.
type RichVariant struct {
	Params        []Param
	WireID        string
	MaxMode       bool
	DefaultMax    bool
	DefaultNonMax bool
}

// RichModel is one AvailableModels definition before grouping.
type RichModel struct {
	Name         string
	DisplayName  string
	DefaultOn    bool
	Thinking     bool
	Images       bool
	ImagesStated bool
	Context      int64
	ContextMax   int64
	AutoMax      int64
	AutoExtended int64
	LegacySlugs  []string
	Aliases      []string
	Variants     []RichVariant
	Blocked      map[string]map[string]bool
}

// BuildUsableModelsRequest encodes an empty GetUsableModels request.
func BuildUsableModelsRequest() []byte {
	return []byte{}
}

// BuildAvailableModelsRequest encodes the AvailableModels request the
// reference discovery sends.
func BuildAvailableModelsRequest() []byte {
	var out Writer
	out.Bool(availableLongContext, true)
	out.Bool(availableParameters, true)
	out.Bool(availableMarkdown, true)
	out.Bool(availableEffortModes, true)
	return out.BytesOut()
}

// UnframeUnary returns the message inside a unary Connect envelope, or the
// body unchanged when it is already raw protobuf. A valid message never
// starts with a zero tag, so a leading zero byte reliably marks a frame.
func UnframeUnary(body []byte) []byte {
	if len(body) < framePrefix || (body[0] != frameData && body[0] != frameEnd) {
		return body
	}
	size := uint64(body[1])<<24 | uint64(body[2])<<16 | uint64(body[3])<<8 | uint64(body[4])
	if size > uint64(MaxProtoBytes) || uint64(len(body)-framePrefix) < size {
		return body
	}
	return body[framePrefix : framePrefix+size]
}

// ParseUsableModels decodes a GetUsableModels response into runnable rows.
func ParseUsableModels(body []byte) ([]UsableModel, error) {
	fields, err := Parse(UnframeUnary(body))
	if err != nil {
		return nil, err
	}
	nested, err := Nested(fields, usableModelsField)
	if err != nil {
		return nil, err
	}
	models := make([]UsableModel, 0, len(nested))
	for _, row := range nested {
		id := strings.TrimSpace(FirstString(row, detailModelID))
		if id == "" {
			continue
		}
		models = append(models, UsableModel{
			ID:        id,
			Name:      usableDisplayName(row, id),
			MaxMode:   Flag(row, detailMaxMode),
			Reasoning: present(row, detailThinking),
		})
	}
	return models, nil
}

func usableDisplayName(row []Field, fallback string) string {
	for _, number := range []int{detailDisplayName, detailDisplayShort, detailDisplayModel} {
		if name := strings.TrimSpace(FirstString(row, number)); name != "" {
			return name
		}
	}
	if aliases := Strings(row, detailAliases); len(aliases) > 0 && strings.TrimSpace(aliases[0]) != "" {
		return strings.TrimSpace(aliases[0])
	}
	return fallback
}

// ParseAvailableModels decodes an AvailableModels response into definitions.
func ParseAvailableModels(body []byte) ([]RichModel, error) {
	fields, err := Parse(UnframeUnary(body))
	if err != nil {
		return nil, err
	}
	nested, err := Nested(fields, 2)
	if err != nil {
		return nil, err
	}
	models := make([]RichModel, 0, len(nested))
	for _, row := range nested {
		model, ok, err := parseRichModel(row)
		if err != nil {
			return nil, err
		}
		if ok {
			models = append(models, model)
		}
	}
	return models, nil
}

func parseRichModel(row []Field) (RichModel, bool, error) {
	name := strings.TrimSpace(FirstString(row, richName))
	if name == "" || Flag(row, richChatOnly) || Flag(row, richRetention) {
		return RichModel{}, false, nil
	}
	if present(row, richSupportsAgent) && !Flag(row, richSupportsAgent) {
		return RichModel{}, false, nil
	}
	model := RichModel{
		Name:         name,
		DisplayName:  richDisplayName(row, name),
		DefaultOn:    Flag(row, richDefaultOn),
		Thinking:     Flag(row, richThinking),
		ImagesStated: present(row, richImages),
		Images:       Flag(row, richImages),
		Context:      int64(Uint(row, richContext)),
		ContextMax:   int64(Uint(row, richContextMax)),
		AutoMax:      int64(Uint(row, richAutoMax)),
		AutoExtended: int64(Uint(row, richAutoExtended)),
		LegacySlugs:  Strings(row, richLegacySlugs),
		Aliases:      Strings(row, richAliases),
		Blocked:      map[string]map[string]bool{},
	}
	definitions, err := Nested(row, richDefinitions)
	if err != nil {
		return RichModel{}, false, err
	}
	for _, definition := range definitions {
		collectBlockedParams(model.Blocked, definition)
	}
	variants, err := Nested(row, richVariants)
	if err != nil {
		return RichModel{}, false, err
	}
	for _, variant := range variants {
		model.Variants = append(model.Variants, parseRichVariant(variant))
	}
	return model, true, nil
}

func richDisplayName(row []Field, fallback string) string {
	for _, number := range []int{richClientName, richServerName} {
		if name := strings.TrimSpace(FirstString(row, number)); name != "" {
			return name
		}
	}
	return fallback
}

func collectBlockedParams(blocked map[string]map[string]bool, definition []Field) {
	id := strings.TrimSpace(FirstString(definition, definitionID))
	if id == "" {
		return
	}
	types, err := Nested(definition, definitionType)
	if err != nil {
		return
	}
	for _, paramType := range types {
		markBlockedValues(blocked, id, paramType, booleanValues, booleanBlocked)
		markBlockedValues(blocked, id, paramType, enumValues, enumBlocked)
	}
}

func markBlockedValues(blocked map[string]map[string]bool, id string, paramType []Field, container, blockedField int) {
	containers, err := Nested(paramType, container)
	if err != nil {
		return
	}
	for _, group := range containers {
		values, err := Nested(group, 1)
		if err != nil {
			continue
		}
		for _, value := range values {
			if !Flag(value, blockedField) {
				continue
			}
			name := strings.TrimSpace(FirstString(value, valueName))
			if name == "" {
				continue
			}
			if blocked[id] == nil {
				blocked[id] = map[string]bool{}
			}
			blocked[id][name] = true
		}
	}
}

func parseRichVariant(row []Field) RichVariant {
	variant := RichVariant{
		WireID:        strings.TrimSpace(FirstString(row, variantLegacySlug)),
		MaxMode:       Flag(row, variantMaxMode),
		DefaultMax:    Flag(row, variantDefaultMax),
		DefaultNonMax: Flag(row, variantDefaultNonMax),
	}
	params, err := Nested(row, variantParams)
	if err != nil {
		return variant
	}
	for _, param := range params {
		id := strings.TrimSpace(FirstString(param, paramID))
		if id == "" {
			continue
		}
		variant.Params = append(variant.Params, Param{ID: id, Value: strings.TrimSpace(FirstString(param, paramValue))})
	}
	if variant.WireID == "" {
		variant.WireID = strings.TrimSpace(FirstString(row, variantString))
	}
	return variant
}

func present(fields []Field, number int) bool {
	for _, field := range fields {
		if field.Number == number {
			return true
		}
	}
	return false
}

// JoinModels filters rich definitions by usable entitlement and returns the
// variants to group. When usableOK is false the usable RPC failed, so no
// entitlement filtering applies; an empty usable list is authoritative.
func JoinModels(usable []UsableModel, usableOK bool, rich []RichModel) []Variant {
	entitled := map[string]bool{}
	for _, model := range usable {
		entitled[model.ID] = true
	}
	variants := make([]Variant, 0, len(rich)*2)
	for _, model := range rich {
		variants = append(variants, joinModel(entitled, usableOK, model)...)
	}
	return variants
}

func joinModel(entitled map[string]bool, usableOK bool, model RichModel) []Variant {
	if len(model.Variants) == 0 {
		if usableOK && !entitledAny(entitled, joinCandidates(model.Name, "", model)) {
			return nil
		}
		return []Variant{{
			BaseID: model.Name, BaseName: model.DisplayName, WireID: model.Name,
			Images: model.Images, Reasoning: model.Thinking, Context: richContextOf(model, false, ""),
		}}
	}
	seen := map[string]bool{}
	variants := make([]Variant, 0, len(model.Variants))
	for index, variant := range model.Variants {
		if blockedVariant(model.Blocked, variant) {
			continue
		}
		if usableOK && !entitledAny(entitled, variantCandidates(model.Name, variant.WireID, model.Aliases)) {
			continue
		}
		signature := variantSignature(model.Name, variant)
		if seen[signature] {
			continue
		}
		seen[signature] = true
		variants = append(variants, RichVariantToVariant(model, variant, index))
	}
	return variants
}

func joinCandidates(base, wire string, model RichModel) []string {
	candidates := []string{base}
	if wire != "" {
		candidates = append(candidates, wire)
	}
	candidates = append(candidates, model.LegacySlugs...)
	candidates = append(candidates, model.Aliases...)
	return candidates
}

// variantCandidates names the entitlement of one rich variant: its own wire
// id, the base model, and the model aliases. Sibling legacy slugs never
// entitle a variant on their own.
func variantCandidates(base, wire string, aliases []string) []string {
	candidates := []string{base}
	if wire != "" {
		candidates = append(candidates, wire)
	}
	return append(candidates, aliases...)
}

func entitledAny(entitled map[string]bool, candidates []string) bool {
	for _, candidate := range candidates {
		if candidate != "" && entitled[candidate] {
			return true
		}
	}
	return false
}

func blockedVariant(blocked map[string]map[string]bool, variant RichVariant) bool {
	for _, param := range variant.Params {
		if blocked[param.ID][param.Value] {
			return true
		}
	}
	return false
}

func variantSignature(base string, variant RichVariant) string {
	params := append([]Param(nil), variant.Params...)
	sort.Slice(params, func(i, j int) bool {
		if params[i].ID != params[j].ID {
			return params[i].ID < params[j].ID
		}
		return params[i].Value < params[j].Value
	})
	var key strings.Builder
	key.WriteString(base)
	if variant.MaxMode {
		key.WriteString("\x00max")
	}
	for _, param := range params {
		key.WriteString("\x00" + param.ID + "=" + param.Value)
	}
	return key.String()
}

// RichVariantToVariant converts one rich variant into a grouping variant,
// deriving the wire id and reasoning state the reference uses.
func RichVariantToVariant(model RichModel, variant RichVariant, index int) Variant {
	wire := variant.WireID
	if wire == "" {
		wire = fmt.Sprintf("%s@variant-%d", model.Name, index+1)
	}
	effort := EffortOf(variant.Params)
	return Variant{
		BaseID: model.Name, BaseName: model.DisplayName, WireID: wire,
		Params: variant.Params, MaxMode: variant.MaxMode,
		DefaultMax: variant.DefaultMax, DefaultNonMax: variant.DefaultNonMax,
		Images:    model.Images,
		Reasoning: variantReasoning(effort, variant.Params, model.Thinking),
		Context:   richContextOf(model, variant.MaxMode, contextParam(variant.Params)),
	}
}

func variantReasoning(effort string, params []Param, thinking bool) bool {
	switch effort {
	case SelectOff:
		return false
	case "":
		for _, param := range params {
			if param.ID == "thinking" && strings.ToLower(param.Value) == "true" {
				return true
			}
		}
		return thinking
	default:
		return true
	}
}

func contextParam(params []Param) string {
	for _, param := range params {
		if param.ID == "context" {
			return param.Value
		}
	}
	return ""
}

func richContextOf(model RichModel, maxMode bool, override string) int64 {
	if parsed := ParseContextValue(override); parsed > 0 {
		return parsed
	}
	if maxMode && model.ContextMax > 0 {
		return model.ContextMax
	}
	if model.Context > 0 {
		return model.Context
	}
	if model.AutoExtended > 0 {
		return model.AutoExtended
	}
	return model.AutoMax
}

// ParseContextValue parses a context parameter such as "1m" or "200k".
func ParseContextValue(value string) int64 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	multiplier := int64(1)
	if last := trimmed[len(trimmed)-1]; last == 'm' || last == 'M' || last == 'k' || last == 'K' {
		trimmed = trimmed[:len(trimmed)-1]
		if last == 'm' || last == 'M' {
			multiplier = 1000000
		} else {
			multiplier = 1000
		}
	}
	amount, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || amount <= 0 {
		return 0
	}
	return int64(amount * float64(multiplier))
}

// GroupsToListed converts grouped lanes into catalog rows, keeping the
// exact request tuple of every member for account-scoped selection.
func GroupsToListed(groups []Group) []catalog.Listed {
	listed := make([]catalog.Listed, 0, len(groups))
	for _, group := range groups {
		listed = append(listed, groupToListed(group))
	}
	return listed
}

func groupToListed(group Group) catalog.Listed {
	entry := catalog.Listed{ID: group.ID, Name: group.Name}
	if group.Context > 0 {
		context := group.Context
		entry.ContextWindow = &context
	}
	tools := true
	entry.SupportsTools = &tools
	if group.Images {
		vision := true
		entry.SupportsVision = &vision
	}
	efforts := make([]string, 0, len(group.Members))
	toggle := false
	reasoning := false
	for _, member := range group.Members {
		entry.Cursor = append(entry.Cursor, catalog.CursorVariant{
			GroupID: group.ID, Selection: member.Selection, Effort: member.Effort,
			ModelID: member.ModelID, DetailsID: member.DetailsID, MaxMode: member.MaxMode,
			Params: cursorParams(member.Params), Context: memberContext(member, group),
			Images: group.Images, Reasoning: memberReasoning(member),
		})
		if member.Effort != "" {
			efforts = append(efforts, member.Effort)
		}
		if member.Selection == SelectToggle {
			toggle = true
		}
		if memberReasoning(member) {
			reasoning = true
		}
	}
	sort.Strings(efforts)
	entry.ReasoningEfforts = efforts
	if toggle {
		latch := true
		entry.ReasoningToggle = &latch
	}
	if reasoning {
		stated := true
		entry.SupportsReasoning = &stated
	}
	return entry
}

func memberContext(member Config, group Group) int64 {
	for _, param := range member.Params {
		if param.ID == "context" {
			if parsed := ParseContextValue(param.Value); parsed > 0 {
				return parsed
			}
		}
	}
	return group.Context
}

func memberReasoning(member Config) bool {
	if member.Effort != "" {
		return true
	}
	return member.Selection == SelectToggle
}

func cursorParams(params []Param) []catalog.CursorParam {
	out := make([]catalog.CursorParam, 0, len(params))
	for _, param := range params {
		out = append(out, catalog.CursorParam{ID: param.ID, Value: param.Value})
	}
	return out
}
