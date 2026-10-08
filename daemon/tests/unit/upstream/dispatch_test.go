package dispatch_test

import (
	"context"
	"errors"
	"github.com/jonaskahn/relo/internal/inference"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/upstream"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/openaichat"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestExecuteStreaming(t *testing.T) {
	t.Run("upstream SSE is relayed frame by frame", func(t *testing.T) {
		origin := newUpstream(t, upstreamOptions{fixture: "openai/chat_streaming.txt", sse: true})
		recorder := httptest.NewRecorder()
		executor := newExecutor(t)

		result, err := executor.Execute(context.Background(), streamExchange(origin.URL), recorder)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if result.Status != http.StatusOK {
			t.Fatalf("status = %d, want 200", result.Status)
		}
		if result.Usage.InputTokens != 9 || result.Usage.OutputTokens != 2 || result.Usage.CacheReadTokens != 3 {
			t.Fatalf("usage = %+v, want the upstream counts", result.Usage)
		}
		if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
			t.Fatalf("Content-Type = %q, want text/event-stream", got)
		}
		body := recorder.Body.String()
		if !strings.Contains(body, "Hello") || !strings.Contains(body, "data: [DONE]") {
			t.Fatalf("body = %q, want the relayed chunks and the sentinel", body)
		}
		if !strings.Contains(body, "\"finish_reason\":\"stop\"") {
			t.Fatalf("body = %q, want a finish reason", body)
		}
	})

	t.Run("the credential is applied by the dispatcher", func(t *testing.T) {
		var seen string
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1}}\n\ndata: [DONE]\n\n"))
		}))
		t.Cleanup(origin.Close)
		recorder := httptest.NewRecorder()
		if _, err := newExecutor(t).Execute(context.Background(), streamExchange(origin.URL), recorder); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if seen != "Bearer sk-test" {
			t.Fatalf("Authorization = %q, want the resolved credential", seen)
		}
	})

	t.Run("a stream with no usage is relayed with its terminal", func(t *testing.T) {
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		}))
		t.Cleanup(origin.Close)
		recorder := httptest.NewRecorder()
		exchange := streamExchange(origin.URL)
		exchange.Final = false
		result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
		if err != nil {
			t.Fatalf("Execute() error = %v, want the relayed stream", err)
		}
		if result.Status != http.StatusOK || !result.Wrote || result.ZeroTokens {
			t.Fatalf("result = %+v, want a written 200", result)
		}
		body := recorder.Body.String()
		if !strings.Contains(body, "hi") || !strings.Contains(body, "data: [DONE]") {
			t.Fatalf("body = %q, want the content beside the sentinel", body)
		}
		if !strings.Contains(body, "\"finish_reason\":\"stop\"") {
			t.Fatalf("body = %q, want the synthesized terminal", body)
		}
	})

	t.Run("a silent last attempt is relayed the same way", func(t *testing.T) {
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		}))
		t.Cleanup(origin.Close)
		recorder := httptest.NewRecorder()
		result, err := newExecutor(t).Execute(context.Background(), streamExchange(origin.URL), recorder)
		if err != nil {
			t.Fatalf("Execute() error = %v, want the relayed stream", err)
		}
		if result.Status != http.StatusOK || !result.Wrote || result.ZeroTokens {
			t.Fatalf("result = %+v, want a written 200", result)
		}
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", recorder.Code)
		}
		body := recorder.Body.String()
		if !strings.Contains(body, "hi") || !strings.Contains(body, "data: [DONE]") {
			t.Fatalf("body = %q, want the content beside the sentinel", body)
		}
	})
}

