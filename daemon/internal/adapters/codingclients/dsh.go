// DeepSeek Harness config: the Cordis patch entry Relo maintains.
//
// The harness reads $DSH_HOME/profiles/<name>/cordis.patch.yml, a list whose
// item id llm-pi-ai carries the provider map. Relo writes the web and the
// headless profile, and in each maintains only the relo entry inside
// config.providers, leaving every other provider and every other list item
// alone. The headless profile also gets a default model (dsh_default.go),
// because it has no flag to choose one. The key never touches the harness's files: the
// entry names apiKeyEnv, which the harness resolves per request with the
// launch environment winning, and Relo publishes RELO_DSH_API_KEY into that
// environment itself.
package codingclients

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// dshPatchItem is the Cordis list item carrying the provider map.
	dshPatchItem = "llm-pi-ai"
	// dshConfigPath is the path Cordis reads under one provider map.
	dshConfigPath = "config"
	// dshProvidersPath is the provider map under the patch entry's config.
	dshProvidersPath = "providers"
)

// parseDSH refuses a patch Relo cannot read. The document is a list at its
// top level, so the map-shaped helper every other client uses would refuse
// the very shape Cordis writes.
func parseDSH(content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	var document []any
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		return fmt.Errorf("%w (%v)", ErrUnrecognisedFile, err)
	}
	return nil
}

// MergeDSH returns the Cordis patch with Relo's provider entry in it.
func MergeDSH(existing, baseURL string, models []ModelRef) (string, error) {
	return mergeDSH(existing, baseURL, models, false)
}

// mergeDSH is MergeDSH, plus Relo's default model when the profile has no
// model flag of its own. The default is cut before the provider goes back in,
// so a second merge lays the patch out exactly as the first did.
func mergeDSH(existing, baseURL string, models []ModelRef, withDefault bool) (string, error) {
	address, err := requireBase(baseURL)
	if err != nil {
		return "", fmt.Errorf("dsh: %w", err)
	}
	if err := parseDSH(existing); err != nil {
		return "", err
	}
	lines, err := dropDSHRelo(splitLines(existing))
	if err == nil && withDefault {
		lines, err = cutDSHDefault(lines, false)
	}
	if err != nil {
		return "", err
	}
	lines, err = insertDSHRelo(lines, dshModel(address, models))
	if err != nil {
		return "", err
	}
	if withDefault {
		lines = appendDSHDefault(lines, models)
	}
	merged := joinLines(lines, lineEnding(existing))
	if err := parseDSH(merged); err != nil {
		return "", err
	}
	return merged, nil
}

// StripDSH removes Relo's provider entry and leaves the rest of the patch.
// The file stays on disk: a Cordis patch with no relo entry is still the
// operator's own document. A file in the settings shape an earlier Relo wrote
// is stripped in that shape, so a machine that wired the harness before the
// patch existed keeps no block behind.
func StripDSH(existing string) (string, error) {
	if strings.TrimSpace(existing) == "" {
		return "", nil
	}
	if !dshIsPatch(existing) {
		return stripYAMLMap(existing, "providers")
	}
	if err := parseDSH(existing); err != nil {
		return "", err
	}
	lines, err := dropDSHRelo(splitLines(existing))
	if err == nil {
		// A default the operator pointed at another provider stays theirs.
		lines, err = cutDSHDefault(lines, true)
	}
	if err != nil {
		return "", err
	}
	merged := joinLines(lines, lineEnding(existing))
	if strings.TrimSpace(merged) == "" {
		return "", nil
	}
	if err := parseDSH(merged); err != nil {
		return "", err
	}
	return merged, nil
}

// dshIsPatch reports whether a file is the Cordis patch list rather than the
// settings map an earlier Relo wrote under providers.
func dshIsPatch(content string) bool {
	for _, line := range splitLines(content) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "---") {
			continue
		}
		return strings.HasPrefix(trimmed, "-")
	}
	return false
}

