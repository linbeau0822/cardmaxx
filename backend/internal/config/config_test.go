package config

import (
	"strings"
	"testing"
)

func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://cardmaxx:cardmaxx@localhost:5432/cardmaxx")
	t.Setenv("PLAID_CLIENT_ID", "client-id")
	t.Setenv("PLAID_SECRET", "secret")
	t.Setenv("PLAID_ENV", "sandbox")
}

func TestLoadDefaults(t *testing.T) {
	setValidEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.Server.Port != defaultPort {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, defaultPort)
	}
	if cfg.Server.Addr() != ":8080" {
		t.Errorf("Server.Addr() = %q, want %q", cfg.Server.Addr(), ":8080")
	}
	if got, want := cfg.Server.AllowedOrigins, defaultAllowedOrigins; len(got) != 1 || got[0] != want {
		t.Errorf("Server.AllowedOrigins = %v, want [%s]", got, want)
	}
	if cfg.Plaid.Env != PlaidSandbox {
		t.Errorf("Plaid.Env = %q, want %q", cfg.Plaid.Env, PlaidSandbox)
	}
}

func TestLoadReportsEveryMissingVariable(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PLAID_CLIENT_ID", "")
	t.Setenv("PLAID_SECRET", "")
	t.Setenv("PLAID_ENV", "sandbox")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() succeeded with no configuration, want error")
	}
	for _, want := range []string{"DATABASE_URL", "PLAID_CLIENT_ID", "PLAID_SECRET"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	tests := map[string]struct {
		key, value, wantIn string
	}{
		"unknown plaid env": {"PLAID_ENV", "staging", "PLAID_ENV"},
		"non numeric port":  {"SERVER_PORT", "http", "SERVER_PORT"},
		"out of range port": {"SERVER_PORT", "70000", "between 1 and 65535"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			setValidEnv(t)
			t.Setenv(tc.key, tc.value)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() succeeded with %s=%q, want error", tc.key, tc.value)
			} else if !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("error %q does not contain %q", err, tc.wantIn)
			}
		})
	}
}

func TestLoadParsesMultipleOrigins(t *testing.T) {
	setValidEnv(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173, https://cardmaxx.app ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	want := []string{"http://localhost:5173", "https://cardmaxx.app"}
	if len(cfg.Server.AllowedOrigins) != len(want) {
		t.Fatalf("AllowedOrigins = %v, want %v", cfg.Server.AllowedOrigins, want)
	}
	for i := range want {
		if cfg.Server.AllowedOrigins[i] != want[i] {
			t.Errorf("AllowedOrigins[%d] = %q, want %q", i, cfg.Server.AllowedOrigins[i], want[i])
		}
	}
}