func TestExecuteComplete(t *testing.T) {
	t.Run("a non-streaming completion is relayed as JSON", func(t *testing.T) {
		origin := newUpstream(t, upstreamOptions{fixture: "openai/chat_nonstreaming.json"})
		recorder := httptest.NewRecorder()
		result, err := newExecutor(t).Execute(context.Background(), completeExchange(origin.URL), recorder)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if result.Status != http.StatusOK || !result.RelayedBody {
			t.Fatalf("result = %+v, want a relayed 200", result)
		}
		if result.Usage.InputTokens != 5 || result.Usage.OutputTokens != 2 {
			t.Fatalf("usage = %+v, want the upstream counts", result.Usage)
		}
		if got := recorder.Header().Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		body := recorder.Body.String()
		if !strings.Contains(body, "Hi there") || !strings.Contains(body, "\"object\":\"chat.completion\"") {
			t.Fatalf("body = %q, want the client completion", body)
		}
	})

	t.Run("a completion with no usage is retryable until the last attempt", func(t *testing.T) {
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`))
		}))
		t.Cleanup(origin.Close)
		recorder := httptest.NewRecorder()
		exchange := completeExchange(origin.URL)
		exchange.Final = false
		result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
		if !errors.Is(err, upstream.ErrZeroTokens) {
			t.Fatalf("Execute() error = %v, want %v", err, upstream.ErrZeroTokens)
		}
		if result.Status != http.StatusTooManyRequests || !result.Retryable || result.Wrote || !result.ZeroTokens {
			t.Fatalf("result = %+v, want a retryable zero-token 429", result)
		}
		if recorder.Body.Len() != 0 {
			t.Fatalf("body = %q, want nothing written before the last attempt", recorder.Body.String())
		}
	})

	t.Run("an upstream error object inside a 200 becomes a failure", func(t *testing.T) {
		origin := newUpstream(t, upstreamOptions{fixture: "openai/chat_error.json"})
		recorder := httptest.NewRecorder()
		_, err := newExecutor(t).Execute(context.Background(), completeExchange(origin.URL), recorder)
		if err == nil {
			t.Fatal("Execute() error = nil, want a failure")
		}
		if recorder.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", recorder.Code)
		}
	})

	t.Run("a malformed completion becomes a failure", func(t *testing.T) {
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{not json"))
		}))
		t.Cleanup(origin.Close)
		recorder := httptest.NewRecorder()
		if _, err := newExecutor(t).Execute(context.Background(), completeExchange(origin.URL), recorder); err == nil {
			t.Fatal("Execute() error = nil, want a decode failure")
		}
		if recorder.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", recorder.Code)
		}
	})
}

func TestExecuteFailures(t *testing.T) {
	t.Run("upstream error response mapped to ErrorInfo", func(t *testing.T) {
		origin := newUpstream(t, upstreamOptions{fixture: "openai/chat_error.json", status: http.StatusUnauthorized})
		recorder := httptest.NewRecorder()
		result, err := newExecutor(t).Execute(context.Background(), completeExchange(origin.URL), recorder)
		if !errors.Is(err, upstream.ErrUpstreamStatus) {
			t.Fatalf("Execute() error = %v, want %v", err, upstream.ErrUpstreamStatus)
		}
		if result.Status != http.StatusUnauthorized {
			t.Fatalf("status = %d, want the upstream status", result.Status)
		}
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("client status = %d, want 401", recorder.Code)
		}
		body := recorder.Body.String()
		if !strings.Contains(body, "invalid_api_key") || !strings.Contains(body, "Incorrect API key") {
			t.Fatalf("body = %q, want the upstream failure", body)
		}
	})

	t.Run("an unreachable upstream becomes a gateway error", func(t *testing.T) {
		origin := newUpstream(t, upstreamOptions{fixture: "openai/chat_nonstreaming.json"})
		address := origin.URL
		origin.Close()
		recorder := httptest.NewRecorder()
		result, err := newExecutor(t).Execute(context.Background(), completeExchange(address), recorder)
		if !errors.Is(err, upstream.ErrUpstreamFailed) {
			t.Fatalf("Execute() error = %v, want %v", err, upstream.ErrUpstreamFailed)
		}
		if result.Status != http.StatusBadGateway || recorder.Code != http.StatusBadGateway {
			t.Fatalf("status = %d / %d, want 502", result.Status, recorder.Code)
		}
	})

	t.Run("a missing credential is refused before the network", func(t *testing.T) {
		exchange := completeExchange("https://example.invalid")
		exchange.Options.CredentialRef = ""
		recorder := httptest.NewRecorder()
		_, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
		if !errors.Is(err, wire.ErrMissingCredential) {
			t.Fatalf("Execute() error = %v, want %v", err, wire.ErrMissingCredential)
		}
		if recorder.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", recorder.Code)
		}
	})

	t.Run("a malformed upstream frame is reported to the client", func(t *testing.T) {
		// The codec every family offers can reject a frame it cannot read. The
		// relay reports that failure in the stream, so the client sees a stated
		// error instead of a stream that simply stopped.
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {not json\n\n"))
		}))
		t.Cleanup(origin.Close)
		recorder := httptest.NewRecorder()
		if _, err := newExecutor(t).Execute(context.Background(), streamExchange(origin.URL), recorder); err != nil {
			t.Fatalf("Execute() error = %v, want the failure reported in the stream", err)
		}
		body := recorder.Body.String()
		if !strings.Contains(body, "stream_error") {
			t.Fatalf("body = %q, want the decoder failure reported", body)
		}
		if strings.Contains(body, `"finish_reason":"`) {
			t.Fatalf("body = %q, want no completion terminal for a failed stream", body)
		}
	})

	t.Run("a client that disappears is reported as a relay failure", func(t *testing.T) {
		origin := newUpstream(t, upstreamOptions{fixture: "openai/chat_streaming.txt", sse: true})
		result, err := newExecutor(t).Execute(context.Background(), streamExchange(origin.URL), abandonedClient{})
		if !errors.Is(err, upstream.ErrRelayFailed) {
			t.Fatalf("Execute() error = %v, want %v", err, upstream.ErrRelayFailed)
		}
		if result.Status != http.StatusOK {
			t.Fatalf("status = %d, want the upstream status", result.Status)
		}
	})
}

