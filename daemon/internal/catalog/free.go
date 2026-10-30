package catalog

import (
	"regexp"
	"strings"
)

// freeIDToken matches the word free where it is one token in an identifier:
// -free, _free, :free, /free, and a space-delimited form all count, while a
// word such as freedom does not.
var freeIDToken = regexp.MustCompile(`(?:^|[-_:/ ])free(?:$|[-_:/ ])`)

// IsConversationalCategory reports the model categories a free-chat count
// covers. Media, embedding, and unknown categories are never counted.
func IsConversationalCategory(category Category) bool {
	switch category {
	case CategoryChat, CategoryReasoning, CategoryVision:
		return true
	default:
		return false
	}
}

// IDFreeToken reports whether an identifier names free as one of its own
// tokens.
func IDFreeToken(id string) bool {
	return freeIDToken.MatchString(strings.ToLower(strings.TrimSpace(id)))
}

// FreeModelFacts are the signs one model entry offers about its cost.
type FreeModelFacts struct {
	ProviderMark *bool
	ListingMark  *bool
	ID           string
	Prices       Prices
	Category     Category
}

// IsFreeModel reports whether a conversational model is free. A provider or
// listing mark is enough, and so are explicit zero input and output rates.
// Missing rates prove nothing. The identifier rule is the fallback for a
// gateway that names free models without stating either mark or a price.
func IsFreeModel(model FreeModelFacts) bool {
	if !IsConversationalCategory(model.Category) {
		return false
	}
	if model.ProviderMark != nil && *model.ProviderMark {
		return true
	}
	if model.ListingMark != nil && *model.ListingMark {
		return true
	}
	if model.Prices.Input != nil && model.Prices.Output != nil &&
		*model.Prices.Input == 0 && *model.Prices.Output == 0 {
		return true
	}
	return IDFreeToken(model.ID)
}

// FreePrices projects a provider or listing free mark into explicit zero
// input and output rates when no rates were stated. Persisting those facts is
// what lets availability survive a reload without a schema change.
func FreePrices(prices Prices, mark *bool) Prices {
	if mark == nil || !*mark || prices.Input != nil || prices.Output != nil {
		return prices
	}
	zero := int64(0)
	prices.Input = &zero
	prices.Output = &zero
	return prices
}
