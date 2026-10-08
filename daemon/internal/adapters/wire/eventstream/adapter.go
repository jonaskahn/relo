// Package eventstream is the AWS event-stream frame codec: it cuts the
// length-prefixed messages Bedrock streams apart.
package eventstream

import (
	"bytes"
	"io"
)

// Adapter wraps a binary eventstream io.Reader into an SSE-formatted io.Reader.
type Adapter struct {
	decoder *Decoder
	buf     bytes.Buffer
	eof     bool
}

// NewAdapter creates an Adapter from an eventstream reader.
func NewAdapter(r io.Reader) *Adapter {
	return &Adapter{
		decoder: NewDecoder(r),
	}
}

// Read implements io.Reader.
func (a *Adapter) Read(p []byte) (int, error) {
	for a.buf.Len() == 0 && !a.eof {
		msg, err := a.decoder.DecodeNext()
		if err != nil {
			a.eof = true
			if err == io.EOF {
				return 0, io.EOF
			}
			return 0, err
		}

		eventType := msg.Headers[":event-type"]
		if eventType != "" {
			a.buf.WriteString("event: ")
			a.buf.WriteString(eventType)
			a.buf.WriteString("\n")
		}
		a.buf.WriteString("data: ")
		a.buf.Write(msg.Payload)
		a.buf.WriteString("\n\n")
	}

	if a.buf.Len() > 0 {
		return a.buf.Read(p)
	}

	return 0, io.EOF
}
