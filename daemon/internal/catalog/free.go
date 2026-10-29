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

// IsFreeModel reports whether a conversational model is free. A provider or
// listing mark is enough, and so are explicit zero input and output rates.
// Missing rates prove nothing. The identifier rule is the fallback for a
// gateway that names free models without stating either mark or a price.
func IsFreeModel(providerFree, listingFree *bool, id string, input, output *int64, category Category) bool {
	if !IsConversationalCategory(category) {
		return false
	}
	if providerFree != nil && *providerFree {
		return true
	}
	if listingFree != nil && *listingFree {
		return true
	}
	if input != nil && output != nil && *input == 0 && *output == 0 {
		return true
	}
	return IDFreeToken(id)
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