// DSHHasRelo reports whether the patch already names a relo provider.
func DSHHasRelo(existing string) (bool, error) {
	if strings.TrimSpace(existing) == "" {
		return false, nil
	}
	if err := parseDSH(existing); err != nil {
		return false, err
	}
	document, err := dshDocument(existing)
	if err != nil {
		return false, err
	}
	for _, item := range document {
		row, _ := item.(map[string]any)
		if row == nil || row["id"] != dshPatchItem {
			continue
		}
		providers, err := dshProviders(row)
		if err != nil {
			return false, err
		}
		_, found := providers[reloProviderID]
		return found, nil
	}
	return false, nil
}

// DSHPointsAt reports whether the relo entry in the patch is the one Relo
// would write now. The rest of the document is the operator's own.
func DSHPointsAt(existing, baseURL string, models []ModelRef) bool {
	address, err := requireBase(baseURL)
	if err != nil {
		return false
	}
	wanted, found := dshRenderedRelo(address, models)
	if !found {
		return false
	}
	document, err := dshDocument(existing)
	if err != nil {
		return false
	}
	for _, item := range document {
		row, _ := item.(map[string]any)
		if row == nil || row["id"] != dshPatchItem {
			continue
		}
		providers, err := dshProviders(row)
		if err != nil {
			return false
		}
		entry, found := providers[reloProviderID]
		if !found {
			return false
		}
		return reflect.DeepEqual(entry, wanted)
	}
	return false
}

// dshRenderedRelo is the relo entry the patch Relo would write holds,
// parsed back out of the document it renders.
func dshRenderedRelo(address string, models []ModelRef) (any, bool) {
	item := "- " + strings.ReplaceAll(dshModel(address, models), "\n", "\n  ")
	var want []any
	if yaml.Unmarshal([]byte(item), &want) != nil || len(want) == 0 {
		return nil, false
	}
	row, _ := want[0].(map[string]any)
	providers, err := dshProviders(row)
	if err != nil {
		return nil, false
	}
	entry, found := providers[reloProviderID]
	return entry, found
}

func dshDocument(existing string) ([]any, error) {
	if strings.TrimSpace(existing) == "" {
		return nil, nil
	}
	var document []any
	if err := yaml.Unmarshal([]byte(existing), &document); err != nil {
		return nil, fmt.Errorf("%w (%v)", ErrUnrecognisedFile, err)
	}
	return document, nil
}

func dshProviders(item map[string]any) (map[string]any, error) {
	config, _ := item[dshConfigPath].(map[string]any)
	if item[dshConfigPath] != nil && config == nil {
		return nil, fmt.Errorf("%w (config is not a map)", ErrUnrecognisedFile)
	}
	if config == nil {
		return map[string]any{}, nil
	}
	raw, found := config[dshProvidersPath]
	if !found || raw == nil {
		return map[string]any{}, nil
	}
	providers, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w (providers is not a map)", ErrUnrecognisedFile)
	}
	return providers, nil
}

func dshModel(address string, models []ModelRef) string {
	var body strings.Builder
	body.WriteString("id: " + dshPatchItem + "\n")
	body.WriteString("config:\n")
	body.WriteString("  providers:\n")
	body.WriteString("    relo:\n")
	body.WriteString("      displayName: Relo\n")
	body.WriteString("      api: openai-completions\n")
	body.WriteString("      baseURL: " + yamlQuote(address) + "\n")
	body.WriteString("      apiKeyEnv: " + EnvKeyName(ClientDSH) + "\n")
	body.WriteString("      compat:\n")
	body.WriteString("        supportsDeveloperRole: false\n")
	body.WriteString("        maxTokensField: max_tokens\n")
	body.WriteString(dshModels(models))
	return body.String()
}

func dshModels(models []ModelRef) string {
	ordered := orderedModels(models)
	if len(ordered) == 0 {
		return "      models: []\n"
	}
	var body strings.Builder
	body.WriteString("      models:\n")
	for _, model := range ordered {
		body.WriteString("        - id: " + yamlQuote(model.ID) + "\n")
		body.WriteString("          name: " + yamlQuote(dshModelName(model)) + "\n")
		for _, field := range dshLimitFields(model) {
			body.WriteString(field + "\n")
		}
		body.WriteString(dshInput(model))
		if efforts := dshEfforts(model); efforts != "" {
			body.WriteString(efforts)
		}
		if compat := dshThinkingCompat(model); compat != "" {
			body.WriteString(compat)
		}
	}
	return body.String()
}

