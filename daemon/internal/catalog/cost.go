package catalog

// CostSource names where a price came from.
type CostSource string

const (
	// CostFromCatalog is the rate the catalog holds, whoever published it.
	CostFromCatalog CostSource = "catalog"
	// CostUnknown is the absence of any price.
	CostUnknown CostSource = "unknown"
)

const tokensPerMillion = 1_000_000

// TokenCounts are the counts one usage event reports. Input is inclusive of the
// cache read and cache write counts, the way providers report it.
type TokenCounts struct {
	Input      int64
	Output     int64
	CacheRead  int64
	CacheWrite int64
}

// UncountedInput returns the prompt tokens billed at the plain input rate, so
// a cached token is never charged twice.
func (t TokenCounts) UncountedInput() int64 {
	plain := t.Input - t.CacheRead - t.CacheWrite
	if plain < 0 {
		return 0
	}
	return plain
}

// Cost is the priced result of one usage event. A nil Micros means no price
// is known, which stays NULL in storage instead of becoming a zero cost.
type Cost struct {
	Micros *int64
	Source CostSource
}

// CostOf returns the cost of one event at the given rates. A model with no
// published rate stays unpriced rather than free, so a usage row says the
// price is unknown instead of reporting a cost of nothing.
func CostOf(prices Prices, tokens TokenCounts) Cost {
	rates := effectiveRates(prices, tokens.Input)
	if rates.Input == nil && rates.Output == nil {
		return Cost{Source: CostUnknown}
	}
	total := micros(tokens.UncountedInput(), rates.Input) +
		micros(tokens.Output, rates.Output) +
		micros(tokens.CacheRead, rateOr(rates.CacheRead, rates.Input)) +
		micros(tokens.CacheWrite, rateOr(rates.CacheWrite, rates.Input))
	return Cost{Micros: &total, Source: CostFromCatalog}
}

func effectiveRates(prices Prices, promptTokens int64) Prices {
	if !prices.LongContext(promptTokens) {
		return prices
	}
	return Prices{
		Input:      rateOr(prices.ExtInput, prices.Input),
		Output:     rateOr(prices.ExtOutput, prices.Output),
		CacheRead:  rateOr(prices.ExtCacheRead, rateOr(prices.CacheRead, prices.Input)),
		CacheWrite: rateOr(prices.ExtCacheWrite, rateOr(prices.CacheWrite, prices.Input)),
	}
}

func rateOr(rate, fallback *int64) *int64 {
	if rate != nil {
		return rate
	}
	return fallback
}

func micros(tokens int64, rate *int64) int64 {
	if tokens <= 0 || rate == nil || *rate <= 0 {
		return 0
	}
	return (tokens**rate + tokensPerMillion/2) / tokensPerMillion
}
