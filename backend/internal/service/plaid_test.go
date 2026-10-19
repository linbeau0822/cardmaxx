package service

import (
	"testing"

	"github.com/google/uuid"

	"github.com/linbeau0822/cardmaxx/backend/internal/database/db"
)

func TestMatchCreditCardByName(t *testing.T) {
	sapphire := db.CreditCard{ID: uuid.New(), Issuer: "Chase", Name: "Sapphire Preferred"}
	freedom := db.CreditCard{ID: uuid.New(), Issuer: "Chase", Name: "Freedom Unlimited"}
	gold := db.CreditCard{ID: uuid.New(), Issuer: "American Express", Name: "Gold Card"}
	catalog := []db.CreditCard{sapphire, freedom, gold}

	tests := map[string]struct {
		accountName string
		want        *uuid.UUID
	}{
		"matches issuer and name, case-insensitive": {
			accountName: "CHASE SAPPHIRE PREFERRED CREDIT CARD",
			want:        &sapphire.ID,
		},
		"matches a different card in the same catalogue": {
			accountName: "Chase Freedom Unlimited",
			want:        &freedom.ID,
		},
		"plaid sandbox name matches no issuer": {
			accountName: "Plaid Diamond 12.5% APR Interest Credit Card",
			want:        nil,
		},
		"issuer with no card name match is not enough": {
			accountName: "Chase Checking",
			want:        nil,
		},
		"ambiguous when more than one card matches": {
			accountName: "Chase Sapphire Preferred Freedom Unlimited",
			want:        nil,
		},
		"empty catalogue matches nothing": {
			accountName: "Chase Sapphire Preferred",
			want:        nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := catalog
			if name == "empty catalogue matches nothing" {
				c = nil
			}
			got := matchCreditCardByName(tc.accountName, c)
			if (got == nil) != (tc.want == nil) {
				t.Fatalf("matchCreditCardByName() = %v, want %v", got, tc.want)
			}
			if got != nil && *got != *tc.want {
				t.Errorf("matchCreditCardByName() = %v, want %v", *got, *tc.want)
			}
		})
	}
}
