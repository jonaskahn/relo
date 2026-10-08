// Catalog files are the provider blocks Relo merges into agents that keep
// their own model list. A write inserts the relo entry and leaves every other
// key alone. A removal takes that entry back out and leaves the file in place.
package codingclients

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// CatalogAgent reports whether Relo writes this agent's provider block itself.
func CatalogAgent(id string) bool {
	switch id {
	case ClientPi, ClientPrime, ClientAside, ClientOMO, ClientZCode, ClientCline,
		ClientOMP, ClientGajae, ClientDSH, ClientMCode, ClientRaycast, ClientKimi:
		return true
	default:
		return false
	}
}

// CatalogKind reports whether a stored file is one of those provider blocks.
func CatalogKind(kind string) bool {
	switch kind {
	case FilePi, FilePrime, FileAside, FileOMO, FileZCode, FileClineProviders, FileClineModels,
		FileOMP, FileGajae, FileDSH, FileMCode, FileRaycast, FileKimi:
		return true
	default:
		return false
	}
}

// KeptDocument is what remains when stripping the block would leave a file
// with nothing in it. The file stays on disk.
func KeptDocument(kind string) string {
	if kind == FileKimi {
		return "\n"
	}
	return "{}\n"
}

// MergeKind returns the file with Relo's provider block in it.
func MergeKind(kind, existing, baseURL string, models []ModelRef) (string, error) {
	address, err := catalogAddress(kind, baseURL)
	if err != nil {
		return "", err
	}
	switch kind {
	case FilePi, FilePrime, FileAside, FileOMO, FileZCode, FileClineProviders:
		return mergeJSONKind(kind, existing, address, models)
	case FileOMP, FileGajae, FileDSH, FileMCode:
		return mergeYAMLKind(kind, existing, address, models)
	default:
		return mergeSpecialKind(kind, existing, address, models)
	}
}

func mergeJSONKind(kind, existing, address string, models []ModelRef) (string, error) {
	switch kind {
	case FilePi, FilePrime:
		return mergeJSON(existing, []string{"providers", reloProviderID}, piProvider(kind, address, models, true))
	case FileAside, FileOMO:
		return mergeJSON(existing, []string{"providers", reloProviderID}, piProvider(kind, address, models, false))
	case FileZCode:
		return mergeJSON(existing, []string{"providers", reloProviderID}, zcodeProvider(address, models))
	default:
		return mergeJSON(existing, []string{"providers", reloProviderID}, clineSettings(address))
	}
}

func mergeYAMLKind(kind, existing, address string, models []ModelRef) (string, error) {
	switch kind {
	case FileOMP:
		return mergeYAMLMap(existing, "providers", ompEntry(address, models))
	case FileGajae:
		return mergeYAMLMap(existing, "providers", gajaeEntry(address, models))
	case FileDSH:
		return mergeYAMLMap(existing, "providers", dshEntry(address, models))
	default:
		return mergeYAMLMap(existing, "", mcodeEntry(address, models))
	}
}

func mergeSpecialKind(kind, existing, address string, models []ModelRef) (string, error) {
	switch kind {
	case FileClineModels:
		return mergeClineModels(existing, address, models)
	case FileRaycast:
		return mergeRaycast(existing, address, models)
	case FileKimi:
		return mergeKimi(existing, address, models)
	default:
		return "", fmt.Errorf("%s: %w", kind, ErrUnrecognisedFile)
	}
}

