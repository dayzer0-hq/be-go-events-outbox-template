// Package httpx holds HTTP handlers.
package httpx

import (
	"context"
	"encoding/json"
	"net/http"
)

// Pinger is the one thing Health needs from a database.
//
// It is an interface rather than *pgxpool.Pool so the handler can be tested
// WITHOUT a running Postgres. That matters more than it looks: your setup
// ticket says `go test ./...` must be green on the untouched skeleton, and a
// test suite that needs Docker up first is red for everybody who has not read
// ahead.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Health answers GET /healthz with a JSON body.
//
// It reports EVERY dependency separately rather than one overall ok. Collapsing
// Postgres and Redis into a single boolean means ahealth check that goes red tells
// you something is wrong and not which one, which is the question you actually
// have at 2am.
func Health(db Pinger, cache Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := map[string]string{"status": "ok", "database": "ok", "redis": "ok"}
		code := http.StatusOK
		if err := db.Ping(r.Context()); err != nil {
			body["status"] = "degraded"
			body["database"] = err.Error()
			code = http.StatusServiceUnavailable
		}
		if err := cache.Ping(r.Context()); err != nil {
			body["status"] = "degraded"
			body["redis"] = err.Error()
			code = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(body)
	}
}
