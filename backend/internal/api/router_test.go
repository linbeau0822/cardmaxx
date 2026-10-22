package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/linbeau0822/cardmaxx/backend/internal/config"
	"github.com/linbeau0822/cardmaxx/backend/internal/database/db"
	"github.com/linbeau0822/cardmaxx/backend/internal/plaid"
	"github.com/linbeau0822/cardmaxx/backend/internal/service"
)

// fakeQuerier is an in-memory stand-in for service.Querier, so these tests
// exercise the HTTP layer without a database.
type fakeQuerier struct {
	accounts         []db.Account
	transactions     []db.Transaction
	plaidItems       []db.PlaidItem
	cards            []db.CreditCard
	rewardCategories map[uuid.UUID][]db.ListCardRewardCategoriesRow
	categorySpend    []db.GetTrailingYearCategorySpendRow
	linkCardErr      error
}

func (f *fakeQuerier) CreatePlaidItem(_ context.Context, arg db.CreatePlaidItemParams) (db.PlaidItem, error) {
	return db.PlaidItem{ID: uuid.New(), UserID: arg.UserID, PlaidItemID: arg.PlaidItemID, AccessToken: arg.AccessToken}, nil
}
func (f *fakeQuerier) ListPlaidItemsByUser(_ context.Context, _ uuid.UUID) ([]db.PlaidItem, error) {
	return f.plaidItems, nil
}
func (f *fakeQuerier) UpdatePlaidItemCursor(_ context.Context, _ db.UpdatePlaidItemCursorParams) error {
	return nil
}
func (f *fakeQuerier) UpsertAccount(_ context.Context, arg db.UpsertAccountParams) (db.Account, error) {
	return db.Account{ID: uuid.New(), PlaidAccountID: arg.PlaidAccountID, Name: arg.Name, Type: arg.Type}, nil
}
func (f *fakeQuerier) GetAccountByPlaidAccountID(_ context.Context, _ string) (db.Account, error) {
	return db.Account{}, pgx.ErrNoRows
}
func (f *fakeQuerier) ListAccountsByUser(_ context.Context, _ uuid.UUID) ([]db.Account, error) {
	return f.accounts, nil
}
func (f *fakeQuerier) UpdateAccountCreditCard(_ context.Context, arg db.UpdateAccountCreditCardParams) (db.Account, error) {
	if f.linkCardErr != nil {
		return db.Account{}, f.linkCardErr
	}
	return db.Account{ID: arg.ID, CreditCardID: arg.CreditCardID}, nil
}
func (f *fakeQuerier) UpsertTransaction(_ context.Context, arg db.UpsertTransactionParams) (db.Transaction, error) {
	return db.Transaction{ID: uuid.New(), PlaidTransactionID: arg.PlaidTransactionID}, nil
}
func (f *fakeQuerier) DeleteTransactionByPlaidID(_ context.Context, _ string) error {
	return nil
}
func (f *fakeQuerier) ListTransactionsByUser(_ context.Context, _ db.ListTransactionsByUserParams) ([]db.ListTransactionsByUserRow, error) {
	rows := make([]db.ListTransactionsByUserRow, len(f.transactions))
	for i, t := range f.transactions {
		rows[i] = db.ListTransactionsByUserRow{ID: t.ID, PlaidTransactionID: t.PlaidTransactionID}
	}
	return rows, nil
}
func (f *fakeQuerier) GetOrCreateCategoryByName(_ context.Context, name string) (db.Category, error) {
	return db.Category{ID: uuid.New(), Name: name}, nil
}
func (f *fakeQuerier) GetSpendingByMonth(_ context.Context, _ uuid.UUID) ([]db.GetSpendingByMonthRow, error) {
	return nil, nil
}
func (f *fakeQuerier) GetSpendingByCategoryForMonth(_ context.Context, _ db.GetSpendingByCategoryForMonthParams) ([]db.GetSpendingByCategoryForMonthRow, error) {
	return nil, nil
}
func (f *fakeQuerier) GetAllCategoriesForUser(_ context.Context, _ uuid.UUID) ([]string, error) {
	return nil, nil
}
func (f *fakeQuerier) GetTopMerchants(_ context.Context, _ db.GetTopMerchantsParams) ([]db.GetTopMerchantsRow, error) {
	return nil, nil
}
func (f *fakeQuerier) ListCreditCards(_ context.Context) ([]db.CreditCard, error) {
	return f.cards, nil
}
func (f *fakeQuerier) GetCreditCard(_ context.Context, id uuid.UUID) (db.CreditCard, error) {
	for _, c := range f.cards {
		if c.ID == id {
			return c, nil
		}
	}
	return db.CreditCard{}, pgx.ErrNoRows
}
func (f *fakeQuerier) ListCardRewardCategories(_ context.Context, cardID uuid.UUID) ([]db.ListCardRewardCategoriesRow, error) {
	return f.rewardCategories[cardID], nil
}
func (f *fakeQuerier) GetTrailingYearCategorySpend(_ context.Context, _ uuid.UUID) ([]db.GetTrailingYearCategorySpendRow, error) {
	return f.categorySpend, nil
}