// StripKind removes the provider block and leaves the rest of the file.
func StripKind(kind, existing string) (string, error) {
	var (
		stripped string
		err      error
	)
	switch kind {
	case FilePi, FilePrime, FileAside, FileOMO, FileZCode, FileClineProviders, FileClineModels:
		stripped, err = stripJSON(existing, []string{"providers", reloProviderID})
	case FileOMP, FileGajae, FileDSH:
		stripped, err = stripYAMLMap(existing, "providers")
	case FileMCode:
		stripped, err = stripYAMLMap(existing, "")
	case FileRaycast:
		stripped, err = stripRaycast(existing)
	case FileKimi:
		stripped, err = stripKimi(existing)
	default:
		return "", fmt.Errorf("%s: %w", kind, ErrUnrecognisedFile)
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(stripped) == "" {
		return KeptDocument(kind), nil
	}
	return stripped, nil
}

// KindHasRelo reports whether the file already names a relo provider.
func KindHasRelo(kind, existing string) (bool, error) {
	switch kind {
	case FilePi, FilePrime, FileAside, FileOMO, FileZCode, FileClineProviders, FileClineModels:
		return jsonHas(existing, []string{"providers", reloProviderID})
	case FileOMP, FileGajae, FileDSH:
		return yamlHas(existing, "providers")
	case FileMCode:
		return yamlHas(existing, "")
	case FileRaycast:
		return raycastHas(existing)
	case FileKimi:
		return kimiHas(existing)
	default:
		return false, fmt.Errorf("%s: %w", kind, ErrUnrecognisedFile)
	}
}

// KindPointsAt reports whether the relo provider in the file is the one Relo
// would write now. The rest of the document is the client's own.
func KindPointsAt(kind, existing, baseURL string, models []ModelRef) bool {
	address, err := catalogAddress(kind, baseURL)
	if err != nil {
		return false
	}
	switch kind {
	case FilePi, FilePrime, FileAside, FileOMO, FileZCode, FileClineProviders:
		return jsonKindPointsAt(kind, existing, address, models)
	case FileOMP, FileGajae, FileDSH, FileMCode:
		return yamlKindPointsAt(kind, existing, address, models)
	default:
		return specialKindPointsAt(kind, existing, address, models)
	}
}

func jsonKindPointsAt(kind, existing, address string, models []ModelRef) bool {
	switch kind {
	case FilePi, FilePrime:
		return jsonPointsAt(existing, piProvider(kind, address, models, true))
	case FileAside, FileOMO:
		return jsonPointsAt(existing, piProvider(kind, address, models, false))
	case FileZCode:
		return jsonPointsAt(existing, zcodeProvider(address, models))
	default:
		return jsonPointsAt(existing, clineSettings(address))
	}
}

func yamlKindPointsAt(kind, existing, address string, models []ModelRef) bool {
	switch kind {
	case FileOMP:
		return yamlPointsAt(existing, "providers", ompEntry(address, models))
	case FileGajae:
		return yamlPointsAt(existing, "providers", gajaeEntry(address, models))
	case FileDSH:
		return yamlPointsAt(existing, "providers", dshEntry(address, models))
	default:
		return yamlPointsAt(existing, "", mcodeEntry(address, models))
	}
}

func specialKindPointsAt(kind, existing, address string, models []ModelRef) bool {
	switch kind {
	case FileClineModels:
		return jsonPointsAt(existing, clineModelsValue(address, models))
	case FileRaycast:
		return raycastPointsAt(existing, address, models)
	case FileKimi:
		return kimiPointsAt(existing, address, models)
	default:
		return false
	}
}

func catalogAddress(kind, baseURL string) (string, error) {
	trimmed := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return "", fmt.Errorf("%w (no address is served)", ErrUnrecognisedFile)
	}
	if catalogUsesV1(kind) {
		return trimmed + "/v1", nil
	}
	return trimmed, nil
}

func catalogUsesV1(kind string) bool {
	switch kind {
	case FilePi, FilePrime, FileAside, FileOMO, FileOMP, FileGajae, FileKimi, FileZCode:
		return true
	default:
		return false
	}
}

func kindClient(kind string) string {
	switch kind {
	case FilePi:
		return ClientPi
	case FilePrime:
		return ClientPrime
	case FileAside:
		return ClientAside
	case FileOMO:
		return ClientOMO
	case FileZCode:
		return ClientZCode
	case FileClineProviders, FileClineModels:
		return ClientCline
	case FileOMP:
		return ClientOMP
	case FileGajae:
		return ClientGajae
	case FileDSH:
		return ClientDSH
	case FileMCode:
		return ClientMCode
	case FileRaycast:
		return ClientRaycast
	case FileKimi:
		return ClientKimi
	default:
		return ""
	}
}

