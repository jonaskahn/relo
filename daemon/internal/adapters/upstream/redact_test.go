package upstream

import (
	"strings"
	"testing"
)

// TestRedactHeadersMasksWhatCarriesACredential covers the header side of a
// capture: the value of a header that names a credential is hidden, the scheme
// it was sent with stays, and every other header is untouched.
func TestRedactHeadersMasksWhatCarriesACredential(t *testing.T) {
	headers := map[string][]string{
		"Authorization":                {"Bearer eyJ0eXAiOiJhdCtqd3Q"},
		"X-Api-Key":                    {"rlo_ak_ace6d4259537986f46c61f9d9788a3ff_4FiMdVHp"},
		"x_goog_api_key":               {"AIzaSyExample"},
		"X-Xai-Token-Auth":             {"xai-grok-cli"},
		"X-Amz-Security-Token":         {"session"},
		"Cookie":                       {"session=abc123"},
		"Set-Cookie":                   {"__cf_bm=a3YZenU"},
		"Content-Type":                 {"application/json"},
		"X-Ratelimit-Remaining-Tokens": {"21"},
		"User-Agent":                   {"relo-grok/1.0.13"},
	}
	redacted := RedactHeaders(headers)

	masked := map[string]string{
		"Authorization":        "Bearer " + maskedSecret,
		"X-Api-Key":            maskedSecret,
		"x_goog_api_key":       maskedSecret,
		"X-Xai-Token-Auth":     maskedSecret,
		"X-Amz-Security-Token": maskedSecret,
		"Cookie":               maskedSecret,
		"Set-Cookie":           maskedSecret,
	}
	for name, want := range masked {
		if got := redacted[name]; len(got) != 1 || got[0] != want {
			t.Fatalf("%s = %v, want %q", name, got, want)
		}
	}
	kept := map[string]string{
		"Content-Type":                 "application/json",
		"X-Ratelimit-Remaining-Tokens": "21",
		"User-Agent":                   "relo-grok/1.0.13",
	}
	for name, want := range kept {
		if got := redacted[name]; len(got) != 1 || got[0] != want {
			t.Fatalf("%s = %v, want %q", name, got, want)
		}
	}
}

// TestRedactHeadersCopiesEveryValue covers the aliasing a live message would
// otherwise share with its own log.
func TestRedactHeadersCopiesEveryValue(t *testing.T) {
	headers := map[string][]string{"Content-Type": {"application/json"}}
	redacted := RedactHeaders(headers)
	headers["Content-Type"][0] = "text/plain"
	if redacted["Content-Type"][0] != "application/json" {
		t.Fatalf("redacted headers = %v, want the value at capture time", redacted)
	}
}

// TestRedactBodyMasksCredentialFields covers the body side: a field that names
// a credential loses its value, and a field that only mentions tokens keeps
// its own.
func TestRedactBodyMasksCredentialFields(t *testing.T) {
	body := []byte("{\"api_key\":\"sk-live-123\",\"max_tokens\":1024,\"tokens\":[1,2]," +
		"\"nested\":{\"client_secret\":\"shh\"},\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}]}")
	redacted := string(RedactBody(body))
	for _, leaked := range []string{"sk-live-123", "shh"} {
		if strings.Contains(redacted, leaked) {
			t.Fatalf("redacted body = %s, want %q hidden", redacted, leaked)
		}
	}
	for _, kept := range []string{"\"max_tokens\":1024", "\"tokens\":[1,2]", "\"content\":\"hi\""} {
		if !strings.Contains(redacted, kept) {
			t.Fatalf("redacted body = %s, want %s kept", redacted, kept)
		}
	}
	if !strings.Contains(redacted, maskedSecret) {
		t.Fatalf("redacted body = %s, want the mask in place of the value", redacted)
	}
}

// TestRedactBodyLeavesEverythingElseByteForByte covers the fidelity a log is
// read for: a body with no credential field is copied, not rewritten.
func TestRedactBodyLeavesEverythingElseByteForByte(t *testing.T) {
	for _, body := range []string{
		"{\"model\":\"gpt-4o\",\"stream\":true,\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}",
		"not json at all {\"model\":",
		"",
	} {
		if got := string(RedactBody([]byte(body))); got != body {
			t.Fatalf("RedactBody(%q) = %q, want the body unchanged", body, got)
		}
	}
}

// TestRedactBodyNeverAliasesTheBytes covers the corruption masking in place
// would cause: the bytes handed in are the ones the live path still reads.
func TestRedactBodyNeverAliasesTheBytes(t *testing.T) {
	body := []byte("{\"api_key\":\"sk-live-123\"}")
	redacted := RedactBody(body)
	if string(body) != "{\"api_key\":\"sk-live-123\"}" {
		t.Fatalf("input body = %s, want it untouched", body)
	}
	redacted[1] = 'X'
	if string(body) != "{\"api_key\":\"sk-live-123\"}" {
		t.Fatalf("input body = %s, want it detached from the capture", body)
	}
}
