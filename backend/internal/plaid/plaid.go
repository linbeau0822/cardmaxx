// Package plaid is the boundary between CardMaxx and the Plaid API.
//
// Two rules hold this boundary:
//
//  1. Nothing outside this package imports a Plaid SDK or sees a Plaid wire
//     type. The types declared here are CardMaxx's own, which is what makes it
//     possible to swap or mock the provider.
//  2. Access tokens never leave the backend. They are written to plaid_items
//     and read only by this package; no type here that carries a token is
//     JSON-serialisable towards a client.
package plaid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/linbeau0822/cardmaxx/backend/internal/config"
)

// requestTimeout bounds a single call to Plaid.
const requestTimeout = 20 * time.Second

// LinkToken is a short-lived token the frontend hands to Plaid Link to start
// the account-connection flow. It is the only Plaid credential that is safe to
// return to a browser.
type LinkToken struct {
	Token      string    `json:"link_token"`
	Expiration time.Time `json:"expiration"`
}

// ItemCredentials is the result of exchanging a public token: a long-lived
// access token plus the Plaid Item it belongs to.
//
// This type is deliberately not JSON-tagged. It is a secret that belongs in
// plaid_items and nowhere else.
type ItemCredentials struct {
	ItemID      string
	AccessToken string
}

// Transaction is one transaction as CardMaxx models it, already converted off
// Plaid's wire format. Amounts are integer cents, positive for money leaving
// the account, matching the transactions table and Plaid's own sign
// convention, so no sign flip happens at this boundary.
type Transaction struct {
	PlaidTransactionID   string
	PlaidAccountID       string
	AmountCents          int64
	Date                 time.Time
	Name                 string
	MerchantName         *string
	Category             *string
	Pending              bool
	PendingTransactionID *string
}

// Account is one account exposed by a Plaid Item.
type Account struct {
	PlaidAccountID string
	Name           string
	OfficialName   *string
	Type           string
	Subtype        *string
	Mask           *string
}

// SyncResult is one page of Plaid's incremental transaction feed.
//
// Plaid's /transactions/sync is cursor based and returns three disjoint sets.
// Modelling removals explicitly (rather than only additions) is what keeps the
// local copy of a user's spend correct over time.
type SyncResult struct {
	Added    []Transaction
	Modified []Transaction
	// RemovedIDs are plaid_transaction_id values that must be deleted locally.
	RemovedIDs []string
	Accounts   []Account
	NextCursor string
	HasMore    bool
}