func dshModelName(model ModelRef) string {
	if label := namedConfigLabel(model); label != "" {
		return label
	}
	return model.ID
}

func dshLimitFields(model ModelRef) []string {
	fields := make([]string, 0, 2)
	if model.ContextWindow != nil {
		fields = append(fields, "          contextWindow: "+strconv.FormatInt(*model.ContextWindow, 10))
	}
	if model.MaxOutput != nil {
		fields = append(fields, "          maxTokens: "+strconv.FormatInt(*model.MaxOutput, 10))
	}
	return fields
}

func dshInput(model ModelRef) string {
	if model.Vision != nil && *model.Vision {
		return "          input: [text, image]\n"
	}
	return "          input: [text]\n"
}

// dshEffortLevel maps Relo's rung names onto the closed set of thinking levels
// the harness accepts as a reasoningEfforts key. The wire value stays Relo's
// own spelling: the harness sends whatever the entry names, and Relo
// normalizes upstream.
var dshEffortLevel = map[string]string{
	"none":    "off",
	"off":     "off",
	"minimal": "minimal",
	"low":     "low",
	"medium":  "medium",
	"high":    "high",
	"xhigh":   "xhigh",
	"max":     "max",
}

// dshEfforts renders the effort ladder a reasoning model accepts. A model
// that stated no ladder offers the shared list; one that stated none takes
// reasoningEfforts false, which is what turns the harness's effort menu off.
// A rung the harness cannot name is dropped rather than written: one unknown
// key fails the whole llm-pi-ai entry at boot, taking every Relo route with it.
func dshEfforts(model ModelRef) string {
	if model.Reasoning == nil || !*model.Reasoning {
		return "          reasoningEfforts: false\n"
	}
	efforts := model.ReasoningEfforts
	if efforts == nil {
		efforts = ReasoningEfforts()
	}
	ladder := dshEffortLadder(model, efforts)
	if len(ladder) == 0 {
		// The harness refuses both an empty ladder and one offering nothing
		// beyond off, so a model left with neither takes reasoning off.
		return "          reasoningEfforts: false\n"
	}
	var body strings.Builder
	body.WriteString("          reasoningEfforts:\n")
	for _, rung := range ladder {
		body.WriteString("            " + rung.level + ":")
		if rung.wire != "" {
			body.WriteString(" " + rung.wire)
		}
		body.WriteString("\n")
	}
	return body.String()
}

// dshRung is one line of a reasoningEfforts ladder: the thinking level the
// harness offers, and the wire value it sends to reach it.
type dshRung struct{ level, wire string }

// dshEffortLadder translates Relo's rungs into the levels the harness names,
// keeping the first spelling of a level so a model stating both none and off
// offers one rung. A toggle adds an empty off, which sends nothing at all.
func dshEffortLadder(model ModelRef, efforts []string) []dshRung {
	ladder := make([]dshRung, 0, len(efforts)+1)
	seen := make(map[string]bool, len(efforts)+1)
	for _, effort := range efforts {
		level, known := dshEffortLevel[effort]
		if !known || seen[level] {
			continue
		}
		seen[level] = true
		ladder = append(ladder, dshRung{level: level, wire: effort})
	}
	if model.ReasoningToggle != nil && *model.ReasoningToggle && !seen["off"] {
		ladder = append([]dshRung{{level: "off"}}, ladder...)
	}
	return ladder
}

// dshThinkingCompat marks the DeepSeek family for the harness's thinking
// format. A model outside that family carries no compat key of its own.
func dshThinkingCompat(model ModelRef) string {
	name := strings.ToLower(model.ID + " " + model.Upstream)
	if !strings.Contains(name, "deepseek") {
		return ""
	}
	return "          compat: { thinkingFormat: deepseek }\n"
}