// TestExecuteAggregatesAStreamOnlyUpstream covers the connection whose
// upstream refuses a one-shot request: the client asked for one complete
// body, so the relay asks for the stream on its behalf and reassembles it.
func TestExecuteAggregatesAStreamOnlyUpstream(t *testing.T) {
	t.Run("a one-shot client gets the streamed answer as one body", func(t *testing.T) {
		var upstreamBody string
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			upstreamBody = string(raw)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write(fixtureBytes(t, "openai/chat_streaming.txt"))
		}))
		t.Cleanup(origin.Close)
		recorder := httptest.NewRecorder()
		exchange := aggregateExchange(origin.URL)
		result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if result.Status != http.StatusOK || !result.RelayedBody {
			t.Fatalf("result = %+v, want a relayed 200", result)
		}
		if result.Usage.InputTokens != 9 || result.Usage.OutputTokens != 2 {
			t.Fatalf("usage = %+v, want the upstream counts", result.Usage)
		}
		if !strings.Contains(upstreamBody, `"stream":true`) {
			t.Fatalf("upstream body = %q, want the streamed request the upstream requires", upstreamBody)
		}
		if exchange.Request.Stream {
			t.Fatal("the canonical request was mutated, want its own stream flag untouched")
		}
		if got := recorder.Header().Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want the single body the client asked for", got)
		}
		body := recorder.Body.String()
		if !strings.Contains(body, "Hello world") {
			t.Fatalf("body = %q, want the reassembled text", body)
		}
		if !strings.Contains(body, `"object":"chat.completion"`) {
			t.Fatalf("body = %q, want a complete completion", body)
		}
	})

	t.Run("a stream with no usage is retryable for a one-shot client too", func(t *testing.T) {
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		}))
		t.Cleanup(origin.Close)
		recorder := httptest.NewRecorder()
		exchange := aggregateExchange(origin.URL)
		exchange.Final = false
		result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
		if !errors.Is(err, upstream.ErrZeroTokens) {
			t.Fatalf("Execute() error = %v, want %v", err, upstream.ErrZeroTokens)
		}
		if result.Status != http.StatusTooManyRequests || !result.Retryable || result.Wrote || !result.ZeroTokens {
			t.Fatalf("result = %+v, want a retryable zero-token 429", result)
		}
		if recorder.Body.Len() != 0 {
			t.Fatalf("body = %q, want nothing written before the last attempt", recorder.Body.String())
		}
	})

	t.Run("a streamed error object becomes the client's failure", func(t *testing.T) {
		origin := newUpstream(t, upstreamOptions{fixture: "openai/chat_streaming_error.txt", sse: true})
		recorder := httptest.NewRecorder()
		_, err := newExecutor(t).Execute(context.Background(), aggregateExchange(origin.URL), recorder)
		if err == nil {
			t.Fatal("Execute() error = nil, want a failure")
		}
		if recorder.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", recorder.Code)
		}
		body := recorder.Body.String()
		if strings.Contains(body, "partial") {
			t.Fatalf("body = %q, want a failure rather than the partial text", body)
		}
		if !strings.Contains(body, "the upstream stream failed") {
			t.Fatalf("body = %q, want the upstream's message", body)
		}
	})

	t.Run("a streaming client on the same connection stays frame by frame", func(t *testing.T) {
		origin := newUpstream(t, upstreamOptions{fixture: "openai/chat_streaming.txt", sse: true})
		recorder := httptest.NewRecorder()
		exchange := aggregateExchange(origin.URL)
		exchange.Request.Stream = true
		if _, err := newExecutor(t).Execute(context.Background(), exchange, recorder); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
			t.Fatalf("Content-Type = %q, want the client stream untouched", got)
		}
		if !strings.Contains(recorder.Body.String(), "data: [DONE]") {
			t.Fatalf("body = %q, want the relayed stream", recorder.Body.String())
		}
	})
}

