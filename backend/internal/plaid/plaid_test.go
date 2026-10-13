package plaid

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/linbeau0822/cardmaxx/backend/internal/config"
)

func TestNewClientSelectsEnvironment(t *testing.T) {
	tests := map[config.PlaidEnv]string{
		config.PlaidSandbox:    "https://sandbox.plaid.com",
		config.PlaidProduction: "https://production.plaid.com",
	}

	for env, wantURL := range tests {
		t.Run(string(env), func(t *testing.T) {
			client, err := NewClient(config.PlaidConfig{ClientID: "id", Secret: "secret", Env: env})
			if err != nil {
				t.Fatalf("NewClient() returned error: %v", err)
			}
			if client.BaseURL() != wantURL {
				t.Errorf("BaseURL() = %q, want %q", client.BaseURL(), wantURL)
			}
		})
	}
}

func TestNewClientRejectsUnknownEnvironment(t *testing.T) {
	if _, err := NewClient(config.PlaidConfig{Env: "staging"}); err == nil {
		t.Fatal("NewClient() accepted an unknown Plaid environment, want an error")
	}
}

// testClient points a Client at a local httptest server instead of
// sandbox.plaid.com, so these tests make no network calls.
func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &Client{clientID: "id", secret: "secret", baseURL: server.URL, httpClient: server.Client()}
}

func respondJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode fixture response: %v", err)
	}
}

func TestCreateLinkTokenSendsClientUserID(t *testing.T) {
	userID := uuid.New()
	var gotBody map[string]any
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/link/token/create" {
			t.Errorf("path = %q, want /link/token/create", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		respondJSON(t, w, map[string]any{
			"link_token": "link-sandbox-abc",
			"expiration": "2026-01-01T00:00:00Z",
		})
	})

	token, err := client.CreateLinkToken(context.Background(), userID)
	if err != nil {
		t.Fatalf("CreateLinkToken() returned error: %v", err)
	}
	if token.Token != "link-sandbox-abc" {
		t.Errorf("Token = %q, want %q", token.Token, "link-sandbox-abc")
	}
	if gotBody["client_id"] != "id" || gotBody["secret"] != "secret" {
		t.Errorf("request body missing credentials: %v", gotBody)
	}
	user, _ := gotBody["user"].(map[string]any)
	if user["client_user_id"] != userID.String() {
		t.Errorf("user.client_user_id = %v, want %q", user["client_user_id"], userID.String())
	}
}

func TestExchangePublicTokenReturnsCredentials(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(t, w, map[string]any{
			"access_token": "access-sandbox-xyz",
			"item_id":      "item-123",
		})
	})

	creds, err := client.ExchangePublicToken(context.Background(), "public-sandbox-abc")
	if err != nil {
		t.Fatalf("ExchangePublicToken() returned error: %v", err)
	}
	if creds.AccessToken != "access-sandbox-xyz" || creds.ItemID != "item-123" {
		t.Errorf("creds = %+v, want AccessToken=access-sandbox-xyz ItemID=item-123", creds)
	}
}

func TestSyncTransactionsConvertsAmountsAndAccounts(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(t, w, map[string]any{
			"accounts": []map[string]any{
				{"account_id": "acc-1", "name": "Checking", "type": "depository", "subtype": "checking"},
			},
			"added": []map[string]any{
				{
					"transaction_id": "tx-1",
					"account_id":     "acc-1",
					"amount":         12.34,
					"date":           "2026-01-15",
					"name":           "Coffee Shop",
					"pending":        false,
				},
			},
			"modified": []map[string]any{},
			"removed": []map[string]any{
				{"transaction_id": "tx-old"},
			},
			"next_cursor": "cursor-2",
			"has_more":    true,
		})
	})

	result, err := client.SyncTransactions(context.Background(), "access-token", "cursor-1")
	if err != nil {
		t.Fatalf("SyncTransactions() returned error: %v", err)
	}
	if !result.HasMore || result.NextCursor != "cursor-2" {
		t.Errorf("HasMore/NextCursor = %v/%q, want true/cursor-2", result.HasMore, result.NextCursor)
	}
	if len(result.Accounts) != 1 || result.Accounts[0].PlaidAccountID != "acc-1" {
		t.Fatalf("Accounts = %+v, want one account acc-1", result.Accounts)
	}
	if len(result.Added) != 1 {
		t.Fatalf("Added = %+v, want 1 transaction", result.Added)
	}
	if got, want := result.Added[0].AmountCents, int64(1234); got != want {
		t.Errorf("AmountCents = %d, want %d", got, want)
	}
	if len(result.RemovedIDs) != 1 || result.RemovedIDs[0] != "tx-old" {
		t.Errorf("RemovedIDs = %v, want [tx-old]", result.RemovedIDs)
	}
}

func TestCallReturnsAPIErrorOnNon200(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		respondJSON(t, w, map[string]any{
			"error_type":    "INVALID_REQUEST",
			"error_code":    "INVALID_FIELD",
			"error_message": "public_token is invalid",
			"request_id":    "req-1",
		})
	})

	_, err := client.ExchangePublicToken(context.Background(), "bad-token")
	if err == nil {
		t.Fatal("ExchangePublicToken() returned no error for a 400 response")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.Code != "INVALID_FIELD" || apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("apiErr = %+v, want Code=INVALID_FIELD StatusCode=400", apiErr)
	}
}
