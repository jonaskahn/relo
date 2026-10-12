// Capturing a duplex Cursor exchange: what it targeted and why it stopped.
package upstream

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/cursor"
)

// captureCursorExchange records the target and headers of one duplex attempt.
// The codec writes its Connect frames straight into the stream as it builds
// them, so there is no buffered body to keep: what an operator needs from a
// turn that went wrong is where it went and under which credential.
func captureCursorExchange(ex Exchange) *CapturedMessage {
	base := strings.TrimSuffix(firstNonEmpty(ex.Options.BaseURL, cursor.DefaultBaseURL), "/")
	headers := http.Header{}
	for name, value := range cursor.Headers(ex.Options.CredentialRef, "", "") {
		headers.Set(name, value)
	}
	headers.Set("Content-Type", "application/connect+proto")
	return &CapturedMessage{
		Method: http.MethodPost, URL: base + cursor.RunPath, Headers: RedactHeaders(headers),
	}
}

// captureCursorRefusal records what ended a duplex attempt. Cursor answers a
// refused turn with a Connect error rather than a status, so the refusal text
// is the only record of what the upstream said.
func captureCursorRefusal(runErr error) *CapturedMessage {
	if runErr == nil {
		return nil
	}
	captured := &CapturedMessage{Status: refusalStatus(runErr), Body: RedactBody([]byte(runErr.Error()))}
	var refusal cursor.EndStream
	if errors.As(runErr, &refusal) {
		captured.Body = RedactBody([]byte(refusal.Error()))
	}
	return captured
}

// firstNonEmpty returns the first value that is set.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
