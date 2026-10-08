package upstream

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestCaptureBytesKeepsTheLimitAndReportsTheRest covers the boundary a
// captured body is cut at: one byte under is whole, exactly at the limit is
// whole, and past it is truncated to the limit.
func TestCaptureBytesKeepsTheLimitAndReportsTheRest(t *testing.T) {
	cases := []struct {
		name      string
		size      int
		limit     int64
		wantLen   int
		truncated bool
	}{
		{"under the limit", 10, 16, 10, false},
		{"exactly the limit", 16, 16, 16, false},
		{"one past the limit", 17, 16, 16, true},
		{"far past the limit", 4000, 16, 16, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			body := bytes.Repeat([]byte("x"), testCase.size)
			kept, truncated := CaptureBytes(body, testCase.limit)
			if len(kept) != testCase.wantLen {
				t.Fatalf("kept %d bytes, want %d", len(kept), testCase.wantLen)
			}
			if truncated != testCase.truncated {
				t.Fatalf("truncated = %v, want %v", truncated, testCase.truncated)
			}
			if !bytes.Equal(kept, body[:testCase.wantLen]) {
				t.Fatal("the kept bytes are not the head of the body")
			}
		})
	}
}

// TestCaptureSinkNeverHoldsTheStreamBack covers the tee a live reply is
// captured through: every byte is reported written, so the relay is never
// delayed or cut short by the cap.
func TestCaptureSinkNeverHoldsTheStreamBack(t *testing.T) {
	sink := newCaptureSink(8)
	written, err := sink.Write([]byte("12345"))
	if err != nil || written != 5 {
		t.Fatalf("Write() = %d, %v, want 5 bytes reported", written, err)
	}
	written, err = sink.Write([]byte("67890"))
	if err != nil || written != 5 {
		t.Fatalf("Write() = %d, %v, want 5 bytes reported", written, err)
	}
	body, truncated := sink.bytes()
	if string(body) != "12345678" {
		t.Fatalf("captured %q, want the first eight bytes", body)
	}
	if !truncated {
		t.Fatal("truncated = false, want the bytes past the cap reported")
	}
}

// TestCaptureRequestLeavesTheSendUntouched covers the copy an attempt keeps:
// the body is still complete after it is read, the length is set, and the
// credential the provider is sent is not the one the capture holds.
func TestCaptureRequestLeavesTheSendUntouched(t *testing.T) {
	const body = "{\"model\":\"gpt-4o\",\"messages\":[]}"
	request, err := http.NewRequest(http.MethodPost, "https://api.example.test/v1/chat/completions",
		strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer sk-test")
	request.Header.Set("Content-Type", "application/json")

	captured, err := captureRequest(request, MaxCaptureBytes)
	if err != nil {
		t.Fatalf("captureRequest() error = %v", err)
	}
	if captured.Method != http.MethodPost || captured.URL != request.URL.String() {
		t.Fatalf("captured = %+v, want the method and target", captured)
	}
	if got := captured.Headers["Authorization"]; len(got) != 1 || got[0] != "Bearer "+maskedSecret {
		t.Fatalf("captured Authorization = %v, want the masked credential header", got)
	}
	if got := request.Header.Get("Authorization"); got != "Bearer sk-test" {
		t.Fatalf("live Authorization = %q, want the credential the provider is sent", got)
	}
	if string(captured.Body) != body {
		t.Fatalf("captured body = %q, want the encoded request", captured.Body)
	}
	if captured.Truncated {
		t.Fatal("a small body was reported truncated")
	}
	restored, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("read the restored body: %v", err)
	}
	if string(restored) != body {
		t.Fatalf("restored body = %q, want the request unchanged", restored)
	}
	if request.ContentLength != int64(len(body)) {
		t.Fatalf("ContentLength = %d, want %d", request.ContentLength, len(body))
	}
	if request.GetBody == nil {
		t.Fatal("GetBody = nil, want a body a retry could read again")
	}
}

// TestCaptureResponseKeepsStatusAndHeaders covers the reply side, whose body
// arrives through the sink the relay streamed it through.
func TestCaptureResponseKeepsStatusAndHeaders(t *testing.T) {
	response := &http.Response{StatusCode: http.StatusTooManyRequests,
		Header: http.Header{"Content-Type": []string{"application/json"}}}
	sink := newCaptureSink(MaxCaptureBytes)
	if _, err := sink.Write([]byte("{\"error\":\"slow down\"}")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	captured := captureResponse(response, sink)
	if captured.Status != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want the upstream status", captured.Status)
	}
	if string(captured.Body) != "{\"error\":\"slow down\"}" {
		t.Fatalf("body = %q, want the upstream error", captured.Body)
	}
	if got := captured.Headers["Content-Type"]; len(got) != 1 {
		t.Fatalf("headers = %v, want the upstream content type", captured.Headers)
	}
}