// fakePlaidClient is an in-memory stand-in for service.PlaidClient, so these
// tests make no network calls to Plaid.
type fakePlaidClient struct {
	syncErr error
}

func (f *fakePlaidClient) CreateLinkToken(_ context.Context, _ uuid.UUID) (plaid.LinkToken, error) {
	return plaid.LinkToken{Token: "link-sandbox-test"}, nil
}
func (f *fakePlaidClient) ExchangePublicToken(_ context.Context, publicToken string) (plaid.ItemCredentials, error) {
	if publicToken == "bad-token" {
		return plaid.ItemCredentials{}, &plaid.APIError{StatusCode: http.StatusBadRequest, Message: "public_token is invalid"}
	}
	return plaid.ItemCredentials{ItemID: "item-test", AccessToken: "access-test"}, nil
}
func (f *fakePlaidClient) SyncTransactions(_ context.Context, _, _ string) (plaid.SyncResult, error) {
	if f.syncErr != nil {
		return plaid.SyncResult{}, f.syncErr
	}
	return plaid.SyncResult{}, nil
}

// newTestRouter builds the real router over fakes, so every handler's logic
// runs with no database and no network calls.
func newTestRouter() http.Handler {
	return newTestRouterWith(&fakeQuerier{}, &fakePlaidClient{})
}

