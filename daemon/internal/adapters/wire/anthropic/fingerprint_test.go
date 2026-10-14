package anthropic

import (
	"bytes"
	"fmt"
	"regexp"
	"testing"
)

func TestXXH64Empty(t *testing.T) {
	if got := xxh64(nil, 0); got != 0xEF46DB3751D8E999 {
		t.Fatalf("xxh64(empty) = %x, want ef46db3751d8e999", got)
	}
}

func TestPatchBillingCCHUsesLow20Bits(t *testing.T) {
	body := []byte(`{"system":"cch=00000"}`)
	patched := patchBillingCCH(body)
	token := fmt.Sprintf("cch=%05x", xxh64(body, cchSeed)&0xFFFFF)
	if !bytes.Contains(patched, []byte(token)) || bytes.Contains(patched, []byte(cchPlaceholder)) {
		t.Fatalf("patched = %s, want %s", patched, token)
	}
}

func TestStainlessNames(t *testing.T) {
	if stainlessOS("darwin") != "MacOS" || stainlessOS("linux") != "Linux" || stainlessArch("amd64") != "x64" || stainlessArch("arm64") != "arm64" {
		t.Fatalf("os = %s, arch = %s", stainlessOS("darwin"), stainlessArch("amd64"))
	}
}

func TestContextBetaApplies(t *testing.T) {
	cases := map[string]bool{
		"claude-opus-4-6":          true,
		"claude-sonnet-4-6":        true,
		"claude-sonnet-4-5":        true,
		"claude-haiku-3-5":         true,
		"claude-opus-4-7":          false,
		"claude-opus-5-0":          false,
		"":                         true,
		"not-a-model":              true,
		"claude-opus-4-5-20251101": true,
	}
	for model, want := range cases {
		if got := contextBetaApplies(model); got != want {
			t.Fatalf("contextBetaApplies(%q) = %v, want %v", model, got, want)
		}
	}
}

func TestNewRequestUUID(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := map[string]bool{}
	for range 100 {
		id := newRequestUUID()
		if !pattern.MatchString(id) {
			t.Fatalf("newRequestUUID() = %q, want a v4 UUID", id)
		}
		if seen[id] {
			t.Fatalf("newRequestUUID() repeated %q", id)
		}
		seen[id] = true
	}
}
