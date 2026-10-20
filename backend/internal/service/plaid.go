// Package service holds business logic that sits between the HTTP layer and
// the database. Phase 2's only service is PlaidService: linking accounts,
// exchanging tokens and syncing transactions.
package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/linbeau0822/cardmaxx/backend/internal/database/db"
	"github.com/linbeau0822/cardmaxx/backend/internal/plaid"
	"github.com/linbeau0822/cardmaxx/backend/internal/reward"
)

// Querier is the slice of db.Querier that PlaidService needs. Declaring it
// here, rather than depending on db.Querier directly, keeps the SQL this
// package may run visible in one place and lets it be tested with a fake
// instead of a live database.
type Querier interface {
	CreatePlaidItem(ctx context.Context, arg db.CreatePlaidItemParams) (db.PlaidItem, error)
	ListPlaidItemsByUser(ctx context.Context, userID uuid.UUID) ([]db.PlaidItem, error)
	UpdatePlaidItemCursor(ctx context.Context, arg db.UpdatePlaidItemCursorParams) error
	UpsertAccount(ctx context.Context, arg db.UpsertAccountParams) (db.Account, error)
	GetAccountByPlaidAccountID(ctx context.Context, plaidAccountID string) (db.Account, error)
	ListAccountsByUser(ctx context.Context, userID uuid.UUID) ([]db.Account, error)
	UpdateAccountCreditCard(ctx context.Context, arg db.UpdateAccountCreditCardParams) (db.Account, error)
	UpsertTransaction(ctx context.Context, arg db.UpsertTransactionParams) (db.Transaction, error)
	DeleteTransactionByPlaidID(ctx context.Context, plaidTransactionID string) error
	ListTransactionsByUser(ctx context.Context, arg db.ListTransactionsByUserParams) ([]db.ListTransactionsByUserRow, error)
	GetOrCreateCategoryByName(ctx context.Context, name string) (db.Category, error)
	GetSpendingByMonth(ctx context.Context, userID uuid.UUID) ([]db.GetSpendingByMonthRow, error)
	GetSpendingByCategoryForMonth(ctx context.Context, arg db.GetSpendingByCategoryForMonthParams) ([]db.GetSpendingByCategoryForMonthRow, error)
	GetAllCategoriesForUser(ctx context.Context, userID uuid.UUID) ([]string, error)
	GetTopMerchants(ctx context.Context, arg db.GetTopMerchantsParams) ([]db.GetTopMerchantsRow, error)
	ListCreditCards(ctx context.Context) ([]db.CreditCard, error)
	GetCreditCard(ctx context.Context, id uuid.UUID) (db.CreditCard, error)
	ListCardRewardCategories(ctx context.Context, creditCardID uuid.UUID) ([]db.ListCardRewardCategoriesRow, error)
	GetTrailingYearCategorySpend(ctx context.Context, userID uuid.UUID) ([]db.GetTrailingYearCategorySpendRow, error)
}

// PlaidClient is the slice of plaid.Client that PlaidService needs.
type PlaidClient interface {
	CreateLinkToken(ctx context.Context, userID uuid.UUID) (plaid.LinkToken, error)
	ExchangePublicToken(ctx context.Context, publicToken string) (plaid.ItemCredentials, error)
	SyncTransactions(ctx context.Context, accessToken, cursor string) (plaid.SyncResult, error)
}

// PlaidService links Plaid Items to users and keeps their accounts and
// transactions in sync.
type PlaidService struct {
	queries Querier
	plaid   PlaidClient
}

func NewPlaidService(queries Querier, plaidClient PlaidClient) *PlaidService {
	return &PlaidService{queries: queries, plaid: plaidClient}
}

// CreateLinkToken requests a Plaid Link token for the given user.
func (s *PlaidService) CreateLinkToken(ctx context.Context, userID uuid.UUID) (plaid.LinkToken, error) {
	return s.plaid.CreateLinkToken(ctx, userID)
}