func dollarKey(kind string) string {
	return "$" + EnvKeyName(kindClient(kind))
}

func braceKey(kind string) string {
	return "${" + EnvKeyName(kindClient(kind)) + "}"
}

func mergeJSON(existing string, path []string, raw string) (string, error) {
	merged, err := spliceKey(existing, path, raw)
	if err != nil {
		return "", err
	}
	return ensureNewline(merged), nil
}

func stripJSON(existing string, path []string) (string, error) {
	if strings.TrimSpace(existing) == "" {
		return "", nil
	}
	stripped, err := deleteKey(existing, path)
	if err != nil {
		return "", err
	}
	return ensureNewline(stripped), nil
}

func jsonPointsAt(existing, raw string) bool {
	got, ok, err := jsonRaw(existing, []string{"providers", reloProviderID})
	if err != nil || !ok {
		return false
	}
	return jsonEqual(got, raw)
}

func piProvider(kind, address string, models []ModelRef, withKey bool) string {
	fields := []string{
		`"baseUrl":` + jsonString(address),
		`"api":"openai-completions"`,
	}
	if withKey {
		fields = append(fields, `"apiKey":`+jsonString(dollarKey(kind)))
	}
	fields = append(fields, `"models":`+piModels(models))
	return "{" + strings.Join(fields, ",") + "}"
}

func zcodeProvider(address string, models []ModelRef) string {
	return `{"baseURL":` + jsonString(address) + `,"apiKey":` + jsonString(dollarKey(FileZCode)) + `,"models":` + idStrings(models) + `}`
}

func clineSettings(address string) string {
	return `{"settings":{"provider":"relo","baseUrl":` + jsonString(address) +
		`,"protocol":"openai-responses","client":"openai","apiKey":` + jsonString(dollarKey(FileClineProviders)) + `}}`
}

func mergeClineModels(existing, address string, models []ModelRef) (string, error) {
	merged, err := mergeJSON(existing, []string{"providers", reloProviderID}, clineModelsValue(address, models))
	if err != nil || strings.TrimSpace(existing) != "" {
		return merged, err
	}
	return mergeJSON(merged, []string{"version"}, "1")
}

func clineModelsValue(address string, models []ModelRef) string {
	ordered := orderedModels(models)
	provider := `{"name":"Relo","baseUrl":` + jsonString(address)
	if len(ordered) > 0 {
		provider += `,"defaultModelId":` + jsonString(ordered[0].ID)
	}
	provider += `}`
	return `{"provider":` + provider + `,"models":` + clineModelMap(ordered) + `}`
}

