package templates

import (
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
)

func TestOpenCodeFreeKeepsModelsTheGatewayMarksFree(t *testing.T) {
	for _, entry := range []struct {
		id   string
		kept bool
		why  string
	}{
		{"exo-free", true, "the suffix states the lane"},
		{"space-bunny-free", true, "the suffix states the lane"},
		{"mimo-v2.6-flash-free", true, "the suffix states the lane"},
		{"big-pickle", true, "the gateway publishes it free without saying so"},
		{"union-alpha", false, "the gateway publishes it on a wire this connection does not speak"},
		{"gpt-5.5", false, "a paid model is not free access"},
		{"big-pickle-preview", false, "an exception covers one id, not a family"},
		{"freemodel", false, "a name that merely contains free is not a model"},
		{"", false, "a blank id is not a model"},
		{"jev-1.13-free", false, "Jev answers on an endpoint Relo does not speak"},
	} {
		if got := freeModel(entry.id); got != entry.kept {
			t.Errorf("freeModel(%q) = %t, want %t: %s", entry.id, got, entry.kept, entry.why)
		}
	}
}

func TestOpenCodeFreeAnswersEachModelOnItsOwnProtocol(t *testing.T) {
	for _, entry := range []struct {
		id     string
		format catalog.APIFormat
	}{
		{"muse-spark-1.3-contributor-free", catalog.FormatOpenAIResp},
		{"muse-spark-1.2-contributor-free", catalog.FormatOpenAIResp},
		{"exo-free", catalog.FormatOpenAIChat},
		{"big-pickle", catalog.FormatOpenAIChat},
		{"EXO-FREE", catalog.FormatOpenAIChat},
	} {
		if got := freeModelFormat(entry.id); got != entry.format {
			t.Errorf("freeModelFormat(%q) = %q, want %q", entry.id, got, entry.format)
		}
	}
}

func TestOpenCodeFreeOnlyAnswersStreams(t *testing.T) {
	// The gateway refuses a one-shot request on this lane, so the connection
	// has to say so rather than let a client asking for one complete body send
	// the form that is refused.
	template, found := Get("opencode-free", nil)
	if !found {
		t.Fatal("opencode-free is not registered")
	}
	if !template.RequiresStream {
		t.Fatal("opencode-free does not declare that its upstream only streams")
	}
}

func TestOpenCodeFreeFilterRunsWithoutTheModelCatalog(t *testing.T) {
	template, found := Get("opencode-free", nil)
	if !found || template.FilterModels == nil {
		t.Fatal("opencode-free has no model filter")
	}
	if !template.FilterModels("big-pickle") {
		t.Fatal("a free model was dropped without a catalog copy")
	}
	if template.FilterModels("gpt-5.5") {
		t.Fatal("a paid model was kept without a catalog copy")
	}
}
