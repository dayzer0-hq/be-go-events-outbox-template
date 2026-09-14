package store

import (
	"context"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"
)

// DefaultRedisAddr matches docker-compose.yml. Override with REDIS_ADDR.
const DefaultRedisAddr = "localhost:6379"

// OpenRedis connects and verifies the connection before returning it.
func OpenRedis(ctx context.Context) (*redis.Client, error) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = DefaultRedisAddr
	}
	c := redis.NewClient(&redis.Options{Addr: addr})
	// redis.NewClient does not dial. Ping here so a Redis that is not up is an
	// error at startup rather than on the first request.
	if err := c.Ping(ctx).Err(); err != nil {
		c.Close()
		return nil, fmt.Errorf("ping redis %s: %w (is `docker compose up -d` running?)", addr, err)
	}
	return c, nil
}

// RedisPinger adapts *redis.Client to the httpx.Pinger interface.
//
// go-redis returns *redis.StatusCmd from Ping rather than an error, so the
// client does not satisfy Pinger on its own. The adapter lives here rather than
// widening Pinger to fit, because Pinger's whole job is to be the smallest
// thing the health handler needs — one method, returning an error, satisfiable
// by a stub in a test with no server running.
type RedisPinger struct{ *redis.Client }

// Ping reports whether Redis answered.
func (p RedisPinger) Ping(ctx context.Context) error { return p.Client.Ping(ctx).Err() }
