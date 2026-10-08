// Codex model catalog: building and writing the catalog file.
package codingclients

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jonaskahn/relo/internal/catalog"
)

const (
	codexCatalogFile     = "relo-model-catalog.json"
	codexModelsCache     = "models_cache.json"
	codexDefaultContext  = int64(128000)
	codexBaseInstruction = "You are a coding assistant."
	codexContextPercent  = 95
	identityMaxLength    = 128
	identityPunctuation  = "._/@:+-[]~"
	neutralIdentity      = "You are a coding agent. Do not claim to be GPT-5 or to be made by OpenAI."
	codexCacheFetchedAt  = "2000-01-01T00:00:00Z"
	codexCacheVersion    = "0.0.0"
)

type codexRow = map[string]any

var codexRoutedOnly = []string{
	"model_messages", "availability_nux", "service_tiers", "service_tier",
	"default_service_tier", "additional_speed_tiers", "use_responses_lite",
	"supports_websockets", "supports_experimental_context",
	"supports_reasoning_summaries", "multi_agent_version",
	"multi_agent_reasoning_effort", "auto_compact_token_limit",
	"web_search_tool_type",
}

var codexIdentity = regexp.MustCompile(`You are Codex, (?:a coding agent|an agent) based on GPT-[0-9]+(?:\.[0-9]+)*\.`)

// CodexCatalogPath is the catalog file Codex reads from model_catalog_json,
// kept beside the configuration Relo edits.
func CodexCatalogPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), codexCatalogFile)
}

// WriteCodexCatalog writes the model list Codex's picker reads. Every row is
// cloned from a native row in Codex's own models cache, so it carries the
// fields the client needs; a machine with no cache gets a minimal row. A model
// with no known context window keeps the window Codex accepts. Every row names
// its reasoning levels, because Codex refuses a catalog entry that omits the
// field; a model that does not reason gets an empty list.
func WriteCodexCatalog(path string, models []ModelRef) error {
	encoded, err := json.MarshalIndent(map[string]any{"models": BuildCodexRows(models, ReadCodexTemplates(filepath.Dir(path)))}, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(encoded, '\n'), 0o600)
}

// BuildCodexRows is the catalog Codex's picker and the /v1/models catalog
// shape both read: one row per published model, cloned from a native template
// when one is cached.
func BuildCodexRows(models []ModelRef, templates CodexTemplates) []map[string]any {
	rows := make([]map[string]any, 0, len(models))
	for _, model := range models {
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		rows = append(rows, ensureCodexInstructions(codexRowOf(model, len(rows), templates)))
	}
	return rows
}

func ensureCodexInstructions(row codexRow) codexRow {
	if instructionText(row) != "" {
		return row
	}
	row["base_instructions"] = codexBaseInstruction
	return row
}

// ReadCodexTemplates loads native rows from Codex's models cache in the given
// home. A missing or empty cache yields no templates, and every row then
// falls back to the smallest shape Codex accepts.
func ReadCodexTemplates(codexHome string) CodexTemplates {
	return readCodexTemplates(filepath.Join(codexHome, codexModelsCache))
}

// SyncCodexCache rewrites Codex's models cache the way a refresh does: native
// rows stay, Relo rows replace any earlier Relo or namespaced leftovers, and
// the stamp is old so Codex rereads the catalog rather than treating this
// cache as current.
func SyncCodexCache(codexHome string, models []ModelRef) error {
	path := filepath.Join(codexHome, codexModelsCache)
	existing := readCodexRows(path)
	natives := nativeCacheRows(existing)
	rows := append(natives, BuildCodexRows(models, templatesFromRows(natives))...)
	return writeCodexCache(path, rows)
}

// StripReloFromCodexCache drops the Relo rows Relo wrote into Codex's cache
// and leaves every other row, including native GPT entries, as they were.
func StripReloFromCodexCache(codexHome string) error {
	path := filepath.Join(codexHome, codexModelsCache)
	existing := readCodexRows(path)
	if len(existing) == 0 {
		return nil
	}
	kept := make([]codexRow, 0, len(existing))
	changed := false
	for _, row := range existing {
		slug, _ := row["slug"].(string)
		if isReloCatalogSlug(slug) {
			changed = true
			continue
		}
		kept = append(kept, row)
	}
	if !changed {
		return nil
	}
	return writeCodexCache(path, kept)
}