func clineModelMap(models []ModelRef) string {
	if len(models) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(models))
	for _, model := range models {
		name := model.ID
		if label := namedConfigLabel(model); label != "" {
			name = label
		}
		parts = append(parts, jsonString(model.ID)+`:{"id":`+jsonString(model.ID)+`,"name":`+jsonString(name)+`}`)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func piModels(models []ModelRef) string {
	ordered := orderedModels(models)
	if len(ordered) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(ordered))
	for _, model := range ordered {
		parts = append(parts, piModel(model))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func piModel(model ModelRef) string {
	parts := []string{`"id":` + jsonString(model.ID)}
	parts = append(parts, modelLimitFields(model, func(name string, value int64) string {
		return `"` + name + `":` + strconv.FormatInt(value, 10)
	})...)
	return "{" + strings.Join(parts, ",") + "}"
}

func idStrings(models []ModelRef) string {
	ordered := orderedModels(models)
	if len(ordered) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(ordered))
	for _, model := range ordered {
		parts = append(parts, jsonString(model.ID))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func ompEntry(address string, models []ModelRef) string {
	return "relo:\n  baseUrl: " + yamlQuote(address) + "\n  api: openai-completions\n  apiKey: " + yamlQuote(braceKey(FileOMP)) + "\n" + ompModels(models)
}

func ompModels(models []ModelRef) string {
	ordered := orderedModels(models)
	if len(ordered) == 0 {
		return "  models: []\n"
	}
	var body strings.Builder
	body.WriteString("  models:\n")
	for _, model := range ordered {
		body.WriteString("    - id: " + yamlQuote(model.ID) + "\n")
		for _, field := range modelLimitFields(model, func(name string, value int64) string {
			return "      " + name + ": " + strconv.FormatInt(value, 10)
		}) {
			body.WriteString(field + "\n")
		}
	}
	return body.String()
}

func modelLimitFields(model ModelRef, format func(name string, value int64) string) []string {
	fields := make([]string, 0, 2)
	if model.ContextWindow != nil {
		fields = append(fields, format("contextWindow", *model.ContextWindow))
	}
	if model.MaxOutput != nil {
		fields = append(fields, format("maxTokens", *model.MaxOutput))
	}
	return fields
}

func gajaeEntry(address string, models []ModelRef) string {
	return "relo:\n  baseUrl: " + yamlQuote(address) + "\n  api: openai-completions\n" + yamlIDModels(models)
}

func dshEntry(address string, models []ModelRef) string {
	return "relo:\n  baseUrl: " + yamlQuote(address) + "\n  apiKey: " + yamlQuote(braceKey(FileDSH)) + "\n" + yamlIDModels(models)
}

func mcodeEntry(address string, models []ModelRef) string {
	return "relo:\n  baseURL: " + yamlQuote(address) + "\n  apiKey: " + yamlQuote(braceKey(FileMCode)) + "\n" + yamlIDModels(models)
}

func yamlIDModels(models []ModelRef) string {
	ordered := orderedModels(models)
	if len(ordered) == 0 {
		return "  models: []\n"
	}
	var body strings.Builder
	body.WriteString("  models:\n")
	for _, model := range ordered {
		body.WriteString("    - id: " + yamlQuote(model.ID) + "\n")
	}
	return body.String()
}

func mergeYAMLMap(existing, parent, entry string) (string, error) {
	if err := parseYAML(existing); err != nil {
		return "", err
	}
	lines, err := dropYAMLRelo(splitLines(existing), parent)
	if err != nil {
		return "", err
	}
	lines, err = insertYAMLRelo(lines, parent, entry)
	if err != nil {
		return "", err
	}
	merged := joinLines(lines, lineEnding(existing))
	if err := parseYAML(merged); err != nil {
		return "", err
	}
	return merged, nil
}

func stripYAMLMap(existing, parent string) (string, error) {
	if strings.TrimSpace(existing) == "" {
		return "", nil
	}
	if err := parseYAML(existing); err != nil {
		return "", err
	}
	lines, err := dropYAMLRelo(splitLines(existing), parent)
	if err != nil {
		return "", err
	}
	merged := joinLines(lines, lineEnding(existing))
	if strings.TrimSpace(merged) == "" {
		return "", nil
	}
	if err := parseYAML(merged); err != nil {
		return "", err
	}
	return merged, nil
}

func yamlHas(existing, parent string) (bool, error) {
	if strings.TrimSpace(existing) == "" {
		return false, nil
	}
	if err := parseYAML(existing); err != nil {
		return false, err
	}
	var document map[string]any
	if err := yaml.Unmarshal([]byte(existing), &document); err != nil {
		return false, fmt.Errorf("%w (%v)", ErrUnrecognisedFile, err)
	}
	if parent == "" {
		_, found := document[reloProviderID]
		return found, nil
	}
	raw, ok := document[parent]
	if !ok || raw == nil {
		return false, nil
	}
	providers, ok := raw.(map[string]any)
	if !ok {
		return false, fmt.Errorf("%w (%s is not a map)", ErrUnrecognisedFile, parent)
	}
	_, found := providers[reloProviderID]
	return found, nil
}

func yamlPointsAt(existing, parent, entry string) bool {
	if err := parseYAML(existing); err != nil {
		return false
	}
	var document map[string]any
	if yaml.Unmarshal([]byte(existing), &document) != nil {
		return false
	}
	var want map[string]any
	if yaml.Unmarshal([]byte(entry), &want) != nil {
		return false
	}
	if parent == "" {
		return reflect.DeepEqual(document[reloProviderID], want[reloProviderID])
	}
	providers, _ := document[parent].(map[string]any)
	if providers == nil {
		return false
	}
	return reflect.DeepEqual(providers[reloProviderID], want[reloProviderID])
}

func dropYAMLRelo(lines []string, parent string) ([]string, error) {
	if parent == "" {
		return dropTopLevelKey(lines, reloProviderID), nil
	}
	start, child, end, found, err := yamlMapBlock(lines, parent)
	if err != nil || !found {
		return lines, err
	}
	for index := start + 1; index < end; index++ {
		if stop, found := reloBlockEnd(lines, index, end, child); found {
			updated := append([]string{}, lines[:index]...)
			return append(updated, lines[stop:]...), nil
		}
	}
	return lines, nil
}

func reloBlockEnd(lines []string, index, end, child int) (int, bool) {
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

func insertYAMLRelo(lines []string, parent, entry string) ([]string, error) {
	if parent == "" {
		return appendBlock(lines, strings.TrimSuffix(entry, "\n")), nil
	}
	_, child, end, found, err := yamlMapBlock(lines, parent)
	if err != nil {
		return nil, err
	}
	if !found {
		block := parent + ":\n" + indentBlock(entry, 2)
		return appendBlock(lines, strings.TrimSuffix(block, "\n")), nil
	}
	body := splitLines(strings.TrimSuffix(indentBlock(entry, child), "\n"))
	updated := append([]string{}, lines[:end]...)
	updated = append(updated, body...)
	return append(updated, lines[end:]...), nil
}

func yamlMapBlock(lines []string, key string) (start, child, end int, found bool, err error) {
	for index, line := range lines {
		if indentOf(line) != 0 || yamlKey(line) != key {
			continue
		}
		if inlineYAML(line, key) {
			return 0, 0, 0, false, fmt.Errorf("%w (%s is not a block)", ErrUnrecognisedFile, key)
		}
		end = len(lines)
		child = -1
		for next := index + 1; next < len(lines); next++ {
			if strings.TrimSpace(lines[next]) == "" || strings.HasPrefix(strings.TrimSpace(lines[next]), "#") {
				continue
			}
			if indentOf(lines[next]) == 0 {
				end = next
				break
			}
			if child < 0 {
				child = indentOf(lines[next])
			}
		}
		if child < 0 {
			child = 2
		}
		return index, child, end, true, nil
	}
	return 0, 0, 0, false, nil
}

func inlineYAML(line, key string) bool {
	trimmed := strings.TrimSpace(line)
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, key+":"))
	return rest != ""
}

func dropTopLevelKey(lines []string, key string) []string {
	for index, line := range lines {
		if indentOf(line) != 0 || yamlKey(line) != key {
			continue
		}
		stop := index + 1
		for stop < len(lines) {
			if strings.TrimSpace(lines[stop]) == "" {
				stop++
				continue
			}
			if indentOf(lines[stop]) == 0 {
				break
			}
			stop++
		}
		for stop > index+1 && strings.TrimSpace(lines[stop-1]) == "" {
			stop--
		}
		updated := append([]string{}, lines[:index]...)
		return append(updated, lines[stop:]...)
	}
	return lines
}

func indentBlock(block string, spaces int) string {
	pad := strings.Repeat(" ", spaces)
	lines := splitLines(strings.TrimRight(block, "\n"))
	for index, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines[index] = pad + line
	}
	return strings.Join(lines, "\n") + "\n"
}

func mergeRaycast(existing, address string, models []ModelRef) (string, error) {
	if err := parseYAML(existing); err != nil {
		return "", err
	}
	lines, err := dropRaycast(splitLines(existing))
	if err != nil {
		return "", err
	}
	lines, err = insertRaycast(lines, raycastItem(address, models))
	if err != nil {
		return "", err
	}
	merged := joinLines(lines, lineEnding(existing))
	if err := parseYAML(merged); err != nil {
		return "", err
	}
	return merged, nil
}

func stripRaycast(existing string) (string, error) {
	if strings.TrimSpace(existing) == "" {
		return "", nil
	}
	if err := parseYAML(existing); err != nil {
		return "", err
	}
	lines, err := dropRaycast(splitLines(existing))
	if err != nil {
		return "", err
	}
	return joinLines(lines, lineEnding(existing)), nil
}

func raycastHas(existing string) (bool, error) {
	if strings.TrimSpace(existing) == "" {
		return false, nil
	}
	if err := parseYAML(existing); err != nil {
		return false, err
	}
	var document map[string]any
	if err := yaml.Unmarshal([]byte(existing), &document); err != nil {
		return false, fmt.Errorf("%w (%v)", ErrUnrecognisedFile, err)
	}
	items, ok := document["providers"].([]any)
	if document["providers"] != nil && !ok {
		return false, fmt.Errorf("%w (providers is not a list)", ErrUnrecognisedFile)
	}
	for _, item := range items {
		row, _ := item.(map[string]any)
		if row["id"] == reloProviderID {
			return true, nil
		}
	}
	return false, nil
}

func raycastPointsAt(existing, address string, models []ModelRef) bool {
	var document map[string]any
	if yaml.Unmarshal([]byte(existing), &document) != nil {
		return false
	}
	var want map[string]any
	if yaml.Unmarshal([]byte(raycastItem(address, models)), &want) != nil {
		return false
	}
	items, _ := document["providers"].([]any)
	for _, item := range items {
		row, _ := item.(map[string]any)
		if row["id"] == reloProviderID {
			return reflect.DeepEqual(row, want)
		}
	}
	return false
}

func raycastItem(address string, models []ModelRef) string {
	var body strings.Builder
	body.WriteString("id: " + reloProviderID + "\n")
	body.WriteString("name: Relo\n")
	body.WriteString("base_url: " + yamlQuote(address) + "\n")
	ordered := orderedModels(models)
	if len(ordered) == 0 {
		body.WriteString("models: []\n")
		return body.String()
	}
	body.WriteString("models:\n")
	for _, model := range ordered {
		name := model.ID
		if label := namedConfigLabel(model); label != "" {
			name = label
		}
		body.WriteString("  - id: " + yamlQuote(model.ID) + "\n")
		body.WriteString("    name: " + yamlQuote(name) + "\n")
	}
	return body.String()
}

func dropRaycast(lines []string) ([]string, error) {
	start, itemIndent, end, found, err := raycastList(lines)
	if err != nil || !found {
		return lines, err
	}
	updated := append([]string{}, lines[:start+1]...)
	index := start + 1
	for index < end {
		line := lines[index]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			updated = append(updated, line)
			index++
			continue
		}
		if indentOf(line) == itemIndent && strings.HasPrefix(strings.TrimSpace(line), "-") {
			stop := raycastItemEnd(lines, index, end, itemIndent)
			if !raycastItemIsRelo(lines[index:stop]) {
				updated = append(updated, lines[index:stop]...)
			}
			index = stop
			continue
		}
		updated = append(updated, line)
		index++
	}
	return append(updated, lines[end:]...), nil
}

