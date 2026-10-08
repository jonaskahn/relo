package activity

import "errors"

// ErrCaptureNotFound reports a capture this state database does not hold.
var ErrCaptureNotFound = errors.New("request capture not found")

// CaptureHeader is one stored capture as a listing reports it, without the
// body: the body of a large request is only read when an operator asks for
// it.
type CaptureHeader struct {
	ID        int64
	Ordinal   *int
	Kind      string
	Method    string
	URL       string
	Status    int
	Headers   map[string][]string
	BodyBytes int64
	Truncated bool
}

// CaptureBody is the body of one stored capture.
type CaptureBody struct {
	Body      []byte
	BodyBytes int64
	Truncated bool
}