// CodexCatalogLists reports whether a catalog file names every published
// model. A missing file, or one that omits a slug Relo publishes, does not.
func CodexCatalogLists(path string, models []ModelRef) bool {
	rows := readCodexRows(path)
	listed := map[string]string{}
	for _, row := range rows {
		slug, _ := row["slug"].(string)
		name, _ := row["display_name"].(string)
		if slug != "" {
			listed[slug] = name
		}
	}
	for _, model := range models {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		if listed[id] != ConfigDisplayName(model) {
			return false
		}
	}
	return true
}

// PublishedCodexSlugs reads the slugs a catalog file lists. A file that is
// missing or unreadable lists none.
func PublishedCodexSlugs(path string) map[string]bool {
	slugs := map[string]bool{}
	if strings.TrimSpace(path) == "" {
		return slugs
	}
	for _, row := range readCodexRows(path) {
		if slug, ok := row["slug"].(string); ok {
			slugs[slug] = true
		}
	}
	return slugs
}

func readCodexRows(path string) []codexRow {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var file struct {
		Models []codexRow `json:"models"`
	}
	if err := json.Unmarshal(content, &file); err != nil {
		return nil
	}
	return file.Models
}

// CodexTemplates are the native rows Codex cached from its own service. A
// native row has no provider prefix in its slug; rows Relo or another tool
// routed through Codex are not templates.
type CodexTemplates struct {
	bySlug map[string]codexRow
	base   codexRow
}

func readCodexTemplates(cachePath string) CodexTemplates {
	return templatesFromRows(readCodexRows(cachePath))
}

func templatesFromRows(rows []codexRow) CodexTemplates {
	templates := CodexTemplates{bySlug: map[string]codexRow{}}
	for _, row := range rows {
		slug, _ := row["slug"].(string)
		if !isNativeCodexSlug(slug) {
			continue
		}
		templates.bySlug[slug] = row
		if templates.base == nil || (row["visibility"] == "list" && templates.base["visibility"] != "list") {
			templates.base = row
		}
	}
	return templates
}

func nativeCacheRows(rows []codexRow) []codexRow {
	kept := make([]codexRow, 0, len(rows))
	for _, row := range rows {
		slug, _ := row["slug"].(string)
		if isNativeCodexSlug(slug) {
			kept = append(kept, row)
		}
	}
	return kept
}

func isNativeCodexSlug(slug string) bool {
	return slug != "" && !strings.Contains(slug, "/") && !isReloCatalogSlug(slug)
}

func isReloCatalogSlug(slug string) bool {
	return strings.HasPrefix(slug, catalog.ClientPrefix) || strings.HasPrefix(slug, catalog.RoutePrefix)
}

func writeCodexCache(path string, models []codexRow) error {
	if models == nil {
		models = []codexRow{}
	}
	encoded, err := json.MarshalIndent(map[string]any{
		"fetched_at":     codexCacheFetchedAt,
		"client_version": codexCacheVersion,
		"models":         models,
	}, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(encoded, '\n'), 0o600)
}

func codexRowOf(model ModelRef, priority int, templates CodexTemplates) codexRow {
	if model.ChatGPT {
		if native, found := templates.bySlug[model.Upstream]; found {
			return nativeClone(model, priority, native)
		}
	}
	return routedClone(model, priority, templates.base)
}

func nativeClone(model ModelRef, priority int, native codexRow) codexRow {
	row := cloneRow(native)
	row["slug"] = model.ID
	row["display_name"] = codexDisplayName(model)
	row["priority"] = priority
	row["visibility"] = "list"
	row["upgrade"] = nil
	return row
}

func routedClone(model ModelRef, priority int, template codexRow) codexRow {
	row := fallbackRow()
	if template != nil {
		row = cloneRow(template)
	}
	// Codex refuses a catalog row that has neither base_instructions nor
	// model_messages.instructions_template. Native templates now keep the
	// prompt only under model_messages, which a routed clone must drop.
	instructions := routedInstructions(row, model)
	for _, field := range codexRoutedOnly {
		delete(row, field)
	}
	row["base_instructions"] = instructions
	row["slug"] = model.ID
	row["display_name"] = codexDisplayName(model)
	row["description"] = "Routed via Relo"
	row["priority"] = priority
	row["visibility"] = "list"
	row["upgrade"] = nil
	row["tool_mode"] = "code_mode_only"
	window := routedWindow(model)
	row["context_window"] = window
	row["max_context_window"] = window
	row["effective_context_window_percent"] = codexContextPercent
	row["input_modalities"] = codexModalities(model)
	row["supports_parallel_tool_calls"] = model.Tools == nil || *model.Tools
	applyRoutedReasoning(row, model)
	return row
}

