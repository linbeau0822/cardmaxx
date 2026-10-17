// Package reward computes what a credit card earns on a given spending
// pattern. It is pure arithmetic over integer cents and basis points - no
// database access, no HTTP - so it is trivially testable and reusable by
// both the per-card "what does this card earn me" calculation (Phase 4c)
// and the recommendation/optimization engine (Phase 5-6).
package reward

// CategoryRule is one bonus-category reward rule for a card: it earns
// RateBps on spend in CategoryName, up to CapCents per year (nil = uncapped).
// Spend beyond the cap earns the card's base rate instead.
type CategoryRule struct {
	CategoryName string
	RateBps      int32
	CapCents     *int64
}

// CategorySpend is a user's total spend in one category over whatever period
// is being evaluated (the caller decides the window - e.g. trailing 12
// months - CalculateRewards just applies rates to it).
type CategorySpend struct {
	CategoryName string
	TotalCents   int64
}

// CalculateRewards totals the rewards a card earns on spend: each category
// matching a rule earns that rule's rate up to its cap, with any spend above
// the cap (and spend in categories with no rule) earning the card's base
// rate.
func CalculateRewards(baseRateBps int32, rules []CategoryRule, spend []CategorySpend) int64 {
	byCategory := make(map[string]CategoryRule, len(rules))
	for _, rule := range rules {
		byCategory[rule.CategoryName] = rule
	}

	var totalCents int64
	for _, s := range spend {
		rule, ok := byCategory[s.CategoryName]
		if !ok {
			totalCents += earnCents(s.TotalCents, baseRateBps)
			continue
		}
		bonusSpendCents := s.TotalCents
		if rule.CapCents != nil && bonusSpendCents > *rule.CapCents {
			bonusSpendCents = *rule.CapCents
		}
		totalCents += earnCents(bonusSpendCents, rule.RateBps)
		totalCents += earnCents(s.TotalCents-bonusSpendCents, baseRateBps)
	}
	return totalCents
}

// earnCents applies a basis-point rate to a cents amount, truncating to
// whole cents like the card issuer would.
func earnCents(cents int64, rateBps int32) int64 {
	return cents * int64(rateBps) / 10000
}

// AnnualValueCents nets a card's rewards earned over a year against its
// annual fee - the number that determines whether a card is worth holding.
func AnnualValueCents(rewardsEarnedCents int64, annualFeeCents int32) int64 {
	return rewardsEarnedCents - int64(annualFeeCents)
}
