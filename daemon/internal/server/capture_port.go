// Capture redaction: the bounds and scrubbing captured messages pass through.
package server

// CaptureRedactor bounds one captured body and scrubs what the log keeps, so
// the transport records an exchange without keeping its secrets. The
// composition root backs it with the capture helpers, which this package
// never imports directly.
type CaptureRedactor interface {
	// MaxCaptureBytes bounds one captured body.
	MaxCaptureBytes() int64
	// RedactHeaders scrubs the secrets out of captured headers.
	RedactHeaders(headers map[string][]string) map[string][]string
	// RedactBody scrubs the secrets out of a captured body.
	RedactBody(body []byte) []byte
	// CaptureBytes keeps at most limit bytes of one body, reporting whether
	// more had to be left out.
	CaptureBytes(body []byte, limit int64) ([]byte, bool)
}
