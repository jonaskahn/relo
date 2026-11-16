// Routing one intercepted request: which side answers it, what the other side
// is asked when the first refuses, and what the request looks like to each.
package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// Binding is what a native model id is bound to: the name Relo's data plane
// resolves for the target, and how the two sides share the request.
type Binding struct {
	// Rewrite is the model Relo is asked for in place of the one the client
	// named.
	Rewrite string
	// VendorFirst asks the vendor first and replays the request to Relo only
	// when the vendor refuses it. Otherwise Relo answers in its place.
	VendorFirst bool
	// VendorFallback replays a request Relo refused to the vendor. It applies
	// only when Relo answers first.
	VendorFallback bool
	// Reason is what the log says decided the request.
	Reason string
}

// side is one of the two places a request can be answered.
type side int

const (
	sideVendor side = iota
	sideDataPlane
)

func (s side) String() string {
	if s == sideDataPlane {
		return "data-plane"
	}
	return "vendor"
}

// plan is a request's route: who answers first, who is asked if that side
// refuses before it has answered, and the body each side is sent.
type plan struct {
	first    side
	second   side
	fallback bool
	// dataBody is what Relo is sent. It differs from the client's body only when
	// a binding rewrote the model.
	dataBody []byte
	reason   string
}

func (p *Proxy) planFor(r *http.Request, body []byte) plan {
	routed := plan{first: sideVendor, dataBody: body, reason: "native"}
	if r.Method != http.MethodPost || p.opts.DataPlaneURL == "" ||
		(r.URL.Path != messagesPath && r.URL.Path != countTokensPath) {
		return routed
	}
	var probe struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return routed
	}
	model := strings.TrimSpace(probe.Model)
	if p.opts.Bind != nil {
		if binding, bound := p.opts.Bind(model); bound {
			return bindingPlan(binding, body)
		}
	}
	if p.opts.Routes != nil && p.opts.Routes(model) {
		return plan{first: sideDataPlane, dataBody: body, reason: "alias"}
	}
	return routed
}

func bindingPlan(binding Binding, body []byte) plan {
	rewritten := rewriteModel(body, binding.Rewrite)
	routed := plan{dataBody: rewritten, reason: binding.Reason}
	if binding.VendorFirst {
		routed.first, routed.second, routed.fallback = sideVendor, sideDataPlane, true
		return routed
	}
	routed.first, routed.second, routed.fallback = sideDataPlane, sideVendor, binding.VendorFallback
	return routed
}

// rewriteModel sets the model of a JSON request body and leaves every other
// field's bytes as they were. A body that is not an object is returned as it
// came.
func rewriteModel(body []byte, model string) []byte {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return body
	}
	encoded, err := json.Marshal(model)
	if err != nil {
		return body
	}
	fields["model"] = encoded
	rewritten, err := json.Marshal(fields)
	if err != nil {
		return body
	}
	return rewritten
}

// handle answers one intercepted request. A model Relo serves goes to Relo's
// own surface, everything else is relayed to the vendor verbatim, and a plan
// with a second side asks it once when the first refuses before answering.
func (p *Proxy) handle(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	route := p.planFor(r, body)
	response, attempted, err := p.attempt(r, body, route)
	if err != nil {
		writeError(w, http.StatusBadGateway, "the upstream is unavailable")
		return
	}
	defer func() { _ = response.Body.Close() }()
	p.opts.Logger.Debug("proxy request", "path", r.URL.Path, "reason", route.reason,
		"side", attempted.String(), "status", response.StatusCode)
	copyHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	flush(w, response.Body)
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "the request could not be read")
		return nil, false
	}
	if len(body) > maxBodyBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "the request is too large")
		return nil, false
	}
	return body, true
}

// attempt asks the first side and, when the plan allows it and that side
// refused before answering, the second. It never asks a third.
func (p *Proxy) attempt(r *http.Request, body []byte, route plan) (*http.Response, side, error) {
	response, err := p.send(r, body, route, route.first)
	if !route.fallback || !refused(r, response, err) {
		return response, route.first, err
	}
	if response != nil {
		_ = response.Body.Close()
	}
	p.opts.Logger.Info("proxy fallback", "path", r.URL.Path, "reason", route.reason,
		"from", route.first.String(), "to", route.second.String())
	response, err = p.send(r, body, route, route.second)
	return response, route.second, err
}

// refused reports whether a side turned the request away before answering it:
// it could not be reached, or it said to wait or that it had failed. A caller's
// own mistake, a 400 or a 413, is not something the other side would fix. A
// client that went away is not a refusal.
func refused(r *http.Request, response *http.Response, err error) bool {
	if err != nil {
		return r.Context().Err() == nil
	}
	return response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError
}

func (p *Proxy) send(r *http.Request, body []byte, route plan, to side) (*http.Response, error) {
	var outbound *http.Request
	var err error
	if to == sideDataPlane {
		target := strings.TrimRight(p.opts.DataPlaneURL, "/") + r.URL.Path
		outbound, err = http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(route.dataBody))
		if err != nil {
			return nil, err
		}
		copyHeaders(outbound.Header, r.Header)
		// The caller's credential belongs to the vendor, so Relo is asked under
		// its own key instead.
		outbound.Header.Del("Authorization")
		outbound.Header.Del("X-Api-Key")
		if p.opts.DataPlaneKey != nil {
			if key := p.opts.DataPlaneKey(); key != "" {
				outbound.Header.Set("X-Api-Key", key)
			}
		}
		return p.client.Do(outbound)
	}
	target := strings.TrimRight(p.opts.Upstream, "/") + r.URL.RequestURI()
	outbound, err = http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	copyHeaders(outbound.Header, r.Header)
	return p.client.Do(outbound)
}
