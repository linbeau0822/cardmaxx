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
	accounts     []db.Account
	transactions []db.Transaction
	plaidItems   []db.PlaidItem
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
	return NewRouter(cfg, logger, svc, uuid.New())
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

func TestUnimplementedEndpointsReturn501(t *testing.T) {
	routes := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/cards"},
		{http.MethodGet, "/api/v1/recommendations"},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			rec := do(t, route.method, route.path, nil)

			if rec.Code != http.StatusNotImplemented {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
			}

			var body errorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not valid JSON: %v (body: %s)", err, rec.Body.String())
			}
			if body.Error.Code != CodeNotImplemented {
				t.Errorf("error.code = %q, want %q", body.Error.Code, CodeNotImplemented)
			}
			if body.Error.Message == "" {
				t.Error("error.message is empty, want an explanation of what is missing")
			}
		})
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