func newTestRouterWith(q service.Querier, p service.PlaidClient) http.Handler {
	cfg := config.ServerConfig{AllowedOrigins: []string{"http://localhost:5173"}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := service.NewPlaidService(q, p)
	return NewRouter(cfg, logger, svc, nil, uuid.New())
}

func do(t *testing.T, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return doOn(t, newTestRouter(), method, target, headers, nil)
}

func doOn(t *testing.T, handler http.Handler, method, target string, headers map[string]string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHealth(t *testing.T) {
	rec := do(t, http.MethodGet, "/health", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got, want := rec.Header().Get("Content-Type"), "application/json; charset=utf-8"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v (body: %s)", err, rec.Body.String())
	}
	if body["status"] != "ok" {
		t.Errorf(`body = %v, want {"status":"ok"}`, body)
	}
}

func TestCreateLinkToken(t *testing.T) {
	rec := do(t, http.MethodPost, "/api/v1/plaid/link-token", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body plaid.LinkToken
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if body.Token != "link-sandbox-test" {
		t.Errorf("link_token = %q, want link-sandbox-test", body.Token)
	}
}

func TestExchangeTokenRequiresPublicToken(t *testing.T) {
	rec := doOn(t, newTestRouter(), http.MethodPost, "/api/v1/plaid/exchange-token", nil, []byte(`{}`))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestExchangeTokenMalformedBody(t *testing.T) {
	rec := doOn(t, newTestRouter(), http.MethodPost, "/api/v1/plaid/exchange-token", nil, []byte(`not json`))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestExchangeTokenSuccess(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"public_token": "public-sandbox-abc"})
	rec := doOn(t, newTestRouter(), http.MethodPost, "/api/v1/plaid/exchange-token", nil, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["item_id"] != "item-test" {
		t.Errorf("item_id = %q, want item-test", resp["item_id"])
	}
}

func TestExchangeTokenSurfacesPlaidRejection(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"public_token": "bad-token"})
	rec := doOn(t, newTestRouter(), http.MethodPost, "/api/v1/plaid/exchange-token", nil, body)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}

func TestSync(t *testing.T) {
	rec := do(t, http.MethodPost, "/api/v1/plaid/sync", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestSyncFailureReturns500(t *testing.T) {
	q := &fakeQuerier{plaidItems: []db.PlaidItem{{ID: uuid.New(), PlaidItemID: "item-test"}}}
	handler := newTestRouterWith(q, &fakePlaidClient{syncErr: errors.New("boom")})
	rec := doOn(t, handler, http.MethodPost, "/api/v1/plaid/sync", nil, nil)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestListAccounts(t *testing.T) {
	rec := do(t, http.MethodGet, "/api/v1/accounts", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestListTransactions(t *testing.T) {
	rec := do(t, http.MethodGet, "/api/v1/transactions", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestListCards(t *testing.T) {
	q := &fakeQuerier{cards: []db.CreditCard{{ID: uuid.New(), Name: "Sapphire Preferred", Issuer: "Chase"}}}
	handler := newTestRouterWith(q, &fakePlaidClient{})
	rec := doOn(t, handler, http.MethodGet, "/api/v1/cards", nil, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body map[string][]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v (body: %s)", err, rec.Body.String())
	}
	if len(body["cards"]) != 1 {
		t.Fatalf("cards = %v, want 1 entry", body["cards"])
	}
	if body["cards"][0]["name"] != "Sapphire Preferred" {
		t.Errorf("cards[0].name = %v, want Sapphire Preferred", body["cards"][0]["name"])
	}
}

func TestLinkAccountCard(t *testing.T) {
	accountID := uuid.New()
	cardID := uuid.New()
	q := &fakeQuerier{accounts: []db.Account{{ID: accountID}}}
	handler := newTestRouterWith(q, &fakePlaidClient{})

	body, _ := json.Marshal(map[string]string{"credit_card_id": cardID.String()})
	rec := doOn(t, handler, http.MethodPatch, "/api/v1/accounts/"+accountID.String(), nil, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	var account db.Account
	if err := json.Unmarshal(rec.Body.Bytes(), &account); err != nil {
		t.Fatalf("response is not valid JSON: %v (body: %s)", err, rec.Body.String())
	}
	if account.CreditCardID == nil || *account.CreditCardID != cardID {
		t.Errorf("credit_card_id = %v, want %s", account.CreditCardID, cardID)
	}
}

func TestLinkAccountCardNotFound(t *testing.T) {
	q := &fakeQuerier{linkCardErr: pgx.ErrNoRows}
	handler := newTestRouterWith(q, &fakePlaidClient{})

	rec := doOn(t, handler, http.MethodPatch, "/api/v1/accounts/"+uuid.New().String(), nil, []byte(`{}`))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestLinkAccountCardRejectsBadID(t *testing.T) {
	rec := doOn(t, newTestRouter(), http.MethodPatch, "/api/v1/accounts/not-a-uuid", nil, []byte(`{}`))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCardValue(t *testing.T) {
	cardID := uuid.New()
	q := &fakeQuerier{
		cards:         []db.CreditCard{{ID: cardID, Name: "Freedom Unlimited", BaseRewardRateBps: 150, AnnualFeeCents: 0}},
		categorySpend: []db.GetTrailingYearCategorySpendRow{{CategoryName: "FOOD_AND_DRINK", TotalCents: 10000}},
	}
	handler := newTestRouterWith(q, &fakePlaidClient{})

	rec := doOn(t, handler, http.MethodGet, "/api/v1/cards/"+cardID.String()+"/value", nil, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body map[string]int64
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v (body: %s)", err, rec.Body.String())
	}
	if body["rewards_earned_cents"] != 150 {
		t.Errorf("rewards_earned_cents = %d, want 150 (1.5%% of $100)", body["rewards_earned_cents"])
	}
	if body["annual_value_cents"] != 150 {
		t.Errorf("annual_value_cents = %d, want 150", body["annual_value_cents"])
	}
}

func TestCardValueNotFound(t *testing.T) {
	rec := do(t, http.MethodGet, "/api/v1/cards/"+uuid.New().String()+"/value", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestRecommendations(t *testing.T) {
	heldCard := db.CreditCard{ID: uuid.New(), Name: "Flat 1%", BaseRewardRateBps: 100}
	diningCard := db.CreditCard{ID: uuid.New(), Name: "Dining 4%", AnnualFeeCents: 9500}
	q := &fakeQuerier{
		cards:    []db.CreditCard{heldCard, diningCard},
		accounts: []db.Account{{ID: uuid.New(), CreditCardID: &heldCard.ID}},
		rewardCategories: map[uuid.UUID][]db.ListCardRewardCategoriesRow{
			diningCard.ID: {{CategoryName: "FOOD_AND_DRINK", RewardRateBps: 400}},
		},
		categorySpend: []db.GetTrailingYearCategorySpendRow{{CategoryName: "FOOD_AND_DRINK", TotalCents: 1_000_000}},
	}
	handler := newTestRouterWith(q, &fakePlaidClient{})

	rec := doOn(t, handler, http.MethodGet, "/api/v1/recommendations", nil, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body struct {
		Recommendations []struct {
			Card                  db.CreditCard `json:"card"`
			IncrementalValueCents int64         `json:"incremental_value_cents"`
			CategoriesWon         []struct {
				CategoryName    string `json:"category_name"`
				PreviousRateBps int32  `json:"previous_rate_bps"`
				NewRateBps      int32  `json:"new_rate_bps"`
			} `json:"categories_won"`
		} `json:"recommendations"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v (body: %s)", err, rec.Body.String())
	}

	if len(body.Recommendations) != 1 {
		t.Fatalf("got %d recommendations, want 1 (the held card must be excluded)", len(body.Recommendations))
	}
	got := body.Recommendations[0]
	if got.Card.ID != diningCard.ID {
		t.Errorf("recommended card = %s, want %s", got.Card.ID, diningCard.ID)
	}
	// Dining earns 4% of $10,000 = $400 instead of the held card's 1% ($100),
	// a $300 gain, minus the $95 annual fee = $205.
	wantCents := int64(400_00 - 100_00 - 95_00)
	if got.IncrementalValueCents != wantCents {
		t.Errorf("incremental_value_cents = %d, want %d", got.IncrementalValueCents, wantCents)
	}
	if len(got.CategoriesWon) != 1 || got.CategoriesWon[0].CategoryName != "FOOD_AND_DRINK" {
		t.Fatalf("categories_won = %v, want [FOOD_AND_DRINK]", got.CategoriesWon)
	}
	if got.CategoriesWon[0].PreviousRateBps != 100 || got.CategoriesWon[0].NewRateBps != 400 {
		t.Errorf("categories_won[0] rates = %d -> %d, want 100 -> 400", got.CategoriesWon[0].PreviousRateBps, got.CategoriesWon[0].NewRateBps)
	}
}

func TestOptimizePortfolio(t *testing.T) {
	cheapFlat := db.CreditCard{ID: uuid.New(), Name: "Flat 1%", BaseRewardRateBps: 100}
	pricyDining := db.CreditCard{ID: uuid.New(), Name: "Dining 4%", AnnualFeeCents: 9500, BaseRewardRateBps: 100}
	q := &fakeQuerier{
		cards: []db.CreditCard{cheapFlat, pricyDining},
		rewardCategories: map[uuid.UUID][]db.ListCardRewardCategoriesRow{
			pricyDining.ID: {{CategoryName: "FOOD_AND_DRINK", RewardRateBps: 400}},
		},
		categorySpend: []db.GetTrailingYearCategorySpendRow{{CategoryName: "FOOD_AND_DRINK", TotalCents: 1_000_000}},
	}
	handler := newTestRouterWith(q, &fakePlaidClient{})

	rec := doOn(t, handler, http.MethodGet, "/api/v1/recommendations/portfolio", nil, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body struct {
		Options []struct {
			Cards              []db.CreditCard `json:"cards"`
			RewardsEarnedCents int64           `json:"rewards_earned_cents"`
			AnnualFeesCents    int64           `json:"annual_fees_cents"`
			NetValueCents      int64           `json:"net_value_cents"`
			MarginalValueCents int64           `json:"marginal_value_cents"`
		} `json:"options"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v (body: %s)", err, rec.Body.String())
	}

	if len(body.Options) != 2 {
		t.Fatalf("got %d options, want 2 (one per catalogue size up to 2 cards)", len(body.Options))
	}

	// Size 1: dining's 4% on $10,000 ($400) beats its $95 fee, and beats
	// flat's 1% ($100) outright - dining alone should win.
	size1 := body.Options[0]
	if len(size1.Cards) != 1 || size1.Cards[0].ID != pricyDining.ID {
		t.Fatalf("size-1 option cards = %v, want [%s]", size1.Cards, pricyDining.ID)
	}
	if want := int64(400_00 - 95_00); size1.NetValueCents != want {
		t.Errorf("size-1 net_value_cents = %d, want %d", size1.NetValueCents, want)
	}

	// Size 2: holding both adds nothing (dining already wins the only spend
	// category, flat has no category to add), so the marginal value of the
	// second card is exactly flat's $0 fee minus nothing gained.
	size2 := body.Options[1]
	if size2.MarginalValueCents != 0 {
		t.Errorf("size-2 marginal_value_cents = %d, want 0 (second card adds nothing here)", size2.MarginalValueCents)
	}
}

func TestSpendingByCategory(t *testing.T) {
	rec := do(t, http.MethodGet, "/api/v1/analysis/spending", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestListTransactionsRejectsBadLimit(t *testing.T) {
	rec := do(t, http.MethodGet, "/api/v1/transactions?limit=0", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	rec = do(t, http.MethodGet, "/api/v1/transactions?offset=-1", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUnknownRouteReturnsJSON404(t *testing.T) {
	rec := do(t, http.MethodGet, "/api/v1/nope", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var body errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v (body: %s)", err, rec.Body.String())
	}
	if body.Error.Code != CodeNotFound {
		t.Errorf("error.code = %q, want %q", body.Error.Code, CodeNotFound)
	}
}

func TestMethodNotAllowedReturnsJSON405(t *testing.T) {
	rec := do(t, http.MethodDelete, "/health", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestCORSAllowsConfiguredOriginOnly(t *testing.T) {
	allowed := do(t, http.MethodGet, "/health", map[string]string{"Origin": "http://localhost:5173"})
	if got := allowed.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Access-Control-Allow-Origin = %q, want the request origin", got)
	}

	denied := do(t, http.MethodGet, "/health", map[string]string{"Origin": "https://evil.example"})
	if got := denied.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q for a disallowed origin, want it unset", got)
	}
}

func TestCORSPreflight(t *testing.T) {
	rec := do(t, http.MethodOptions, "/api/v1/transactions", map[string]string{
		"Origin":                        "http://localhost:5173",
		"Access-Control-Request-Method": http.MethodGet,
	})

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("Access-Control-Allow-Methods is unset on a preflight response")
	}
}
