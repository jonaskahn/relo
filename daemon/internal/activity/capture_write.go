package activity

import "context"

// The kinds of message a request capture holds: the request an agent sent and
// the answer it read, the request Relo sent to one provider account, and what
// that provider answered.
const (
	CaptureAgentRequest     = "agent_request"
	CaptureAgentResponse    = "agent_response"
	CaptureProviderRequest  = "provider_request"
	CaptureProviderResponse = "provider_response"
)

// CaptureWrite is one message written beside a usage row. A nil Ordinal belongs
// to a message that is not tied to one upstream attempt, which is the
// request an agent sent.
type CaptureWrite struct {
	EventID   int64
	Ordinal   *int
	Kind      string
	Method    string
	URL       string
	Status    int
	Headers   map[string][]string
	Body      []byte
	Truncated bool
}

// CaptureWriter stores the messages captured beside usage rows.
type CaptureWriter interface {
	Append(ctx context.Context, eventID int64, captures []CaptureWrite) error
}