func raycastItemEnd(lines []string, index, end, itemIndent int) int {
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

func raycastItemIsRelo(lines []string) bool {
	for _, line := range lines {
		if value, ok := yamlScalar(line, "id"); ok && value == reloProviderID {
			return true
		}
	}
	return false
}

func yamlScalar(line, key string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "-")
	trimmed = strings.TrimSpace(trimmed)
	name, rest, found := strings.Cut(trimmed, ":")
	if !found || strings.Trim(strings.TrimSpace(name), `"'`) != key {
		return "", false
	}
	return strings.Trim(strings.TrimSpace(rest), `"'`), true
}

func insertRaycast(lines []string, item string) ([]string, error) {
	_, itemIndent, end, found, err := raycastList(lines)
	if err != nil {
		return nil, err
	}
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
	if !found {
		block := "providers:\n" + strings.Join(rendered, "\n")
		return appendBlock(lines, block), nil
	}
	updated := append([]string{}, lines[:end]...)
	updated = append(updated, rendered...)
	return append(updated, lines[end:]...), nil
}

func raycastList(lines []string) (start, itemIndent, end int, found bool, err error) {
	start, _, end, found, err = yamlMapBlock(lines, "providers")
	if err != nil || !found {
		return start, 2, end, found, err
	}
	for index := start + 1; index < end; index++ {
		if strings.TrimSpace(lines[index]) == "" || strings.HasPrefix(strings.TrimSpace(lines[index]), "#") {
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(lines[index]), "-") {
			return 0, 0, 0, false, fmt.Errorf("%w (providers is not a list)", ErrUnrecognisedFile)
		}
		return start, indentOf(lines[index]), end, true, nil
	}
	return start, 2, end, true, nil
}

