// Cursor Connect end-stream errors: why the server stopped a stream.
//
// A Cursor RPC that refuses mid-stream answers with an end-of-stream frame
// carrying a Connect error envelope, and gRPC trailers repeat the verdict as a
// status code. Both name the same refusal in two shapes, so both are read here
// and answered in one vocabulary.
package cursor

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// EndStream is one refusal Cursor reported for a stream it opened. Code is the
// service's own enum, Name its spelling, Message the human text, and Status
// what that code means to a caller: a credential or model problem is the
// caller's to fix, while a rate limit or a provider fault is worth another
// account. Retryable reports that the service itself called the failure
// transient, whatever the status says.
type EndStream struct {
	Code      int
	Name      string
	Message   string
	Status    int
	Retryable bool
}

// Error names the refusal in the caller's terms.
func (e EndStream) Error() string {
	if e.Message == "" {
		return "cursor refused the request: " + e.Name
	}
	return "cursor refused the request: " + e.Name + ": " + e.Message
}

// Cursor's service error enum, as the pinned reference client spells it. Only
// the codes Relo classifies are named: an unnamed code still classifies by
// number, and its number is what the message carries.
const (
	errBadAPIKey              = 1
	errNotLoggedIn            = 2
	errInvalidAuthID          = 3
	errNotHighEnoughPerm      = 4
	errBadModelName           = 5
	errUserNotFound           = 6
	errFreeRateLimit          = 7
	errProRateLimit           = 8
	errFreeUsageLimit         = 9
	errProUsageLimit          = 10
	errTokenNotFound          = 11
	errTokenExpired           = 12
	errUpstreamRateLimit      = 14
	errRequiresLogin          = 18
	errMaxTokens              = 20
	errUserAborted            = 21
	errGenericRateLimit       = 22
	errProUserOnly            = 23
	errVisionRateLimit        = 28
	errFileNotFound           = 33
	errAPIKeyRateLimit        = 34
	errDebounced              = 35
	errUnauthorized           = 38
	errNotFound               = 39
	errResourceExhausted      = 41
	errBadUserAPIKey          = 42
	errConversationLong       = 43
	errUsagePricingRequired   = 44
	errPricingRequiredChange  = 45
	errNoGitHubCredentials    = 46
	errNoGitHubAccess         = 47
	errNoGitHubAppAccess      = 48
	errMultipleGitHubOwners   = 49
	errRateLimited            = 50
	errRateLimitedChangeable  = 51
	errSuspiciousUsageBlocked = 54
	errModelBlocked           = 58
	errModelUnsupported       = 61
	errUnsupportedRegion      = 64
	errAccountClosed          = 65
)

