// Relay ports: the credential and execution boundary the data plane runs on.
package server

import (
	"context"
	"net/http"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
)

// Relay resolves the credential one attempt runs with and sends the attempt
// upstream. The composition root backs it with the credential source and the
// dispatch executor, so this package never imports either adapter.
type Relay interface {
	// Authorize resolves the credential one attempt runs with.
	Authorize(ctx context.Context, request catalog.AuthRequest) (catalog.Authorization, error)
	// RefreshRejected renews a token a request just refused.
	RefreshRejected(ctx context.Context, providerID, credentialID, rejected string) (catalog.Authorization, error)
	// Report records what an attempt's status means for its credential.
	Report(ctx context.Context, outcome Outcome) error
	// Execute sends one exchange upstream and writes the client response.
	Execute(ctx context.Context, exchange Exchange, w http.ResponseWriter) (Result, error)
}

// Outcome is what one attempt's status says about the credential behind it.
type Outcome struct {
	ProviderID   string
	CredentialID string
	Status       int
	RetryAfter   time.Duration
	// RefreshSettled reports that a refused token was already handled, so the
	// caller must not treat that status as a fresh refusal.
	RefreshSettled bool
}

// CapturedMessage is one HTTP message as the log keeps it: the method or the
// status, the target, every header, and the body bytes.
type CapturedMessage struct {
	Method    string
	URL       string
	Status    int
	Headers   map[string][]string
	Body      []byte
	Truncated bool
}

// Capture holds what one attempt sent to a provider and what that provider
// answered, which the caller stores beside the usage row of the request.
type Capture struct {
	Request  *CapturedMessage
	Response *CapturedMessage
}

// Policy is how one connection asked the relay to fail over.
type Policy struct {
	// SwitchOn4xx and SwitchOn5xx move a request to another account when the
	// upstream refuses with a status of that class the relay does not always
	// retry, such as a 400 or a 501.
	SwitchOn4xx bool
	SwitchOn5xx bool
}

// AllowsClass reports whether the policy admits a status of this class, apart
// from whether the relay always retries that particular status.
func (p Policy) AllowsClass(status int) bool {
	switch {
	case status >= 400 && status < 500:
		return p.SwitchOn4xx
	case status >= 500 && status < 600:
		return p.SwitchOn5xx
	default:
		return false
	}
}

// Exchange is one upstream attempt: what to send, to whom, and how the client
// wants the answer rendered. The codec and the endpoint arrive with routing,
// so the relay never reads the catalog.
type Exchange struct {
	ProviderID string
	Codec      wire.CodecModule
	Options    wire.CodecOpts
	Request    *inference.Request
	Inbound    wire.InboundCodec
	Surface    string
	RequestID  string
	// RequiresStream reports that the connection's upstream only answers a
	// streamed request, so a client that asked for one complete body is
	// served by reassembling the stream rather than by a one-shot request the
	// upstream would refuse.
	RequiresStream bool
	// CredentialLabel names the account in a log, never the secret.
	CredentialLabel string
	// CallWait is how long one attempt waits for response headers, and then
	// how long the body may stay silent between bytes. A stream that keeps
	// sending is read until the provider ends it. Zero means no wait.
	CallWait time.Duration
	// Final reports that no further attempt follows, so a failure has to be
	// written to the client rather than reported to the caller.
	Final bool
	// Policy is the connection's own failover choice: which statuses the relay
	// would otherwise hand back to the client are worth another account.
	Policy Policy
	// OnUnauthorized renews a token a request just refused. The relay calls it
	// once, before anything is written to the client, and resends with what
	// it returns.
	OnUnauthorized func(ctx context.Context) (catalog.Authorization, error)
}

// Result reports how one attempt ended, for logging, routing feedback, and
// usage recording.
type Result struct {
	Status     int
	Usage      inference.UsageReport
	Duration   time.Duration
	Model      string
	Credential string
	// CredentialID is the stored account the attempt ran on, which is what a
	// retry excludes and what a model refusal is remembered against. The label
	// beside it is for humans, and the two are not interchangeable.
	CredentialID string
	// HeaderLatency is how long the upstream took to answer at all, which is
	// what a fastest-first group orders by.
	HeaderLatency time.Duration
	// Retryable reports a failure this attempt did not write to the client, so
	// the caller may try the next candidate.
	Retryable bool
	// ClassSwitch reports a failure the connection's own policy made
	// retryable although the relay does not always retry its status. The
	// walk treats it like any other retryable failure: each attempt waits
	// its retry window, and every account of the connection answers once
	// before the request leaves it.
	ClassSwitch bool
	// Wrote reports that this attempt already sent the client its answer, so
	// a later failure must not write a second one.
	Wrote bool
	// ModelAccess reports a refusal that is about the model rather than the
	// account: the account may not use this model, so the next attempt belongs
	// to another account of the same connection rather than to another
	// provider, and the account itself stays in rotation.
	ModelAccess bool
	// ZeroTokens reports a 2xx that carried no token usage at all, which the
	// relay turns into a retryable rate limit. The account's own health is
	// not blamed for it.
	ZeroTokens bool
	// RetryAfter is how long the upstream asked the caller to wait.
	RetryAfter  time.Duration
	RelayedBody bool
	// RateLimits are the quota windows the upstream reported beside its
	// response, which is the freshest reading Relo gets without asking.
	RateLimits []activity.WindowSample
	// Capture holds what this attempt sent and what its provider answered,
	// which the caller stores beside the usage row of the request.
	Capture *Capture
	// RefreshSettled reports that a 401 was already handled by OnUnauthorized
	// failing, so the caller must not treat the status as a fresh refusal.
	RefreshSettled bool
}
