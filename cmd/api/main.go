// Command api is the HTTP entry point.
//
// It starts, answers /healthz, and does nothing else on purpose — the endpoints
// your brief asks for are yours to add. See README.md.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"dayzer0/be-go-events-outbox/internal/httpx"
	"dayzer0/be-go-events-outbox/internal/store"
)

func main() {
	ctx := context.Background()
	pool, err := store.Open(ctx)
	if err != nil {
		// Fatal on purpose: a service that starts without its database answers
		// every request wrongly, which is harder to diagnose than not starting.
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	rdb, err := store.OpenRedis(ctx)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer rdb.Close()

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", httpx.Health(pool, store.RedisPinger{Client: rdb}))

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
