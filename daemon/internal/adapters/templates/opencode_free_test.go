package templates

import (
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/modelsdev"
	"github.com/jonaskahn/relo/internal/catalog"
)

func TestOpenCodeFreeKeepsModelsWithoutAPaidInputPrice(t *testing.T) {
	free := int64(0)
	paid := int64(1)
	index := &modelsdev.Index{Providers: map[string]modelsdev.Provider{
		"opencode": {Models: map[string]modelsdev.Model{
			"free": {Prices: catalog.Prices{Input: &free}},
			"paid": {Prices: catalog.Prices{Input: &paid}},
			"long": {Prices: catalog.Prices{Input: &free, ExtInput: &paid}},
		}},
	}}

	template, found := Get("opencode-free", index)
	if !found || template.FilterModels == nil {
		t.Fatal("opencode-free has no model filter")
	}
	if !template.FilterModels("free") {
		t.Fatal("a zero input price was dropped")
	}
	if template.FilterModels("paid") || template.FilterModels("long") || template.FilterModels("missing") {
		t.Fatal("a paid or unknown model was kept")
	}

	empty, found := Get("opencode-free", nil)
	if !found || empty.FilterModels == nil || empty.FilterModels("free") {
		t.Fatal("a missing catalog kept a model")
	}
}