func routedWindow(model ModelRef) int64 {
	if model.ContextWindow != nil && *model.ContextWindow > 0 {
		return *model.ContextWindow
	}
	return codexDefaultContext
}

func applyRoutedReasoning(row codexRow, model ModelRef) {
	if levels := codexLevelsFor(model); len(levels) > 0 {
		row["supported_reasoning_levels"] = levels
		row["default_reasoning_level"] = codexDefaultEffort(levels)
	} else {
		row["supported_reasoning_levels"] = []codexReasoningLevel{}
		delete(row, "default_reasoning_level")
	}
}

func fallbackRow() codexRow {
	return codexRow{
		"shell_type": "unified_exec", "supported_in_api": true,
		"base_instructions":                 codexBaseInstruction,
		"include_apps_usage_instructions":   true,
		"include_plugin_usage_instructions": false,
		"node_repl_disabled":                false,
		"node_repl_auto_review_required":    false,
		"supports_reasoning_summaries":      false,
		"supports_image_detail_original":    false,
		"default_reasoning_summary":         "none",
		"support_verbosity":                 true,
		"default_verbosity":                 "low",
		"apply_patch_tool_type":             "freeform",
		"truncation_policy":                 map[string]any{"mode": "tokens", "limit": 10000},
		"experimental_supported_tools":      []string{},
		"comp_hash":                         "relo",
	}
}

func routedInstructions(row codexRow, model ModelRef) string {
	identity := routedIdentity(model)
	if model.Connection == catalog.RouteLabelPrefix {
		return identity
	}
	if text := instructionText(row); text != "" {
		return codexIdentity.ReplaceAllLiteralString(text, identity)
	}
	return identity
}

func instructionText(row codexRow) string {
	if text, ok := row["base_instructions"].(string); ok && strings.TrimSpace(text) != "" {
		return text
	}
	messages, ok := row["model_messages"].(map[string]any)
	if !ok {
		return ""
	}
	text, _ := messages["instructions_template"].(string)
	return text
}

func cloneRow(row codexRow) codexRow {
	encoded, err := json.Marshal(row)
	if err != nil {
		return codexRow{}
	}
	clone := codexRow{}
	if err := json.Unmarshal(encoded, &clone); err != nil {
		return codexRow{}
	}
	return clone
}

func routedIdentity(model ModelRef) string {
	if model.Connection == catalog.RouteLabelPrefix {
		name := strings.TrimSpace(model.Name)
		if name == "" {
			name = strings.TrimSpace(model.ID)
		}
		if name == "" {
			return "You are a coding agent, powered by Relo."
		}
		return fmt.Sprintf("You are a coding agent, powered by the Relo %s.", name)
	}
	name := strings.TrimSpace(model.Upstream)
	if name == "" || len(name) > identityMaxLength {
		return neutralIdentity
	}
	for _, character := range name {
		plain := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || strings.ContainsRune(identityPunctuation, character)
		if !plain {
			return neutralIdentity
		}
	}
	return fmt.Sprintf("You are a coding agent powered by the %[1]s. If asked which model you are, identify as %[1]s. "+
		"Do not claim to be a different model or to have a different creator.", name)
}

func codexDisplayName(model ModelRef) string {
	return ConfigDisplayName(model)
}

func codexModalities(model ModelRef) []string {
	if model.Vision != nil && *model.Vision {
		return []string{"text", "image"}
	}
	return []string{"text"}
}

type codexReasoningLevel struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

func codexLevelsFor(model ModelRef) []codexReasoningLevel {
	if model.Reasoning == nil || !*model.Reasoning {
		return nil
	}
	efforts := model.ReasoningEfforts
	if efforts == nil {
		efforts = []string{"low", "medium", "high", "xhigh"}
	}
	levels := make([]codexReasoningLevel, 0, len(efforts))
	for _, effort := range efforts {
		levels = append(levels, codexReasoningLevel{Effort: effort, Description: codexEffortDescription(effort)})
	}
	return levels
}

func codexDefaultEffort(levels []codexReasoningLevel) string {
	for _, level := range levels {
		if level.Effort == "medium" {
			return "medium"
		}
	}
	return levels[0].Effort
}

func codexEffortDescription(effort string) string {
	switch effort {
	case "low":
		return "Fast responses with lighter reasoning"
	case "medium":
		return "Balances speed and reasoning depth for everyday tasks"
	case "high":
		return "Greater reasoning depth for complex problems"
	case "xhigh":
		return "Extra high reasoning depth for complex problems"
	default:
		return effort
	}
}
