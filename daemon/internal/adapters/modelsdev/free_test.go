package modelsdev

import (
	"encoding/json"
	"testing"
)

func TestParseReadsProviderAndListingFreeMarks(t *testing.T) {
	raw := []byte(`{"gateway":{"id":"gateway","name":"Gateway","free":true,"models":{"marked":{"id":"marked"},"paid":{"id":"paid","free":false}}}}`)
	var document map[string]rawProvider
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	provider := parseIndexProvider("gateway", document["gateway"])
	if provider.IsFree == nil || !*provider.IsFree {
		t.Fatal("provider free mark was not read")
	}
	if provider.Models["marked"].IsFree == nil || !*provider.Models["marked"].IsFree {
		t.Fatal("a model without its own mark inherits the provider mark")
	}
	if provider.Models["paid"].IsFree == nil || *provider.Models["paid"].IsFree {
		t.Fatal("a listing's own false mark overrides the provider mark")
	}
}
