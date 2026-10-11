package plaid

import (
	"context"
	"errors"
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

// The client is a declared boundary with no implementation yet. These tests
// pin that contract so the TODOs cannot be forgotten silently: they fail the
// moment a method starts doing real work and must be updated with it.
func TestClientMethodsAreNotImplementedYet(t *testing.T) {
	client, err := NewClient(config.PlaidConfig{ClientID: "id", Secret: "secret", Env: config.PlaidSandbox})
	if err != nil {
		t.Fatalf("NewClient() returned error: %v", err)
	}
	ctx := context.Background()

	if _, err := client.CreateLinkToken(ctx, uuid.New()); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("CreateLinkToken() error = %v, want ErrNotImplemented", err)
	}
	if _, err := client.ExchangePublicToken(ctx, "public-token"); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("ExchangePublicToken() error = %v, want ErrNotImplemented", err)
	}
	if _, err := client.SyncTransactions(ctx, "access-token", ""); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("SyncTransactions() error = %v, want ErrNotImplemented", err)
	}
}