// TestExecuteClientAbortIsNotAnUpstreamFault covers a caller that hangs up
// mid-answer: the attempt ends as an abandoned send the next request
// ignores, not as a fault counted against the account behind it.
func TestExecuteClientAbortIsNotAnUpstreamFault(t *testing.T) {
	hangUp := func(t *testing.T, first string) *httptest.Server {
		t.Helper()
		stop := make(chan struct{})
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			if first != "" {
				_, _ = io.WriteString(w, first)
			} else {
				w.WriteHeader(http.StatusOK)
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case <-r.Context().Done():
			case <-stop:
			}
		}))
		t.Cleanup(func() {
			close(stop)
			origin.Close()
		})
		return origin
	}
	abandoned := func(t *testing.T, ctx context.Context, exchange upstream.Exchange, w http.ResponseWriter) upstream.Result {
		t.Helper()
		result, err := newExecutor(t).Execute(ctx, exchange, w)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() error = %v, want the caller going away", err)
		}
		if result.Status != upstream.StatusClientAborted {
			t.Fatalf("status = %d, want the abandoned send", result.Status)
		}
		if result.Retryable || result.ClassSwitch || result.Wrote {
			t.Fatalf("result = %+v, want no retry, no switch, and nothing written", result)
		}
		if upstream.Retryable(result.Status) {
			t.Fatalf("status = %d, want a status the relay never retries", result.Status)
		}
		if verdict := account.ClassifyStatus(result.Status); verdict != account.VerdictHealthy {
			t.Fatalf("verdict = %v, want the account left alone", verdict)
		}
		return result
	}

	t.Run("a hangup before headers is abandoned", func(t *testing.T) {
		origin := hangUp(t, "")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		abandoned(t, ctx, streamExchange(origin.URL), httptest.NewRecorder())
	})

	t.Run("a hangup mid-stream is abandoned", func(t *testing.T) {
		origin := hangUp(t, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\n")
		ctx, cancel := context.WithCancel(context.Background())
		exchange := streamExchange(origin.URL)
		exchange.CallWait = time.Minute
		exchange.Final = false
		done := make(chan upstream.Result)
		go func() {
			result, _ := newExecutor(t).Execute(ctx, exchange, httptest.NewRecorder())
			done <- result
		}()
		time.Sleep(200 * time.Millisecond)
		cancel()
		result := <-done
		if result.Status != upstream.StatusClientAborted {
			t.Fatalf("status = %d, want the abandoned send", result.Status)
		}
		if result.Retryable || result.ClassSwitch {
			t.Fatalf("result = %+v, want no retry and no switch", result)
		}
		// The partial frame went out before the hangup, so the answer is
		// marked written even though its caller is gone.
		if !result.Wrote {
			t.Fatalf("result = %+v, want the partial answer marked written", result)
		}
	})

	t.Run("a hangup mid-body is abandoned", func(t *testing.T) {
		origin := hangUp(t, "")
		ctx, cancel := context.WithCancel(context.Background())
		exchange := completeExchange(origin.URL)
		exchange.CallWait = time.Minute
		exchange.Final = false
		done := make(chan upstream.Result)
		go func() {
			result, _ := newExecutor(t).Execute(ctx, exchange, httptest.NewRecorder())
			done <- result
		}()
		time.Sleep(200 * time.Millisecond)
		cancel()
		result := <-done
		if result.Status != upstream.StatusClientAborted {
			t.Fatalf("status = %d, want the abandoned send", result.Status)
		}
		if result.Retryable || result.ClassSwitch || result.Wrote {
			t.Fatalf("result = %+v, want no retry, no switch, and nothing written", result)
		}
	})
}

// aggregateExchange is a one-shot client's exchange on a connection whose
// upstream only answers streams.
func aggregateExchange(baseURL string) upstream.Exchange {
	exchange := completeExchange(baseURL)
	exchange.RequiresStream = true
	return exchange
}

// upstreamOptions describes one mock provider endpoint.
type upstreamOptions struct {
	fixture string
	sse     bool
	status  int
}

