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

// CardProfile is one card's earning rules, for comparing it against other
// cards' rules per spend category.
type CardProfile struct {
	ID          string
	BaseRateBps int32
	Rules       []CategoryRule
}

// rateForCategory returns the rate (and cap, if any) this card pays on a
// category: its bonus rule if it has one, otherwise its uncapped base rate.
func (c CardProfile) rateForCategory(categoryName string) (rateBps int32, capCents *int64) {
	for _, rule := range c.Rules {
		if rule.CategoryName == categoryName {
			return rule.RateBps, rule.CapCents
		}
	}
	return c.BaseRateBps, nil
}

// CategoryWinner is which card earned a spend category's rewards in a
// PortfolioRewards comparison, and at what rate.
type CategoryWinner struct {
	CardID  string
	RateBps int32
}

// PortfolioRewards sums what a set of cards earns on spend when each
// category routes to whichever card in the set pays the best rate for it -
// the way a rational cardholder actually spends, rather than putting every
// purchase on one card. It also reports which card won each category, so a
// caller can explain why a card beats what the user already has.
//
// Spend that overflows the winning card's cap keeps earning on that same
// card's base rate rather than shifting to a second card's bonus rate;
// routing overflow across cards is portfolio optimization (Phase 6), not
// single-card recommendation (Phase 5).
//
// Ties keep the earlier card in `cards`, so a currently-held card beats an
// equally-rated candidate instead of crediting a new card for no gain.
func PortfolioRewards(cards []CardProfile, spend []CategorySpend) (totalCents int64, winners map[string]CategoryWinner) {
	winners = make(map[string]CategoryWinner, len(spend))
	for _, s := range spend {
		var best CardProfile
		var bestRate int32 = -1
		var bestCap *int64
		for _, c := range cards {
			rate, cap := c.rateForCategory(s.CategoryName)
			if rate > bestRate {
				best, bestRate, bestCap = c, rate, cap
			}
		}
		if bestRate < 0 {
			continue // no cards to earn with
		}

		bonusCents := s.TotalCents
		if bestCap != nil && bonusCents > *bestCap {
			bonusCents = *bestCap
		}
		totalCents += earnCents(bonusCents, bestRate)
		totalCents += earnCents(s.TotalCents-bonusCents, best.BaseRateBps)
		winners[s.CategoryName] = CategoryWinner{CardID: best.ID, RateBps: bestRate}
	}
	return totalCents, winners
}

// AnnualValueCents nets a card's rewards earned over a year against its
// annual fee - the number that determines whether a card is worth holding.
func AnnualValueCents(rewardsEarnedCents int64, annualFeeCents int32) int64 {
	return rewardsEarnedCents - int64(annualFeeCents)
}
