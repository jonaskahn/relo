// Capturing a duplex Cursor exchange: what it targeted and why it stopped.
package upstream

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/cursor"
)

// captureCursorExchange records the target and headers of one duplex attempt.
// The framed Run request the codec writes is kept as the body, so the traffic
// log shows the exact bytes the upstream parsed.
func captureCursorExchange(ex Exchange) *CapturedMessage {
	base := strings.TrimSuffix(firstNonEmpty(ex.Options.BaseURL, cursor.DefaultBaseURL), "/")
	headers := http.Header{}
	for name, value := range cursor.Headers(ex.Options.CredentialRef, "", "") {
		// The capture rebuilds headers it never sent: an empty request id
		// would claim a header the turn did not carry, so it is left out.
		if value == "" {
			continue
		}
		headers.Set(name, value)
	}
	headers.Set("Content-Type", "application/connect+proto")
	return &CapturedMessage{
		Method: http.MethodPost, URL: base + cursor.RunPath, Headers: RedactHeaders(headers),
	}
}

// recordCursorFrame keeps the framed Run request one duplex attempt wrote.
// Binary frames pass redaction through byte for byte, and the tracing log
// keeps the full body: a refused turn is debugged from what was sent.
func recordCursorFrame(att attempt) func([]byte) {
	return func(frame []byte) {
		if att.capture == nil || att.capture.Request == nil {
			return
		}
		att.capture.Request.Body = RedactBody(frame)
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