// ExchangePublicToken trades a public token from Plaid Link for an access
// token and persists the resulting Item.
func (s *PlaidService) ExchangePublicToken(ctx context.Context, userID uuid.UUID, publicToken string) (db.PlaidItem, error) {
	creds, err := s.plaid.ExchangePublicToken(ctx, publicToken)
	if err != nil {
		return db.PlaidItem{}, fmt.Errorf("exchange public token: %w", err)
	}
	item, err := s.queries.CreatePlaidItem(ctx, db.CreatePlaidItemParams{
		UserID:      userID,
		PlaidItemID: creds.ItemID,
		AccessToken: creds.AccessToken,
	})
	if err != nil {
		return db.PlaidItem{}, fmt.Errorf("store plaid item: %w", err)
	}
	return item, nil
}

// ItemSyncResult summarizes one Item's sync for the API response.
type ItemSyncResult struct {
	PlaidItemID string `json:"plaid_item_id"`
	Added       int    `json:"added"`
	Modified    int    `json:"modified"`
	Removed     int    `json:"removed"`
}

// SyncAllForUser pulls new transaction activity for every Item the user has
// linked.
func (s *PlaidService) SyncAllForUser(ctx context.Context, userID uuid.UUID) ([]ItemSyncResult, error) {
	items, err := s.queries.ListPlaidItemsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list plaid items: %w", err)
	}

	results := make([]ItemSyncResult, 0, len(items))
	for _, item := range items {
		result, err := s.syncItem(ctx, item)
		if err != nil {
			return nil, fmt.Errorf("sync item %s: %w", item.PlaidItemID, err)
		}
		results = append(results, result)
	}
	return results, nil
}

// syncItem pages through /transactions/sync for one Item until Plaid reports
// no more pages, persisting as it goes. The cursor is saved after each page
// so a failure partway through does not re-fetch what already landed.
func (s *PlaidService) syncItem(ctx context.Context, item db.PlaidItem) (ItemSyncResult, error) {
	result := ItemSyncResult{PlaidItemID: item.PlaidItemID}
	cursor := ""
	if item.TransactionCursor != nil {
		cursor = *item.TransactionCursor
	}

	catalog, err := s.queries.ListCreditCards(ctx)
	if err != nil {
		return ItemSyncResult{}, fmt.Errorf("list credit cards: %w", err)
	}

	for {
		page, err := s.plaid.SyncTransactions(ctx, item.AccessToken, cursor)
		if err != nil {
			return ItemSyncResult{}, err
		}

		for _, a := range page.Accounts {
			account, err := s.queries.UpsertAccount(ctx, db.UpsertAccountParams{
				PlaidItemID:    item.ID,
				PlaidAccountID: a.PlaidAccountID,
				Name:           a.Name,
				OfficialName:   a.OfficialName,
				Type:           a.Type,
				Subtype:        a.Subtype,
				Mask:           a.Mask,
			})
			if err != nil {
				return ItemSyncResult{}, fmt.Errorf("upsert account %s: %w", a.PlaidAccountID, err)
			}
			if err := s.autoLinkCard(ctx, item.UserID, account, catalog); err != nil {
				return ItemSyncResult{}, err
			}
		}

		for _, t := range append(page.Added, page.Modified...) {
			if err := s.upsertTransaction(ctx, t); err != nil {
				return ItemSyncResult{}, err
			}
		}
		result.Added += len(page.Added)
		result.Modified += len(page.Modified)

		for _, id := range page.RemovedIDs {
			if err := s.queries.DeleteTransactionByPlaidID(ctx, id); err != nil {
				return ItemSyncResult{}, fmt.Errorf("delete transaction %s: %w", id, err)
			}
		}
		result.Removed += len(page.RemovedIDs)

		cursor = page.NextCursor
		if err := s.queries.UpdatePlaidItemCursor(ctx, db.UpdatePlaidItemCursorParams{
			ID:                item.ID,
			TransactionCursor: &cursor,
		}); err != nil {
			return ItemSyncResult{}, fmt.Errorf("save cursor: %w", err)
		}

		if !page.HasMore {
			break
		}
	}
	return result, nil
}

