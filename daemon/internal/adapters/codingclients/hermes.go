// Hermes config: merging Relo into its file and reading it back.
package codingclients

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	hermesFenceStart = "# >>> relo (managed) >>>"
	hermesFenceEnd   = "# <<< relo (managed) <<<"
)

func hermesKeyRef() string {
	return "${" + EnvKeyName(ClientHermes) + "}"
}

// MergeHermes returns a Hermes configuration with Relo's provider inside a
// fenced block. A providers map the operator already has gains that block as
// one more entry; a file without one gains the map. The operator's selected
// model is left alone.
func MergeHermes(existing, baseURL string, models []ModelRef) (string, error) {
	address, err := requireBase(baseURL)
	if err != nil {
		return "", fmt.Errorf("hermes: %w", err)
	}
	if err := parseYAML(existing); err != nil {
		return "", fmt.Errorf("hermes: %w", err)
	}
	ending := lineEnding(existing)
	lines := removeFence(splitLines(existing))
	lines, err = removeHermesRelo(lines)
	if err != nil {
		return "", fmt.Errorf("hermes: %w", err)
	}
	lines = insertHermes(lines, address, models)
	merged := joinLines(lines, ending)
	if err := parseYAML(merged); err != nil {
		return "", fmt.Errorf("hermes: %w", err)
	}
	return merged, nil
}

// StripHermes removes the fenced block and leaves the rest of the file.
func StripHermes(existing string) (string, error) {
	if strings.TrimSpace(existing) == "" {
		return "", nil
	}
	if err := parseYAML(existing); err != nil {
		return "", fmt.Errorf("hermes: %w", err)
	}
	lines := removeFence(splitLines(existing))
	if len(lines) == 0 {
		return "", nil
	}
	return joinLines(lines, lineEnding(existing)), nil
}

func hermesPointsAt(existing, baseURL string, models []ModelRef) bool {
	address, err := requireBase(baseURL)
	if err != nil {
		return false
	}
	got, found := hermesFenceEntry(existing)
	if !found {
		return false
	}
	entry := hermesEntry(address, models, "")
	// A file that had no providers map keeps that map inside the fence. A file
	// that already had one keeps only the relo entry inside it.
	wrapped := "providers:\n" + hermesEntry(address, models, "  ")
	return yamlEqual(got, entry) || yamlEqual(got, wrapped)
}

func hermesFenceEntry(content string) (string, bool) {
	lines := splitLines(content)
	start, end := -1, -1
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == hermesFenceStart && start < 0:
			start = index
		case trimmed == hermesFenceEnd && start >= 0:
			end = index
		}
		if end >= 0 {
			break
		}
	}
	if start < 0 || end <= start+1 {
		return "", false
	}
	return dedent(lines[start+1 : end]), true
}

func dedent(lines []string) string {
	margin := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		width := indentOf(line)
		if margin < 0 || width < margin {
			margin = width
		}
	}
	if margin < 0 {
		margin = 0
	}
	out := make([]string, len(lines))
	for index, line := range lines {
		if len(line) >= margin {
			out[index] = line[margin:]
			continue
		}
		out[index] = strings.TrimLeft(line, " \t")
	}
	return strings.Join(out, "\n") + "\n"
}

func yamlEqual(left, right string) bool {
	var got, want any
	if yaml.Unmarshal([]byte(left), &got) != nil || yaml.Unmarshal([]byte(right), &want) != nil {
		return false
	}
	return reflect.DeepEqual(got, want)
}

func hermesHasRelo(existing string) (bool, error) {
	if strings.TrimSpace(existing) == "" {
		return false, nil
	}
	if err := parseYAML(existing); err != nil {
		return false, err
	}
	return hermesReloOutsideFence(existing)
}

func hermesFragment(baseURL string, models []ModelRef) (string, error) {
	address, err := requireBase(baseURL)
	if err != nil {
		return "", fmt.Errorf("hermes: %w", err)
	}
	return hermesBody(address, models, ""), nil
}

func parseYAML(content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	var document map[string]any
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		return fmt.Errorf("%w (%v)", ErrUnrecognisedFile, err)
	}
	return nil
}

func hermesReloOutsideFence(content string) (bool, error) {
	_, found, err := findHermesRelo(removeFence(splitLines(content)))
	return found, err
}

func removeFence(lines []string) []string {
	kept := make([]string, 0, len(lines))
	inside := false
	for _, line := range lines {
		switch {
		case strings.TrimSpace(line) == hermesFenceStart:
			inside = true
		case strings.TrimSpace(line) == hermesFenceEnd:
			inside = false
		case !inside:
			kept = append(kept, line)
		}
	}
	return kept
}

