// Capture sink: bounded request and response recording.
package upstream

import (
	"bytes"
	"io"
	"net/http"
)

// MaxCaptureBytes bounds one captured body. A larger body is kept up to the
// limit and marked truncated, and the request or the stream itself is never
// cut short because of the log. The cap is what keeps the state database
// proportional to the request count rather than to the traffic: a capture
// exists so an operator can see the shape of an exchange, not archive it.
const MaxCaptureBytes = 16 << 10

// CapturedMessage is one HTTP message as the log keeps it: the method or the
// status, the target, every header, and the body bytes.
type CapturedMessage struct {
	Method    string
	URL       string
	Status    int
	Headers   map[string][]string
	Body      []byte
	Truncated bool
}

// Capture holds what one attempt sent to a provider and what that provider
// answered, which the caller stores beside the usage row of the request.
type Capture struct {
	Request  *CapturedMessage
	Response *CapturedMessage
}

type captureSink struct {
	body      bytes.Buffer
	limit     int64
	truncated bool
}

func newCaptureSink(limit int64) *captureSink {
	return &captureSink{limit: limit}
}

// Write collects the bytes of the stream and always reports them written, so
// the relay it is teed from is unaffected by the cap.
func (s *captureSink) Write(p []byte) (int, error) {
	room := s.limit - int64(s.body.Len())
	switch {
	case room <= 0:
		if len(p) > 0 {
			s.truncated = true
		}
	case int64(len(p)) > room:
		_, _ = s.body.Write(p[:room])
		s.truncated = true
	default:
		_, _ = s.body.Write(p)
	}
	return len(p), nil
}

func (s *captureSink) bytes() ([]byte, bool) {
	return s.body.Bytes(), s.truncated
}

func captureRequest(request *http.Request, limit int64) (*CapturedMessage, error) {
	message := &CapturedMessage{
		Method: request.Method, URL: request.URL.String(), Headers: RedactHeaders(request.Header),
	}
	if request.Body == nil {
		return message, nil
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return message, err
	}
	_ = request.Body.Close()
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	request.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	message.Body, message.Truncated = CaptureBytes(body, limit)
	message.Body = RedactBody(message.Body)
	return message, nil
}

func captureResponse(response *http.Response, sink *captureSink) *CapturedMessage {
	body, truncated := sink.bytes()
	return &CapturedMessage{
		Status: response.StatusCode, Headers: RedactHeaders(response.Header),
		Body: RedactBody(body), Truncated: truncated,
	}
}

// CaptureBytes keeps at most limit bytes of one body, reporting whether more
// had to be left out.
func CaptureBytes(body []byte, limit int64) ([]byte, bool) {
	if int64(len(body)) <= limit {
		return body, false
	}
	kept := make([]byte, limit)
	copy(kept, body[:limit])
	return kept, true
}

func capturedBody(response *http.Response, body []byte, limit int64) *CapturedMessage {
	kept, truncated := CaptureBytes(body, limit)
	if !truncated && int64(len(body)) >= limit {
		truncated = true
	}
	return &CapturedMessage{
		Status: response.StatusCode, Headers: RedactHeaders(response.Header),
		Body: RedactBody(kept), Truncated: truncated,
	}
}
