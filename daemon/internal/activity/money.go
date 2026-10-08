// Money formatting: micros per million tokens for humans.
package activity

import (
	"fmt"
	"strconv"
)

// FormatMicros renders an integer USD micros amount as dollars.
func FormatMicros(micros int64) string {
	sign := ""
	if micros < 0 {
		sign, micros = "-", -micros
	}
	dollars := strconv.FormatInt(micros/1_000_000, 10)
	return sign + "$" + dollars + "." + fmt.Sprintf("%06d", micros%1_000_000)
}

// FormatMicrosPtr renders a cost that may be unknown, which is what an
// unpriced model leaves behind.
func FormatMicrosPtr(micros *int64) string {
	if micros == nil {
		return "unknown"
	}
	return FormatMicros(*micros)
}
