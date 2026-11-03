package capture

import (
	"bytes"
	"testing"
)

func TestHeadTailKeepsBothEnds(t *testing.T) {
	body := []byte("abcdefghijklmnopqrstuvwxyz")
	kept, truncated := HeadTail(body, 10)
	if !truncated {
		t.Fatal("truncated = false, want the middle dropped")
	}
	if string(kept) != "abcde"+"vwxyz" {
		t.Fatalf("kept = %q, want the first and last five bytes", kept)
	}
}

func TestHeadTailKeepsASmallBodyWhole(t *testing.T) {
	body := []byte("0123456789")
	kept, truncated := HeadTail(body, 10)
	if truncated {
		t.Fatal("truncated = true, want a body exactly at the limit kept whole")
	}
	if !bytes.Equal(kept, body) {
		t.Fatalf("kept = %q, want the body unchanged", kept)
	}
}

func TestSinkHeadTailAcrossChunkBoundaries(t *testing.T) {
	body := []byte("abcdefghijklmnopqrstuvwxyz")
	sink := NewSink(10, true)
	for _, chunk := range [][]byte{body[:3], body[3:11], body[11:]} {
		if written, err := sink.Write(chunk); err != nil || written != len(chunk) {
			t.Fatalf("Write(%q) = %d, %v, want every byte reported", chunk, written, err)
		}
	}
	kept, truncated := sink.Bytes()
	if !truncated || string(kept) != "abcde"+"vwxyz" {
		t.Fatalf("Bytes() = %q, %v, want the head and tail across chunks", kept, truncated)
	}
}

func TestSinkKeepsABodyAtTheLimitWhole(t *testing.T) {
	body := []byte("0123456789")
	sink := NewSink(10, true)
	_, _ = sink.Write(body[:4])
	_, _ = sink.Write(body[4:])
	kept, truncated := sink.Bytes()
	if truncated || !bytes.Equal(kept, body) {
		t.Fatalf("Bytes() = %q, %v, want the whole body at the limit", kept, truncated)
	}
}

func TestSinkHeadOnlyNeverKeepsTheTail(t *testing.T) {
	sink := NewSink(5, false)
	_, _ = sink.Write([]byte("abcdefghij"))
	kept, truncated := sink.Bytes()
	if !truncated || string(kept) != "abcde" {
		t.Fatalf("Bytes() = %q, %v, want the head only", kept, truncated)
	}
}
