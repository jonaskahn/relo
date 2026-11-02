// WorkBuddy and CodeBuddy share one custom-model file: a list of models, or
// an object whose "models" key holds that list.
package codingclients

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const codeBuddyVendor = "Relo"

type codeBuddyEntry struct {
	ID                string              `json:"id"`
	Name              string              `json:"name"`
	Vendor            string              `json:"vendor"`
	URL               string              `json:"url"`
	APIKey            string              `json:"apiKey"`
	UseCustomProtocol bool                `json:"useCustomProtocol"`
	MaxInputTokens    *int64              `json:"maxInputTokens,omitempty"`
	MaxOutputTokens   *int64              `json:"maxOutputTokens,omitempty"`
	SupportsToolCall  *bool               `json:"supportsToolCall,omitempty"`
	SupportsImages    *bool               `json:"supportsImages,omitempty"`
	SupportsReasoning *bool               `json:"supportsReasoning,omitempty"`
	Reasoning         *codeBuddyReasoning `json:"reasoning,omitempty"`
}

type codeBuddyReasoning struct {
	SupportedEfforts []string `json:"supportedEfforts"`
}

// mergeCodeBuddy keeps every model the operator added and replaces Relo's own
// entries with the catalog Relo publishes now. The object form is preferred
// for a missing file because the command line reads only that shape; an
// existing document keeps whichever shape it already had.
func mergeCodeBuddy(existing, address string, models []ModelRef) (string, error) {
	if strings.TrimSpace(existing) == "" {
		return codeBuddyObject(codeBuddyEntryList(address, models)), nil
	}
	object, entries, err := codeBuddyDocument(existing)
	if err != nil {
		return "", err
	}
	merged := append(codeBuddyWithoutRelo(entries), rawCodeBuddyEntries(address, models)...)
	if !object {
		return codeBuddyIndented(merged), nil
	}
	return codeBuddyMergeModels(existing, merged)
}

func stripCodeBuddy(existing string) (string, error) {
	if strings.TrimSpace(existing) == "" {
		return "", nil
	}
	object, entries, err := codeBuddyDocument(existing)
	if err != nil {
		return "", err
	}
	kept := codeBuddyWithoutRelo(entries)
	if len(kept) == len(entries) {
		return ensureNewline(existing), nil
	}
	if !object {
		if len(kept) == 0 {
			return "[]\n", nil
		}
		return codeBuddyIndented(kept), nil
	}
	if len(kept) == 0 {
		stripped, err := deleteKey(existing, []string{"models"})
		if err != nil {
			return "", err
		}
		if emptyDocument(stripped) {
			return "{}\n", nil
		}
		return ensureNewline(stripped), nil
	}
	return codeBuddyMergeModels(existing, kept)
}

func codeBuddyHasRelo(existing string) (bool, error) {
	if strings.TrimSpace(existing) == "" {
		return false, nil
	}
	_, entries, err := codeBuddyDocument(existing)
	if err != nil {
		return false, err
	}
	return len(codeBuddyWithoutRelo(entries)) != len(entries), nil
}

func codeBuddyPointsAt(existing, address string, models []ModelRef) bool {
	_, entries, err := codeBuddyDocument(existing)
	if err != nil {
		return false
	}
	relo := codeBuddyReloEntries(entries)
	if len(relo) == 0 {
		return false
	}
	got, ok := codeBuddyComparable(relo)
	if !ok {
		return false
	}
	want, ok := codeBuddyComparable(rawCodeBuddyEntries(address, models))
	if !ok {
		return false
	}
	return string(got) == string(want)
}

// codeBuddyDocument reads either shape. An object without a models key reads
// as an empty list, and a merge inserts the key where it belongs.
func codeBuddyDocument(existing string) (object bool, entries []json.RawMessage, err error) {
	if !json.Valid([]byte(existing)) {
		return false, nil, fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	switch first := strings.TrimSpace(existing)[0]; first {
	case '[':
		var list []json.RawMessage
		if err := json.Unmarshal([]byte(existing), &list); err != nil {
			return false, nil, fmt.Errorf("%w", ErrUnrecognisedFile)
		}
		return false, list, nil
	case '{':
		raw, found, err := jsonRaw(existing, []string{"models"})
		if err != nil {
			return false, nil, err
		}
		if !found {
			return true, nil, nil
		}
		list, err := codeBuddyModelList(raw)
		if err != nil {
			return false, nil, err
		}
		return true, list, nil
	default:
		return false, nil, fmt.Errorf("%w", ErrUnrecognisedFile)
	}
}

func codeBuddyModelList(raw string) ([]json.RawMessage, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed[0] != '[' {
		return nil, fmt.Errorf("%w (models is not a list)", ErrUnrecognisedFile)
	}
	var list []json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &list); err != nil {
		return nil, fmt.Errorf("%w (models is not a list)", ErrUnrecognisedFile)
	}
	return list, nil
}

