package proxy_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/catalog"
)

// The faults an upstream can play. The client asks for one by using it as
// the model name, so a single mock serves every case without shared state.
const (
	scenarioComplete   = "fault-complete"
	scenarioNoSentinel = "fault-no-sentinel"
	scenarioRuneSplit  = "fault-rune-split"
	scenarioToolJSON   = "fault-tool-json"
	scenarioError      = "fault-error"
	scenarioEmpty      = "fault-empty"
	scenarioDisconnect = "fault-disconnect"
	scenarioStall      = "fault-stall"

	scenarioInput = "rate limited mid-stream"

	eventEnd   = "\n\n"
	fieldBreak = "\n"
)

// faultModelNames lists every model name the mock answers a scenario for, so
// a daemon under test routes each one to this upstream.
func faultModelNames() []string {
	return []string{
		scenarioComplete, scenarioNoSentinel, scenarioRuneSplit, scenarioToolJSON,
		scenarioError, scenarioEmpty, scenarioDisconnect, scenarioStall,
	}
}

// faultUpstream is a deterministic upstream that fails the way real
// providers fail: it aborts mid-chunk, stops without a terminal event,
// splits a frame across writes, reports an error after content, or stalls.
type faultUpstream struct {
	family   catalog.APIFormat
	server   *httptest.Server
	requests atomic.Int64
}

func newFaultUpstream(t *testing.T, family catalog.APIFormat) *faultUpstream {
	t.Helper()
	mock := &faultUpstream{family: family}
	mock.server = httptest.NewServer(http.HandlerFunc(mock.serve))
	t.Cleanup(mock.server.Close)
	return mock
}

func (u *faultUpstream) URL() string {
	return u.server.URL
}

func (u *faultUpstream) requestCount() int {
	return int(u.requests.Load())
}

func (u *faultUpstream) serve(w http.ResponseWriter, r *http.Request) {
	u.requests.Add(1)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	scenario := scenarioOf(body, r.URL.Path)
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	for _, chunk := range u.chunks(scenario) {
		_, _ = w.Write(chunk)
		flusher.Flush()
	}
	u.finish(w, r, scenario)
}

func (u *faultUpstream) chunks(scenario string) [][]byte {
	switch scenario {
	case scenarioNoSentinel, scenarioStall, scenarioDisconnect:
		return textChunks(u.family)
	case scenarioRuneSplit:
		return runeChunks(u.family)
	case scenarioToolJSON:
		return toolChunks(u.family)
	case scenarioError:
		return errorChunks(u.family)
	case scenarioEmpty:
		return nil
	default:
		return completeChunks(u.family)
	}
}

func (u *faultUpstream) finish(w http.ResponseWriter, r *http.Request, scenario string) {
	switch scenario {
	case scenarioDisconnect:
		abortConnection(w)
	case scenarioStall:
		stallUntil(r.Context().Done())
	}
}

// scenarioOf reads the scenario from the model the client asked for: the
// body carries it for most families and the path for Google.
func scenarioOf(body []byte, path string) string {
	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Model != "" {
		return payload.Model
	}
	const marker = "/models/"
	if index := strings.Index(path, marker); index >= 0 {
		return strings.SplitN(path[index+len(marker):], ":", 2)[0]
	}
	return ""
}

// abortConnection closes the connection mid-stream, the way a provider that
// dies leaves an unfinished response behind.
func abortConnection(w http.ResponseWriter) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return
	}
	connection, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	_ = connection.Close()
}

// stallUntil holds the response open until the client gives up, so a test
// can prove the relay ends instead of hanging forever.
func stallUntil(done <-chan struct{}) {
	select {
	case <-done:
	case <-time.After(10 * time.Second):
	}
}

// frame renders one server-sent event the way the family writes it.
func frame(name, data string) []byte {
	if name == "" {
		return []byte("data: " + data + eventEnd)
	}
	return []byte("event: " + name + fieldBreak + "data: " + data + eventEnd)
}

func textChunks(family catalog.APIFormat) [][]byte {
	switch family {
	case catalog.FormatOpenAIChat:
		return [][]byte{
			frame("", `{"choices":[{"index":0,"delta":{"content":"Hello"}}]}`),
			frame("", `{"choices":[{"index":0,"delta":{"content":" world"}}]}`),
		}
	case catalog.FormatOpenAIResp:
		return [][]byte{
			frame("response.output_text.delta", `{"type":"response.output_text.delta","delta":"Hello"}`),
			frame("response.output_text.delta", `{"type":"response.output_text.delta","delta":" world"}`),
		}
	case catalog.FormatAnthropic:
		return [][]byte{
			frame("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
			frame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`),
			frame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`),
		}
	default:
		return [][]byte{
			frame("", `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]}}]}`),
			frame("", `{"candidates":[{"content":{"role":"model","parts":[{"text":" world"}]}}]}`),
		}
	}
}