func mergeKimi(existing, address string, models []ModelRef) (string, error) {
	if err := refuseInlineTOML(existing); err != nil {
		return "", err
	}
	body := kimiBody(address, models)
	if strings.TrimSpace(existing) == "" {
		return "[providers.relo]\n" + body, nil
	}
	lines := dropTOMLSection(splitLines(existing), "[providers.relo]")
	lines = appendBlock(lines, "[providers.relo]\n"+strings.TrimSuffix(body, "\n"))
	return joinLines(lines, lineEnding(existing)), nil
}

func stripKimi(existing string) (string, error) {
	if strings.TrimSpace(existing) == "" {
		return "", nil
	}
	if err := refuseInlineTOML(existing); err != nil {
		return "", err
	}
	return joinLines(dropTOMLSection(splitLines(existing), "[providers.relo]"), lineEnding(existing)), nil
}

func kimiHas(existing string) (bool, error) {
	if strings.TrimSpace(existing) == "" {
		return false, nil
	}
	if err := refuseInlineTOML(existing); err != nil {
		return false, err
	}
	for _, line := range splitLines(existing) {
		if strings.TrimSpace(line) == "[providers.relo]" {
			return true, nil
		}
	}
	return false, nil
}

func kimiPointsAt(existing, address string, models []ModelRef) bool {
	section := tomlSection(existing, "[providers.relo]")
	if section == "" {
		return false
	}
	return strings.TrimSpace(section) == strings.TrimSpace(kimiBody(address, models))
}

