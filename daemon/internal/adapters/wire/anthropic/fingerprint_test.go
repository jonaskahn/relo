package anthropic

import (
	"bytes"
	"fmt"
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