func dropDSHRelo(lines []string) ([]string, error) {
	start, itemIndent, end, found, err := dshList(lines)
	if err != nil || !found {
		return lines, err
	}
	kept, _ := dshWalkItems(lines, dshSpan{start, end, itemIndent}, func(item []string) []string {
		return dshItemWithoutRelo(item, itemIndent)
	})
	return append(kept, lines[end:]...), nil
}

func dshItemIsPatch(lines []string) bool {
	for _, line := range lines {
		if value, ok := yamlScalar(line, "id"); ok && value == dshPatchItem {
			return true
		}
	}
	return false
}

func dshItemEnd(lines []string, index, end, itemIndent int) int {
	stop := index + 1
	for stop < end {
		if strings.TrimSpace(lines[stop]) == "" {
			stop++
			continue
		}
		if indentOf(lines[stop]) <= itemIndent && strings.HasPrefix(strings.TrimSpace(lines[stop]), "-") {
			break
		}
		if indentOf(lines[stop]) < itemIndent {
			break
		}
		stop++
	}
	for stop > index+1 && strings.TrimSpace(lines[stop-1]) == "" {
		stop--
	}
	return stop
}

// dshItemWithoutRelo drops the relo entry from one patch item, leaving its
// other providers and its own keys alone. The whole item goes when the relo
// entry was the only thing in it.
func dshItemWithoutRelo(item []string, itemIndent int) []string {
	body := dshItemBody(item, itemIndent)
	if len(body) == 0 {
		return nil
	}
	kept, _ := dshDropReloBlock(body, itemIndent+4)
	kept = dshWithoutEmptyProviders(kept)
	if strings.TrimSpace(strings.Join(kept, "\n")) == "" {
		return nil
	}
	head := strings.Repeat(" ", itemIndent+2)
	rendered := append([]string{item[0]}, dshIndent(kept, head)...)
	return rendered
}

// dshWithoutEmptyProviders drops a providers key the relo entry left empty,
// and the config key as well when that was all it held. A patch that kept
// either would replace the harness's own llm-pi-ai config with an empty one.
func dshWithoutEmptyProviders(body []string) []string {
	start, _, end, found, err := dshProvidersBlock(body)
	if err != nil || !found || dshHasContent(body[start+1:end]) {
		return body
	}
	body = append(append([]string{}, body[:start]...), body[end:]...)
	configStart, _, configEnd, found, err := dshConfigBlock(body)
	if err != nil || !found || dshHasContent(body[configStart+1:configEnd]) {
		return body
	}
	return append(append([]string{}, body[:configStart]...), body[configEnd:]...)
}

func dshHasContent(lines []string) bool {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return true
		}
	}
	return false
}

// dshItemBody is the item's lines past its own "- id: llm-pi-ai" line,
// indented back to zero so the nested blocks read as top-level keys.
func dshItemBody(item []string, itemIndent int) []string {
	if len(item) < 2 {
		return nil
	}
	body := make([]string, 0, len(item)-1)
	for _, line := range item[1:] {
		if strings.TrimSpace(line) == "" {
			body = append(body, "")
			continue
		}
		body = append(body, line[itemIndent+2:])
	}
	return body
}

func dshIndent(lines []string, pad string) []string {
	indented := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			indented = append(indented, "")
			continue
		}
		indented = append(indented, pad+line)
	}
	return indented
}

func insertDSHRelo(lines []string, item string) ([]string, error) {
	start, itemIndent, end, found, err := dshList(lines)
	if err != nil {
		return nil, err
	}
	rendered := dshRenderedItem(item, itemIndent)
	if !found {
		return appendBlock(lines, strings.Join(rendered, "\n")), nil
	}
	kept, merged := dshWalkItems(lines, dshSpan{start, end, itemIndent}, func(existing []string) []string {
		return dshItemWithRelo(existing, itemIndent, rendered)
	})
	if !merged {
		kept = append(kept, rendered...)
	}
	return append(kept, lines[end:]...), nil
}

// dshSpan is the region of a document the list walk covers: where the items
// begin, where they end, and the indent every item sits at.
type dshSpan struct {
	start, end int
	indent     int
}

