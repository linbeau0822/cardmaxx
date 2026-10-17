package reward

import "testing"

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
