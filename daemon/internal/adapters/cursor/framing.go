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
// payloads and the unconsumed remainder. An end-stream frame ends the
// sequence; its payload carries the Connect error envelope when present.
func SplitFrames(buffered []byte) (payloads [][]byte, end bool, rest []byte, err error) {
	rest = buffered
	for len(rest) >= framePrefix {
		flags := rest[0]
		size := binary.BigEndian.Uint32(rest[1:5])
		if size > MaxFrameBytes {
			return nil, false, buffered, ErrFrameTooLarge
		}
		if uint64(len(rest)-framePrefix) < uint64(size) {
			return payloads, false, rest, nil
		}
		payload := rest[framePrefix : framePrefix+size]
		rest = rest[framePrefix+size:]
		switch {
		case flags&0x01 != 0:
			return nil, false, buffered, ErrFrameCompress
		case flags&frameEnd != 0:
			return payloads, true, rest, nil
		case flags != frameData:
			return nil, false, buffered, ErrFrameTruncated
		default:
			payloads = append(payloads, payload)
		}
	}
	return payloads, false, rest, nil
}

// FrameReader reads Connect frames from a stream.
type FrameReader struct {
	source  io.Reader
	buffer  []byte
	pending [][]byte
	ended   bool
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
		payloads, end, rest, err := SplitFrames(r.buffer)
		if err != nil {
			return nil, false, err
		}
		r.buffer = rest
		if end {
			r.ended = true
		}
		if len(payloads) > 0 {
			r.pending = payloads[1:]
			return payloads[0], false, nil
		}
		if end {
			return nil, true, nil
		}
		chunk := make([]byte, 32<<10)
		read, readErr := r.source.Read(chunk)
		if read > 0 {
			r.buffer = append(r.buffer, chunk[:read]...)
			continue
		}
		if readErr != nil {
			if readErr == io.EOF {
				if len(r.buffer) == 0 {
					r.ended = true
					return nil, true, nil
				}
				return nil, false, ErrFrameTruncated
			}
			return nil, false, readErr
		}
	}
}
