package inference

import "time"

// EventKind identifies the type of one streaming event.
type EventKind int

// Stream event kinds name what one upstream frame became: text, tool-call
// lifecycle, reasoning, usage, or the terminal frame.
const (
	EventTextDelta EventKind = iota
	EventToolCallStart
	EventToolCallDelta
	EventToolCallEnd
	EventReasoningDelta
	EventUsage
	EventTerminal
	EventError
)

// EventReasonStop, EventReasonLength, and EventReasonToolUse are the
// terminal reasons the canonical format reports.
const (
	EventReasonStop    = "stop"
	EventReasonLength  = "length"
	EventReasonToolUse = "tool_use"
	EventReasonError   = "error"
)

// String names the event kind for logs and test failures.
func (k EventKind) String() string {
	switch k {
	case EventTextDelta:
		return "text_delta"
	case EventToolCallStart:
		return "tool_call_start"
	case EventToolCallDelta:
		return "tool_call_delta"
	case EventToolCallEnd:
		return "tool_call_end"
	case EventReasoningDelta:
		return "reasoning_delta"
	case EventUsage:
		return "usage"
	case EventTerminal:
		return "terminal"
	case EventError:
		return "error"
	default:
		return "unknown"
	}
}

// Event is one event in a streaming response. Reasoning carries the
// opaque reasoning state a family replays, alongside any reasoning text.
type Event struct {
	Kind      EventKind        `json:"kind"`
	Text      string           `json:"text,omitempty"`
	ToolCall  *ToolCallDelta   `json:"tool_call,omitempty"`
	Usage     *UsageReport     `json:"usage,omitempty"`
	Error     *ErrorInfo       `json:"error,omitempty"`
	Terminal  *TerminalInfo    `json:"terminal,omitempty"`
	Reasoning *ReasoningConfig `json:"reasoning,omitempty"`
}

// ToolCallDelta carries the tool call fragments a provider streams,
// keyed by Index because parallel calls arrive interleaved.
type ToolCallDelta struct {
	Index     int    `json:"index"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// UsageReport carries the token counts one request consumed.
type UsageReport struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

// TerminalInfo explains why a stream ended.
type TerminalInfo struct {
	Reason string `json:"reason"`
}

// ErrorInfo carries an upstream or relay failure.
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"status"`
	// RetryAfter is how long the upstream asked the caller to wait, when it
	// said so in the body rather than in a header. It never travels to the
	// client: a wait belongs to the account and the next attempt, not to a
	// response already on its way out.
	RetryAfter time.Duration `json:"-"`
}

// The client surfaces a data plane listener serves, named for logs, usage
// rows, and the protocol table that mounts them.
const (
	SurfaceChatCompletions = "chat-completions"
	SurfaceResponses       = "responses"
	SurfaceMessages        = "messages"
)
