package antigravity

import (
	"strings"
	"testing"
)

// TestRefusalNamesTheVendorReason covers the refusal that is not a throttle: a
// disabled account arrives as 403 with a reason code, and an operator needs to
// read why rather than watch the account be retried as if it were busy.
func TestRefusalNamesTheVendorReason(t *testing.T) {
	const body = `{"error":{"code":403,"message":"This service has been disabled in this account for violation of Terms of Service.","status":"PERMISSION_DENIED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"TOS_VIOLATION","domain":"cloudcode-pa.googleapis.com"}]}}`
	code, detail := Refusal([]byte(body))
	if code != "PERMISSION_DENIED (TOS_VIOLATION)" {
		t.Fatalf("Refusal() code = %q, want the status and the reason", code)
	}
	if !strings.Contains(detail, "violation of Terms of Service") {
		t.Fatalf("Refusal() detail = %q, want the vendor's words", detail)
	}
}

// TestRefusalWithoutADetailKeepsTheStatus covers the ordinary refusal, which
// names no reason code: the gRPC status is the part that always survives.
func TestRefusalWithoutADetailKeepsTheStatus(t *testing.T) {
	code, detail := Refusal([]byte(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"Resource has been exhausted (e.g. check quota)."}}`))
	if code != "RESOURCE_EXHAUSTED" {
		t.Fatalf("Refusal() code = %q, want the plain status", code)
	}
	if detail != "Resource has been exhausted (e.g. check quota)." {
		t.Fatalf("Refusal() detail = %q, want the message", detail)
	}
}

// TestRefusalOfAnUnreadableBodyYieldsNothing keeps the caller's fallback: a
// body that is not a JSON envelope is not an error of Relo's own.
func TestRefusalOfAnUnreadableBodyYieldsNothing(t *testing.T) {
	if code, detail := Refusal([]byte("not json")); code != "" || detail != "" {
		t.Fatalf("Refusal() = %q, %q, want nothing", code, detail)
	}
}
