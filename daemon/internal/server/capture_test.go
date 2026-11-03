package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/upstream"
)

type testRedactor struct{}

func (testRedactor) MaxCaptureBytes() int64 { return upstream.MaxCaptureBytes }

func (testRedactor) RedactHeaders(headers map[string][]string) map[string][]string {
	return upstream.RedactHeaders(headers)
}

func (testRedactor) RedactBody(body []byte) []byte { return upstream.RedactBody(body) }

func (testRedactor) CaptureBytes(body []byte, limit int64, tail bool) ([]byte, bool) {
	return upstream.CaptureBytes(body, limit, tail)
}

// TestCaptureInboundKeepsTheRequestWholeUnderTheCaptureCap covers the two
// bounds one read has to hold apart: what the log keeps is cut at the capture
// cap, while the request handed on to the codec stays whole. One limit for
// both would cut real prompts down to a log excerpt.
func TestCaptureInboundKeepsTheRequestWholeUnderTheCaptureCap(t *testing.T) {
	body := strings.Repeat("a", 4*upstream.MaxCaptureBytes)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))

	captured := captureInbound(testRedactor{}, request, upstream.MaxCaptureBytes, true)

	if !captured.Truncated {
		t.Fatal("captured.Truncated = false, want the capture cut at the cap")
	}
	if len(captured.Body) != upstream.MaxCaptureBytes {
		t.Fatalf("capture holds %d bytes, want the %d byte cap", len(captured.Body), upstream.MaxCaptureBytes)
	}
	sent, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("read the request handed on: %v", err)
	}
	if string(sent) != body {
		t.Fatalf("request handed on holds %d bytes, want the whole %d", len(sent), len(body))
	}
}

func TestCaptureInboundKeepsASmallRequestWhole(t *testing.T) {
	body := `{"model":"gpt-4o"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))

	captured := captureInbound(testRedactor{}, request, upstream.MaxCaptureBytes, true)

	if captured.Truncated {
		t.Fatal("captured.Truncated = true, want a small capture kept whole")
	}
	if string(captured.Body) != body {
		t.Fatalf("capture = %q, want the whole body", captured.Body)
	}
}

func TestCaptureRowKeepsHeadAndTailOfEveryKind(t *testing.T) {
	server := &Server{opts: Options{CaptureRedactor: testRedactor{}}}
	head := bytes.Repeat([]byte("h"), upstream.MaxCaptureBytes/2)
	tail := bytes.Repeat([]byte("t"), upstream.MaxCaptureBytes/2)
	body := append(append(head, bytes.Repeat([]byte("m"), 1024)...), tail...)
	for _, kind := range []string{
		activity.CaptureAgentRequest, activity.CaptureAgentResponse,
		activity.CaptureProviderRequest, activity.CaptureProviderResponse,
	} {
		row := server.captureRow(nil, kind, &CapturedMessage{Body: body})
		if !row.Truncated {
			t.Fatalf("%s: Truncated = false, want the middle dropped", kind)
		}
		if len(row.Body) != upstream.MaxCaptureBytes {
			t.Fatalf("%s: body = %d bytes, want the %d-byte cap", kind, len(row.Body), upstream.MaxCaptureBytes)
		}
		want := append(append([]byte(nil), head...), tail...)
		if !bytes.Equal(row.Body, want) {
			t.Fatalf("%s: body does not hold the head and tail", kind)
		}
	}
}

func TestAgentAnswerKeepsAWholeBodyForTheTrafficLog(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 2*upstream.MaxCaptureBytes)
	answer := newAgentAnswer(httptest.NewRecorder(), testRedactor{}, trafficCaptureBytes, false)
	if _, err := answer.Write(body); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	captured, ok := answer.captured()
	if !ok || captured.Truncated || !bytes.Equal(captured.Body, body) {
		t.Fatalf("captured = %d bytes, truncated = %v, want the whole %d", len(captured.Body), captured.Truncated, len(body))
	}
}
