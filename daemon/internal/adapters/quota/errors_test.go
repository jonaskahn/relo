package quota

import (
	"errors"
	"testing"
)

// TestEmptyAmountIsInspectable covers the empty balance refusal with
// errors.Is, so a caller can tell a missing amount from a malformed one.
func TestEmptyAmountIsInspectable(t *testing.T) {
	var amount openCodeAmount
	if err := amount.UnmarshalJSON([]byte("null")); !errors.Is(err, ErrEmptyAmount) {
		t.Fatalf("UnmarshalJSON(null) = %v, want ErrEmptyAmount", err)
	}
	var stated openCodeAmount
	if err := stated.UnmarshalJSON([]byte(`"12.5"`)); err != nil || stated != 12.5 {
		t.Fatalf("UnmarshalJSON(quoted) = %v, %v, want 12.5 and no error", stated, err)
	}
}
