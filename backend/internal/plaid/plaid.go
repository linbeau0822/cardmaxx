// Package plaid is the boundary between CardWise and the Plaid API.
//
// Two rules hold this boundary:
//
//  1. Nothing outside this package imports a Plaid SDK or sees a Plaid wire
//     type. The types declared here are CardWise's own, which is what makes it
//     possible to swap or mock the provider.
//  2. Access tokens never leave the backend. They are written to plaid_items
//     and read only by this package; no type here that carries a token is
//     JSON-serialisable towards a client.
package plaid

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/linbeau0822/cardmaxx/backend/internal/config"
)

// ErrNotImplemented is returned by every call until Phase 2 wires up the real
// Plaid integration.
var ErrNotImplemented = errors.New("plaid: not implemented")

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

// Transaction is one transaction as CardWise models it, already converted off
// Plaid's wire format. Amounts are integer cents, positive for money leaving
// the account, matching the transactions table.
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

// CreateLinkToken requests a Link token for a user.
//
// TODO(phase-2): POST /link/token/create with client_id, secret, the CardWise
// user id as client_user_id, products ["transactions"], and country/language.
func (c *Client) CreateLinkToken(_ context.Context, _ uuid.UUID) (LinkToken, error) {
	return LinkToken{}, ErrNotImplemented
}

// ExchangePublicToken trades the public token produced by Plaid Link for a
// long-lived access token.
//
// TODO(phase-2): POST /item/public_token/exchange and return the access token
// and item id. The access token must be persisted, never returned to a client.
func (c *Client) ExchangePublicToken(_ context.Context, _ string) (ItemCredentials, error) {
	return ItemCredentials{}, ErrNotImplemented
}

// SyncTransactions fetches one page of the incremental transaction feed for an
// Item, starting from cursor ("" for a first sync).
//
// TODO(phase-2): POST /transactions/sync, convert Plaid's float dollar amounts
// to integer cents at this boundary, and return the next cursor so the caller
// can continue while HasMore is true.
func (c *Client) SyncTransactions(_ context.Context, _, _ string) (SyncResult, error) {
	return SyncResult{}, ErrNotImplemented
}