// upsertTransaction resolves a Plaid transaction's account and category to
// their internal ids and persists it.
func (s *PlaidService) upsertTransaction(ctx context.Context, t plaid.Transaction) error {
	account, err := s.queries.GetAccountByPlaidAccountID(ctx, t.PlaidAccountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The account for this transaction has not been upserted yet (it
			// may arrive on this same page, in Plaid's "accounts" list, in
			// any order). Skip it; a later sync picks it up once the account
			// exists.
			return nil
		}
		return fmt.Errorf("look up account %s: %w", t.PlaidAccountID, err)
	}

	var categoryID *uuid.UUID
	if t.Category != nil {
		category, err := s.queries.GetOrCreateCategoryByName(ctx, *t.Category)
		if err != nil {
			return fmt.Errorf("resolve category %s: %w", *t.Category, err)
		}
		categoryID = &category.ID
	}

	_, err = s.queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
		PlaidTransactionID:   t.PlaidTransactionID,
		AccountID:            account.ID,
		CategoryID:           categoryID,
		AmountCents:          t.AmountCents,
		Date:                 t.Date,
		Name:                 t.Name,
		MerchantName:         t.MerchantName,
		Pending:              t.Pending,
		PendingTransactionID: t.PendingTransactionID,
	})
	if err != nil {
		return fmt.Errorf("upsert transaction %s: %w", t.PlaidTransactionID, err)
	}
	return nil
}

// autoLinkCard matches a freshly-synced account to a catalogue card by name
// and persists the match, when it's unambiguous. It never overwrites a
// credit_card_id the account already has - whether set by an earlier
// auto-link or by the user themselves via LinkAccountCard - so a re-sync
// can't undo a user's own choice.
func (s *PlaidService) autoLinkCard(ctx context.Context, userID uuid.UUID, account db.Account, catalog []db.CreditCard) error {
	if account.CreditCardID != nil {
		return nil
	}
	name := account.Name
	if account.OfficialName != nil {
		name = *account.OfficialName
	}
	cardID := matchCreditCardByName(name, catalog)
	if cardID == nil {
		return nil
	}
	if _, err := s.queries.UpdateAccountCreditCard(ctx, db.UpdateAccountCreditCardParams{
		ID:           account.ID,
		UserID:       userID,
		CreditCardID: cardID,
	}); err != nil {
		return fmt.Errorf("auto-link account %s to card %s: %w", account.ID, *cardID, err)
	}
	return nil
}

// matchCreditCardByName finds the single catalogue card whose issuer and
// name both appear in accountName, case-insensitively - e.g. Plaid's
// official name "Chase Sapphire Preferred Card" matches issuer "Chase" and
// name "Sapphire Preferred". Returns nil when no card matches or more than
// one does: an ambiguous name (including every Plaid Sandbox test account,
// whose generic names never name a real issuer) is left for the user to
// resolve via LinkAccountCard instead of being guessed.
func matchCreditCardByName(accountName string, catalog []db.CreditCard) *uuid.UUID {
	lower := strings.ToLower(accountName)
	var matched *uuid.UUID
	for _, c := range catalog {
		if strings.Contains(lower, strings.ToLower(c.Issuer)) && strings.Contains(lower, strings.ToLower(c.Name)) {
			if matched != nil {
				return nil
			}
			id := c.ID
			matched = &id
		}
	}
	return matched
}

// ListAccounts returns every account linked to the user.
func (s *PlaidService) ListAccounts(ctx context.Context, userID uuid.UUID) ([]db.Account, error) {
	return s.queries.ListAccountsByUser(ctx, userID)
}

// ListTransactions returns a page of the user's transactions, most recent
// first.
func (s *PlaidService) ListTransactions(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]db.ListTransactionsByUserRow, error) {
	return s.queries.ListTransactionsByUser(ctx, db.ListTransactionsByUserParams{
		UserID:       userID,
		ResultLimit:  limit,
		ResultOffset: offset,
	})
}

// SpendingByMonth totals the user's reward-eligible spend per calendar month.
func (s *PlaidService) SpendingByMonth(ctx context.Context, userID uuid.UUID) ([]db.GetSpendingByMonthRow, error) {
	return s.queries.GetSpendingByMonth(ctx, userID)
}

// SpendingByCategoryForMonth totals the user's reward-eligible spend per
// category within a single calendar month.
func (s *PlaidService) SpendingByCategoryForMonth(ctx context.Context, userID uuid.UUID, month time.Time) ([]db.GetSpendingByCategoryForMonthRow, error) {
	return s.queries.GetSpendingByCategoryForMonth(ctx, db.GetSpendingByCategoryForMonthParams{UserID: userID, Month: month})
}

