// The planning text a Flash answer sometimes opens with, read past.
package google

import (
	"encoding/json"
	"strings"
)

// A Flash model sometimes opens its visible answer with the planning it did
// before it, as raw JSON the client never meant to see. Only the opening is
// filtered: the leak is a fixed preamble, so once the text stops looking like
// the preamble everything after it is the answer.
//
// The bound stops one runaway answer from holding memory for the rest of the
// stream. Text past it is released as it stands, because it was never read as
// a leak the filter can vouch for; raise the bound and read the whole object
// if a client turns out to send one longer than this.
const planLeakBound = 8 << 10

// planLeakKeys are the planning fields the preamble carries. One is enough to
// recognise it, so the list stays short rather than trying to name every way a
// model can plan.
var planLeakKeys = []string{"tool_thought", "tool_task", "planning", "thought_process"}

type planFilter struct {
	// held is the opening text still being judged, kept only until it either
	// proves to be a leak or to be the answer.
	held string
	// done records that the opening has been judged, so nothing is held after.
	done bool
}

// text admits one visible delta, holding back an opening that may be planning.
func (f *planFilter) text(delta string) string {
	if f.done || delta == "" {
		return delta
	}
	f.held += delta
	if len(f.held) > planLeakBound || f.judged() {
		return f.release()
	}
	return ""
}

// judged reports whether the held opening can be resolved: either it no longer
// opens like a JSON object, or it is one complete enough to read.
func (f *planFilter) judged() bool {
	trimmed := strings.TrimLeft(f.held, " \t\r\n")
	if trimmed == "" {
		return false
	}
	if trimmed[0] != '{' {
		return true
	}
	return json.Valid([]byte(trimmed)) || strings.HasSuffix(trimmed, "}")
}

// release gives up holding, keeping a resolved planning preamble away from
// the client and handing back everything else. Held text past the bound is
// released as it stands: it was never read as JSON, so it is not a leak the
// filter can vouch for, and dropping it would lose the answer.
func (f *planFilter) release() string {
	held := f.held
	f.held = ""
	f.done = true
	if len(held) <= planLeakBound && isPlanLeak(strings.TrimLeft(held, " \t\r\n")) {
		return ""
	}
	return held
}

// flush releases whatever is still held when the stream ends, so a short
// answer is never lost to a filter that never reached a verdict.
func (f *planFilter) flush() string {
	held := f.held
	f.held = ""
	f.done = true
	return held
}

// isPlanLeak reports whether a complete JSON value is the planning preamble
// rather than something the client meant to read.
func isPlanLeak(value string) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &fields); err != nil {
		return false
	}
	for key := range fields {
		for _, leak := range planLeakKeys {
			if key == leak {
				return true
			}
		}
	}
	return false
}
