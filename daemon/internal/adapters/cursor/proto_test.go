package cursor

import (
	"bytes"
	"testing"
)

func TestProtoRoundTrip(t *testing.T) {
	var out Writer
	out.String(1, "claude-4-5-sonnet")
	out.Bool(7, true)
	out.Uint32(15, 200000)
	inner := func() []byte {
		var nested Writer
		nested.String(1, "id")
		nested.String(2, "high")
		return nested.BytesOut()
	}()
	out.Message(3, inner)
	out.Message(3, inner)

	fields, err := Parse(out.BytesOut())
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if FirstString(fields, 1) != "claude-4-5-sonnet" {
		t.Fatalf("model = %q", FirstString(fields, 1))
	}
	if !Flag(fields, 7) {
		t.Fatal("maxMode flag lost")
	}
	if Uint(fields, 15) != 200000 {
		t.Fatalf("context = %d", Uint(fields, 15))
	}
	nested, err := Nested(fields, 3)
	if err != nil {
		t.Fatalf("Nested() error = %v", err)
	}
	if len(nested) != 2 || FirstString(nested[0], 2) != "high" {
		t.Fatalf("nested = %+v", nested)
	}
}

func TestProtoRefusals(t *testing.T) {
	if _, err := Parse(bytes.Repeat([]byte{0xff}, 9)); err == nil {
		t.Fatal("Parse() accepted a malformed varint")
	}
	if _, err := Parse([]byte{0x0a}); err == nil {
		t.Fatal("Parse() accepted a truncated message")
	}
	oversized := append([]byte{0x12, 0xff, 0xff, 0xff, 0xff, 0x0f}, bytes.Repeat([]byte("x"), 16)...)
	if _, err := Parse(oversized); err == nil {
		t.Fatal("Parse() accepted an oversized length")
	}
	if _, err := Parse([]byte{0x1b, 0x01}); err == nil {
		t.Fatal("Parse() accepted an unknown wire type")
	}
}

func TestFrameRoundTrip(t *testing.T) {
	payload := []byte("protobuf-bytes")
	framed := Frame(payload)
	payloads, end, rest, err := SplitFrames(framed)
	if err != nil || end != nil || len(rest) != 0 {
		t.Fatalf("SplitFrames() = %v %q %q %v", len(payloads), end, rest, err)
	}
	if len(payloads) != 1 || !bytes.Equal(payloads[0], payload) {
		t.Fatalf("payloads = %q", payloads)
	}
}

func TestFrameReaderStreams(t *testing.T) {
	var stream bytes.Buffer
	stream.Write(Frame([]byte("first")))
	stream.Write(Frame([]byte("second")))
	reader := NewFrameReader(&stream)
	for _, want := range []string{"first", "second"} {
		payload, ended, err := reader.Next()
		if err != nil || ended || string(payload) != want {
			t.Fatalf("Next() = %q %v %v, want %q", payload, ended, err, want)
		}
	}
	if _, ended, err := reader.Next(); err != nil || !ended {
		t.Fatalf("Next() = %v %v, want end of stream", ended, err)
	}
	if payload, ok := reader.End(); ok {
		t.Fatalf("End() = %q, want no end payload on a closed stream", payload)
	}
}

// TestFrameReaderKeepsTheEndPayload covers the refusal a Cursor stream ends
// with: the end-of-stream frame is the only place the service says why it
// stopped, so the payload must survive the read.
func TestFrameReaderKeepsTheEndPayload(t *testing.T) {
	reason := []byte(`{"error":{"code":"unknown","message":"model not found"}}`)
	end := append([]byte{frameEnd, 0, 0, 0, byte(len(reason))}, reason...)
	var stream bytes.Buffer
	stream.Write(Frame([]byte("first")))
	stream.Write(end)
	reader := NewFrameReader(&stream)
	if payload, _, err := reader.Next(); err != nil || string(payload) != "first" {
		t.Fatalf("Next() = %q %v", payload, err)
	}
	if _, ended, err := reader.Next(); err != nil || !ended {
		t.Fatalf("Next() = %v %v, want end of stream", ended, err)
	}
	payload, ok := reader.End()
	if !ok || !bytes.Equal(payload, reason) {
		t.Fatalf("End() = %q %v, want the refusal payload", payload, ok)
	}
	if refusal, ok := ParseEndStream(payload); !ok || refusal.Message != "model not found" {
		t.Fatalf("ParseEndStream() = %+v %v", refusal, ok)
	}
}

func TestFrameReaderRefusals(t *testing.T) {
	oversized := []byte{0x00, 0xff, 0xff, 0xff, 0xff}
	if _, _, _, err := SplitFrames(oversized); err == nil {
		t.Fatal("SplitFrames() accepted an oversized frame")
	}
	compressed := []byte{0x01, 0x00, 0x00, 0x00, 0x01, 0x00}
	if _, _, _, err := SplitFrames(compressed); err == nil {
		t.Fatal("SplitFrames() accepted a compressed frame")
	}
	reader := NewFrameReader(bytes.NewReader([]byte{0x00, 0x00}))
	if _, _, err := reader.Next(); err == nil {
		t.Fatal("Next() accepted a truncated frame")
	}
}
