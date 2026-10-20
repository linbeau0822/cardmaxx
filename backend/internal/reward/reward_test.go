package reward

import (
	"reflect"
	"testing"
)

func capCents(cents int64) *int64 { return &cents }

func TestCalculateRewards(t *testing.T) {
	tests := map[string]struct {
		baseRateBps int32
		rules       []CategoryRule
		spend       []CategorySpend
		want        int64
	}{
		"no bonus categories earns base rate on everything": {
			baseRateBps: 150, // Freedom Unlimited: flat 1.5%
			spend:       []CategorySpend{{CategoryName: "FOOD_AND_DRINK", TotalCents: 10000}},
			want:        150, // 1.5% of $100
		},
		"uncapped bonus category earns the bonus rate": {
			baseRateBps: 100, // Sapphire Preferred
			rules:       []CategoryRule{{CategoryName: "TRAVEL", RateBps: 200}},
			spend:       []CategorySpend{{CategoryName: "TRAVEL", TotalCents: 10000}},
			want:        200, // 2% of $100, not the 1% base rate
		},
		"capped bonus category falls back to base rate past the cap": {
			baseRateBps: 100, // Amex Gold
			rules:       []CategoryRule{{CategoryName: "GENERAL_MERCHANDISE", RateBps: 400, CapCents: capCents(2_500_000)}},
			spend:       []CategorySpend{{CategoryName: "GENERAL_MERCHANDISE", TotalCents: 3_000_000}},
			// 4% on the first $25,000, 1% base on the remaining $5,000.
			want: 2_500_000*400/10000 + 500_000*100/10000,
		},
		"spend in a category with no rule earns the base rate": {
			baseRateBps: 100,
			rules:       []CategoryRule{{CategoryName: "TRAVEL", RateBps: 200}},
			spend:       []CategorySpend{{CategoryName: "TRANSPORTATION", TotalCents: 10000}},
			want:        100,
		},
		"multiple categories sum": {
			baseRateBps: 100,
			rules: []CategoryRule{
				{CategoryName: "TRAVEL", RateBps: 200},
				{CategoryName: "FOOD_AND_DRINK", RateBps: 300},
			},
			spend: []CategorySpend{
				{CategoryName: "TRAVEL", TotalCents: 10000},
				{CategoryName: "FOOD_AND_DRINK", TotalCents: 5000},
				{CategoryName: "GENERAL_MERCHANDISE", TotalCents: 2000},
			},
			want: 200 + 150 + 20,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := CalculateRewards(tc.baseRateBps, tc.rules, tc.spend)
			if got != tc.want {
				t.Errorf("CalculateRewards() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestPortfolioRewards(t *testing.T) {
	flat := CardProfile{ID: "flat", BaseRateBps: 100}
	flat2 := CardProfile{ID: "flat-2", BaseRateBps: 100}
	dining := CardProfile{ID: "dining", BaseRateBps: 100, Rules: []CategoryRule{{CategoryName: "FOOD_AND_DRINK", RateBps: 400}}}
	cappedDining := CardProfile{ID: "capped-dining", BaseRateBps: 100, Rules: []CategoryRule{{CategoryName: "FOOD_AND_DRINK", RateBps: 400, CapCents: capCents(500_000)}}}
	betterFlat := CardProfile{ID: "better-flat", BaseRateBps: 200}

	tests := map[string]struct {
		cards           []CardProfile
		spend           []CategorySpend
		wantCents       int64
		wantAllocations map[string][]CategoryAllocation
	}{
		"no cards earns nothing": {
			spend:           []CategorySpend{{CategoryName: "FOOD_AND_DRINK", TotalCents: 10000}},
			wantCents:       0,
			wantAllocations: map[string][]CategoryAllocation{},
		},
		"single card earns its own rate": {
			cards:     []CardProfile{flat},
			spend:     []CategorySpend{{CategoryName: "FOOD_AND_DRINK", TotalCents: 10000}},
			wantCents: 100,
			wantAllocations: map[string][]CategoryAllocation{
				"FOOD_AND_DRINK": {{CardID: "flat", RateBps: 100, SpendCents: 10000}},
			},
		},
		"bonus card beats a flat card on its own category": {
			cards:     []CardProfile{flat, dining},
			spend:     []CategorySpend{{CategoryName: "FOOD_AND_DRINK", TotalCents: 10000}},
			wantCents: 400, // dining's 4%, not flat's 1%
			wantAllocations: map[string][]CategoryAllocation{
				"FOOD_AND_DRINK": {{CardID: "dining", RateBps: 400, SpendCents: 10000}},
			},
		},
		"flat card still wins categories the bonus card doesn't cover": {
			cards: []CardProfile{flat, dining},
			spend: []CategorySpend{
				{CategoryName: "FOOD_AND_DRINK", TotalCents: 10000},
				{CategoryName: "TRAVEL", TotalCents: 5000},
			},
			wantCents: 400 + 50,
			wantAllocations: map[string][]CategoryAllocation{
				"FOOD_AND_DRINK": {{CardID: "dining", RateBps: 400, SpendCents: 10000}},
				"TRAVEL":         {{CardID: "flat", RateBps: 100, SpendCents: 5000}},
			},
		},
		"spend past the winning card's cap falls back to that card's own base rate when no other card is in the set": {
			cards: []CardProfile{cappedDining},
			spend: []CategorySpend{{CategoryName: "FOOD_AND_DRINK", TotalCents: 1_000_000}},
			// 4% on the first $5,000, 1% base on the remaining $5,000.
			wantCents: 500_000*400/10000 + 500_000*100/10000,
			wantAllocations: map[string][]CategoryAllocation{
				"FOOD_AND_DRINK": {
					{CardID: "capped-dining", RateBps: 400, SpendCents: 500_000},
					{CardID: "capped-dining", RateBps: 100, SpendCents: 500_000},
				},
			},
		},
		"spend past the winning card's cap spills to a second card's better base rate": {
			cards: []CardProfile{cappedDining, betterFlat},
			spend: []CategorySpend{{CategoryName: "FOOD_AND_DRINK", TotalCents: 1_000_000}},
			// 4% on the first $5,000 (capped-dining's bonus), then
			// better-flat's 2% base on the rest - beats falling back to
			// capped-dining's own 1% base.
			wantCents: 500_000*400/10000 + 500_000*200/10000,
			wantAllocations: map[string][]CategoryAllocation{
				"FOOD_AND_DRINK": {
					{CardID: "capped-dining", RateBps: 400, SpendCents: 500_000},
					{CardID: "better-flat", RateBps: 200, SpendCents: 500_000},
				},
			},
		},
		"ties keep the earlier card": {
			cards:     []CardProfile{flat, flat2},
			spend:     []CategorySpend{{CategoryName: "FOOD_AND_DRINK", TotalCents: 10000}},
			wantCents: 100,
			wantAllocations: map[string][]CategoryAllocation{
				"FOOD_AND_DRINK": {{CardID: "flat", RateBps: 100, SpendCents: 10000}},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotCents, gotAllocations := PortfolioRewards(tc.cards, tc.spend)
			if gotCents != tc.wantCents {
				t.Errorf("PortfolioRewards() cents = %d, want %d", gotCents, tc.wantCents)
			}
			if !reflect.DeepEqual(gotAllocations, tc.wantAllocations) {
				t.Errorf("PortfolioRewards() allocations = %v, want %v", gotAllocations, tc.wantAllocations)
			}
		})
	}
}

func TestAnnualValueCents(t *testing.T) {
	tests := map[string]struct {
		rewardsEarnedCents int64
		annualFeeCents     int32
		want               int64
	}{
		"no annual fee":          {rewardsEarnedCents: 50000, annualFeeCents: 0, want: 50000},
		"fee reduces value":      {rewardsEarnedCents: 50000, annualFeeCents: 9500, want: 40500},
		"fee can exceed rewards": {rewardsEarnedCents: 10000, annualFeeCents: 32500, want: -22500},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := AnnualValueCents(tc.rewardsEarnedCents, tc.annualFeeCents)
			if got != tc.want {
				t.Errorf("AnnualValueCents() = %d, want %d", got, tc.want)
			}
		})
	}
}