// dshWalkItems copies a document's lines, handing every patch item to edit.
// It reports whether the document held one to edit, so a caller knows
// whether its own entry still has to be appended.
func dshWalkItems(lines []string, span dshSpan, edit func([]string) []string) ([]string, bool) {
	kept := append([]string{}, lines[:span.start]...)
	edited := false
	for index := span.start; index < span.end; {
		line := lines[index]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			kept = append(kept, line)
			index++
			continue
		}
		if indentOf(line) == span.indent && strings.HasPrefix(strings.TrimSpace(line), "-") {
			stop := dshItemEnd(lines, index, span.end, span.indent)
			if dshItemIsPatch(lines[index:stop]) {
				kept = append(kept, edit(lines[index:stop])...)
				edited = true
			} else {
				kept = append(kept, lines[index:stop]...)
			}
			index = stop
			continue
		}
		kept = append(kept, line)
		index++
	}
	return kept, edited
}

func dshRenderedItem(item string, itemIndent int) []string {
	rest := splitLines(strings.TrimRight(item, "\n"))
	rendered := make([]string, 0, len(rest))
	rendered = append(rendered, strings.Repeat(" ", itemIndent)+"- "+rest[0])
	pad := strings.Repeat(" ", itemIndent+2)
	for _, line := range rest[1:] {
		if strings.TrimSpace(line) == "" {
			rendered = append(rendered, line)
			continue
		}
		rendered = append(rendered, pad+line)
	}
	return rendered
}

// dshItemWithRelo puts the relo entry back into one patch item, keeping the
// other providers the operator listed in the same item. Only the providers
// subtree of the rendered item is grafted on: the item's own id and config
// keys are already in the copy Relo is editing.
func dshItemWithRelo(item []string, itemIndent int, rendered []string) []string {
	body := dshItemBody(item, itemIndent)
	kept, _ := dshDropReloBlock(body, itemIndent+4)
	if strings.TrimSpace(strings.Join(kept, "\n")) == "" {
		return rendered
	}
	entry := dshItemBody(rendered, itemIndent)
	providers := dshProvidersSubtree(entry)
	if providers == nil {
		return rendered
	}
	// The old providers block stays only for the keys Relo does not own; the
	// grafted block carries the relo entry alone, and the two are merged under
	// one providers key rather than written as two.
	kept = dshReplaceProviders(kept, providers)
	head := strings.Repeat(" ", itemIndent+2)
	return append([]string{item[0]}, dshIndent(kept, head)...)
}

// dshReplaceProviders grafts Relo's relo entry into the providers map the
// item already has, keeping every sibling provider the operator listed. An
// item with no providers map yet gains the whole block.
func dshReplaceProviders(kept, rendered []string) []string {
	configStart, configIndent, configEnd, found, err := dshConfigBlock(kept)
	if err != nil || !found {
		return append(kept, rendered...)
	}
	for index := configStart + 1; index < configEnd; index++ {
		if indentOf(kept[index]) != configIndent+2 || yamlKey(kept[index]) != dshProvidersPath {
			continue
		}
		end := dshBlockEnd(kept, index, configEnd, configIndent+2)
		grafted := append([]string{}, kept[:end]...)
		grafted = append(grafted, rendered[1:]...)
		return append(grafted, kept[end:]...)
	}
	block := dshIndent(rendered, strings.Repeat(" ", configIndent+2))
	grafted := append([]string{}, kept[:configEnd]...)
	grafted = append(grafted, block...)
	return append(grafted, kept[configEnd:]...)
}

// dshProvidersSubtree is the providers block of a rendered item, including
// its key line. A rendered item that carries no providers block yields nil.
func dshProvidersSubtree(body []string) []string {
	start, _, end, found, err := yamlMapBlock(body, dshConfigPath)
	if err != nil || !found {
		return nil
	}
	for index := start + 1; index < end; index++ {
		if indentOf(body[index]) == 2 && yamlKey(body[index]) == dshProvidersPath {
			stop := index + 1
			for stop < end {
				if strings.TrimSpace(body[stop]) == "" {
					stop++
					continue
				}
				if indentOf(body[stop]) <= 2 {
					break
				}
				stop++
			}
			return body[index:stop]
		}
	}
	return nil
}

