// Package capture bounds the bytes a capture keeps without cutting the live
// message it is read from.
package capture

import "bytes"

// MaxBytes is the state-sized capture bound: enough of an exchange to debug
// its shape, small enough that the database grows with request count.
const MaxBytes = 16 << 10

// Bytes keeps at most limit bytes from the head of one body, reporting whether
// more had to be left out.
func Bytes(body []byte, limit int64) ([]byte, bool) {
	if limit < 0 {
		limit = 0
	}
	if int64(len(body)) <= limit {
		return body, false
	}
	kept := make([]byte, limit)
	copy(kept, body[:limit])
	return kept, true
}

// HeadTail keeps the first half and the last half of limit bytes, dropping only
// the middle. A body that already fits is kept whole and is not truncated.
func HeadTail(body []byte, limit int64) ([]byte, bool) {
	if limit < 0 {
		limit = 0
	}
	if int64(len(body)) <= limit {
		return body, false
	}
	head, tail := headTailBytes(limit)
	kept := make([]byte, 0, limit)
	kept = append(kept, body[:head]...)
	kept = append(kept, body[int64(len(body))-tail:]...)
	return kept, true
}

// Sink collects a stream in one capture shape. It always reports every byte
// written, so the relay it is teed from is never held back by the bound.
type Sink struct {
	limit int64
	tail  bool

	// body holds everything while the stream still fits, and the head alone
	// once a head-only capture overflows. A head-and-tail capture moves to
	// head and ring when it overflows.
	body     bytes.Buffer
	overflow bool
	head     []byte
	ring     []byte
	ringAt   int
	ringFill int
}

// NewSink returns a sink that keeps a head-only or head-and-tail capture.
func NewSink(limit int64, tail bool) *Sink {
	if limit < 0 {
		limit = 0
	}
	return &Sink{limit: limit, tail: tail}
}

// Write keeps the bytes this capture shape needs and reports all of p written.
func (s *Sink) Write(p []byte) (int, error) {
	if !s.tail {
		s.writeHead(p)
		return len(p), nil
	}
	if !s.overflow && int64(s.body.Len()+len(p)) <= s.limit {
		_, _ = s.body.Write(p)
		return len(p), nil
	}
	if !s.overflow {
		s.startHeadTail()
	}
	s.writeTail(p)
	return len(p), nil
}

func (s *Sink) writeHead(p []byte) {
	room := s.limit - int64(s.body.Len())
	if room <= 0 {
		s.overflow = s.overflow || len(p) > 0
		return
	}
	if int64(len(p)) > room {
		s.overflow = true
		p = p[:room]
	}
	_, _ = s.body.Write(p)
}

// startHeadTail moves a capture that outgrew its bound to the head and ring.
func (s *Sink) startHeadTail() {
	s.overflow = true
	body := s.body.Bytes()
	headLen, tailLen := headTailBytes(s.limit)
	s.ring = make([]byte, tailLen)
	if int64(len(body)) <= headLen {
		s.head = append([]byte(nil), body...)
		return
	}
	s.head = append([]byte(nil), body[:headLen]...)
	for _, b := range body[headLen:] {
		s.pushTail(b)
	}
}

func (s *Sink) writeTail(p []byte) {
	headLen, _ := headTailBytes(s.limit)
	room := int(headLen) - len(s.head)
	if room > 0 {
		kept := min(room, len(p))
		s.head = append(s.head, p[:kept]...)
		p = p[kept:]
	}
	for _, b := range p {
		s.pushTail(b)
	}
}

// headTailBytes splits a head-and-tail bound so both ends of a body cut in two
// agree, including an odd bound.
func headTailBytes(limit int64) (int64, int64) {
	head := limit / 2
	return head, limit - head
}

func (s *Sink) pushTail(b byte) {
	if len(s.ring) == 0 {
		return
	}
	s.ring[s.ringAt] = b
	s.ringAt = (s.ringAt + 1) % len(s.ring)
	if s.ringFill < len(s.ring) {
		s.ringFill++
	}
}

// Bytes returns the captured body and whether the capture had to leave bytes
// out.
func (s *Sink) Bytes() ([]byte, bool) {
	if !s.overflow {
		return append([]byte(nil), s.body.Bytes()...), false
	}
	if !s.tail {
		return append([]byte(nil), s.body.Bytes()...), true
	}
	kept := make([]byte, 0, len(s.head)+s.ringFill)
	kept = append(kept, s.head...)
	return append(kept, s.tailBytes()...), true
}

func (s *Sink) tailBytes() []byte {
	if s.ringFill < len(s.ring) {
		return append([]byte(nil), s.ring[:s.ringFill]...)
	}
	tail := make([]byte, 0, len(s.ring))
	tail = append(tail, s.ring[s.ringAt:]...)
	return append(tail, s.ring[:s.ringAt]...)
}
