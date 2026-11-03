// Capture sink: bounded request and response recording.
package upstream

import (
	"bytes"
	"io"
	"net/http"

	capturelog "github.com/jonaskahn/relo/internal/capture"
)

// MaxCaptureBytes bounds one captured body. A larger body is kept up to the
// limit and marked truncated, and the request or the stream itself is never
// cut short because of the log. The cap is what keeps the state database
// proportional to the request count rather than to the traffic: a capture
// exists so an operator can see the shape of an exchange, not archive it.
const MaxCaptureBytes = capturelog.MaxBytes

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
	inner *capturelog.Sink
}

func newCaptureSink(limit int64, tail bool) *captureSink {
	return &captureSink{inner: capturelog.NewSink(limit, tail)}
}

// captureLimit is how much of one body the exchange keeps: the traffic-log
// bound when one is asked for, and the state-sized cap otherwise.
func (e Exchange) captureLimit() int64 {
	if e.CaptureLimit > 0 {
		return e.CaptureLimit
	}
	return MaxCaptureBytes
}

// Write collects the bytes of the stream and always reports them written, so
// the relay it is teed from is unaffected by the cap.
func (s *captureSink) Write(p []byte) (int, error) {
	return s.inner.Write(p)
}

func (s *captureSink) bytes() ([]byte, bool) {
	return s.inner.Bytes()
}

func captureRequest(request *http.Request, limit int64, tail bool) (*CapturedMessage, error) {
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
	message.Body, message.Truncated = CaptureBytes(body, limit, tail)
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

// CaptureBytes keeps the head of one body, or its head and tail when tail is
// set, reporting whether more had to be left out.
func CaptureBytes(body []byte, limit int64, tail bool) ([]byte, bool) {
	if tail {
		return capturelog.HeadTail(body, limit)
	}
	return capturelog.Bytes(body, limit)
}

func capturedBody(response *http.Response, body []byte, limit int64, tail bool) *CapturedMessage {
	kept, truncated := CaptureBytes(body, limit, tail)
	return &CapturedMessage{
		Status: response.StatusCode, Headers: RedactHeaders(response.Header),
		Body: RedactBody(kept), Truncated: truncated,
	}
}