// dshDropReloBlock removes the relo entry from one patch item's body and
// reports the indent its keys sit at. The body has been indented back to the
// document root, so the providers map is found under config at the nesting
// every Cordis patch item uses.
func dshDropReloBlock(body []string, childIndent int) ([]string, int) {
	start, providers, end, found, err := dshProvidersBlock(body)
	if err != nil || !found {
		return body, childIndent
	}
	for index := start + 1; index < end; index++ {
		stop, found := dshReloEnd(body, index, end, providers)
		if !found {
			continue
		}
		kept := append([]string{}, body[:index]...)
		return append(kept, body[stop:]...), providers
	}
	return body, childIndent
}

// dshConfigBlock finds the config map inside one item's body: the key at the
// item's indent, its children one level in, and where the map ends.
func dshConfigBlock(body []string) (start, indent, end int, found bool, err error) {
	for index, line := range body {
		if yamlKey(line) != dshConfigPath {
			continue
		}
		if inlineYAML(line, dshConfigPath) {
			return 0, 0, 0, false, fmt.Errorf("%w (config is not a block)", ErrUnrecognisedFile)
		}
		indent = indentOf(line)
		end = len(body)
		for next := index + 1; next < len(body); next++ {
			if strings.TrimSpace(body[next]) == "" {
				continue
			}
			if indentOf(body[next]) <= indent {
				end = next
				break
			}
		}
		return index, indent, end, true, nil
	}
	return 0, 0, 0, false, nil
}

// dshProvidersBlock finds the providers map inside one item's body and the
// indent its provider entries sit at.
func dshProvidersBlock(body []string) (start, child, end int, found bool, err error) {
	configStart, configIndent, configEnd, found, err := dshConfigBlock(body)
	if err != nil || !found {
		return 0, 0, 0, false, err
	}
	for index := configStart + 1; index < configEnd; index++ {
		if indentOf(body[index]) == configIndent+2 && yamlKey(body[index]) == dshProvidersPath {
			return index, indentOf(body[index]) + 2, dshBlockEnd(body, index, configEnd, indentOf(body[index])), true, nil
		}
	}
	return 0, 0, 0, false, nil
}

// dshBlockEnd is the first line at or above indent that ends the block the
// key on start opens.
func dshBlockEnd(lines []string, start, end, indent int) int {
	stop := start + 1
	for stop < end {
		if strings.TrimSpace(lines[stop]) == "" {
			stop++
			continue
		}
		if indentOf(lines[stop]) <= indent {
			break
		}
		stop++
	}
	for stop > start+1 && strings.TrimSpace(lines[stop-1]) == "" {
		stop--
	}
	return stop
}

func dshReloEnd(lines []string, index, end, child int) (int, bool) {
	line := lines[index]
	if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
		return 0, false
	}
	if indentOf(line) != child || yamlKey(line) != reloProviderID {
		return 0, false
	}
	stop := index + 1
	for stop < end {
		if strings.TrimSpace(lines[stop]) == "" {
			stop++
			continue
		}
		if indentOf(lines[stop]) <= child {
			break
		}
		stop++
	}
	for stop > index+1 && strings.TrimSpace(lines[stop-1]) == "" {
		stop--
	}
	return stop, true
}

// dshList finds the top-level document list the Cordis patch is. A patch
// whose top level is a map is not refused — it is a document Relo does not
// recognise, so the merge leaves its shape to the caller.
func dshList(lines []string) (start, itemIndent, end int, found bool, err error) {
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "---") {
			continue
		}
		if !strings.HasPrefix(trimmed, "-") {
			return 0, 0, 0, false, nil
		}
		end = len(lines)
		for next := index + 1; next < len(lines); next++ {
			if strings.TrimSpace(lines[next]) == "" {
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(lines[next]), "---") {
				end = next
				break
			}
		}
		return index, indentOf(line), end, true, nil
	}
	return 0, 0, 0, false, nil
}