var endStreamNames = map[int]string{
	errBadAPIKey:              "BAD_API_KEY",
	errNotLoggedIn:            "NOT_LOGGED_IN",
	errInvalidAuthID:          "INVALID_AUTH_ID",
	errNotHighEnoughPerm:      "NOT_HIGH_ENOUGH_PERMISSIONS",
	errBadModelName:           "BAD_MODEL_NAME",
	errUserNotFound:           "USER_NOT_FOUND",
	errFreeRateLimit:          "FREE_USER_RATE_LIMIT_EXCEEDED",
	errProRateLimit:           "PRO_USER_RATE_LIMIT_EXCEEDED",
	errFreeUsageLimit:         "FREE_USER_USAGE_LIMIT",
	errProUsageLimit:          "PRO_USER_USAGE_LIMIT",
	errTokenNotFound:          "AUTH_TOKEN_NOT_FOUND",
	errTokenExpired:           "AUTH_TOKEN_EXPIRED",
	errUpstreamRateLimit:      "OPENAI_RATE_LIMIT_EXCEEDED",
	errRequiresLogin:          "AGENT_REQUIRES_LOGIN",
	errMaxTokens:              "MAX_TOKENS",
	errUserAborted:            "USER_ABORTED_REQUEST",
	errGenericRateLimit:       "GENERIC_RATE_LIMIT_EXCEEDED",
	errProUserOnly:            "PRO_USER_ONLY",
	errVisionRateLimit:        "GPT_4_VISION_PREVIEW_RATE_LIMIT",
	errFileNotFound:           "FILE_NOT_FOUND",
	errAPIKeyRateLimit:        "API_KEY_RATE_LIMIT",
	errDebounced:              "DEBOUNCED",
	errUnauthorized:           "UNAUTHORIZED",
	errNotFound:               "NOT_FOUND",
	errResourceExhausted:      "RESOURCE_EXHAUSTED",
	errBadUserAPIKey:          "BAD_USER_API_KEY",
	errConversationLong:       "CONVERSATION_TOO_LONG",
	errUsagePricingRequired:   "USAGE_PRICING_REQUIRED",
	errPricingRequiredChange:  "USAGE_PRICING_REQUIRED_CHANGEABLE",
	errNoGitHubCredentials:    "GITHUB_NO_USER_CREDENTIALS",
	errNoGitHubAccess:         "GITHUB_USER_NO_ACCESS",
	errNoGitHubAppAccess:      "GITHUB_APP_NO_ACCESS",
	errMultipleGitHubOwners:   "GITHUB_MULTIPLE_OWNERS",
	errRateLimited:            "RATE_LIMITED",
	errRateLimitedChangeable:  "RATE_LIMITED_CHANGEABLE",
	errSuspiciousUsageBlocked: "SUSPICIOUS_USAGE_BLOCKED",
	errModelBlocked:           "MODEL_BLOCKED",
	errModelUnsupported:       "MODEL_NO_LONGER_SUPPORTED",
	errUnsupportedRegion:      "UNSUPPORTED_REGION",
	errAccountClosed:          "ACCOUNT_CLOSED",
}

// endStreamStatuses groups the service's error codes by the status each one
// means to a caller. The grouping is the reference client's own map, so a
// refusal classifies the way the released Cursor CLI classifies it: a rate
// limit stays worth another account, everything else is the caller's to fix.
var endStreamStatuses = []struct {
	status int
	codes  []int
}{
	{http.StatusUnauthorized, []int{errBadAPIKey, errNotLoggedIn, errInvalidAuthID, errTokenNotFound,
		errTokenExpired, errRequiresLogin, errUnauthorized, errBadUserAPIKey, errAccountClosed}},
	{http.StatusForbidden, []int{errNotHighEnoughPerm, errProUserOnly, errNoGitHubCredentials,
		errNoGitHubAccess, errNoGitHubAppAccess, errMultipleGitHubOwners, errSuspiciousUsageBlocked,
		errModelBlocked, errUnsupportedRegion}},
	{http.StatusNotFound, []int{errBadModelName, errUserNotFound, errFileNotFound, errNotFound, errModelUnsupported}},
	{http.StatusTooManyRequests, []int{errFreeRateLimit, errProRateLimit, errFreeUsageLimit,
		errProUsageLimit, errUpstreamRateLimit, errGenericRateLimit, errVisionRateLimit,
		errAPIKeyRateLimit, errDebounced, errResourceExhausted, errUsagePricingRequired,
		errPricingRequiredChange, errRateLimited, errRateLimitedChangeable}},
	{http.StatusRequestEntityTooLarge, []int{errMaxTokens, errConversationLong}},
}

// retryableCodes are the service codes that mean a transient fault, so a
// refusal carrying one is worth another account rather than a fix.
var retryableCodes = map[int]bool{
	13: true, 25: true, 55: true, 56: true, 57: true, 59: true, 63: true,
}

func statusForCode(code int) int {
	for _, group := range endStreamStatuses {
		for _, member := range group.codes {
			if member == code {
				return group.status
			}
		}
	}
	return http.StatusBadRequest
}

// endStreamDetail is one binary error detail attached to a Connect error.
type endStreamDetail struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// endStreamEnvelope is the Connect error frame Cursor sends.
type endStreamEnvelope struct {
	Error *struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Details []endStreamDetail `json:"details"`
	} `json:"error"`
}

