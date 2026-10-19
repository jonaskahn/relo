package server

import (
	"testing"

	apptemplates "github.com/jonaskahn/relo/internal/application/templatesettings"
)

func TestTemplateSettingResponseCarriesToolCutoff(t *testing.T) {
	response := toTemplateSettingResponse(apptemplates.Setting{ToolCutoff: true})
	if !response.ToolCutoff {
		t.Fatal("tool_cutoff = false, want the stored switch on the API response")
	}
	off := false
	patch := toTemplateSettingPatch(templateSettingPatchRequest{ToolCutoff: &off})
	if patch.ToolCutoff == nil || *patch.ToolCutoff {
		t.Fatalf("patch tool cutoff = %v, want the supplied off switch", patch.ToolCutoff)
	}
}
