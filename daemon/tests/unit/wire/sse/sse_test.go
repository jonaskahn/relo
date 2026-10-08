package sse_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/sse"
)

func TestReaderNext(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []wire.SSEEvent
	}{
		{name: "parse single event", input: "data: hello\n\n", want: []wire.SSEEvent{{Data: "hello"}}},
		{name: "parse multi-line data event", input: "data: one\ndata: two\n\n", want: []wire.SSEEvent{{Data: "one\ntwo"}}},
		{name: "parse event with name and id", input: "event: ping\nid: 7\ndata: x\n\n", want: []wire.SSEEvent{{Name: "ping", ID: "7", Data: "x"}}},
		{name: "comment lines are ignored", input: ": keepalive\n\n"},
		{name: "comments between events leave both intact", input: "data: one\n\n: keep-alive\n\ndata: two\n\n", want: []wire.SSEEvent{{Data: "one"}, {Data: "two"}}},
		{name: "a comment inside an event keeps the name and payload", input: "event: ping\n: comment\ndata: x\n\n", want: []wire.SSEEvent{{Name: "ping", Data: "x"}}},
		{name: "empty data lines preserved", input: "data: \n\n", want: []wire.SSEEvent{{Data: ""}}},
		{name: "blank lines before an event are skipped", input: "\n\ndata: x\n\n", want: []wire.SSEEvent{{Data: "x"}}},
		{name: "unknown fields are ignored", input: "retry: 500\ndata: x\n\n", want: []wire.SSEEvent{{Data: "x"}}},
		{name: "trailing event without a blank line is delivered", input: "data: tail", want: []wire.SSEEvent{{Data: "tail"}}},
		{name: "two events in one stream", input: "data: one\n\ndata: two\n\n", want: []wire.SSEEvent{{Data: "one"}, {Data: "two"}}},
		{name: "data without a space keeps the payload", input: "data:nospace\n\n", want: []wire.SSEEvent{{Data: "nospace"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := sse.NewReader(strings.NewReader(tt.input))
			var got []wire.SSEEvent
			for {
				event, err := reader.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("Next() error = %v", err)
				}
				got = append(got, event)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("read %d events (%+v), want %d", len(got), got, len(tt.want))
			}
			for index := range got {
				if got[index] != tt.want[index] {
					t.Fatalf("event %d = %+v, want %+v", index, got[index], tt.want[index])
				}
			}
		})
	}

	t.Run("DONE sentinel returns EOF", func(t *testing.T) {
		reader := sse.NewReader(strings.NewReader("data: [DONE]\n\n"))
		if _, err := reader.Next(); !errors.Is(err, io.EOF) {
			t.Fatalf("Next() error = %v, want io.EOF", err)
		}
	})

	t.Run("oversized event is rejected", func(t *testing.T) {
		reader := sse.NewReader(strings.NewReader("data: " + strings.Repeat("x", 1<<20) + "\n\n"))
		if _, err := reader.Next(); !errors.Is(err, sse.ErrEventTooLarge) {
			t.Fatalf("Next() error = %v, want %v", err, sse.ErrEventTooLarge)
		}
	})

	t.Run("empty stream ends immediately", func(t *testing.T) {
		if _, err := sse.NewReader(strings.NewReader("")).Next(); !errors.Is(err, io.EOF) {
			t.Fatalf("Next() error = %v, want io.EOF", err)
		}
	})
}

func TestWriteEvent(t *testing.T) {
	t.Run("fields and flush are written", func(t *testing.T) {
		recorder := newFlushRecorder()
		event := wire.SSEEvent{Name: "message", ID: "7", Data: "one\ntwo"}
		if err := sse.WriteEvent(recorder, event); err != nil {
			t.Fatalf("WriteEvent() error = %v", err)
		}
		const want = "event: message\nid: 7\ndata: one\ndata: two\n\n"
		if recorder.Body.String() != want {
			t.Fatalf("body = %q, want %q", recorder.Body.String(), want)
		}
		if recorder.flushCount != 1 {
			t.Fatalf("flush count = %d, want 1", recorder.flushCount)
		}
	})

	t.Run("unnamed events omit the event field", func(t *testing.T) {
		recorder := newFlushRecorder()
		if err := sse.WriteEvents(recorder, []wire.SSEEvent{{Data: "a"}, {Data: "b"}}); err != nil {
			t.Fatalf("WriteEvents() error = %v", err)
		}
		if recorder.Body.String() != "data: a\n\ndata: b\n\n" {
			t.Fatalf("body = %q", recorder.Body.String())
		}
		if recorder.flushCount != 2 {
			t.Fatalf("flush count = %d, want 2", recorder.flushCount)
		}
	})

	t.Run("a failing writer reports an error", func(t *testing.T) {
		if err := sse.WriteEvent(failingWriter{}, wire.SSEEvent{Data: "x"}); err == nil {
			t.Fatal("WriteEvent() error = nil, want a write failure")
		}
	})

	t.Run("RequireFlusher rejects plain writers", func(t *testing.T) {
		if err := sse.RequireFlusher(plainWriter{}); !errors.Is(err, sse.ErrNoFlusher) {
			t.Fatalf("RequireFlusher() error = %v, want %v", err, sse.ErrNoFlusher)
		}
		if err := sse.RequireFlusher(newFlushRecorder()); err != nil {
			t.Fatalf("RequireFlusher() error = %v, want nil", err)
		}
	})
}

