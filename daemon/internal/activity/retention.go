package activity

import "errors"

// MinUsageDays is the shortest request-detail window the ledger keeps. No
// deletion rule touches a row from a day inside it, whatever budget it is
// measured against, so the console can promise a floor on every cleanup.
const MinUsageDays = 3

var (
	// ErrInvalidRetention reports a budget a retention run cannot honor.
	ErrInvalidRetention = errors.New("invalid retention settings")
	// ErrRetentionBusy reports a cleanup that is already running, which a
	// caller answers without waiting behind it.
	ErrRetentionBusy = errors.New("retention cleanup is already running")
)

// RetentionBudget is the usage-log budget: how long events are kept, how many,
// and how many bytes they may occupy. A zero field leaves that budget
// unlimited, so a fresh install keeps everything until an operator says
// otherwise.
type RetentionBudget struct {
	UsageDays   int
	MaxEvents   int64
	MaxBytes    int64
	UpdatedAtMs int64
}

// RetentionReport is what one retention run did, or what a preview would do.
// Its field names are the ones the console reads, so a rename is an API
// change.
type RetentionReport struct {
	Config              RetentionBudget
	AgeCutoffMs         int64
	RowsBefore          int64
	RowsDeleted         int64
	LiveBytesBefore     int64
	LiveBytesAfter      int64
	EstimatedBytesFreed int64
}

// CleanupReport is what one maintenance sweep did: the days it closed before
// deleting anything, the request rows the budget removed, the captured bodies
// it purged, and the space the database held before and after compaction.
type CleanupReport struct {
	FinalizedDays   int
	RowsDeleted     int64
	CapturesDeleted int64
	LiveBytesBefore int64
	LiveBytesAfter  int64
	Vacuumed        bool
}
