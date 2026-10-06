// Package config loads and validates application configuration.
//
// Configuration is read from the process environment exactly once, at start-up,
// into an immutable struct. The rest of the application depends on that struct
// instead of reading os.Getenv, which keeps configuration discoverable and the
// packages that consume it testable.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// PlaidEnv identifies which Plaid environment the application talks to.
type PlaidEnv string

const (
	PlaidSandbox    PlaidEnv = "sandbox"
	PlaidProduction PlaidEnv = "production"
)

// Config is the fully validated configuration for one process.
type Config struct {
	DatabaseURL string
	Plaid       PlaidConfig
	Server      ServerConfig
}

// PlaidConfig holds the credentials used to talk to Plaid. These are secrets
// and must never be sent to the frontend.
type PlaidConfig struct {
	ClientID string
	Secret   string
	Env      PlaidEnv
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port              int
	AllowedOrigins    []string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// Addr returns the TCP address the HTTP server should listen on.
func (s ServerConfig) Addr() string {
	return fmt.Sprintf(":%d", s.Port)
}

// Default server timeouts. They are not configurable yet: a single sensible set
// of values is easier to reason about than six more environment variables.
const (
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 15 * time.Second
	defaultWriteTimeout      = 30 * time.Second
	defaultIdleTimeout       = 60 * time.Second
	defaultShutdownTimeout   = 15 * time.Second

	defaultPort           = 8080
	defaultAllowedOrigins = "http://localhost:5173"
)

// Load reads configuration from the environment and validates it.
//
// All problems are collected and returned together so a misconfigured
// deployment reports every missing variable at once rather than one per restart.
func Load() (Config, error) {
	var problems []string

	cfg := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Plaid: PlaidConfig{
			ClientID: os.Getenv("PLAID_CLIENT_ID"),
			Secret:   os.Getenv("PLAID_SECRET"),
			Env:      PlaidEnv(envOr("PLAID_ENV", string(PlaidSandbox))),
		},
		Server: ServerConfig{
			AllowedOrigins:    splitAndTrim(envOr("CORS_ALLOWED_ORIGINS", defaultAllowedOrigins)),
			ReadHeaderTimeout: defaultReadHeaderTimeout,
			ReadTimeout:       defaultReadTimeout,
			WriteTimeout:      defaultWriteTimeout,
			IdleTimeout:       defaultIdleTimeout,
			ShutdownTimeout:   defaultShutdownTimeout,
		},
	}

	if cfg.DatabaseURL == "" {
		problems = append(problems, "DATABASE_URL is required")
	}
	if cfg.Plaid.ClientID == "" {
		problems = append(problems, "PLAID_CLIENT_ID is required")
	}
	if cfg.Plaid.Secret == "" {
		problems = append(problems, "PLAID_SECRET is required")
	}
	switch cfg.Plaid.Env {
	case PlaidSandbox, PlaidProduction:
	default:
		problems = append(problems, fmt.Sprintf("PLAID_ENV must be %q or %q, got %q",
			PlaidSandbox, PlaidProduction, cfg.Plaid.Env))
	}

	port, err := parsePort(envOr("SERVER_PORT", strconv.Itoa(defaultPort)))
	if err != nil {
		problems = append(problems, "SERVER_PORT: "+err.Error())
	}
	cfg.Server.Port = port

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return cfg, nil
}

func parsePort(raw string) (int, error) {
	port, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number", raw)
	}
	if port < 1 || port > 65535 {
		return 0, errors.New("must be between 1 and 65535")
	}
	return port, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func splitAndTrim(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
