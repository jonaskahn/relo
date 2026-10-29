// The refusal a request earns when it cannot fit the target's context.
package server

import (
	"errors"
	"fmt"
)

// contextLimitError carries what the estimate and the limit were, so each
// client surface can say the refusal in the words its client acts on.
type contextLimitError struct {
	Model     string
	Estimated int64
	Limit     int64
}

func (e *contextLimitError) Error() string {
	return fmt.Sprintf("%s: input uses %d tokens but %s accepts %d",
		errContextLimitExceeded, e.Estimated, e.Model, e.Limit)
}

// Unwrap keeps errContextLimitExceeded the condition callers branch on.
func (e *contextLimitError) Unwrap() error { return errContextLimitExceeded }

// contextLimitOf finds the numbers behind a refusal, if it is one.
func contextLimitOf(cause error) (*contextLimitError, bool) {
	var limit *contextLimitError
	if !errors.As(cause, &limit) {
		return nil, false
	}
	return limit, true
}
