// Package service holds business logic that sits between the HTTP layer and
// the database. Phase 2's only service is PlaidService: linking accounts,
// exchanging tokens and syncing transactions.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/linbeau0822/cardmaxx/backend/internal/database/db"
	"github.com/linbeau0822/cardmaxx/backend/internal/plaid"
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
	UpsertTransaction(ctx context.Context, arg db.UpsertTransactionParams) (db.Transaction, error)
	DeleteTransactionByPlaidID(ctx context.Context, plaidTransactionID string) error
	ListTransactionsByUser(ctx context.Context, arg db.ListTransactionsByUserParams) ([]db.ListTransactionsByUserRow, error)
	GetOrCreateCategoryByName(ctx context.Context, name string) (db.Category, error)
	GetSpendingByMonth(ctx context.Context, userID uuid.UUID) ([]db.GetSpendingByMonthRow, error)
	GetSpendingByCategoryForMonth(ctx context.Context, arg db.GetSpendingByCategoryForMonthParams) ([]db.GetSpendingByCategoryForMonthRow, error)
	GetAllCategoriesForUser(ctx context.Context, userID uuid.UUID) ([]string, error)
	GetTopMerchants(ctx context.Context, arg db.GetTopMerchantsParams) ([]db.GetTopMerchantsRow, error)
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

	for {
		page, err := s.plaid.SyncTransactions(ctx, item.AccessToken, cursor)
		if err != nil {
			return ItemSyncResult{}, err
		}

		for _, a := range page.Accounts {
			if _, err := s.queries.UpsertAccount(ctx, db.UpsertAccountParams{
				PlaidItemID:    item.ID,
				PlaidAccountID: a.PlaidAccountID,
				Name:           a.Name,
				OfficialName:   a.OfficialName,
				Type:           a.Type,
				Subtype:        a.Subtype,
				Mask:           a.Mask,
			}); err != nil {
				return ItemSyncResult{}, fmt.Errorf("upsert account %s: %w", a.PlaidAccountID, err)
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
