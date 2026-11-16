// Claude Code's model picker: the catalog the vendor serves for the `cc`
// surface, with Relo's models added so they can be chosen without a settings
// file. Every failure hands the client the vendor's own bytes.
package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// CatalogRow is one Relo model offered in the picker.
type CatalogRow struct {
	ID   string
	Name string
	// ContextWindow is the model's window in tokens, zero when unknown.
	ContextWindow int64
}

const (
	// maxCatalogBytes bounds a catalog Relo buffers, before and after the rewrite.
	maxCatalogBytes = 16 << 20
	// maxCatalogRowBytes and maxCatalogAddedBytes bound what one rewrite adds.
	maxCatalogRowBytes   = 256 << 10
	maxCatalogAddedBytes = 2 << 20
	maxCatalogAddedRows  = 4096
)

var (
	modelSelectorPath = regexp.MustCompile(`^/api/(?:organizations/[^/]+/)?model_selector/cc/?$`)
	bootstrapPath     = regexp.MustCompile(`^/api/claude_cli/bootstrap/?$`)
	// desktopEntrypoint matches the processes Claude Desktop starts. They share
	// the catalog file a plain `claude` reads, but Relo's rows are not for them.
	desktopEntrypoint = regexp.MustCompile(`^claude-cli/\S+ \(external, (claude-desktop|claude-desktop-3p|local-agent)[,)]`)
	// strippedRowKeys are what a cloned row must not inherit from the model it
	// was copied from: they describe that model, not the one added.
	strippedRowKeys = []string{
		"disabled", "disabled_reason", "badge", "tooltip", "description", "fast_mode",
		"notice", "selection_notice",
	}
	versionKey = regexp.MustCompile(`(?i)version`)
)

type catalogKind int

const (
	notCatalog catalogKind = iota
	kindModelSelector
	kindBootstrap
)

// catalogKindOf reports which of the two catalogs a request asks for. Only a
// plain GET qualifies, and a model selector request from a process Claude
// Desktop started is left alone.
func catalogKindOf(r *http.Request) catalogKind {
	if r.Method != http.MethodGet {
		return notCatalog
	}
	switch {
	case modelSelectorPath.MatchString(r.URL.Path):
		if desktopEntrypoint.MatchString(r.Header.Get("User-Agent")) {
			return notCatalog
		}
		return kindModelSelector
	case bootstrapPath.MatchString(r.URL.Path):
		return kindBootstrap
	}
	return notCatalog
}

// serveCatalog relays a catalog request and adds Relo's rows to a good answer.
// Accept-Encoding is dropped from the request so the transport negotiates gzip
// itself and hands back decoded bytes; every other path stays a verbatim relay.
func (p *Proxy) serveCatalog(w http.ResponseWriter, r *http.Request, kind catalogKind) {
	response, err := p.fetchCatalog(r)
	if err != nil {
		writeError(w, http.StatusBadGateway, "the upstream is unavailable")
		return
	}
	defer func() { _ = response.Body.Close() }()
	rows := p.opts.Catalog()
	if len(rows) == 0 || response.StatusCode < 200 || response.StatusCode > 299 {
		relay(w, response, nil)
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxCatalogBytes+1))
	if err != nil || len(body) > maxCatalogBytes {
		// Too big or cut short: what was read goes first, the rest follows as it is.
		relay(w, response, body)
		return
	}
	rewritten, added := rewriteCatalog(kind, body, rows)
	if added == 0 {
		relay(w, response, body)
		return
	}
	p.opts.Logger.Debug("proxy catalog", "path", r.URL.Path, "added", added)
	copyHeaders(w.Header(), response.Header)
	for _, name := range []string{"Etag", "Digest", "Content-Md5"} {
		w.Header().Del(name)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(rewritten)
}

func (p *Proxy) fetchCatalog(r *http.Request) (*http.Response, error) {
	target := strings.TrimRight(p.opts.Upstream, "/") + r.URL.RequestURI()
	outbound, err := http.NewRequestWithContext(r.Context(), r.Method, target, nil)
	if err != nil {
		return nil, err
	}
	copyHeaders(outbound.Header, r.Header)
	outbound.Header.Del("Accept-Encoding")
	return p.client.Do(outbound)
}

// relay answers with the vendor's response as it came, after any bytes of its
// body that were already read.
func relay(w http.ResponseWriter, response *http.Response, read []byte) {
	copyHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	flush(w, io.MultiReader(bytes.NewReader(read), response.Body))
}

// rewriteCatalog adds the rows to a catalog body and counts how many it added.
// A body it cannot read, a shape it does not know, or a limit the additions
// would pass returns the body unchanged with a count of zero.
func rewriteCatalog(kind catalogKind, body []byte, rows []CatalogRow) ([]byte, int) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var parsed any
	if err := decoder.Decode(&parsed); err != nil {
		return body, 0
	}
	root, isObject := parsed.(map[string]any)
	if !isObject {
		return body, 0
	}
	var added int
	if kind == kindBootstrap {
		added = addBootstrapOptions(root, rows)
	} else {
		added = addSurfaceRows(root, rows)
	}
	if added == 0 {
		return body, 0
	}
	encoded, err := json.Marshal(root)
	if err != nil || len(encoded) > maxCatalogBytes {
		return body, 0
	}
	return encoded, added
}

