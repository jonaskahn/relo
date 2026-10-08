package catalog

// Prices is what a million tokens of one model cost, in integer USD micros.
// A nil field means the rate is unknown, which stays distinct from a rate of
// zero, so an unpriced model never reports a cost of nothing.
//
// The ext tier prices a request whose prompt exceeds ExtThreshold. A model
// with no long-context tier leaves every ext field nil.
type Prices struct {
	Input         *int64
	Output        *int64
	CacheRead     *int64
	CacheWrite    *int64
	ExtThreshold  *int64
	ExtInput      *int64
	ExtOutput     *int64
	ExtCacheRead  *int64
	ExtCacheWrite *int64
}

// Overlay layers an operator's rates over the published ones, field by
// field, so a rate the operator did not set keeps the catalog value.
func (p Prices) Overlay(override Prices) Prices {
	return Prices{
		Input:         pick(override.Input, p.Input),
		Output:        pick(override.Output, p.Output),
		CacheRead:     pick(override.CacheRead, p.CacheRead),
		CacheWrite:    pick(override.CacheWrite, p.CacheWrite),
		ExtThreshold:  pick(override.ExtThreshold, p.ExtThreshold),
		ExtInput:      pick(override.ExtInput, p.ExtInput),
		ExtOutput:     pick(override.ExtOutput, p.ExtOutput),
		ExtCacheRead:  pick(override.ExtCacheRead, p.ExtCacheRead),
		ExtCacheWrite: pick(override.ExtCacheWrite, p.ExtCacheWrite),
	}
}

// Known reports whether any rate is stated, which is what decides if a
// request can be priced at all.
func (p Prices) Known() bool {
	return p.Input != nil || p.Output != nil
}

// LongContext reports whether the ext tier applies to a prompt of the given
// size. The threshold doubles as the switch: a model without one never uses
// the tier.
func (p Prices) LongContext(promptTokens int64) bool {
	return p.ExtThreshold != nil && promptTokens > *p.ExtThreshold
}

func pick(override, catalog *int64) *int64 {
	if override != nil {
		return override
	}
	return catalog
}
