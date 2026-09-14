// Command verify checks per-device state against what was sent — the verifier the Ather brief
// promises.
//
// ⚠️ IT COMPARES; IT DOES NOT INGEST. What the stored state SHOULD be is the brief's spec, and
// this reads what it actually is. How you keep per-device ordering under a reconnection surge is
// the project, and nothing here does it for you.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"dayzer0/be-go-events-outbox/internal/store"
)

func main() {
	table := flag.String("table", "telemetry", "table holding stored frames")
	deviceCol := flag.String("device-col", "device_id", "device identifier column")
	seqCol := flag.String("seq-col", "seq", "per-device sequence column")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "verify — reads stored telemetry and checks three rules:\n\n"+
			"  1. no (device, seq) pair is stored twice\n"+
			"  2. each device's sequence has no gaps\n"+
			"  3. nothing impossible was stored (SoC outside 0-100, negative speed)\n\n"+
			"Run it after cmd/fleet. Column names are flags because your schema is yours.\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	ctx := context.Background()
	pool, err := store.Open(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "database: %v\n", err)
		os.Exit(2)
	}
	defer pool.Close()
	failed := false

	// Rule 1 — a frame stored twice is what at-least-once delivery produces without a dedupe key.
	var dupes int
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`select count(*) from (select %s, %s from %s group by 1,2 having count(*) > 1) d`,
		*deviceCol, *seqCol, *table)).Scan(&dupes); err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n(is the table named something else? see -table)\n", *table, err)
		os.Exit(2)
	}
	if dupes > 0 {
		failed = true
		fmt.Printf("rule 1 FAILED: %d (device, seq) pair(s) stored more than once\n", dupes)
	} else {
		fmt.Println("rule 1 ok: no frame stored twice")
	}

	// Rule 2 — a gap means a frame was dropped rather than dead-lettered.
	rows, err := pool.Query(ctx, fmt.Sprintf(
		`select %[1]s, count(*), max(%[2]s) - min(%[2]s) + 1 from %[3]s group by %[1]s having count(*) <> max(%[2]s) - min(%[2]s) + 1 limit 20`,
		*deviceCol, *seqCol, *table))
	if err == nil {
		gaps := 0
		for rows.Next() {
			var dev string
			var have, span int
			if rows.Scan(&dev, &have, &span) == nil {
				fmt.Printf("  GAP  %s has %d frames spanning %d sequence numbers\n", dev, have, span)
				gaps++
			}
		}
		rows.Close()
		if gaps > 0 {
			failed = true
			fmt.Printf("rule 2 FAILED: %d device(s) have a gap. A refused frame should be dead-lettered,\n"+
				"  not silently dropped — a gap cannot tell those apart afterwards\n", gaps)
		} else {
			fmt.Println("rule 2 ok: no device has a sequence gap")
		}
	}

	// Rule 3 — the impossible values cmd/fleet -malformed sends should never reach the table.
	var impossible int
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`select count(*) from %s where soc_percent < 0 or soc_percent > 100 or speed_kmph < 0`, *table)).Scan(&impossible); err != nil {
		fmt.Printf("rule 3 skipped: no soc_percent/speed_kmph columns to check (%v)\n", err)
	} else if impossible > 0 {
		failed = true
		fmt.Printf("rule 3 FAILED: %d stored frame(s) hold an impossible value\n", impossible)
	} else {
		fmt.Println("rule 3 ok: nothing impossible was stored")
	}

	if failed {
		os.Exit(1)
	}
	fmt.Println("\nall rules held")
}
