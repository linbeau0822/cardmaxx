// Package reward computes what a credit card earns on a given spending
// pattern. It is pure arithmetic over integer cents and basis points - no
// database access, no HTTP - so it is trivially testable and reusable by
// both the per-card "what does this card earn me" calculation (Phase 4c)
// and the recommendation/optimization engine (Phase 5-6).
package reward

import "sort"

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

// rateTier is one card's claim on a category's spend at a given rate, up to
// capCents (nil = uncapped).
type rateTier struct {
	cardID   string
	rateBps  int32
	capCents *int64
}

// tiersForCategory returns this card's rate tiers for a category: its bonus
// rule (if it has one for this category) ranked ahead of its own base rate,
// which is always included as the uncapped fallback once any bonus cap is
// exhausted.
func (c CardProfile) tiersForCategory(categoryName string) []rateTier {
	for _, rule := range c.Rules {
		if rule.CategoryName == categoryName {
			return []rateTier{
				{cardID: c.ID, rateBps: rule.RateBps, capCents: rule.CapCents},
				{cardID: c.ID, rateBps: c.BaseRateBps},
			}
		}
	}
	return []rateTier{{cardID: c.ID, rateBps: c.BaseRateBps}}
}

// CategoryAllocation is the portion of a category's spend that one card
// earns, and at what rate, within a PortfolioRewards comparison.
type CategoryAllocation struct {
	CardID     string
	RateBps    int32
	SpendCents int64
}

// PortfolioRewards sums what a set of cards earns on spend when each
// category routes to whichever card in the set pays the best rate for it -
// the way a rational cardholder actually spends, rather than putting every
// purchase on one card. Spend that overflows a card's bonus cap spills to
// the next-best rate among every tier in the set - another card's bonus or
// base rate, or (last) the spilling card's own base rate - rather than
// stopping at that one card, so a multi-card portfolio gets credit for
// routing spend to whichever card actually covers it. It also reports each
// category's allocations (highest rate first), so a caller can explain why
// a card beats what the user already has.
//
// Ties keep the earlier card in `cards`, so a currently-held card beats an
// equally-rated candidate instead of crediting a new card for no gain.
func PortfolioRewards(cards []CardProfile, spend []CategorySpend) (totalCents int64, allocations map[string][]CategoryAllocation) {
	allocations = make(map[string][]CategoryAllocation, len(spend))
	for _, s := range spend {
		var tiers []rateTier
		for _, c := range cards {
			tiers = append(tiers, c.tiersForCategory(s.CategoryName)...)
		}
		sort.SliceStable(tiers, func(i, j int) bool { return tiers[i].rateBps > tiers[j].rateBps })

		remainingCents := s.TotalCents
		for _, t := range tiers {
			if remainingCents <= 0 {
				break
			}
			amountCents := remainingCents
			if t.capCents != nil && amountCents > *t.capCents {
				amountCents = *t.capCents
			}
			if amountCents <= 0 {
				continue
			}
			totalCents += earnCents(amountCents, t.rateBps)
			allocations[s.CategoryName] = append(allocations[s.CategoryName], CategoryAllocation{CardID: t.cardID, RateBps: t.rateBps, SpendCents: amountCents})
			remainingCents -= amountCents
		}
	}
	return totalCents, allocations
}

// AnnualValueCents nets a card's rewards earned over a year against its
// annual fee - the number that determines whether a card is worth holding.
func AnnualValueCents(rewardsEarnedCents int64, annualFeeCents int32) int64 {
	return rewardsEarnedCents - int64(annualFeeCents)
}