func removeHermesRelo(lines []string) ([]string, error) {
	index, found, err := findHermesRelo(lines)
	if err != nil || !found {
		return lines, err
	}
	indent := indentOf(lines[index])
	end := index + 1
	for end < len(lines) {
		line := lines[end]
		if strings.TrimSpace(line) == "" {
			end++
			continue
		}
		if indentOf(line) <= indent {
			break
		}
		end++
	}
	for end > index+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	updated := append([]string{}, lines[:index]...)
	return append(updated, lines[end:]...), nil
}

func findHermesRelo(lines []string) (int, bool, error) {
	providers, child, end, shape, err := hermesProviders(lines)
	if err != nil || shape != hermesBlock {
		return 0, false, err
	}
	for index := providers + 1; index < end; index++ {
		line := lines[index]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if indentOf(line) != child {
			continue
		}
		if yamlKey(line) == reloProviderID {
			return index, true, nil
		}
	}
	return 0, false, nil
}

const (
	hermesAbsent = iota
	hermesBlock
	hermesInline
)

func hermesProviders(lines []string) (key, child, end, shape int, err error) {
	for index, line := range lines {
		if indentOf(line) != 0 || yamlKey(line) != "providers" {
			continue
		}
		if strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "providers:")) != "" {
			return index, 0, index + 1, hermesInline, fmt.Errorf("%w (providers is not a block)", ErrUnrecognisedFile)
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
		return index, child, end, hermesBlock, nil
	}
	return 0, 0, 0, hermesAbsent, nil
}

func insertHermes(lines []string, address string, models []ModelRef) []string {
	_, child, end, shape, err := hermesProviders(lines)
	if err != nil || shape == hermesInline {
		return lines
	}
	if shape == hermesAbsent {
		block := splitLines(strings.TrimSuffix(hermesWrapped(address, models), "\n"))
		return appendBlock(lines, strings.Join(block, "\n")+"\n")
	}
	indent := strings.Repeat(" ", child)
	body := hermesFenced(address, models, indent)
	fenced := splitLines(strings.TrimSuffix(body, "\n"))
	updated := append([]string{}, lines[:end]...)
	updated = append(updated, fenced...)
	return append(updated, lines[end:]...)
}

func hermesWrapped(address string, models []ModelRef) string {
	return hermesFenceStart + "\nproviders:\n" + hermesEntry(address, models, "  ") + hermesFenceEnd + "\n"
}

func hermesFenced(address string, models []ModelRef, indent string) string {
	return indent + hermesFenceStart + "\n" + hermesEntry(address, models, indent) + indent + hermesFenceEnd + "\n"
}

func hermesBody(address string, models []ModelRef, indent string) string {
	return hermesFenced(address, models, indent)
}

func hermesEntry(address string, models []ModelRef, indent string) string {
	nested := indent + "  "
	lines := []string{
		indent + "relo:",
		nested + "api: " + yamlQuote(address),
		nested + "api_key: " + yamlQuote(hermesKeyRef()),
		nested + "api_mode: chat_completions",
		nested + "discover_models: false",
	}
	lines = append(lines, hermesModels(models, nested)...)
	return strings.Join(lines, "\n") + "\n"
}

func hermesModels(models []ModelRef, indent string) []string {
	ordered := orderedModels(models)
	if len(ordered) == 0 {
		return []string{indent + "models: {}"}
	}
	lines := []string{indent + "models:"}
	deeper := indent + "  "
	for _, model := range ordered {
		lines = append(lines, hermesModel(model, deeper)...)
	}
	return lines
}

func hermesModel(model ModelRef, indent string) []string {
	fields := make([]string, 0, 2)
	if name := namedConfigLabel(model); name != "" {
		fields = append(fields, indent+"  name: "+yamlQuote(name))
	}
	if model.Vision != nil {
		fields = append(fields, indent+"  supports_vision: "+strconv.FormatBool(*model.Vision))
	}
	if len(fields) == 0 {
		return []string{indent + model.ID + ": {}"}
	}
	lines := []string{indent + model.ID + ":"}
	return append(lines, fields...)
}

func yamlKey(line string) string {
	trimmed := strings.TrimSpace(line)
	name, _, found := strings.Cut(trimmed, ":")
	if !found {
		return ""
	}
	return strings.Trim(strings.TrimSpace(name), `"'`)
}

func yamlQuote(value string) string {
	return jsonString(value)
}

func indentOf(line string) int {
	count := 0
	for _, r := range line {
		if r != ' ' && r != '\t' {
			break
		}
		count++
	}
	return count
}

func lineEnding(content string) string {
	if strings.Contains(content, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func joinLines(lines []string, ending string) string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, ending) + ending
}
