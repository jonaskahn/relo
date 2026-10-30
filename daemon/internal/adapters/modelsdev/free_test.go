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

func TestParseSnapshotKeepsProviderAndListingFreeMarks(t *testing.T) {
	raw := []byte(`{"providers":[{"id":"gateway","name":"Gateway","free":true,"models":[{"id":"inherited"},{"id":"paid","is_free":false}]}]}`)
	idx, err := parseIndex(raw)
	if err != nil {
		t.Fatalf("parseIndex() error = %v", err)
	}
	provider, found := idx.Providers["gateway"]
	if !found {
		t.Fatal("snapshot provider was not indexed")
	}
	if provider.IsFree == nil || !*provider.IsFree {
		t.Fatal("snapshot provider free mark was not read")
	}
	if provider.Models["inherited"].IsFree == nil || !*provider.Models["inherited"].IsFree {
		t.Fatal("a snapshot model without its own mark inherits the provider mark")
	}
	if provider.Models["paid"].IsFree == nil || *provider.Models["paid"].IsFree {
		t.Fatal("a snapshot model's own false mark overrides the provider mark")
	}
	if got := provider.FreeModels(); got != 2 {
		t.Fatalf("FreeModels() = %d, want the inherited model and the provider mark above the listing's false", got)
	}
}

func TestBoolMarksKeepTheFirstStatedValue(t *testing.T) {
	free, paid := true, false
	if got := firstBool(nil, &free, &paid); got == nil || !*got {
		t.Fatal("firstBool() skipped the first stated mark")
	}
	if got := firstBool(&paid); got == nil || *got {
		t.Fatal("firstBool() did not keep a stated false")
	}
	if got := firstBool(nil, nil); got != nil {
		t.Fatalf("firstBool() = %v, want unknown", got)
	}
}
