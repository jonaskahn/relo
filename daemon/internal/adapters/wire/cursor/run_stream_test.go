package cursor

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	adapterscursor "github.com/jonaskahn/relo/internal/adapters/cursor"
)

type reconnectTransport struct {
	attempts atomic.Int64
	failures int
	response []byte
}

func (t *reconnectTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if int(t.attempts.Add(1)) <= t.failures {
		_ = request.Body.Close()
		return nil, io.ErrClosedPipe
	}
	if err := consumeFirstFrame(request.Body); err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{},
		Body: io.NopCloser(bytes.NewReader(t.response)), Request: request,
	}, nil
}

func consumeFirstFrame(body io.Reader) error {
	header := make([]byte, 5)
	if _, err := io.ReadFull(body, header); err != nil {
		return err
	}
	_, err := io.CopyN(io.Discard, body, int64(binary.BigEndian.Uint32(header[1:])))
	return err
}

func TestRunRetriesOnceWhenTheStreamClosesBeforeHeaders(t *testing.T) {
	transport := &reconnectTransport{
		failures: 1,
		response: adapterscursor.Frame(serverUpdate(14, turnEndedPayload())),
	}
	runner := &Runner{
		Client: &http.Client{Transport: transport}, BaseURL: "https://cursor.test", Token: "token",
		Request: testRequest(), Selection: testSelection(),
	}
	if _, err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v, want the retried stream to finish", err)
	}
	if got := transport.attempts.Load(); got != 2 {
		t.Fatalf("attempts = %d, want one retry", got)
	}
}

func TestRunSurfacesASecondClosedStream(t *testing.T) {
	transport := &reconnectTransport{failures: 2}
	runner := &Runner{
		Client: &http.Client{Transport: transport}, BaseURL: "https://cursor.test", Token: "token",
		Request: testRequest(), Selection: testSelection(),
	}
	_, err := runner.Run(context.Background())
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Run() error = %v, want the closed pipe", err)
	}
	if got := transport.attempts.Load(); got != 2 {
		t.Fatalf("attempts = %d, want the one allowed retry", got)
	}
}

func newStreamRunner(server *httptest.Server) *Runner {
	return &Runner{
		Client: h2Client(server.Client()), BaseURL: server.URL, Token: "token",
		Request: testRequest(), Selection: testSelection(),
	}
}

func startStreamServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func beginTurn(w http.ResponseWriter, r *http.Request) *adapterscursor.FrameReader {
	w.Header().Set("Content-Type", "application/connect+proto")
	w.WriteHeader(http.StatusOK)
	w.(http.Flusher).Flush()
	reader := adapterscursor.NewFrameReader(r.Body)
	_, _, _ = reader.Next()
	return reader
}

func flushFrame(w http.ResponseWriter, payload []byte) {
	_, _ = w.Write(adapterscursor.Frame(payload))
	w.(http.Flusher).Flush()
}

// A finished turn must release its stream: Cursor serves one live run per
// conversation, so a stream left open stalls the next request on it.
func TestRunReleasesStreamAfterTurn(t *testing.T) {
	released := make(chan struct{})
	server := startStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		beginTurn(w, r)
		flushFrame(w, serverUpdate(14, turnEndedPayload()))
		select {
		case <-r.Context().Done():
			close(released)
		case <-time.After(3 * time.Second):
		}
	})
	if _, err := newStreamRunner(server).Run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}
	select {
	case <-released:
	case <-time.After(3 * time.Second):
		t.Fatal("the stream stayed open after the turn ended")
	}
}

func TestRunSendsHeartbeats(t *testing.T) {
	server := startStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		reader := beginTurn(w, r)
		payload, _, err := reader.Next()
		if err != nil {
			return
		}
		fields, err := adapterscursor.Parse(payload)
		if err != nil || len(fields) != 1 || fields[0].Number != clientHeartbeat {
			return
		}
		flushFrame(w, serverUpdate(14, turnEndedPayload()))
		_, _ = io.Copy(io.Discard, r.Body)
	})
	runner := newStreamRunner(server)
	runner.HeartbeatEvery = 10 * time.Millisecond
	if _, err := runner.Run(context.Background()); err != nil {
		t.Fatalf("a turn that waits for a heartbeat should finish: %v", err)
	}
}

func TestRunFailsWhenStreamGoesSilent(t *testing.T) {
	server := startStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		beginTurn(w, r)
		<-r.Context().Done()
	})
	runner := newStreamRunner(server)
	runner.IdleTimeout = 100 * time.Millisecond
	_, err := runner.Run(context.Background())
	if !errors.Is(err, ErrIdle) {
		t.Fatalf("error = %v, want ErrIdle", err)
	}
}

// Cursor may attach a trace context to an exec; it is not the exec's variant.
func TestRunIgnoresExecTraceContext(t *testing.T) {
	server := startStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		reader := beginTurn(w, r)
		var message adapterscursor.Writer
		message.Uint32(1, 1)
		message.String(15, "exec-1")
		message.Message(execSpanContext, []byte{0x0a, 0x01, 0x61})
		message.Empty(execContext)
		var envelope adapterscursor.Writer
		envelope.Message(2, message.BytesOut())
		flushFrame(w, envelope.BytesOut())
		_, _, _ = reader.Next()
		flushFrame(w, serverUpdate(14, turnEndedPayload()))
		_, _ = io.Copy(io.Discard, r.Body)
	})
	if _, err := newStreamRunner(server).Run(context.Background()); err != nil {
		t.Fatalf("a traced exec should be answered by its variant: %v", err)
	}
}
