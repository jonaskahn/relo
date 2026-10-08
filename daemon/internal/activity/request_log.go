package activity

import "context"

// RequestEvent is one completed request the usage log stores.
type RequestEvent struct {
	RequestID string
	Timestamp int64
	Provider  string
	Model     string
	// RequestedModel is what the client asked for, which is what a log shows
	// when a group or a bare identifier resolved to this provider model.
	RequestedModel string
	// GroupID names the group that chose this provider, empty when the
	// caller named a model instead.
	GroupID         string
	CredentialLabel string
	// CredentialID names the account that served the request by its stored
	// identifier, so two accounts that share a label stay distinct in a
	// rollup. It is empty on a request no stored account served, and on rows
	// written before the column existed.
	CredentialID        string
	Surface             string
	Status              int
	DurationMs          int64
	InputTokens         int
	OutputTokens        int
	CacheReadTokens     int
	CacheWriteTokens    int
	EstimatedCostMicros *int64
	RouteProvider       string
	RouteReason         string
	// Origin names where a request came from: external traffic a client sent
	// through a data plane port, or internal traffic the console's chat
	// tester sent to prove a connection works.
	Origin string
	// ClientKeyID and ClientKeyName name the inbound key that presented the
	// request. The name is denormalised so a row keeps the name the key had
	// when the request was made, even after the key is renamed or retired.
	ClientKeyID   string
	ClientKeyName string
	// ClientApp names the coding client the inbound key was issued for. It is
	// stored on the event so a log filter still finds the row after the key
	// is removed.
	ClientApp string
	// Warnings are the advisory mismatches this request asked of the model
	// that served it, stored with the row so a log keeps the note.
	Warnings []string
	// Attempts is how many upstream sends the request made, and Retried
	// reports whether that was more than one. Both are sums a rollup reads.
	Attempts int
	Retried  bool
}

// RequestAttempt is one upstream attempt of a logical request, in ordinal
// order, so retries and failovers stay visible.
type RequestAttempt struct {
	EventID         int64
	Ordinal         int
	Provider        string
	Model           string
	CredentialLabel string
	// CredentialID names the stored account the attempt ran on, which keeps
	// two accounts that share a label apart in a report.
	CredentialID string
	Status       int
	ErrorCode    string
	DurationMs   int64
	InputTokens  int
	OutputTokens int
}

// RequestWriter appends completed requests to the usage log.
type RequestWriter interface {
	AppendEvent(ctx context.Context, event RequestEvent) (int64, error)
	AppendAttempt(ctx context.Context, attempt RequestAttempt) error
}
