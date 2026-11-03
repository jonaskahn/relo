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

func TestBytesKeepsTheHeadAtTheLimit(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		limit     int64
		want      string
		truncated bool
	}{
		{"under the limit", "abc", 5, "abc", false},
		{"exactly at the limit", "abcde", 5, "abcde", false},
		{"over the limit", "abcdefgh", 5, "abcde", true},
		{"zero limit", "abc", 0, "", true},
		{"negative limit", "abc", -1, "", true},
		{"empty body", "", 5, "", false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			kept, truncated := Bytes([]byte(testCase.body), testCase.limit)
			if string(kept) != testCase.want || truncated != testCase.truncated {
				t.Fatalf("Bytes(%q, %d) = %q, %v, want %q, %v",
					testCase.body, testCase.limit, kept, truncated, testCase.want, testCase.truncated)
			}
		})
	}
}

func TestHeadTailSplitsAnOddLimitAndClampsNegativeLimits(t *testing.T) {
	body := []byte("0123456789")
	kept, truncated := HeadTail(body, 5)
	if !truncated || string(kept) != "01"+"789" {
		t.Fatalf("HeadTail(odd) = %q, %v, want the smaller head and larger tail", kept, truncated)
	}
	kept, truncated = HeadTail(body, -1)
	if !truncated || len(kept) != 0 {
		t.Fatalf("HeadTail(negative) = %q, %v, want an empty truncated capture", kept, truncated)
	}
}

func TestNewSinkClampsANegativeLimit(t *testing.T) {
	sink := NewSink(-1, true)
	written, err := sink.Write([]byte("abc"))
	if err != nil || written != 3 {
		t.Fatalf("Write() = %d, %v, want every byte reported", written, err)
	}
	kept, truncated := sink.Bytes()
	if !truncated || len(kept) != 0 {
		t.Fatalf("Bytes() = %q, %v, want an empty truncated capture", kept, truncated)
	}
}

func TestSinkHeadOnlyReportsBytesPastAZeroLimit(t *testing.T) {
	sink := NewSink(0, false)
	if written, err := sink.Write([]byte("abc")); err != nil || written != 3 {
		t.Fatalf("Write() = %d, %v, want every byte reported", written, err)
	}
	kept, truncated := sink.Bytes()
	if !truncated || len(kept) != 0 {
		t.Fatalf("Bytes() = %q, %v, want an empty truncated capture", kept, truncated)
	}
}

func TestSinkHeadTailMovesAFilledBodyIntoTheRing(t *testing.T) {
	sink := NewSink(10, true)
	for _, chunk := range []string{"0123456", "789abcdef"} {
		if written, err := sink.Write([]byte(chunk)); err != nil || written != len(chunk) {
			t.Fatalf("Write(%q) = %d, %v, want every byte reported", chunk, written, err)
		}
	}
	kept, truncated := sink.Bytes()
	if !truncated || string(kept) != "01234bcdef" {
		t.Fatalf("Bytes() = %q, %v, want the head and the last five bytes", kept, truncated)
	}
}

func TestSinkHeadTailWrapsTheRingOnceItIsFull(t *testing.T) {
	sink := NewSink(10, true)
	for _, chunk := range []string{"0123456789", "a"} {
		_, _ = sink.Write([]byte(chunk))
	}
	kept, truncated := sink.Bytes()
	if !truncated || string(kept) != "01234"+"6789a" {
		t.Fatalf("Bytes() = %q, %v, want the wrapped ring as the tail", kept, truncated)
	}
}

func TestSinkHeadTailWithAZeroLimitDropsEverything(t *testing.T) {
	sink := NewSink(0, true)
	if written, err := sink.Write([]byte("abc")); err != nil || written != 3 {
		t.Fatalf("Write() = %d, %v, want every byte reported", written, err)
	}
	kept, truncated := sink.Bytes()
	if !truncated || len(kept) != 0 {
		t.Fatalf("Bytes() = %q, %v, want an empty truncated capture", kept, truncated)
	}
}

func TestSinkBytesReturnsTheWholeBodyWithoutExposingItsBuffer(t *testing.T) {
	sink := NewSink(10, true)
	_, _ = sink.Write([]byte("abc"))
	first, _ := sink.Bytes()
	first[0] = 'x'
	second, truncated := sink.Bytes()
	if truncated || !bytes.Equal(second, []byte("abc")) {
		t.Fatalf("Bytes() = %q, %v, want an independent copy of the body", second, truncated)
	}
}
