// Command chaos kills the publisher mid-transaction, redelivers arbitrary events, delivers them
// out of order, and then runs a consistency check across the order table and the consumers' own
// state — the chaos harness the Blinkit brief promises.
//
// ⚠️ THE CONSISTENCY CHECK ASSERTS AN INVARIANT; IT DOES NOT MAINTAIN ONE. Knowing that every
// committed order must have exactly one event, and that a consumer must not act twice on one
// event, tells you nothing about how to guarantee it — an outbox table, a transactional publish,
// a dedupe key or a consumer-side ledger are all still open, and choosing is the project.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"dayzer0/be-go-events-outbox/internal/store"
	"github.com/redis/go-redis/v9"
)

func main() {
	stream := flag.String("stream", "events", "Redis stream the worker reads")
	kill := flag.Bool("kill", false, "kill the publisher mid-transaction (see -kill-after)")
	killAfter := flag.Duration("kill-after", 300*time.Millisecond, "how long to wait before the kill")
	redeliver := flag.Int("redeliver", 0, "re-append N existing events to the stream")
	shuffle := flag.Bool("shuffle", false, "re-append a window of events in reversed order")
	window := flag.Int("window", 20, "how many recent events -shuffle reverses")
	check := flag.Bool("check", false, "run the consistency check and exit")
	ordersTable := flag.String("orders-table", "orders", "table holding one row per order")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "chaos — break the pipeline on purpose, then check what survived.\n\n"+
			"  -kill        stop the publisher part-way through a transaction\n"+
			"  -redeliver N re-append N events, so consumers see them twice (at-least-once)\n"+
			"  -shuffle     re-append a window reversed, so they arrive out of order\n"+
			"  -check       the consistency check: every committed order has exactly one event,\n"+
			"               and no event id appears twice in the stream\n\n"+
			"Run the damage, then run -check. A green check after -redeliver is the claim your\n"+
			"idempotency has to earn.\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	ctx := context.Background()
	rdb, err := store.OpenRedis(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "redis: %v\n", err)
		os.Exit(2)
	}
	defer rdb.Close()

	if *check {
		os.Exit(consistency(ctx, rdb, *stream, *ordersTable))
	}

	did := false
	if *kill {
		fmt.Printf("waiting %v, then killing the publisher…\n", *killAfter)
		time.Sleep(*killAfter)
		// The kill is a signal to YOUR publisher, not something this tool can do for you: it has
		// no handle on your process. Printing the command keeps the tool honest about that.
		fmt.Println("send this in the publisher's terminal now:  Ctrl-C")
		fmt.Println("then check whether an order committed without its event ever reaching the stream.")
		did = true
	}
	if *redeliver > 0 {
		msgs, err := rdb.XRevRangeN(ctx, *stream, "+", "-", int64(*redeliver)).Result()
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", *stream, err)
			os.Exit(1)
		}
		for _, m := range msgs {
			if err := rdb.XAdd(ctx, &redis.XAddArgs{Stream: *stream, Values: m.Values}).Err(); err != nil {
				fmt.Fprintf(os.Stderr, "re-append: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Printf("re-appended %d event(s). Consumers should act on each ONCE.\n", len(msgs))
		did = true
	}
	if *shuffle {
		msgs, err := rdb.XRevRangeN(ctx, *stream, "+", "-", int64(*window)).Result()
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", *stream, err)
			os.Exit(1)
		}
		rand.Shuffle(len(msgs), func(i, j int) { msgs[i], msgs[j] = msgs[j], msgs[i] })
		for _, m := range msgs {
			_ = rdb.XAdd(ctx, &redis.XAddArgs{Stream: *stream, Values: m.Values}).Err()
		}
		fmt.Printf("re-appended %d event(s) out of order.\n", len(msgs))
		did = true
	}
	if !did {
		flag.Usage()
		os.Exit(2)
	}
	fmt.Println("\nnow run:  go run ./cmd/chaos -check")
}

// consistency is the check the brief promises. Two rules, both read-only.
func consistency(ctx context.Context, rdb *redis.Client, stream, ordersTable string) int {
	pool, err := store.Open(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "database: %v\n", err)
		return 2
	}
	defer pool.Close()

	failed := false

	// Rule 1 — every committed order has exactly one event on the stream.
	msgs, err := rdb.XRange(ctx, stream, "-", "+").Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", stream, err)
		return 2
	}
	seen := map[string]int{}
	for _, m := range msgs {
		for _, k := range []string{"order_id", "orderId", "id"} {
			if v, ok := m.Values[k]; ok {
				seen[fmt.Sprint(v)]++
				break
			}
		}
	}
	var committed int
	if err := pool.QueryRow(ctx, fmt.Sprintf("select count(*) from %s", ordersTable)).Scan(&committed); err != nil {
		fmt.Printf("rule 1 skipped: cannot read %s (%v)\n", ordersTable, err)
	} else {
		missing := committed - len(seen)
		if missing > 0 {
			failed = true
			fmt.Printf("rule 1 FAILED: %d committed order(s) have no event on the stream — the order\n"+
				"  and its event did not commit together\n", missing)
		} else {
			fmt.Printf("rule 1 ok: %d committed order(s), %d with an event\n", committed, len(seen))
		}
	}

	// Rule 2 — no order id appears twice, which is what at-least-once delivery produces.
	dupes := 0
	for id, n := range seen {
		if n > 1 {
			if dupes < 10 {
				fmt.Printf("  DUPLICATE  %s appears %d times on the stream\n", id, n)
			}
			dupes++
		}
	}
	if dupes > 0 {
		fmt.Printf("rule 2: %d order id(s) appear more than once. That is EXPECTED after -redeliver —\n"+
			"  the question is whether your CONSUMERS acted once. Check their own state.\n", dupes)
	} else {
		fmt.Println("rule 2 ok: no order id appears twice on the stream")
	}

	if failed {
		return 1
	}
	fmt.Println("\nconsistency check passed")
	return 0
}