// AllCategories returns every reward-eligible category the user has ever
// spent in, ordered by all-time total.
func (s *PlaidService) AllCategories(ctx context.Context, userID uuid.UUID) ([]string, error) {
	return s.queries.GetAllCategoriesForUser(ctx, userID)
}

// TopMerchants ranks the user's all-time reward-eligible spend by merchant.
func (s *PlaidService) TopMerchants(ctx context.Context, userID uuid.UUID, limit int32) ([]db.GetTopMerchantsRow, error) {
	return s.queries.GetTopMerchants(ctx, db.GetTopMerchantsParams{UserID: userID, ResultLimit: limit})
}

// CardWithRewardCategories is a catalogue card plus its bonus-earning rules.
type CardWithRewardCategories struct {
	db.CreditCard
	RewardCategories []db.ListCardRewardCategoriesRow `json:"reward_categories"`
}

// ListCards returns the credit-card catalogue, each with its bonus-category
// rules. The catalogue is global reference data, not scoped to any user.
func (s *PlaidService) ListCards(ctx context.Context) ([]CardWithRewardCategories, error) {
	cards, err := s.queries.ListCreditCards(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credit cards: %w", err)
	}

	result := make([]CardWithRewardCategories, len(cards))
	for i, card := range cards {
		rewardCategories, err := s.queries.ListCardRewardCategories(ctx, card.ID)
		if err != nil {
			return nil, fmt.Errorf("list reward categories for card %s: %w", card.ID, err)
		}
		result[i] = CardWithRewardCategories{CreditCard: card, RewardCategories: rewardCategories}
	}
	return result, nil
}

// LinkAccountCard links (or, when creditCardID is nil, unlinks) one of the
// user's accounts to a catalogue card. Returns pgx.ErrNoRows if accountID
// doesn't exist or doesn't belong to the user.
func (s *PlaidService) LinkAccountCard(ctx context.Context, userID, accountID uuid.UUID, creditCardID *uuid.UUID) (db.Account, error) {
	return s.queries.UpdateAccountCreditCard(ctx, db.UpdateAccountCreditCardParams{
		ID:           accountID,
		UserID:       userID,
		CreditCardID: creditCardID,
	})
}

// CardValue is a catalogue card's reward value against the user's own
// trailing-12-month spend.
type CardValue struct {
	RewardsEarnedCents int64 `json:"rewards_earned_cents"`
	AnnualFeeCents     int32 `json:"annual_fee_cents"`
	AnnualValueCents   int64 `json:"annual_value_cents"`
}

// ValueForCard computes what cardID would have earned the user over their
// trailing 12 months of reward-eligible spend, net of its annual fee.
// Returns pgx.ErrNoRows if cardID isn't in the catalogue.
func (s *PlaidService) ValueForCard(ctx context.Context, userID, cardID uuid.UUID) (CardValue, error) {
	card, err := s.queries.GetCreditCard(ctx, cardID)
	if err != nil {
		return CardValue{}, fmt.Errorf("get credit card: %w", err)
	}
	profile, err := s.cardProfile(ctx, card)
	if err != nil {
		return CardValue{}, err
	}
	spend, err := s.trailingYearSpend(ctx, userID)
	if err != nil {
		return CardValue{}, err
	}

	earnedCents := reward.CalculateRewards(profile.BaseRateBps, profile.Rules, spend)
	return CardValue{
		RewardsEarnedCents: earnedCents,
		AnnualFeeCents:     card.AnnualFeeCents,
		AnnualValueCents:   reward.AnnualValueCents(earnedCents, card.AnnualFeeCents),
	}, nil
}

// cardProfile loads a catalogue card's reward rules as a reward.CardProfile,
// keyed by its UUID string so reward.PortfolioRewards can report which card
// won a category without the reward package importing uuid itself.
func (s *PlaidService) cardProfile(ctx context.Context, card db.CreditCard) (reward.CardProfile, error) {
	rewardRows, err := s.queries.ListCardRewardCategories(ctx, card.ID)
	if err != nil {
		return reward.CardProfile{}, fmt.Errorf("list reward categories for card %s: %w", card.ID, err)
	}
	rules := make([]reward.CategoryRule, len(rewardRows))
	for i, rc := range rewardRows {
		rules[i] = reward.CategoryRule{CategoryName: rc.CategoryName, RateBps: rc.RewardRateBps, CapCents: rc.RewardCapCents}
	}
	return reward.CardProfile{ID: card.ID.String(), BaseRateBps: card.BaseRewardRateBps, Rules: rules}, nil
}