func newUpstream(t *testing.T, opts upstreamOptions) *httptest.Server {
	t.Helper()
	body := fixtureBytes(t, opts.fixture)
	status := opts.status
	if status == 0 {
		status = http.StatusOK
	}
	contentType := "application/json"
	if opts.sse {
		contentType = "text/event-stream"
	}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(origin.Close)
	return origin
}

// fixtureBytes reads one recorded provider answer.
func fixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(testkit.FixturePath(name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

func newExecutor(t *testing.T) *upstream.Executor {
	t.Helper()
	logger, _ := testkit.TestLogger(t)
	return upstream.NewExecutor(&http.Client{}, logger)
}

func streamExchange(baseURL string) upstream.Exchange {
	exchange := completeExchange(baseURL)
	exchange.Request.Stream = true
	return exchange
}

func completeExchange(baseURL string) upstream.Exchange {
	return upstream.Exchange{
		ProviderID: "openai",
		Codec:      openaichat.NewCodec(baseURL),
		Options: wire.CodecOpts{
			BaseURL: baseURL, CredentialRef: "sk-test",
			AuthMethod: wire.AuthAPIKey, KeyHeader: string(catalog.KeyHeaderBearer),
		},
		Request:         &inference.Request{Model: "gpt-4o", Messages: []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hi"}}}}},
		Inbound:         openaichat.NewChatCompletionsCodec(),
		Surface:         inference.SurfaceChatCompletions,
		RequestID:       "req-test",
		CredentialLabel: "default",
		// The failure has to reach the client, which only happens once no
		// further attempt follows.
		Final: true,
	}
}

// abandonedClient is a client writer that fails every write, the way a
// disconnected client behaves.
type abandonedClient struct{}

func (abandonedClient) Header() http.Header       { return http.Header{} }
func (abandonedClient) Write([]byte) (int, error) { return 0, errors.New("client is gone") }
func (abandonedClient) WriteHeader(int)           {}
func (abandonedClient) Flush()                    {}

// TestExecuteKeepsReadingWhileTheProviderSends covers the long live stream:
// the call wait bounds silence between bytes, not the length of the answer,
// so a provider that sends for several call waits still finishes its turn.
func TestExecuteKeepsReadingWhileTheProviderSends(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for round := 0; round < 6; round++ {
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"x\"}}]}\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(20 * time.Millisecond)
		}
		_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":2}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(origin.Close)

	recorder := httptest.NewRecorder()
	exchange := streamExchange(origin.URL)
	// The answer takes about 120ms, twice the call wait, while no single gap
	// comes close to it.
	exchange.CallWait = 60 * time.Millisecond
	result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != http.StatusOK || result.ZeroTokens || !result.Wrote {
		t.Fatalf("result = %+v, want a served 200", result)
	}
	if result.Usage.OutputTokens != 2 {
		t.Fatalf("usage = %+v, want the counts the provider sent after several waits", result.Usage)
	}
}

// TestExecuteStallsAreRetryableGatewayTimeouts covers a body that goes silent
// longer than the call wait: a complete body and a reassembled stream each
// fail as a retryable 504, never as the synthetic empty completion that would
// blame the account. A live stream instead keeps what arrived beside its
// error frame: the client already read the partial answer, so there is
// nothing to retry.
func TestExecuteStallsAreRetryableGatewayTimeouts(t *testing.T) {
	silent := func(first string) *httptest.Server {
		stop := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			if first != "" {
				_, _ = io.WriteString(w, first)
			} else {
				w.WriteHeader(http.StatusOK)
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case <-r.Context().Done():
			case <-stop:
			}
		}))
		t.Cleanup(func() {
			close(stop)
			server.Close()
		})
		return server
	}

	tests := []struct {
		name     string
		exchange func(string) upstream.Exchange
		first    string
	}{
		{"complete body", completeExchange, ""},
		{"reassembled stream", aggregateExchange, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			origin := silent(test.first)
			recorder := httptest.NewRecorder()
			exchange := test.exchange(origin.URL)
			exchange.CallWait = 50 * time.Millisecond
			exchange.Final = false
			result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
			if !errors.Is(err, upstream.ErrUpstreamStall) {
				t.Fatalf("Execute() error = %v, want the stall cause", err)
			}
			if result.Status != http.StatusGatewayTimeout || !result.Retryable || result.Wrote || result.ZeroTokens {
				t.Fatalf("result = %+v, want a retryable 504 that wrote nothing", result)
			}
			if recorder.Body.Len() != 0 {
				t.Fatalf("body = %q, want no partial answer before a retry", recorder.Body.String())
			}
		})
	}

	t.Run("stream", func(t *testing.T) {
		origin := silent("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\n")
		recorder := httptest.NewRecorder()
		exchange := streamExchange(origin.URL)
		exchange.CallWait = 50 * time.Millisecond
		exchange.Final = false
		result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
		if !errors.Is(err, upstream.ErrUpstreamStall) {
			t.Fatalf("Execute() error = %v, want the stall cause", err)
		}
		if result.Status != http.StatusOK || !result.Wrote || result.Retryable || result.ZeroTokens {
			t.Fatalf("result = %+v, want a written 200 that stays out of rotation", result)
		}
		body := recorder.Body.String()
		if !strings.Contains(body, "partial") || !strings.Contains(body, "data: [DONE]") {
			t.Fatalf("body = %q, want the partial answer beside the sentinel", body)
		}
	})
}