func codeBuddyWithoutRelo(entries []json.RawMessage) []json.RawMessage {
	kept := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		if !codeBuddyIsRelo(entry) {
			kept = append(kept, entry)
		}
	}
	return kept
}

func codeBuddyReloEntries(entries []json.RawMessage) []json.RawMessage {
	relo := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		if codeBuddyIsRelo(entry) {
			relo = append(relo, entry)
		}
	}
	return relo
}

func codeBuddyIsRelo(raw json.RawMessage) bool {
	var row struct {
		Vendor string `json:"vendor"`
	}
	return json.Unmarshal(raw, &row) == nil && row.Vendor == codeBuddyVendor
}

// codeBuddyComparable orders Relo's entries by id and drops the apiKey, which
// WorkBuddy may encrypt at rest. Drift detection must not fire on that.
func codeBuddyComparable(entries []json.RawMessage) ([]byte, bool) {
	rows := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		var row map[string]any
		if err := json.Unmarshal(entry, &row); err != nil {
			return nil, false
		}
		delete(row, "apiKey")
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		left, _ := rows[i]["id"].(string)
		right, _ := rows[j]["id"].(string)
		return left < right
	})
	encoded, err := json.Marshal(rows)
	return encoded, err == nil
}

func rawCodeBuddyEntries(address string, models []ModelRef) []json.RawMessage {
	entries := codeBuddyEntryList(address, models)
	raw := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		encoded, err := json.Marshal(entry)
		if err != nil {
			continue
		}
		raw = append(raw, encoded)
	}
	return raw
}

func codeBuddyEntryList(address string, models []ModelRef) []codeBuddyEntry {
	ordered := orderedModels(models)
	entries := make([]codeBuddyEntry, 0, len(ordered))
	for _, model := range ordered {
		name := namedConfigLabel(model)
		if name == "" {
			name = model.ID
		}
		entry := codeBuddyEntry{
			ID: model.ID, Name: name, Vendor: codeBuddyVendor,
			URL: address + "/chat/completions", APIKey: braceKey(FileCodeBuddy),
			UseCustomProtocol: false,
			MaxInputTokens:    model.ContextWindow, MaxOutputTokens: model.MaxOutput,
			SupportsToolCall: model.Tools, SupportsImages: model.Vision,
			SupportsReasoning: model.Reasoning,
		}
		if efforts := codeBuddyEfforts(model); len(efforts) > 0 {
			entry.Reasoning = &codeBuddyReasoning{SupportedEfforts: efforts}
		}
		entries = append(entries, entry)
	}
	return entries
}

func codeBuddyEfforts(model ModelRef) []string {
	if model.Reasoning == nil || !*model.Reasoning {
		return nil
	}
	return modelEfforts(model)
}

func codeBuddyObject(entries []codeBuddyEntry) string {
	raw := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		encoded, err := json.Marshal(entry)
		if err != nil {
			continue
		}
		raw = append(raw, encoded)
	}
	list := strings.TrimSuffix(codeBuddyIndented(raw), "\n")
	return "{\n  \"models\": " + strings.ReplaceAll(list, "\n", "\n  ") + "\n}\n"
}

// codeBuddyIndented renders the model list as plain JSON with two-space
// indentation.
func codeBuddyIndented(entries []json.RawMessage) string {
	if len(entries) == 0 {
		return "[]\n"
	}
	encoded, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return "[]\n"
	}
	return string(encoded) + "\n"
}

// codeBuddyMergeModels rewrites only the models member, keeping every other
// key byte-for-byte and the document's own indentation for the list.
func codeBuddyMergeModels(existing string, entries []json.RawMessage) (string, error) {
	value := strings.TrimSuffix(codeBuddyIndented(entries), "\n")
	if prefix := codeBuddyKeyIndent(existing); prefix != "" {
		value = strings.ReplaceAll(value, "\n", "\n"+prefix)
	}
	replaced, err := spliceKey(existing, []string{"models"}, value)
	if err != nil {
		return "", err
	}
	return ensureNewline(replaced), nil
}

// codeBuddyKeyIndent is the indentation before the models key when it starts
// its own line. A compact document has none, and its list stays compact.
func codeBuddyKeyIndent(existing string) string {
	loc, err := findMember(existing, skipSpace(existing, 0), "models")
	if err != nil || !loc.found {
		return ""
	}
	line := strings.LastIndex(existing[:loc.keyStart], "\n")
	if line < 0 {
		return ""
	}
	prefix := existing[line+1 : loc.keyStart]
	if strings.TrimSpace(prefix) != "" {
		return ""
	}
	return prefix
}
