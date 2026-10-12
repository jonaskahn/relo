package cursor

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// errorDetailBytes encodes one aiserver.v1.ErrorDetails the way Cursor attaches
// it to a Connect error frame.
func errorDetailBytes(t *testing.T, code int, title, body string, retryable bool) string {
	t.Helper()
	var custom Writer
	custom.String(1, title)
	custom.String(2, body)
	if retryable {
		custom.Bool(4, true)
	}
	var detail Writer
	detail.Uint32(1, uint32(code))
	detail.Message(2, custom.BytesOut())
	return base64.StdEncoding.EncodeToString(detail.BytesOut())
}

func refusalFrame(t *testing.T, code int, title, body string, retryable bool) []byte {
	t.Helper()
	frame, err := json.Marshal(map[string]any{"error": map[string]any{
		"code": "unknown", "message": "fallback text",
		"details": []map[string]string{{
			"type": "aiserver.v1.ErrorDetails", "value": errorDetailBytes(t, code, title, body, retryable),
		}},
	}})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return frame
}

// TestEndStreamClassifies is the verdict a Cursor refusal carries: a bad key,
// a missing model, and a rate limit each mean something different to a
// caller, so each must land on its own status.
func TestEndStreamClassifies(t *testing.T) {
	cases := []struct {
		name      string
		code      int
		status    int
		wantName  string
		retryable bool
	}{
		{"an expired sign-in stops the turn", errTokenExpired, http.StatusUnauthorized, "AUTH_TOKEN_EXPIRED", false},
		{"an unknown model is the caller's to fix", errBadModelName, http.StatusNotFound, "BAD_MODEL_NAME", false},
		{"a rate limit is worth another account", errRateLimited, http.StatusTooManyRequests, "RATE_LIMITED", false},
		{"a permissions refusal stops the turn", errNotHighEnoughPerm, http.StatusForbidden, "NOT_HIGH_ENOUGH_PERMISSIONS", false},
		{"a plan rate limit is worth another account", errProRateLimit, http.StatusTooManyRequests, "PRO_USER_RATE_LIMIT_EXCEEDED", false},
		{"an internal fault is worth another account", 59, http.StatusServiceUnavailable, "ERROR_59", true},
		{"an unclassified code is a bad request", 36, http.StatusBadRequest, "ERROR_36", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refusal, ok := ParseEndStream(refusalFrame(t, tc.code, "", "", false))
			if !ok {
				t.Fatal("ParseEndStream() read no refusal")
			}
			if refusal.Status != tc.status || refusal.Name != tc.wantName || refusal.Retryable != tc.retryable {
				t.Fatalf("ParseEndStream() = %+v, want status %d, name %s, retryable %v",
					refusal, tc.status, tc.wantName, tc.retryable)
			}
		})
	}
}

// TestEndStreamPrefersTheServiceText covers the message an operator reads: the
// title and body the service wrote beat the enum name.
func TestEndStreamPrefersTheServiceText(t *testing.T) {
	refusal, ok := ParseEndStream(refusalFrame(t, errBadModelName, "Unknown model", " claude-9 ", false))
	if !ok {
		t.Fatal("ParseEndStream() read no refusal")
	}
	if refusal.Message != "Unknown model: claude-9" {
		t.Fatalf("Message = %q, want the title and body joined", refusal.Message)
	}
	if !strings.Contains(refusal.Error(), "BAD_MODEL_NAME") {
		t.Fatalf("Error() = %q, want the refusal named", refusal.Error())
	}
}

// TestEndStreamCleanEnd covers the distinction the reader depends on: a stream
// that ended without a refusal names no error at all.
func TestEndStreamCleanEnd(t *testing.T) {
	if _, ok := ParseEndStream(nil); ok {
		t.Fatal("ParseEndStream() read a refusal from an empty end frame")
	}
	if _, ok := ParseEndStream([]byte("not json")); ok {
		t.Fatal("ParseEndStream() read a refusal from a broken end frame")
	}
	if _, ok := ParseEndStream([]byte(`{}`)); ok {
		t.Fatal("ParseEndStream() read a refusal from an end frame without one")
	}
}

// TestEndStreamRetryableFlag covers the service calling a failure transient
// itself, which outranks the status its code would otherwise map to.
func TestEndStreamRetryableFlag(t *testing.T) {
	refusal, ok := ParseEndStream(refusalFrame(t, errBadModelName, "Try later", "", true))
	if !ok || !refusal.Retryable {
		t.Fatalf("ParseEndStream() = %+v %v, want a retryable refusal", refusal, ok)
	}
}

// TestTrailerRefusal covers the second shape the same verdict arrives in: a
// proxy or an HTTP/1 fallback that only carries the gRPC status.
func TestTrailerRefusal(t *testing.T) {
	trailer := http.Header{}
	trailer.Set("Grpc-Status", "7")
	trailer.Set("Grpc-Message", "permission%20denied")
	refusal, ok := TrailerRefusal(trailer)
	if !ok || refusal.Status != http.StatusForbidden || refusal.Message != "permission denied" {
		t.Fatalf("TrailerRefusal() = %+v %v", refusal, ok)
	}
	clean := http.Header{}
	clean.Set("Grpc-Status", "0")
	if _, ok := TrailerRefusal(clean); ok {
		t.Fatal("TrailerRefusal() read a refusal from a clean status")
	}
	if _, ok := TrailerRefusal(http.Header{}); ok {
		t.Fatal("TrailerRefusal() read a refusal from absent trailers")
	}
}
