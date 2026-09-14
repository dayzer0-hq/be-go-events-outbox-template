// Command worker is the consumer side.
//
// It connects to Redis, confirms the stream is reachable, and then waits. It
// consumes nothing on purpose — which events exist, and what a consumer does
// with one, is what your brief asks you to build.
//
// Your setup ticket asks you to run this in a second terminal and confirm it
// connects. That is all it is for today.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dayzer0/be-go-events-outbox/internal/store"
)

// StreamName is the Redis stream the worker watches. Your brief may rename it.
const StreamName = "events"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rdb, err := store.OpenRedis(ctx)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer rdb.Close()

	// Read the stream length rather than just pinging. A ping proves Redis is
	// up; this proves the worker can actually address the stream it will read,
	// which is the thing that is broken when a worker "connects" and then
	// silently consumes nothing.
	n, err := rdb.XLen(ctx, StreamName).Result()
	if err != nil {
		log.Fatalf("read stream %q: %v", StreamName, err)
	}
	log.Printf("connected to redis; stream %q holds %d entries", StreamName, n)
	log.Printf("this worker consumes nothing yet — that is ticket 2 onwards")

	<-ctx.Done()
	log.Println("shutting down")
	// A worker that ignores SIGTERM is killed mid-message, which is exactly the
	// at-least-once edge case your brief is about. Handle shutdown properly when
	// you start consuming.
	time.Sleep(50 * time.Millisecond)
}
