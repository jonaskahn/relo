// Claude Desktop profile comparison: parsed values, not formatting.
package codingclients

import "testing"

func TestDesktopProfileListsComparesRenderedValues(t *testing.T) {
	const base = "http://127.0.0.1:10202"
	models := []ModelRef{{ID: "claude-relo-a--gpt-5.5-pro", Upstream: "gpt-5.5-pro", Name: "GPT Pro"}}
	content, err := DesktopProfileContent(base, "rlo_ak_test", models)
	if err != nil {
		t.Fatalf("DesktopProfileContent() error = %v", err)
	}
	if !DesktopProfileLists(string(content), base, models) {
		t.Fatal("the rendered profile does not list its own models")
	}
	if !DesktopProfileLists(string(content), base+"/", models) {
		t.Fatal("a trailing slash changed the comparison")
	}
	if DesktopProfileLists(string(content), "http://127.0.0.1:10203", models) {
		t.Fatal("a different gateway base was accepted")
	}
	if DesktopProfileLists(string(content), base, []ModelRef{{ID: "claude-relo-a--gpt-5.5-mini", Upstream: "gpt-5.5-mini"}}) {
		t.Fatal("a different model roster was accepted")
	}
	if DesktopProfileLists("not json", base, models) {
		t.Fatal("a malformed profile was accepted")
	}

	// The same values in another formatting and with unrelated settings and a
	// different credential still describe the same list.
	reformatted := []byte(`{"theme":"dark","inferenceGatewayApiKey":"other","inferenceModels":` +
		`[{"name":"` + desktopModels(models)[0].Name + `","labelOverride":"GPT Pro",` +
		`"anthropicFamilyTier":"opus","behavesAs":"claude-opus-4-6","isFamilyDefault":true}],` +
		`"modelDiscoveryEnabled":false,"inferenceGatewayBaseUrl":"` + base + `"}`)
	if !DesktopProfileLists(string(reformatted), base, models) {
		t.Fatal("the same list in another formatting was rejected")
	}

	old := []byte(`{"inferenceGatewayBaseUrl":"` + base + `","modelDiscoveryEnabled":false,` +
		`"inferenceModels":[{"name":"` + desktopModels(models)[0].Name + `","labelOverride":"GPT Pro","anthropicFamilyTier":"opus"}]}`)
	if DesktopProfileLists(string(old), base, models) {
		t.Fatal("a profile without behavesAs was accepted")
	}
	discovery := []byte(`{"inferenceGatewayBaseUrl":"` + base + `","modelDiscoveryEnabled":true,` +
		`"inferenceModels":[{"name":"` + desktopModels(models)[0].Name + `","labelOverride":"GPT Pro","anthropicFamilyTier":"opus","behavesAs":"claude-opus-4-6"}]}`)
	if DesktopProfileLists(string(discovery), base, models) {
		t.Fatal("a profile with discovery enabled was accepted")
	}
}

// TestDesktopProfileListsSeesAShippedTableChange pins the migration trigger
// the roster digest cannot see: the same model list renders a different
// behavior after the shipped table changes.
func TestDesktopProfileListsSeesAShippedTableChange(t *testing.T) {
	const base = "http://127.0.0.1:10202"
	models := []ModelRef{{ID: "claude-sonnet-4-6", Upstream: "claude-sonnet-4-6"}}
	content, err := DesktopProfileContent(base, "rlo_ak_test", models)
	if err != nil {
		t.Fatalf("DesktopProfileContent() error = %v", err)
	}
	if !DesktopProfileLists(string(content), base, models) {
		t.Fatal("the rendered profile does not list its own models")
	}
	before := ModelsDigest(models)
	delete(desktopSupportedBehaviors, "claude-sonnet-4-6")
	t.Cleanup(func() { desktopSupportedBehaviors["claude-sonnet-4-6"] = true })

	if ModelsDigest(models) != before {
		t.Fatal("the roster digest moved with a shipped table change")
	}
	if DesktopProfileLists(string(content), base, models) {
		t.Fatal("the profile was accepted after its rendered behavior changed")
	}
}
