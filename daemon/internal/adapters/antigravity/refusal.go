// Antigravity refusals: reading vendor error codes and reasons.
package antigravity

import (
	"encoding/json"
	"fmt"
	"strings"
)

const refusalDetailLimit = 200

// Refusal reads the vendor's machine-readable reason out of a Cloud Code
// Assist refusal body: the gRPC status (for example PERMISSION_DENIED), the
// reason code the vendor attaches when it names one (for example
// TOS_VIOLATION), and the message, collapsed to one bounded line. A body Relo
// cannot read yields nothing rather than an error of its own, because the HTTP
// status is the part that always survives.
func Refusal(body []byte) (code, detail string) {
	envelope := struct {
		Error struct {
			Status  string `json:"status"`
			Message string `json:"message"`
			Details []struct {
				Reason string `json:"reason"`
			} `json:"details"`
		} `json:"error"`
	}{}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", ""
	}
	code = strings.TrimSpace(envelope.Error.Status)
	if reason := firstReason(envelope.Error.Details); reason != "" {
		code = firstNonEmpty(code+" ("+reason+")", code)
	}
	return code, sentence(envelope.Error.Message)
}

// firstReason reports the reason code the vendor named in the refusal details.
func firstReason(details []struct {
	Reason string `json:"reason"`
}) string {
	for _, detail := range details {
		if reason := strings.TrimSpace(detail.Reason); reason != "" {
			return reason
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func sentence(message string) string {
	collapsed := strings.Join(strings.Fields(message), " ")
	runes := []rune(collapsed)
	if len(runes) <= refusalDetailLimit {
		return collapsed
	}
	return string(runes[:refusalDetailLimit]) + "…"
}

// Reason names one refused call the way an operator reads it: the HTTP
// status, the vendor's own code when the body carried one, and its message.
func Reason(status int, code, detail string) string {
	description := fmt.Sprintf("the endpoint answered %d", status)
	if code != "" {
		description += " (" + code + ")"
	}
	if detail != "" {
		description += ": " + detail
	}
	return description
}