// ErrorDetails field numbers inside aiserver.v1.ErrorDetails, and the
// CustomErrorDetails message nested in it.
const (
	detailCode     = 1
	detailCustom   = 2
	customTitle    = 1
	customBody     = 2
	customRetry    = 4
	detailTypeName = "aiserver.v1.ErrorDetails"
)

// ParseEndStream reads the refusal one end-of-stream payload carries. It
// reports false when the payload named no error, which is how a stream that
// ended cleanly differs from one that was refused.
func ParseEndStream(payload []byte) (EndStream, bool) {
	var envelope endStreamEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Error == nil {
		return EndStream{}, false
	}
	detail, ok := errorDetail(envelope.Error.Details)
	refusal := EndStream{
		Name:    envelope.Error.Code,
		Message: envelope.Error.Message,
		Status:  http.StatusBadGateway,
	}
	if ok {
		refusal.Code = detail.code
		refusal.Name = errorName(detail.code)
		if detail.message != "" {
			refusal.Message = detail.message
		}
		refusal.Retryable = detail.retryable
		refusal.Status = statusForCode(detail.code)
	}
	if retryableCodes[refusal.Code] {
		refusal.Retryable = true
		refusal.Status = http.StatusServiceUnavailable
	}
	return refusal, true
}

// errorDetail decodes the service's own error enum out of the frame's binary
// detail. The Connect code alone is a string like "unknown"; the enum is what
// says whether the refusal is a bad key, a missing model, or a rate limit.
func errorDetail(details []endStreamDetail) (parsedDetail, bool) {
	for _, entry := range details {
		if entry.Type != detailTypeName && entry.Type != "type.googleapis.com/"+detailTypeName {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(entry.Value)
		if err != nil {
			continue
		}
		fields, err := Parse(raw)
		if err != nil {
			continue
		}
		detail := parsedDetail{code: int(Uint(fields, detailCode))}
		custom, err := Nested(fields, detailCustom)
		if err == nil && len(custom) > 0 {
			detail.message = joinDetail(FirstString(custom[0], customTitle), FirstString(custom[0], customBody), errorName(detail.code))
			detail.retryable = Uint(custom[0], customRetry) != 0
		}
		return detail, true
	}
	return parsedDetail{}, false
}

type parsedDetail struct {
	code      int
	message   string
	retryable bool
}

func errorName(code int) string {
	if name, known := endStreamNames[code]; known {
		return name
	}
	return "ERROR_" + strconv.Itoa(code)
}

// joinDetail prefers the service's own title and body over the enum name, so
// the message an operator reads is the one Cursor wrote.
func joinDetail(title, body, name string) string {
	parts := make([]string, 0, 2)
	for _, part := range []string{title, body} {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	switch len(parts) {
	case 0:
		return name
	case 1:
		return parts[0]
	default:
		return parts[0] + ": " + parts[1]
	}
}

// TrailerRefusal reads the gRPC trailers a refused stream ends with. Connect
// answers with the frame above, but a proxy or an HTTP/1 fallback may only
// carry the status, so both shapes are read.
func TrailerRefusal(trailer http.Header) (EndStream, bool) {
	status := trailer.Get("Grpc-Status")
	if status == "" || status == "0" {
		return EndStream{}, false
	}
	message, err := url.QueryUnescape(trailer.Get("Grpc-Message"))
	if err != nil {
		message = trailer.Get("Grpc-Message")
	}
	return EndStream{
		Name:    "gRPC status " + status,
		Message: message,
		Status:  statusForGRPC(status),
	}, true
}

// statusForGRPC maps a gRPC status onto the HTTP status a caller sees. The
// codes Cursor answers with are the standard gRPC set.
func statusForGRPC(status string) int {
	switch status {
	case "3":
		return http.StatusBadRequest
	case "5":
		return http.StatusNotFound
	case "7":
		return http.StatusForbidden
	case "8":
		return http.StatusTooManyRequests
	case "13":
		return http.StatusInternalServerError
	case "14":
		return http.StatusServiceUnavailable
	case "16":
		return http.StatusUnauthorized
	default:
		return http.StatusBadGateway
	}
}