// TestExecuteHeaderWaitIsARetryableTimeout covers a provider that never
// answers: the wait for headers is bounded by the call wait, and the attempt
// fails as a retryable 504.
func TestExecuteHeaderWaitIsARetryableTimeout(t *testing.T) {
	stop := make(chan struct{})
	origin := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-stop:
		}
	}))
	t.Cleanup(func() {
		close(stop)
		origin.Close()
	})

	recorder := httptest.NewRecorder()
	exchange := streamExchange(origin.URL)
	exchange.CallWait = 50 * time.Millisecond
	exchange.Final = false
	result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
	if !errors.Is(err, upstream.ErrUpstreamFailed) {
		t.Fatalf("Execute() error = %v, want an upstream failure", err)
	}
	if result.Status != http.StatusGatewayTimeout || !result.Retryable || result.Wrote || result.ZeroTokens {
		t.Fatalf("result = %+v, want a retryable 504 that wrote nothing", result)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("body = %q, want nothing written", recorder.Body.String())
	}
}

// TestExecuteSwitchesAnUnlistedStatusOnTheConnectionsTerms covers the two
// per-connection failover options: a status the relay does not always retry is
// reported to the caller as switchable when the connection allows that class,
// and is written to the client when it does not. A status the relay always
// retries never asks the option at all.
func TestExecuteSwitchesAnUnlistedStatusOnTheConnectionsTerms(t *testing.T) {
	const refusal = `{"error":{"message":"the request was refused","type":"invalid_request_error"}}`
	cases := []struct {
		name       string
		status     int
		policy     upstream.Policy
		wantSwitch bool
	}{
		{"a 4xx switches when the connection allows it", http.StatusBadRequest, upstream.Policy{SwitchOn4xx: true}, true},
		{"a 4xx is the answer when the connection does not", http.StatusBadRequest, upstream.Policy{}, false},
		{"a 5xx switches when the connection allows it", http.StatusNotImplemented, upstream.Policy{SwitchOn5xx: true}, true},
		{"a 5xx is the answer when the connection does not", http.StatusNotImplemented, upstream.Policy{}, false},
		{"a status the relay always retries switches with either option off", http.StatusServiceUnavailable, upstream.Policy{}, true},
		{"a status the relay always retries keeps its retry", http.StatusTooManyRequests, upstream.Policy{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(refusal))
			}))
			t.Cleanup(origin.Close)

			exchange := completeExchange(origin.URL)
			// Nothing follows this attempt, so the relay keeps its choice of
			// what the client sees.
			exchange.Final = false
			exchange.Policy = tc.policy
			recorder := httptest.NewRecorder()
			result, err := newExecutor(t).Execute(context.Background(), exchange, recorder)
			if err == nil {
				t.Fatal("Execute() error = nil, want the upstream refusal")
			}
			if result.Status != tc.status {
				t.Fatalf("status = %d, want %d", result.Status, tc.status)
			}
			if result.Retryable != tc.wantSwitch {
				t.Fatalf("result = %+v, want retryable = %v", result, tc.wantSwitch)
			}
			wantNow := tc.wantSwitch && !upstream.Retryable(tc.status)
			if result.ClassSwitch != wantNow {
				t.Fatalf("result = %+v, want the switch to ride the same retry walk = %v", result, wantNow)
			}
			if tc.wantSwitch {
				if result.Wrote || recorder.Body.Len() != 0 {
					t.Fatalf("body = %q, want nothing written while a switch is still possible", recorder.Body.String())
				}
				return
			}
			if !result.Wrote || recorder.Code != tc.status {
				t.Fatalf("status = %d, body = %s, want the provider's own %d",
					recorder.Code, recorder.Body.String(), tc.status)
			}
		})
	}
}