// trailingYearSpend loads the user's trailing-12-month reward-eligible
// spend by category, in the shape the reward package consumes.
func (s *PlaidService) trailingYearSpend(ctx context.Context, userID uuid.UUID) ([]reward.CategorySpend, error) {
	spendRows, err := s.queries.GetTrailingYearCategorySpend(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get trailing year category spend: %w", err)
	}
	spend := make([]reward.CategorySpend, len(spendRows))
	for i, sp := range spendRows {
		spend[i] = reward.CategorySpend{CategoryName: sp.CategoryName, TotalCents: sp.TotalCents}
	}
	return spend, nil
}

// CardRecommendation is a catalogue card the user doesn't hold, ranked by
// the extra reward value it would add on top of the cards they already
// have - not its value in isolation (ValueForCard already answers that).
// Phase 5 only ever adds one card to the user's current set; evaluating
// whole-portfolio combinations is Phase 6.
type CardRecommendation struct {
	Card                  db.CreditCard `json:"card"`
	IncrementalValueCents int64         `json:"incremental_value_cents"`
	CategoriesWon         []CategoryWin `json:"categories_won"`
}

// CategoryWin is one spend category a recommended card would capture from
// whatever rate the user's current cards earn on it today.
type CategoryWin struct {
	CategoryName    string `json:"category_name"`
	PreviousRateBps int32  `json:"previous_rate_bps"`
	NewRateBps      int32  `json:"new_rate_bps"`
}

// RecommendCards ranks catalogue cards the user doesn't already hold by the
// incremental annual value each would add: what it captures from the
// user's trailing-12-month spend in categories where it beats their
// current cards, net of its annual fee.
func (s *PlaidService) RecommendCards(ctx context.Context, userID uuid.UUID) ([]CardRecommendation, error) {
	accounts, err := s.queries.ListAccountsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	held := make(map[uuid.UUID]bool, len(accounts))
	for _, a := range accounts {
		if a.CreditCardID != nil {
			held[*a.CreditCardID] = true
		}
	}

	catalog, err := s.queries.ListCreditCards(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credit cards: %w", err)
	}
	spend, err := s.trailingYearSpend(ctx, userID)
	if err != nil {
		return nil, err
	}

	var currentProfiles []reward.CardProfile
	for _, card := range catalog {
		if !held[card.ID] {
			continue
		}
		profile, err := s.cardProfile(ctx, card)
		if err != nil {
			return nil, err
		}
		currentProfiles = append(currentProfiles, profile)
	}
	currentTotalCents, currentAllocations := reward.PortfolioRewards(currentProfiles, spend)

	var recommendations []CardRecommendation
	for _, card := range catalog {
		if held[card.ID] {
			continue
		}
		// ponytail: one ListCardRewardCategories query per candidate card
		// via cardProfile - fine at today's catalog size; batch into one
		// query keyed by card ID if a larger catalog makes this slow.
		profile, err := s.cardProfile(ctx, card)
		if err != nil {
			return nil, err
		}

		candidates := make([]reward.CardProfile, len(currentProfiles)+1)
		copy(candidates, currentProfiles)
		candidates[len(currentProfiles)] = profile
		withCandidateCents, allocations := reward.PortfolioRewards(candidates, spend)

		var won []CategoryWin
		for category, catAllocations := range allocations {
			newRateBps, ok := candidateRateFor(catAllocations, profile.ID)
			if !ok {
				continue
			}
			previousRateBps := int32(0)
			if prevAllocations, ok := currentAllocations[category]; ok && len(prevAllocations) > 0 {
				previousRateBps = prevAllocations[0].RateBps
			}
			won = append(won, CategoryWin{CategoryName: category, PreviousRateBps: previousRateBps, NewRateBps: newRateBps})
		}
		sort.Slice(won, func(i, j int) bool { return won[i].CategoryName < won[j].CategoryName })

		recommendations = append(recommendations, CardRecommendation{
			Card:                  card,
			IncrementalValueCents: withCandidateCents - currentTotalCents - int64(card.AnnualFeeCents),
			CategoriesWon:         won,
		})
	}

	sort.Slice(recommendations, func(i, j int) bool {
		return recommendations[i].IncrementalValueCents > recommendations[j].IncrementalValueCents
	})
	return recommendations, nil
}

