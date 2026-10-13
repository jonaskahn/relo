package antigravity

import (
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
)

func TestServes(t *testing.T) {
	prefixes := DefaultPrefixes()
	if !Serves(catalog.Listed{ID: "gemini-3.8-flash"}, prefixes) {
		t.Fatal("Serves(gemini) = false, want the family kept")
	}
	if !Serves(catalog.Listed{ID: "claude-sonnet-4-6"}, prefixes) {
		t.Fatal("Serves(claude) = false, want the family kept")
	}
	if Serves(catalog.Listed{ID: "gpt-oss-120b-medium"}, prefixes) {
		t.Fatal("Serves(gpt) = true, want the other family dropped")
	}
	if Serves(catalog.Listed{ID: "tab_flash_lite_preview"}, prefixes) {
		t.Fatal("Serves(tab) = true, want the helper dropped")
	}
	if Serves(catalog.Listed{ID: "gemini-3.5-flash-low"}, prefixes) {
		t.Fatal("Serves(retired) = true, want the retired SKU dropped")
	}
}

func TestLogical(t *testing.T) {
	logical, found := Logical("gemini-3.8-flash-high")
	if !found || logical != "gemini-3.8-flash" {
		t.Fatalf("Logical() = %q, %v, want the grouped model", logical, found)
	}
	if _, found := Logical("gemini-9.9-flash-high"); found {
		t.Fatal("Logical() found an unknown SKU, want no row")
	}
	if _, found := Logical("claude-sonnet-4-6"); found {
		t.Fatal("Logical() found a Claude model, want no row")
	}
	if Variants("gemini-3.8-flash") == nil {
		t.Fatal("Variants() = nil, want the band rows")
	}
	if Variants("claude-sonnet-4-6") != nil {
		t.Fatal("Variants() named a Claude model, want no rows")
	}
}