func TestRun(t *testing.T) {
	t.Run("transform runs per event and the sentinel closes the stream", func(t *testing.T) {
		recorder := newFlushRecorder()
		seen := 0
		err := sse.Run(sse.RunConfig{
			Upstream: strings.NewReader("data: one\n\ndata: two\n\ndata: [DONE]\n\n"),
			Writer:   recorder,
			Transform: func(event wire.SSEEvent) ([]wire.SSEEvent, error) {
				seen++
				return []wire.SSEEvent{{Data: strings.ToUpper(event.Data)}}, nil
			},
			OnEOF: func() ([]wire.SSEEvent, error) {
				return []wire.SSEEvent{{Data: "closing"}}, nil
			},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if seen != 2 {
			t.Fatalf("transform saw %d events, want 2", seen)
		}
		if recorder.Body.String() != "data: ONE\n\ndata: TWO\n\ndata: closing\n\n" {
			t.Fatalf("body = %q", recorder.Body.String())
		}
	})

	t.Run("clean EOF without sentinel calls OnEOF", func(t *testing.T) {
		recorder := newFlushRecorder()
		err := sse.Run(sse.RunConfig{
			Upstream: strings.NewReader("data: one\n\n"),
			Writer:   recorder,
			OnEOF:    func() ([]wire.SSEEvent, error) { return []wire.SSEEvent{{Data: "terminal"}}, nil },
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if recorder.Body.String() != "data: one\n\ndata: terminal\n\n" {
			t.Fatalf("body = %q", recorder.Body.String())
		}
	})

	t.Run("a silent upstream still closes the client stream", func(t *testing.T) {
		recorder := newFlushRecorder()
		if err := sse.Run(sse.RunConfig{Upstream: strings.NewReader(""), Writer: recorder}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(recorder.Body.String(), "stream_error") {
			t.Fatalf("body = %q, want the synthesized error frame", recorder.Body.String())
		}
	})

	t.Run("an OnEOF failure emits an error frame", func(t *testing.T) {
		recorder := newFlushRecorder()
		err := sse.Run(sse.RunConfig{
			Upstream: strings.NewReader(""),
			Writer:   recorder,
			OnEOF:    func() ([]wire.SSEEvent, error) { return nil, errors.New("terminal repair failed") },
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(recorder.Body.String(), "terminal repair failed") {
			t.Fatalf("body = %q, want the repair failure", recorder.Body.String())
		}
	})

	t.Run("transform error emits error event", func(t *testing.T) {
		recorder := newFlushRecorder()
		err := sse.Run(sse.RunConfig{
			Upstream: strings.NewReader("data: one\n\n"),
			Writer:   recorder,
			Transform: func(wire.SSEEvent) ([]wire.SSEEvent, error) {
				return nil, errors.New("cannot translate")
			},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(recorder.Body.String(), "cannot translate") {
			t.Fatalf("body = %q, want the transform failure", recorder.Body.String())
		}
	})

	t.Run("upstream error mid-stream emits error event", func(t *testing.T) {
		recorder := newFlushRecorder()
		err := sse.Run(sse.RunConfig{Upstream: &failingReader{}, Writer: recorder})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(recorder.Body.String(), "stream_error") {
			t.Fatalf("body = %q, want the read failure", recorder.Body.String())
		}
	})

	t.Run("custom error encoder renders the failure", func(t *testing.T) {
		recorder := newFlushRecorder()
		err := sse.Run(sse.RunConfig{
			Upstream: &failingReader{},
			Writer:   recorder,
			OnError: func(cause error) []wire.SSEEvent {
				return []wire.SSEEvent{{Data: "custom:" + cause.Error()}}
			},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(recorder.Body.String(), "custom:connection reset") {
			t.Fatalf("body = %q, want the custom frame", recorder.Body.String())
		}
	})

	t.Run("missing flusher is an error", func(t *testing.T) {
		err := sse.Run(sse.RunConfig{Upstream: strings.NewReader(""), Writer: plainWriter{}})
		if !errors.Is(err, sse.ErrNoFlusher) {
			t.Fatalf("Run() error = %v, want %v", err, sse.ErrNoFlusher)
		}
	})

	t.Run("client write failures stop the relay", func(t *testing.T) {
		err := sse.Run(sse.RunConfig{Upstream: strings.NewReader("data: one\n\n"), Writer: failingFlusher{}})
		if err == nil {
			t.Fatal("Run() error = nil, want a write failure")
		}
	})

	t.Run("multi-byte UTF-8 split across chunks", func(t *testing.T) {
		payload := "data: {\"text\":\"héllo \xF0\x9F\x98\x80\"}\n\n"
		recorder := newFlushRecorder()
		err := sse.Run(sse.RunConfig{
			Upstream: &chunkedReader{chunks: splitAtMultiByte(payload)},
			Writer:   recorder,
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(recorder.Body.String(), "héllo ") {
			t.Fatalf("body = %q, want the intact multi-byte payload", recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), "\xF0\x9F\x98\x80") {
			t.Fatalf("body = %q, want the emoji bytes intact", recorder.Body.String())
		}
	})

	t.Run("events reach the client before the upstream ends", func(t *testing.T) {
		recorder := newFlushRecorder()
		upstream := &streamingReader{events: []string{"data: one\n\n", "data: two\n\n"}}
		err := sse.Run(sse.RunConfig{Upstream: upstream, Writer: recorder})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if recorder.flushCount < 2 {
			t.Fatalf("flush count = %d, want one flush per event", recorder.flushCount)
		}
	})
}

// flushRecorder counts the flushes WriteEvent performs.
type flushRecorder struct {
	httptest.ResponseRecorder
	flushCount int
}

func newFlushRecorder() *flushRecorder {
	newRecorder := flushRecorder{}
	recorder := &newRecorder
	recorder.Body = &bytes.Buffer{}
	return recorder
}

func (f *flushRecorder) Flush() {
	f.flushCount++
	f.ResponseRecorder.Flush()
}

// plainWriter implements http.ResponseWriter but cannot flush.
type plainWriter struct{}

func (plainWriter) Header() http.Header { return http.Header{} }
func (plainWriter) Write(body []byte) (int, error) {
	return len(body), nil
}
func (plainWriter) WriteHeader(int) {}

type failingWriter struct{}

func (failingWriter) Header() http.Header       { return http.Header{} }
func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("client gone") }
func (failingWriter) WriteHeader(int)           {}

type failingFlusher struct{}

func (failingFlusher) Header() http.Header       { return http.Header{} }
func (failingFlusher) Write([]byte) (int, error) { return 0, errors.New("client gone") }
func (failingFlusher) WriteHeader(int)           {}
func (failingFlusher) Flush()                    {}

// failingReader fails after the first event.
type failingReader struct {
	done bool
}

func (r *failingReader) Read(buffer []byte) (int, error) {
	if !r.done {
		r.done = true
		return copy(buffer, "data: one\n\n"), nil
	}
	return 0, errors.New("connection reset")
}

// chunkedReader hands out pre-split chunks, one per read.
type chunkedReader struct {
	chunks []string
	index  int
}

func (r *chunkedReader) Read(buffer []byte) (int, error) {
	if r.index >= len(r.chunks) {
		return 0, io.EOF
	}
	chunk := r.chunks[r.index]
	r.index++
	return copy(buffer, chunk), nil
}

// streamingReader delivers one event per read.
type streamingReader struct {
	events []string
	index  int
}

func (r *streamingReader) Read(buffer []byte) (int, error) {
	if r.index >= len(r.events) {
		return 0, io.EOF
	}
	chunk := r.events[r.index]
	r.index++
	return copy(buffer, chunk), nil
}

// splitAtMultiByte cuts the payload inside its first multi-byte rune, so
// the reader has to keep bytes across reads.
func splitAtMultiByte(payload string) []string {
	raw := []byte(payload)
	for index := range raw {
		if raw[index] >= 0x80 {
			return []string{string(raw[:index+1]), string(raw[index+1:])}
		}
	}
	return []string{payload}
}