// addBootstrapOptions appends rows to additional_model_options, the list the
// CLI falls back to when it has no served catalog.
func addBootstrapOptions(root map[string]any, rows []CatalogRow) int {
	current, present := root["additional_model_options"]
	options, isList := current.([]any)
	if present && current != nil && !isList {
		return 0
	}
	known := listedOptions(options)
	var added []any
	budget := newCatalogBudget()
	for _, row := range rows {
		if known[row.ID] {
			continue
		}
		entry := map[string]any{"model": row.ID, "name": row.Name, "description": ""}
		if !budget.take(entry) {
			return 0
		}
		known[row.ID] = true
		added = append(added, entry)
	}
	if len(added) == 0 {
		return 0
	}
	root["additional_model_options"] = append(options, added...)
	return len(added)
}

func listedOptions(options []any) map[string]bool {
	known := map[string]bool{}
	for _, option := range options {
		if entry, ok := option.(map[string]any); ok {
			if id, ok := entry["model"].(string); ok {
				known[id] = true
			}
		}
	}
	return known
}

// addSurfaceRows clones a healthy Claude row into the `cc` surface's models.
// The surface sits at the top level, under `config`, or in a
// `model_selector_config` list, depending on which endpoint answered.
func addSurfaceRows(root map[string]any, rows []CatalogRow) int {
	surfaces := ccSurfaces(root)
	// Every surface is planned before any is changed, so a refusal cannot leave
	// a partial list behind.
	budget := newCatalogBudget()
	plans := make([][]any, len(surfaces))
	total := 0
	for index, surface := range surfaces {
		additions, ok := planSurface(surface, rows, budget)
		if !ok {
			return 0
		}
		plans[index] = additions
		total += len(additions)
	}
	for index, surface := range surfaces {
		if len(plans[index]) > 0 {
			surface["models"] = append(surface["models"].([]any), plans[index]...)
		}
	}
	return total
}

// ccSurfaces finds where the body keeps the `cc` surface's model list.
func ccSurfaces(root map[string]any) []map[string]any {
	var surfaces []map[string]any
	if cc(root) {
		surfaces = append(surfaces, root)
	}
	if config, ok := root["config"].(map[string]any); ok && cc(config) {
		surfaces = append(surfaces, config)
	}
	if list, ok := root["model_selector_config"].([]any); ok {
		for _, item := range list {
			if surface, ok := item.(map[string]any); ok && cc(surface) {
				surfaces = append(surfaces, surface)
			}
		}
	}
	return surfaces
}

func cc(surface map[string]any) bool {
	_, hasModels := surface["models"].([]any)
	return hasModels && surface["id"] == "cc"
}

func planSurface(surface map[string]any, rows []CatalogRow, budget *catalogBudget) ([]any, bool) {
	models, _ := surface["models"].([]any)
	template, known := templateRow(models)
	if template == nil {
		return nil, true
	}
	var additions []any
	for _, row := range rows {
		if known[row.ID] {
			continue
		}
		clone := cloneRow(template, row)
		if !budget.take(clone) {
			return nil, false
		}
		known[row.ID] = true
		additions = append(additions, clone)
	}
	return additions, true
}

// templateRow picks the first usable Claude row and answers the ids already
// listed, so a model the vendor lists is never added a second time.
func templateRow(models []any) (map[string]any, map[string]bool) {
	known := map[string]bool{}
	var template map[string]any
	for _, item := range models {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := entry["id"].(string)
		known[id] = true
		if template != nil || !strings.HasPrefix(id, "claude-") || entry["section"] == "deprecated" {
			continue
		}
		if disabled, _ := entry["disabled"].(bool); disabled || entry["disabled_reason"] != nil {
			continue
		}
		template = entry
	}
	return template, known
}

// cloneRow copies a vendor row without what describes that model, then names
// the Relo model. The copy is deep, so no row shares structure with another.
func cloneRow(template map[string]any, row CatalogRow) map[string]any {
	clone := make(map[string]any, len(template))
	for key, value := range template {
		if dropsFromClone(key) {
			continue
		}
		clone[key] = deepCopy(value)
	}
	clone["id"], clone["name"], clone["section"] = row.ID, row.Name, "main"
	if _, has := clone["short_name"]; has {
		clone["short_name"] = row.Name
	}
	if row.ContextWindow > 0 {
		clone["context_window"] = row.ContextWindow
		if runtime, ok := clone["runtime"].(map[string]any); ok {
			runtime["max_input_tokens"] = row.ContextWindow
		}
	}
	return clone
}

func dropsFromClone(key string) bool {
	if versionKey.MatchString(key) {
		return true
	}
	for _, stripped := range strippedRowKeys {
		if key == stripped {
			return true
		}
	}
	return key == "id" || key == "name" || key == "section" || key == "context_window"
}

func deepCopy(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copied := make(map[string]any, len(typed))
		for key, item := range typed {
			copied[key] = deepCopy(item)
		}
		return copied
	case []any:
		copied := make([]any, len(typed))
		for index, item := range typed {
			copied[index] = deepCopy(item)
		}
		return copied
	}
	return value
}

// catalogBudget bounds what one rewrite adds to a catalog.
type catalogBudget struct {
	rows  int
	bytes int
}

func newCatalogBudget() *catalogBudget { return &catalogBudget{} }

// take reserves room for one row, and reports false when it would not fit.
func (b *catalogBudget) take(row any) bool {
	encoded, err := json.Marshal(row)
	if err != nil || len(encoded) > maxCatalogRowBytes {
		return false
	}
	b.rows++
	b.bytes += len(encoded) + 1
	return b.rows <= maxCatalogAddedRows && b.bytes <= maxCatalogAddedBytes
}
