package config

import "fmt"

// QuotaDisplayUsed and QuotaDisplayRemaining are the two ways a quota chart
// reads: the share an account has spent, or the share it has left. Used is
// the default, so an install that never chose keeps the reading every other
// surface states.
const (
	QuotaDisplayUsed      = "used"
	QuotaDisplayRemaining = "remaining"
)

// ValidateQuotaDisplay reports a reading nothing draws.
func ValidateQuotaDisplay(display string) error {
	switch display {
	case QuotaDisplayUsed, QuotaDisplayRemaining:
		return nil
	default:
		return fmt.Errorf("ui.quota_display: %w", ErrInvalidQuotaDisplay)
	}
}
