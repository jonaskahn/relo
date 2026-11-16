// What proxy mode adds to Claude Code's model picker, and the cache it drops
// when that changes.
package platform

import (
	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/adapters/proxy"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
)

// proxyCatalogRows answers the rows the intercept adds to the picker: the same
// models env mode lists, under the ids Claude Code's picker accepts.
func proxyCatalogRows(models func() []appintegration.ModelRef) func() []proxy.CatalogRow {
	return func() []proxy.CatalogRow {
		listed := models()
		rows := make([]proxy.CatalogRow, 0, len(listed))
		for _, model := range listed {
			ref := clientModelRefs([]appintegration.ModelRef{model})[0]
			row := proxy.CatalogRow{ID: codingclients.PickerID(ref), Name: codingclients.ConfigDisplayName(ref)}
			if model.ContextWindow != nil {
				row.ContextWindow = *model.ContextWindow
			}
			rows = append(rows, row)
		}
		return rows
	}
}
