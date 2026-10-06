// Package database owns the PostgreSQL connection pool.
//
// Schema changes are applied exclusively by golang-migrate (see ../../migrations);
// the application never mutates the schema at run time. Type-safe query code
// lives in the generated subpackage ./db.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool defaults. Kept here rather than in config: these are tuning knobs that
// should be changed deliberately with a load profile in hand, not per deploy.
const (
	maxConns          = 10
	minConns          = 2
	maxConnLifetime   = time.Hour
	maxConnIdleTime   = 30 * time.Minute
	healthCheckPeriod = time.Minute
	connectTimeout    = 10 * time.Second
)

// Connect opens a connection pool and verifies the database is reachable.
//
// Failing here is intentional: a server that cannot reach PostgreSQL cannot
// serve its API, so it should refuse to start instead of failing per request.
// The caller owns the returned pool and must Close it.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	cfg.MaxConns = maxConns
	cfg.MinConns = minConns
	cfg.MaxConnLifetime = maxConnLifetime
	cfg.MaxConnIdleTime = maxConnIdleTime
	cfg.HealthCheckPeriod = healthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}
