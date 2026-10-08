// Relay glue: the upstream-backed Relay, redactor, and template source.
package platform

import (
	"context"
	"net/http"

	"github.com/jonaskahn/relo/internal/adapters/templates"
	"github.com/jonaskahn/relo/internal/adapters/upstream"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/server"
)

var (
	_ server.Relay           = Relay{}
	_ server.CaptureRedactor = CaptureRedactor{}
	_ server.Templates       = TemplateSource{}
)

// Relay runs server exchanges on a concrete credential source and executor,
// translating the transport's neutral exchange into the upstream one.
type Relay struct {
	credentials *upstream.Source
	executor    *upstream.Executor
}

// NewRelay returns the Relay over the given credential source and executor.
func NewRelay(credentials *upstream.Source, executor *upstream.Executor) Relay {
	return Relay{credentials: credentials, executor: executor}
}

// Authorize resolves the credential one attempt runs with.
func (r Relay) Authorize(ctx context.Context, request catalog.AuthRequest) (catalog.Authorization, error) {
	return r.credentials.Authorize(ctx, request)
}

// RefreshRejected renews a token a request just refused.
func (r Relay) RefreshRejected(ctx context.Context, providerID, credentialID, rejected string) (catalog.Authorization, error) {
	return r.credentials.RefreshRejected(ctx, providerID, credentialID, rejected)
}

// Report records what an attempt's status means for its credential.
func (r Relay) Report(ctx context.Context, outcome server.Outcome) error {
	return r.credentials.Report(ctx, upstream.Outcome{
		ProviderID: outcome.ProviderID, CredentialID: outcome.CredentialID,
		Status: outcome.Status, RetryAfter: outcome.RetryAfter,
		RefreshSettled: outcome.RefreshSettled,
	})
}

// Execute sends one exchange upstream and writes the client response.
func (r Relay) Execute(ctx context.Context, exchange server.Exchange, w http.ResponseWriter) (server.Result, error) {
	result, err := r.executor.Execute(ctx, relayExchange(exchange), w)
	return relayResult(result), err
}

func relayExchange(exchange server.Exchange) upstream.Exchange {
	return upstream.Exchange{
		ProviderID: exchange.ProviderID, Codec: exchange.Codec,
		Options: exchange.Options, Request: exchange.Request,
		Inbound: exchange.Inbound, Surface: exchange.Surface,
		RequestID:       exchange.RequestID,
		RequiresStream:  exchange.RequiresStream,
		CredentialLabel: exchange.CredentialLabel, CallWait: exchange.CallWait,
		Final: exchange.Final,
		Policy: upstream.Policy{
			SwitchOn4xx: exchange.Policy.SwitchOn4xx,
			SwitchOn5xx: exchange.Policy.SwitchOn5xx,
		},
		OnUnauthorized: exchange.OnUnauthorized,
	}
}

func relayResult(result upstream.Result) server.Result {
	converted := server.Result{
		Status: result.Status, Usage: result.Usage, Duration: result.Duration,
		Model: result.Model, Credential: result.Credential,
		CredentialID: result.CredentialID, HeaderLatency: result.HeaderLatency,
		Retryable: result.Retryable, ClassSwitch: result.ClassSwitch,
		Wrote: result.Wrote, ModelAccess: result.ModelAccess,
		ZeroTokens: result.ZeroTokens, RetryAfter: result.RetryAfter,
		RelayedBody: result.RelayedBody, RateLimits: result.RateLimits,
		RefreshSettled: result.RefreshSettled,
	}
	if result.Capture != nil {
		capture := *result.Capture
		converted.Capture = &server.Capture{
			Request:  relayMessage(capture.Request),
			Response: relayMessage(capture.Response),
		}
	}
	return converted
}

func relayMessage(message *upstream.CapturedMessage) *server.CapturedMessage {
	if message == nil {
		return nil
	}
	return &server.CapturedMessage{
		Method: message.Method, URL: message.URL, Status: message.Status,
		Headers: message.Headers, Body: message.Body, Truncated: message.Truncated,
	}
}

// CaptureRedactor bounds one captured body and scrubs what the log keeps,
// delegating to the capture helpers.
type CaptureRedactor struct{}

// NewCaptureRedactor returns the redactor over the capture helpers.
func NewCaptureRedactor() CaptureRedactor {
	return CaptureRedactor{}
}

// MaxCaptureBytes bounds one captured body.
func (CaptureRedactor) MaxCaptureBytes() int64 {
	return upstream.MaxCaptureBytes
}

// RedactHeaders scrubs the secrets out of captured headers.
func (CaptureRedactor) RedactHeaders(headers map[string][]string) map[string][]string {
	return upstream.RedactHeaders(headers)
}

// RedactBody scrubs the secrets out of a captured body.
func (CaptureRedactor) RedactBody(body []byte) []byte {
	return upstream.RedactBody(body)
}

// CaptureBytes keeps at most limit bytes of one body, reporting whether more
// had to be left out.
func (CaptureRedactor) CaptureBytes(body []byte, limit int64) ([]byte, bool) {
	return upstream.CaptureBytes(body, limit)
}

// TemplateSource resolves the curated connection shapes the relay branches
// on, delegating to the template registry.
type TemplateSource struct{}

// NewTemplateSource returns the source over the template registry.
func NewTemplateSource() TemplateSource {
	return TemplateSource{}
}

// Curated finds a hand-written template by ID.
func (TemplateSource) Curated(id string) (catalog.Template, bool) {
	return templates.Curated(id)
}

// ByFlow finds the sign-in template whose login declares a flow.
func (TemplateSource) ByFlow(flow string) (catalog.Template, bool) {
	return templates.ByFlow(flow)
}
