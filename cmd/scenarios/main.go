// Command scenarios drives named situations against your orchestrator — the scenario runner the
// Juspay brief promises.
//
// ⚠️ IT DRIVES; IT DOES NOT DECIDE. Each scenario sets the provider stubs' behaviour and sends
// payments. Whether your orchestrator routes away from a failing provider, trips a breaker, or
// resolves a charged-but-silent timeout is what it is there to reveal.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type scenario struct {
	name, what string
	pspFlags   string
	payments   int
}

var scenarios = []scenario{
	{"healthy", "every provider answers. The baseline you compare the rest against.",
		"-fail-rate 0 -timeout-rate 0 -charged-but-silent 0", 20},
	{"flaky-provider", "one in four calls fails cleanly. Your router should prefer the ones that work.",
		"-fail-rate 0.25", 40},
	{"provider-down", "nothing answers. A breaker should open rather than queueing every payment behind a timeout.",
		"-timeout-rate 1.0", 20},
	{"charged-but-silent", "one in five CHARGES and never answers. The money moved; you were not told.",
		"-charged-but-silent 0.2", 30},
	{"slow-then-recovered", "high latency, no failures. Tests whether a slow provider trips a breaker it should not.",
		"-latency 2s", 15},
}

func main() {
	list := flag.Bool("list", false, "list the scenarios and exit")
	run := flag.String("run", "", "scenario to run")
	base := flag.String("base", "http://localhost:8080", "base URL of YOUR orchestrator")
	path := flag.String("path", "/payments", "endpoint that takes a payment")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "scenarios — named situations to run your orchestrator through.\n\n"+
			"This does NOT start cmd/psp for you. Each scenario prints the psp flags it needs;\n"+
			"start psp with those in another terminal, then run the scenario. Deliberate: you\n"+
			"should be able to watch the provider's log while the scenario runs.\n\n"+
			"  go run ./cmd/scenarios -list\n"+
			"  go run ./cmd/scenarios -run charged-but-silent\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *list || *run == "" {
		fmt.Println("scenarios:")
		for _, s := range scenarios {
			fmt.Printf("\n  %-20s %d payments\n     %s\n     psp flags: %s\n", s.name, s.payments, s.what, s.pspFlags)
		}
		if *run == "" && !*list {
			fmt.Println("\nPick one with -run.")
		}
		return
	}

	var sc *scenario
	for i := range scenarios {
		if strings.EqualFold(scenarios[i].name, *run) {
			sc = &scenarios[i]
		}
	}
	if sc == nil {
		names := []string{}
		for _, s := range scenarios {
			names = append(names, s.name)
		}
		sort.Strings(names)
		fmt.Fprintf(os.Stderr, "unknown scenario %q. Known: %s\n", *run, strings.Join(names, ", "))
		os.Exit(2)
	}

	fmt.Printf("scenario %s — %s\n", sc.name, sc.what)
	fmt.Printf("start the providers in another terminal:\n  go run ./cmd/psp %s\n\n", sc.pspFlags)

	client := &http.Client{Timeout: 30 * time.Second}
	counts := map[string]int{}
	for i := 0; i < sc.payments; i++ {
		body := fmt.Sprintf(`{"id":"pay-%s-%03d","amount_paise":%d}`, sc.name, i, 10000+i*137)
		t0 := time.Now()
		resp, err := client.Post(*base+*path, "application/json", bytes.NewBufferString(body))
		took := time.Since(t0)
		if err != nil {
			counts[fmt.Sprintf("transport error after %v", took.Round(time.Second))]++
			continue
		}
		resp.Body.Close()
		counts[fmt.Sprintf("HTTP %d", resp.StatusCode)]++
	}
	fmt.Printf("\n%d payments:\n", sc.payments)
	keys := []string{}
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %-34s %d\n", k, counts[k])
	}
	if sc.name == "charged-but-silent" {
		fmt.Println("\nNow ask the provider what really happened:\n" +
			"  curl localhost:9092/razorpay/status/pay-charged-but-silent-000\n" +
			"Any payment you recorded as failed but the provider recorded as CHARGED is money you\n" +
			"have to reconcile. Finding them is the ticket.")
	}
}