// APIError is returned when Plaid rejects a request. error_code is stable and
// machine-readable; see https://plaid.com/docs/errors/.
type APIError struct {
	StatusCode int
	Type       string `json:"error_type"`
	Code       string `json:"error_code"`
	Message    string `json:"error_message"`
	RequestID  string `json:"request_id"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("plaid: %s (status %d, request %s)", e.Message, e.StatusCode, e.RequestID)
}

// Client is the HTTP client for the Plaid API.
type Client struct {
	clientID   string
	secret     string
	baseURL    string
	httpClient *http.Client
}

// NewClient builds a Client for the configured Plaid environment.
func NewClient(cfg config.PlaidConfig) (*Client, error) {
	baseURL, err := baseURLFor(cfg.Env)
	if err != nil {
		return nil, err
	}
	return &Client{
		clientID:   cfg.ClientID,
		secret:     cfg.Secret,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: requestTimeout},
	}, nil
}

// baseURLFor maps a Plaid environment onto its API host.
func baseURLFor(env config.PlaidEnv) (string, error) {
	switch env {
	case config.PlaidSandbox:
		return "https://sandbox.plaid.com", nil
	case config.PlaidProduction:
		return "https://production.plaid.com", nil
	default:
		return "", fmt.Errorf("plaid: unknown environment %q", env)
	}
}

// BaseURL returns the Plaid host this client talks to. Useful for start-up
// logging and tests; the credentials are never exposed.
func (c *Client) BaseURL() string { return c.baseURL }

// call sends a JSON POST to a Plaid endpoint. client_id and secret are
// injected into req by re-marshalling it as a map, which keeps every caller
// below from having to repeat them.
func (c *Client) call(ctx context.Context, path string, req, resp any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("plaid: encode request: %w", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		return fmt.Errorf("plaid: encode request: %w", err)
	}
	fields["client_id"] = c.clientID
	fields["secret"] = c.secret
	body, err = json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("plaid: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("plaid: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("plaid: call %s: %w", path, err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("plaid: read response from %s: %w", path, err)
	}

	if httpResp.StatusCode != http.StatusOK {
		apiErr := &APIError{StatusCode: httpResp.StatusCode}
		if err := json.Unmarshal(respBody, apiErr); err != nil {
			apiErr.Message = string(respBody)
		}
		return apiErr
	}

	if resp == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, resp); err != nil {
		return fmt.Errorf("plaid: decode response from %s: %w", path, err)
	}
	return nil
}

// CreateLinkToken requests a Link token for a user.
func (c *Client) CreateLinkToken(ctx context.Context, userID uuid.UUID) (LinkToken, error) {
	req := map[string]any{
		"client_name":   "CardMaxx",
		"country_codes": []string{"US"},
		"language":      "en",
		"products":      []string{"transactions"},
		"user": map[string]any{
			"client_user_id": userID.String(),
		},
	}
	var resp struct {
		LinkToken  string    `json:"link_token"`
		Expiration time.Time `json:"expiration"`
	}
	if err := c.call(ctx, "/link/token/create", req, &resp); err != nil {
		return LinkToken{}, err
	}
	return LinkToken{Token: resp.LinkToken, Expiration: resp.Expiration}, nil
}

// ExchangePublicToken trades the public token produced by Plaid Link for a
// long-lived access token.
func (c *Client) ExchangePublicToken(ctx context.Context, publicToken string) (ItemCredentials, error) {
	req := map[string]any{"public_token": publicToken}
	var resp struct {
		AccessToken string `json:"access_token"`
		ItemID      string `json:"item_id"`
	}
	if err := c.call(ctx, "/item/public_token/exchange", req, &resp); err != nil {
		return ItemCredentials{}, err
	}
	return ItemCredentials{ItemID: resp.ItemID, AccessToken: resp.AccessToken}, nil
}

// plaidAccount and plaidTransaction mirror the fields CardMaxx needs from
// Plaid's wire format for /transactions/sync.
type plaidAccount struct {
	AccountID    string  `json:"account_id"`
	Name         string  `json:"name"`
	OfficialName *string `json:"official_name"`
	Type         string  `json:"type"`
	Subtype      *string `json:"subtype"`
	Mask         *string `json:"mask"`
}

type plaidTransaction struct {
	TransactionID           string  `json:"transaction_id"`
	AccountID               string  `json:"account_id"`
	Amount                  float64 `json:"amount"`
	Date                    string  `json:"date"`
	Name                    string  `json:"name"`
	MerchantName            *string `json:"merchant_name"`
	Pending                 bool    `json:"pending"`
	PendingTransactionID    *string `json:"pending_transaction_id"`
	PersonalFinanceCategory *struct {
		Primary string `json:"primary"`
	} `json:"personal_finance_category"`
}

// SyncTransactions fetches one page of the incremental transaction feed for an
// Item, starting from cursor ("" for a first sync).
func (c *Client) SyncTransactions(ctx context.Context, accessToken, cursor string) (SyncResult, error) {
	req := map[string]any{"access_token": accessToken}
	if cursor != "" {
		req["cursor"] = cursor
	}

	var resp struct {
		Accounts []plaidAccount     `json:"accounts"`
		Added    []plaidTransaction `json:"added"`
		Modified []plaidTransaction `json:"modified"`
		Removed  []struct {
			TransactionID string `json:"transaction_id"`
		} `json:"removed"`
		NextCursor string `json:"next_cursor"`
		HasMore    bool   `json:"has_more"`
	}
	if err := c.call(ctx, "/transactions/sync", req, &resp); err != nil {
		return SyncResult{}, err
	}

	result := SyncResult{
		NextCursor: resp.NextCursor,
		HasMore:    resp.HasMore,
	}
	for _, a := range resp.Accounts {
		result.Accounts = append(result.Accounts, Account{
			PlaidAccountID: a.AccountID,
			Name:           a.Name,
			OfficialName:   a.OfficialName,
			Type:           a.Type,
			Subtype:        a.Subtype,
			Mask:           a.Mask,
		})
	}
	for _, t := range resp.Added {
		tx, err := convertTransaction(t)
		if err != nil {
			return SyncResult{}, err
		}
		result.Added = append(result.Added, tx)
	}
	for _, t := range resp.Modified {
		tx, err := convertTransaction(t)
		if err != nil {
			return SyncResult{}, err
		}
		result.Modified = append(result.Modified, tx)
	}
	for _, r := range resp.Removed {
		result.RemovedIDs = append(result.RemovedIDs, r.TransactionID)
	}
	return result, nil
}

// convertTransaction converts one Plaid transaction off the wire, turning its
// float dollar amount into integer cents at this boundary so no float ever
// reaches the database.
func convertTransaction(t plaidTransaction) (Transaction, error) {
	date, err := time.Parse(time.DateOnly, t.Date)
	if err != nil {
		return Transaction{}, fmt.Errorf("plaid: parse transaction date %q: %w", t.Date, err)
	}
	var category *string
	if t.PersonalFinanceCategory != nil {
		category = &t.PersonalFinanceCategory.Primary
	}
	return Transaction{
		PlaidTransactionID:   t.TransactionID,
		PlaidAccountID:       t.AccountID,
		AmountCents:          int64(math.Round(t.Amount * 100)),
		Date:                 date,
		Name:                 t.Name,
		MerchantName:         t.MerchantName,
		Category:             category,
		Pending:              t.Pending,
		PendingTransactionID: t.PendingTransactionID,
	}, nil
}