func completeChunks(family catalog.APIFormat) [][]byte {
	return append(textChunks(family), terminalChunks(family)...)
}

func terminalChunks(family catalog.APIFormat) [][]byte {
	switch family {
	case catalog.FormatOpenAIChat:
		return [][]byte{
			frame("", `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`),
			frame("", "[DONE]"),
		}
	case catalog.FormatOpenAIResp:
		completed := `{"type":"response.completed","response":{"id":"resp_fault","status":"completed","usage":{"input_tokens":5,"output_tokens":2}}}`
		return [][]byte{frame("response.completed", completed)}
	case catalog.FormatAnthropic:
		return [][]byte{
			frame("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`),
			frame("message_stop", `{"type":"message_stop"}`),
		}
	default:
		return [][]byte{
			frame("", `{"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2}}`),
		}
	}
}

// runeChunks writes one frame whose multi-byte character straddles two
// writes, which is what a relay sees when a provider flushes mid-character.
func runeChunks(family catalog.APIFormat) [][]byte {
	prefix, suffix := runeFrameParts(family)
	head := append([]byte(prefix+"Xin ch"), 0xc3)
	tail := append([]byte{0xa0}, []byte("o"+suffix)...)
	return [][]byte{head, tail}
}

func runeFrameParts(family catalog.APIFormat) (string, string) {
	switch family {
	case catalog.FormatOpenAIChat:
		return `data: {"choices":[{"index":0,"delta":{"content":"`, `"}}]}` + eventEnd
	case catalog.FormatOpenAIResp:
		return `event: response.output_text.delta` + fieldBreak + `data: {"type":"response.output_text.delta","delta":"`, `"}` + eventEnd
	case catalog.FormatAnthropic:
		return `event: content_block_delta` + fieldBreak + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"`, `"}}` + eventEnd
	default:
		return `data: {"candidates":[{"content":{"parts":[{"text":"`, `"}]}}]}` + eventEnd
	}
}

func toolChunks(family catalog.APIFormat) [][]byte {
	switch family {
	case catalog.FormatOpenAIChat:
		return [][]byte{
			frame("", `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`),
			frame("", `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}`),
			frame("", `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Hanoi\"}"}}]}}]}`),
			frame("", `{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`),
			frame("", "[DONE]"),
		}
	case catalog.FormatOpenAIResp:
		return [][]byte{
			frame("response.output_item.added", `{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"get_weather"}}`),
			frame("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"city\":"}`),
			frame("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"\"Hanoi\"}"}`),
			frame("response.output_item.done", `{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"get_weather","arguments":"{\"city\":\"Hanoi\"}"}}`),
			frame("response.completed", `{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":5,"output_tokens":3}}}`),
		}
	case catalog.FormatAnthropic:
		return [][]byte{
			frame("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{}}}`),
			frame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`),
			frame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"Hanoi\"}"}}`),
			frame("content_block_stop", `{"type":"content_block_stop","index":0}`),
			frame("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`),
			frame("message_stop", `{"type":"message_stop"}`),
		}
	default:
		whole := frame("", `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"Hanoi"}}}]},"finishReason":"STOP"}]}`)
		middle := len(whole) / 2
		return [][]byte{whole[:middle], whole[middle:]}
	}
}

// errorChunks sends content first and fails afterwards, which is the case a
// client must see reported rather than truncated.
func errorChunks(family catalog.APIFormat) [][]byte {
	chunks := append([][]byte{}, textChunks(family)[:2]...)
	if family == catalog.FormatOpenAIChat {
		return append(chunks, frame("", errorPayload(family)))
	}
	return append(chunks, frame("error", errorPayload(family)))
}

func errorPayload(family catalog.APIFormat) string {
	message := `"` + scenarioInput + `"`
	switch family {
	case catalog.FormatOpenAIChat:
		return `{"error":{"message":` + message + `,"type":"rate_limit_error","code":"rate_limit"}}`
	case catalog.FormatOpenAIResp:
		return `{"type":"error","code":"rate_limit","message":` + message + `}`
	case catalog.FormatAnthropic:
		return `{"type":"error","error":{"type":"overloaded_error","message":` + message + `}}`
	default:
		return `{"error":{"code":429,"message":` + message + `,"status":"RESOURCE_EXHAUSTED"}}`
	}
}