// candidateRateFor returns the rate cardID earns within a category's
// allocations, if it earns any of that category at all. Allocations are
// ordered highest-rate first, so the first match is also the best rate that
// card earns in the category.
func candidateRateFor(allocations []reward.CategoryAllocation, cardID string) (rateBps int32, ok bool) {
	for _, a := range allocations {
		if a.CardID == cardID {
			return a.RateBps, true
		}
	}
	return 0, false
}

// maxPortfolioSize caps how many cards OptimizePortfolio will ever consider
// holding at once. The catalogue is small (five cards today), so checking
// every combination up to this size is cheap; raise it only if a much
// larger catalogue makes brute force too slow.
const maxPortfolioSize = 5

// combinations returns every subset of size k from cards, order-independent
// (picking {A, B} is the same combination as {B, A}).
func combinations(cards []db.CreditCard, k int) [][]db.CreditCard {
	var out [][]db.CreditCard
	chosen := make([]db.CreditCard, 0, k)
	var pick func(start int)
	pick = func(start int) {
		if len(chosen) == k {
			out = append(out, append([]db.CreditCard{}, chosen...))
			return
		}
		for i := start; i < len(cards); i++ {
			chosen = append(chosen, cards[i])
			pick(i + 1)
			chosen = chosen[:len(chosen)-1]
		}
	}
	pick(0)
	return out
}

// PortfolioOption is the best combination of catalogue cards at one
// portfolio size.
type PortfolioOption struct {
	Cards              []db.CreditCard `json:"cards"`
	RewardsEarnedCents int64           `json:"rewards_earned_cents"`
	AnnualFeesCents    int64           `json:"annual_fees_cents"`
	NetValueCents      int64           `json:"net_value_cents"`
	// MarginalValueCents is how much more net value this size buys over the
	// best smaller size (or, at size 1, over holding no cards at all) - the
	// number that shows whether the next card is worth its fee.
	MarginalValueCents int64 `json:"marginal_value_cents"`
}

// OptimizePortfolio finds, for every portfolio size from 1 up to the
// catalogue size (capped at maxPortfolioSize), the combination of catalogue
// cards that earns the most net value on the user's trailing-12-month
// spend: rewards earned across the set, routed to whichever card in it pays
// best per category including overflow past any one card's cap, minus the
// combination's total annual fees.
func (s *PlaidService) OptimizePortfolio(ctx context.Context, userID uuid.UUID) ([]PortfolioOption, error) {
	catalog, err := s.queries.ListCreditCards(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credit cards: %w", err)
	}
	spend, err := s.trailingYearSpend(ctx, userID)
	if err != nil {
		return nil, err
	}

	profiles := make(map[uuid.UUID]reward.CardProfile, len(catalog))
	for _, card := range catalog {
		profile, err := s.cardProfile(ctx, card)
		if err != nil {
			return nil, err
		}
		profiles[card.ID] = profile
	}

	maxSize := maxPortfolioSize
	if len(catalog) < maxSize {
		maxSize = len(catalog)
	}

	var options []PortfolioOption
	var previousBestNetCents int64
	for size := 1; size <= maxSize; size++ {
		var best PortfolioOption
		var found bool
		for _, combo := range combinations(catalog, size) {
			comboProfiles := make([]reward.CardProfile, len(combo))
			var feesCents int64
			for i, card := range combo {
				comboProfiles[i] = profiles[card.ID]
				feesCents += int64(card.AnnualFeeCents)
			}
			rewardsCents, _ := reward.PortfolioRewards(comboProfiles, spend)
			netCents := rewardsCents - feesCents
			if !found || netCents > best.NetValueCents {
				found = true
				best = PortfolioOption{Cards: combo, RewardsEarnedCents: rewardsCents, AnnualFeesCents: feesCents, NetValueCents: netCents}
			}
		}
		best.MarginalValueCents = best.NetValueCents - previousBestNetCents
		options = append(options, best)
		previousBestNetCents = best.NetValueCents
	}
	return options, nil
}
