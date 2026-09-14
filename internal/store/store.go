// Package store opens the Postgres connection pool the service reads.
package store

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultDSN matches docker-compose.yml. Override with DATABASE_URL.
const DefaultDSN = "postgres://app:app@localhost:5432/app?sslmode=disable"

// Open connects and verifies the connection before returning it.
func Open(ctx context.Context) (*pgxpool.Pool, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = DefaultDSN
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	// pgxpool.New does not dial. Ping here so a database that is not up is an
	// error at startup rather than on the first request.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping %s: %w (is `docker compose up -d db` running?)", dsn, err)
	}
	return pool, nil
}
