package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a *pgxpool.Pool with sane defaults.
//
// dsn must be a Postgres connection string (e.g. "postgres://user:pass@host:5432/db?sslmode=disable").
// maxConns sets pgxpool.Config.MaxConns; pass 0 to keep pgx's default (greater of 4 or runtime.NumCPU()).
//
// The pool is verified with a Ping before being returned. On Ping failure the pool is closed
// and an error is returned, so callers never receive a half-open pool.
func NewPool(ctx context.Context, dsn string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("new pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}
