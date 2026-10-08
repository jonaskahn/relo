package eventstream

import (
	"bytes"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	headers := map[string]string{
		":message-type": "event",
		":event-type":   "chunk",
		":content-type": "application/json",
	}
	payload := []byte("{\"bytes\":\"hello world\"}")

	frame := EncodeFrame(headers, payload)
	decoder := NewDecoder(bytes.NewReader(frame))

	msg, err := decoder.DecodeNext()
	if err != nil {
		t.Fatalf("DecodeNext() failed: %v", err)
	}

	if msg.Headers[":event-type"] != "chunk" {
		t.Errorf("unexpected event-type: %s", msg.Headers[":event-type"])
	}
	if string(msg.Payload) != string(payload) {
		t.Errorf("expected payload %s, got %s", payload, msg.Payload)
	}
}

func TestCorruptCRC(t *testing.T) {
	headers := map[string]string{":message-type": "event"}
	payload := []byte("test")
	frame := EncodeFrame(headers, payload)

	// Corrupt prelude CRC
	corruptPrelude := append([]byte(nil), frame...)
	corruptPrelude[10] ^= 0xFF
	decoder := NewDecoder(bytes.NewReader(corruptPrelude))
	if _, err := decoder.DecodeNext(); err != ErrPreludeCRCMismatch {
		t.Errorf("expected ErrPreludeCRCMismatch, got %v", err)
	}

	// Corrupt message CRC
	corruptMsg := append([]byte(nil), frame...)
	corruptMsg[len(corruptMsg)-1] ^= 0xFF
	decoder2 := NewDecoder(bytes.NewReader(corruptMsg))
	if _, err := decoder2.DecodeNext(); err != ErrMessageCRCMismatch {
		t.Errorf("expected ErrMessageCRCMismatch, got %v", err)
	}
}
