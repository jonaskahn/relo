// Credential redaction for headers and bodies.
package upstream

import (
	"bytes"
	"regexp"
	"strings"
)

// A capture is read, copied, and shared, so a key that reaches one is a key
// that leaks. Redaction happens on the copy a capture keeps and never on the
// message a provider or an agent sees: what the traffic carried is unchanged,
// and only the log is quieter.
//
// The console masks the same names in the same way, which is what covers a row
// an older daemon already stored.

const maskedSecret = "••••"

var sensitiveHeaderNames = map[string]bool{
	"authorization":      true,
	"proxyauthorization": true,
	"apikey":             true,
	"xapikey":            true,
	"xgoogapikey":        true,
	"xamzsecuritytoken":  true,
	"xapitoken":          true,
	"xauthtoken":         true,
	"xxaitokenauth":      true,
	"authentication":     true,
	"password":           true,
	"cookie":             true,
	"setcookie":          true,
}

var sensitiveHeaderSuffixes = []string{"apikey", "secret", "password", "token"}

var credentialField = regexp.MustCompile(
	`(?i)"(api[-_]?key|apikeys?|access[-_]?token|auth[-_]?token|refresh[-_]?token|id[-_]?token|token|secret|client[-_]?secret|password|authorization|credentials?)"(\s*:\s*)"[^"]*"`,
)

var schemePrefix = regexp.MustCompile(`^[A-Za-z]{2,12}\s+\S`)

// RedactHeaders copies headers with every credential value masked. A header
// set with nothing sensitive in it still comes back copied, so a capture never
// aliases the message the traffic is still using.
func RedactHeaders(headers map[string][]string) map[string][]string {
	redacted := make(map[string][]string, len(headers))
	for name, values := range headers {
		if !sensitiveHeader(name) {
			redacted[name] = append([]string(nil), values...)
			continue
		}
		masked := make([]string, len(values))
		for i, value := range values {
			masked[i] = maskCredential(value)
		}
		redacted[name] = masked
	}
	return redacted
}

// RedactBody copies a captured body with the value of every field that names a
// credential masked. A body that is not JSON, or holds no such field, comes
// back byte for byte: what an operator debugs is what was sent. The copy is
// always a new slice, because the bytes handed in are the ones the live
// request, the decoded refusal, or the relayed answer still reads.
func RedactBody(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	redacted := credentialField.ReplaceAll(body, []byte("$1$2\""+maskedSecret+"\""))
	if !bytes.Equal(redacted, body) {
		return redacted
	}
	return append([]byte(nil), body...)
}

func sensitiveHeader(name string) bool {
	normalized := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(name))
	if sensitiveHeaderNames[normalized] {
		return true
	}
	for _, suffix := range sensitiveHeaderSuffixes {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	return false
}

func maskCredential(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return value
	}
	if schemePrefix.MatchString(trimmed) {
		return strings.SplitN(trimmed, " ", 2)[0] + " " + maskedSecret
	}
	return maskedSecret
}