func kimiBody(address string, models []ModelRef) string {
	return "base_url = " + jsonString(address) + "\napi_key = " + jsonString(braceKey(FileKimi)) + "\nmodels = " + idStrings(models) + "\n"
}

func refuseInlineTOML(existing string) error {
	for _, line := range splitLines(existing) {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "[") {
			continue
		}
		name, _, found := strings.Cut(trimmed, "=")
		if found && strings.TrimSpace(name) == "providers" {
			return fmt.Errorf("%w (providers is not a table)", ErrUnrecognisedFile)
		}
	}
	return nil
}

func dropTOMLSection(lines []string, header string) []string {
	start := -1
	for index, line := range lines {
		if strings.TrimSpace(line) == header {
			start = index
			break
		}
	}
	if start < 0 {
		return lines
	}
	stop := len(lines)
	for index := start + 1; index < len(lines); index++ {
		if strings.HasPrefix(strings.TrimSpace(lines[index]), "[") {
			stop = index
			break
		}
	}
	for stop > start+1 && strings.TrimSpace(lines[stop-1]) == "" {
		stop--
	}
	updated := append([]string{}, lines[:start]...)
	return append(updated, lines[stop:]...)
}

func tomlSection(content, header string) string {
	lines := splitLines(content)
	start := -1
	for index, line := range lines {
		if strings.TrimSpace(line) == header {
			start = index
			break
		}
	}
	if start < 0 {
		return ""
	}
	stop := len(lines)
	for index := start + 1; index < len(lines); index++ {
		if strings.HasPrefix(strings.TrimSpace(lines[index]), "[") {
			stop = index
			break
		}
	}
	return strings.Join(lines[start+1:stop], "\n")
}
