// Connect framing for Cursor RPCs.
package cursor

import (
	"encoding/binary"
	"errors"
	"io"
)

// Framing errors name the malformed envelopes the reader refuses.
var (
	ErrFrameTruncated = errors.New("truncated connect frame")
	ErrFrameTooLarge  = errors.New("connect frame exceeds the size limit")
	ErrFrameCompress  = errors.New("compressed connect frame is not supported")
)

const (
	frameData = 0x00
	frameEnd  = 0x02

	framePrefix = 5
)

// Frame wraps one protobuf payload in a Connect data envelope.
func Frame(payload []byte) []byte {
	framed := make([]byte, 0, framePrefix+len(payload))
	framed = append(framed, frameData, 0, 0, 0, 0)
	binary.BigEndian.PutUint32(framed[1:5], uint32(len(payload)))
	return append(framed, payload...)
}

// SplitFrames decodes every complete frame in buffered bytes, returning the
// payloads, the unconsumed remainder, and the end-of-stream payload when the
// sequence ended. The end payload carries the Connect error envelope that says
// why the server stopped, which is the only reason a truncated turn gives.
func SplitFrames(buffered []byte) (payloads [][]byte, end []byte, rest []byte, err error) {
	rest = buffered
	for len(rest) >= framePrefix {
		flags := rest[0]
		size := binary.BigEndian.Uint32(rest[1:5])
		if size > MaxFrameBytes {
			return nil, nil, buffered, ErrFrameTooLarge
		}
		if uint64(len(rest)-framePrefix) < uint64(size) {
			return payloads, nil, rest, nil
		}
		payload := rest[framePrefix : framePrefix+size]
		rest = rest[framePrefix+size:]
		switch {
		case flags&0x01 != 0:
			return nil, nil, buffered, ErrFrameCompress
		case flags&frameEnd != 0:
			// A clean end sends an empty payload, which must stay non-nil:
			// it is how the reader tells an end frame from no end frame.
			return payloads, append([]byte{}, payload...), rest, nil
		case flags != frameData:
			return nil, nil, buffered, ErrFrameTruncated
		default:
			payloads = append(payloads, payload)
		}
	}
	return payloads, nil, rest, nil
}

// FrameReader reads Connect frames from a stream.
type FrameReader struct {
	source  io.Reader
	buffer  []byte
	pending [][]byte
	ended   bool
	// end is the end-of-stream payload, read once the sequence ends. It
	// names why the server stopped; nil means it stopped cleanly.
	end []byte
}

// NewFrameReader returns a frame reader over one response body.
func NewFrameReader(source io.Reader) *FrameReader {
	return &FrameReader{source: source}
}

// Next returns the next data payload, or ended when the stream closed.
func (r *FrameReader) Next() (payload []byte, ended bool, err error) {
	if len(r.pending) > 0 {
		payload = r.pending[0]
		r.pending = r.pending[1:]
		return payload, false, nil
	}
	if r.ended {
		return nil, true, nil
	}
	for {
		payload, ended, err := r.nextBuffered()
		if err != nil || ended || payload != nil {
			return payload, ended, err
		}
		if err := r.fill(); err != nil {
			if err == io.EOF {
				return nil, true, nil
			}
			return nil, false, err
		}
	}
}

// End returns the end-of-stream payload the server sent, and whether one
// arrived at all. A stream that closed without one said nothing about why.
func (r *FrameReader) End() ([]byte, bool) {
	return r.end, r.end != nil
}

func (r *FrameReader) nextBuffered() ([]byte, bool, error) {
	payloads, end, rest, err := SplitFrames(r.buffer)
	if err != nil {
		return nil, false, err
	}
	r.buffer = rest
	if end != nil {
		r.ended = true
		r.end = end
	}
	if len(payloads) > 0 {
		r.pending = payloads[1:]
		return payloads[0], false, nil
	}
	if end != nil {
		return nil, true, nil
	}
	return nil, false, nil
}

func (r *FrameReader) fill() error {
	chunk := make([]byte, 32<<10)
	read, readErr := r.source.Read(chunk)
	if read > 0 {
		r.buffer = append(r.buffer, chunk[:read]...)
		return nil
	}
	if readErr == nil {
		return nil
	}
	if readErr == io.EOF {
		if len(r.buffer) == 0 {
			r.ended = true
			return io.EOF
		}
		return ErrFrameTruncated
	}
	return readErr
}
