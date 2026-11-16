// Claude Desktop's model picker: the bootstrap claude.ai serves the app, with
// Relo's models added to the Code tab's surfaces. Every failure hands the app
// the vendor's own bytes.
package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"regexp"
)

// desktopSurfaces are the model lists the Code tab of a local Desktop reads:
// `ccd`, and `code` when `ccd` carries none. `ccr`, the remote sessions, is left
// alone: a remote session never reaches this machine, so a model only Relo
// serves could not run there.
var desktopSurfaces = []string{"ccd", "code"}

var desktopBootstrapPath = regexp.MustCompile(`^/(?:edge-api|api)/bootstrap(?:/[A-Za-z0-9-]+/app_start)?/?$`)

// isDesktopBootstrap reports whether a request asks for the bootstrap. Only a
// plain GET qualifies.
func isDesktopBootstrap(r *http.Request) bool {
	return r.Method == http.MethodGet && desktopBootstrapPath.MatchString(r.URL.Path)
}

// rewriteDesktopBootstrap adds the rows to the Code surfaces of a bootstrap body
// and counts how many it added. A body it cannot read, a shape it does not know,
// or a limit the additions would pass returns the body unchanged with zero.
func rewriteDesktopBootstrap(body []byte, rows []CatalogRow) ([]byte, int) {
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
	added := addDesktopRows(root, rows)
	if added == 0 {
		return body, 0
	}
	encoded, err := json.Marshal(root)
	if err != nil || len(encoded) > maxCatalogBytes {
		return body, 0
	}
	return encoded, added
}

// addDesktopRows clones a healthy Claude row into each Code surface. Every
// surface is planned before any is changed, so a refusal cannot leave a partial
// list behind.
func addDesktopRows(root map[string]any, rows []CatalogRow) int {
	list, _ := root["model_selector_config"].([]any)
	var surfaces []map[string]any
	for _, item := range list {
		if surface, ok := item.(map[string]any); ok && isDesktopSurface(surface) {
			surfaces = append(surfaces, surface)
		}
	}
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

func isDesktopSurface(surface map[string]any) bool {
	if _, hasModels := surface["models"].([]any); !hasModels {
		return false
	}
	id, _ := surface["id"].(string)
	for _, listed := range desktopSurfaces {
		if id == listed {
			return true
		}
	}
	return false
}
